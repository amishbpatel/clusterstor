package providers

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	googleAuthURL   = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL  = "https://oauth2.googleapis.com/token"
	googleAboutURL  = "https://www.googleapis.com/drive/v3/about?fields=user(displayName,emailAddress,permissionId),storageQuota(limit,usage)"
	googleDriveScope = "https://www.googleapis.com/auth/drive"
	oauthStateTTL   = 10 * time.Minute
)

var (
	ErrProviderNotConfigured = errors.New("provider not configured")
	ErrInvalidOAuthState     = errors.New("invalid oauth state")
	ErrOAuthExchange         = errors.New("oauth exchange failed")
)

type Config struct {
	APIBaseURL          string
	GoogleClientID      string
	GoogleClientSecret  string
	TokenEncryptionKey  string
}

type Service struct {
	pool       *pgxpool.Pool
	cfg        Config
	httpClient *http.Client
	key        []byte
}

type Account struct {
	ID                string     `json:"id"`
	Provider          string     `json:"provider"`
	ExternalAccountID *string    `json:"external_account_id,omitempty"`
	DisplayName       *string    `json:"display_name,omitempty"`
	Status            string     `json:"status"`
	QuotaTotalBytes   *int64     `json:"quota_total_bytes,omitempty"`
	QuotaUsedBytes    *int64     `json:"quota_used_bytes,omitempty"`
	QuotaFreeBytes    *int64     `json:"quota_free_bytes,omitempty"`
	LastSyncedAt      *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type OAuthStart struct {
	AuthorizationURL string `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

type storedToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type googleAbout struct {
	User struct {
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
		PermissionID string `json:"permissionId"`
	} `json:"user"`
	StorageQuota struct {
		Limit string `json:"limit"`
		Usage string `json:"usage"`
	} `json:"storageQuota"`
}

func NewService(pool *pgxpool.Pool, cfg Config) (*Service, error) {
	key, err := decodeEncryptionKey(cfg.TokenEncryptionKey)
	if err != nil && cfg.TokenEncryptionKey != "" {
		return nil, err
	}
	return &Service{
		pool: pool,
		cfg: cfg,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		key: key,
	}, nil
}

func (s *Service) StartGoogleOAuth(ctx context.Context, userID string) (OAuthStart, error) {
	if s.cfg.GoogleClientID == "" || s.cfg.GoogleClientSecret == "" || len(s.key) != 32 {
		return OAuthStart{}, ErrProviderNotConfigured
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return OAuthStart{}, fmt.Errorf("generate oauth state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(state))
	expiresAt := time.Now().UTC().Add(oauthStateTTL)
	redirectURI := strings.TrimRight(s.cfg.APIBaseURL, "/") + "/api/v1/providers/google_drive/callback"

	_, err := s.pool.Exec(ctx, `
		INSERT INTO provider_oauth_states(user_id,provider,state_hash,redirect_uri,expires_at)
		VALUES ($1::uuid,'google_drive',$2,$3,$4)`,
		userID, hash[:], redirectURI, expiresAt)
	if err != nil {
		return OAuthStart{}, fmt.Errorf("store oauth state: %w", err)
	}

	q := url.Values{}
	q.Set("client_id", s.cfg.GoogleClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", googleDriveScope)
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	q.Set("include_granted_scopes", "true")
	q.Set("state", state)

	return OAuthStart{
		AuthorizationURL: googleAuthURL + "?" + q.Encode(),
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) CompleteGoogleOAuth(ctx context.Context, state, code string) (Account, error) {
	if strings.TrimSpace(state) == "" || strings.TrimSpace(code) == "" {
		return Account{}, ErrInvalidOAuthState
	}
	if s.cfg.GoogleClientID == "" || s.cfg.GoogleClientSecret == "" || len(s.key) != 32 {
		return Account{}, ErrProviderNotConfigured
	}

	hash := sha256.Sum256([]byte(state))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Account{}, fmt.Errorf("begin oauth callback: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID, redirectURI string
	err = tx.QueryRow(ctx, `
		SELECT user_id::text, redirect_uri
		FROM provider_oauth_states
		WHERE state_hash=$1
		  AND provider='google_drive'
		  AND consumed_at IS NULL
		  AND expires_at > now()
		FOR UPDATE`, hash[:]).Scan(&userID, &redirectURI)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrInvalidOAuthState
	}
	if err != nil {
		return Account{}, fmt.Errorf("load oauth state: %w", err)
	}

	token, err := s.exchangeGoogleCode(ctx, code, redirectURI)
	if err != nil {
		return Account{}, err
	}
	about, err := s.fetchGoogleAbout(ctx, token.AccessToken)
	if err != nil {
		return Account{}, err
	}

	stored := storedToken{
		AccessToken: token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType: token.TokenType,
		Scope: token.Scope,
		ExpiresAt: time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second),
	}
	tokenBytes, err := json.Marshal(stored)
	if err != nil {
		return Account{}, fmt.Errorf("encode oauth token: %w", err)
	}
	ciphertext, err := encrypt(s.key, tokenBytes)
	if err != nil {
		return Account{}, fmt.Errorf("encrypt oauth token: %w", err)
	}

	externalID := strings.TrimSpace(about.User.PermissionID)
	if externalID == "" {
		externalID = strings.TrimSpace(about.User.EmailAddress)
	}
	displayName := strings.TrimSpace(about.User.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(about.User.EmailAddress)
	}

	var total, used, free *int64
	if value, ok := parseOptionalInt64(about.StorageQuota.Limit); ok {
		total = &value
	}
	if value, ok := parseOptionalInt64(about.StorageQuota.Usage); ok {
		used = &value
	}
	if total != nil && used != nil {
		value := *total - *used
		if value < 0 {
			value = 0
		}
		free = &value
	}

	var account Account
	err = tx.QueryRow(ctx, `
		INSERT INTO provider_accounts(
			user_id,provider,external_account_id,display_name,token_ciphertext,status,
			quota_total_bytes,quota_used_bytes,quota_free_bytes,last_synced_at,disconnected_at
		) VALUES (
			$1::uuid,'google_drive',$2,$3,$4,'connected',$5,$6,$7,now(),NULL
		)
		ON CONFLICT (user_id,provider) DO UPDATE SET
			external_account_id=EXCLUDED.external_account_id,
			display_name=EXCLUDED.display_name,
			token_ciphertext=EXCLUDED.token_ciphertext,
			status='connected',
			quota_total_bytes=EXCLUDED.quota_total_bytes,
			quota_used_bytes=EXCLUDED.quota_used_bytes,
			quota_free_bytes=EXCLUDED.quota_free_bytes,
			last_synced_at=now(),
			updated_at=now(),
			disconnected_at=NULL
		RETURNING id::text,provider,external_account_id,display_name,status,
			quota_total_bytes,quota_used_bytes,quota_free_bytes,last_synced_at,created_at,updated_at`,
		userID, externalID, nullableString(displayName), ciphertext, total, used, free).
		Scan(&account.ID, &account.Provider, &account.ExternalAccountID, &account.DisplayName, &account.Status,
			&account.QuotaTotalBytes, &account.QuotaUsedBytes, &account.QuotaFreeBytes,
			&account.LastSyncedAt, &account.CreatedAt, &account.UpdatedAt)
	if err != nil {
		return Account{}, fmt.Errorf("store provider account: %w", err)
	}

	if _, err := tx.Exec(ctx, "UPDATE provider_oauth_states SET consumed_at=now() WHERE state_hash=$1", hash[:]); err != nil {
		return Account{}, fmt.Errorf("consume oauth state: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Account{}, fmt.Errorf("commit oauth callback: %w", err)
	}
	return account, nil
}

func (s *Service) ListAccounts(ctx context.Context, userID string) ([]Account, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text,provider,external_account_id,display_name,status,
		       quota_total_bytes,quota_used_bytes,quota_free_bytes,last_synced_at,created_at,updated_at
		FROM provider_accounts
		WHERE user_id=$1::uuid
		ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("list provider accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0)
	for rows.Next() {
		var account Account
		if err := rows.Scan(&account.ID, &account.Provider, &account.ExternalAccountID, &account.DisplayName,
			&account.Status, &account.QuotaTotalBytes, &account.QuotaUsedBytes, &account.QuotaFreeBytes,
			&account.LastSyncedAt, &account.CreatedAt, &account.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan provider account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider accounts: %w", err)
	}
	return accounts, nil
}

func (s *Service) exchangeGoogleCode(ctx context.Context, code, redirectURI string) (googleTokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", s.cfg.GoogleClientID)
	form.Set("client_secret", s.cfg.GoogleClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return googleTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return googleTokenResponse{}, fmt.Errorf("%w: %v", ErrOAuthExchange, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return googleTokenResponse{}, fmt.Errorf("%w: read token response", ErrOAuthExchange)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return googleTokenResponse{}, fmt.Errorf("%w: google returned %s", ErrOAuthExchange, resp.Status)
	}

	var token googleTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return googleTokenResponse{}, fmt.Errorf("%w: decode token response", ErrOAuthExchange)
	}
	if token.AccessToken == "" {
		return googleTokenResponse{}, fmt.Errorf("%w: missing access token", ErrOAuthExchange)
	}
	return token, nil
}

func (s *Service) fetchGoogleAbout(ctx context.Context, accessToken string) (googleAbout, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleAboutURL, nil)
	if err != nil {
		return googleAbout{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return googleAbout{}, fmt.Errorf("fetch google account: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return googleAbout{}, fmt.Errorf("read google account: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return googleAbout{}, fmt.Errorf("fetch google account: google returned %s", resp.Status)
	}

	var about googleAbout
	if err := json.Unmarshal(body, &about); err != nil {
		return googleAbout{}, fmt.Errorf("decode google account: %w", err)
	}
	return about, nil
}

func decodeEncryptionKey(encoded string) ([]byte, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode token encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("CLUSTERSTOR_TOKEN_ENCRYPTION_KEY must decode to exactly 32 bytes")
	}
	return key, nil
}

func encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func parseOptionalInt64(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(value, 10, 64)
	return n, err == nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func cloneBody(body []byte) io.Reader {
	return bytes.NewReader(body)
}
