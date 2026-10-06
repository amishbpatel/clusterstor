package providers

import (
	"context"
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
)

const (
	googleFilesURL      = "https://www.googleapis.com/drive/v3/files"
	googleFolderMIME    = "application/vnd.google-apps.folder"
	defaultDrivePageSize = 100
	maxDrivePageSize     = 500
)

var ErrProviderAccountNotFound = errors.New("provider account not found")

type DriveItem struct {
	NodeID         string     `json:"node_id"`
	Provider       string     `json:"provider"`
	ProviderItemID string     `json:"provider_item_id"`
	ParentItemID   *string    `json:"parent_item_id,omitempty"`
	Name           string     `json:"name"`
	NodeType       string     `json:"node_type"`
	MIMEType       string     `json:"mime_type,omitempty"`
	SizeBytes      *int64     `json:"size_bytes,omitempty"`
	ModifiedAt     *time.Time `json:"modified_at,omitempty"`
}

type DrivePage struct {
	Items          []DriveItem `json:"items"`
	NextPageToken  string      `json:"next_page_token,omitempty"`
}

type googleFilesResponse struct {
	NextPageToken string       `json:"nextPageToken"`
	Files         []googleFile `json:"files"`
}

type googleFile struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	MIMEType     string   `json:"mimeType"`
	Parents      []string `json:"parents"`
	Size         string   `json:"size"`
	ModifiedTime string   `json:"modifiedTime"`
	Trashed      bool     `json:"trashed"`
}

func (s *Service) SyncGoogleDrive(ctx context.Context, userID, pageToken string, pageSize int) (DrivePage, error) {
	if pageSize <= 0 {
		pageSize = defaultDrivePageSize
	}
	if pageSize > maxDrivePageSize {
		pageSize = maxDrivePageSize
	}

	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil {
		return DrivePage{}, err
	}

	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil {
		return DrivePage{}, err
	}

	page, err := s.fetchGoogleFiles(ctx, token.AccessToken, pageToken, pageSize)
	if err != nil {
		return DrivePage{}, err
	}

	items, err := s.upsertGoogleFiles(ctx, userID, accountID, page.Files)
	if err != nil {
		return DrivePage{}, err
	}

	if _, err := s.pool.Exec(ctx,
		"UPDATE provider_accounts SET last_synced_at=now(), updated_at=now() WHERE id=$1::uuid",
		accountID); err != nil {
		return DrivePage{}, fmt.Errorf("update provider sync time: %w", err)
	}

	return DrivePage{Items: items, NextPageToken: page.NextPageToken}, nil
}

func (s *Service) googleCredential(ctx context.Context, userID string) (string, storedToken, error) {
	var accountID string
	var ciphertext []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, token_ciphertext
		FROM provider_accounts
		WHERE user_id=$1::uuid
		  AND provider='google_drive'
		  AND status='connected'
		  AND disconnected_at IS NULL`, userID).
		Scan(&accountID, &ciphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", storedToken{}, ErrProviderAccountNotFound
	}
	if err != nil {
		return "", storedToken{}, fmt.Errorf("load google drive account: %w", err)
	}
	if len(ciphertext) == 0 || len(s.key) != 32 {
		return "", storedToken{}, ErrProviderNotConfigured
	}

	plaintext, err := decrypt(s.key, ciphertext)
	if err != nil {
		return "", storedToken{}, fmt.Errorf("decrypt google credentials: %w", err)
	}

	var token storedToken
	if err := json.Unmarshal(plaintext, &token); err != nil {
		return "", storedToken{}, fmt.Errorf("decode google credentials: %w", err)
	}
	if token.AccessToken == "" && token.RefreshToken == "" {
		return "", storedToken{}, ErrProviderNotConfigured
	}
	return accountID, token, nil
}

func (s *Service) ensureGoogleAccessToken(ctx context.Context, accountID string, token storedToken) (storedToken, error) {
	if token.AccessToken != "" && time.Until(token.ExpiresAt) > time.Minute {
		return token, nil
	}
	if token.RefreshToken == "" {
		return storedToken{}, fmt.Errorf("%w: refresh token unavailable", ErrOAuthExchange)
	}

	form := url.Values{}
	form.Set("client_id", s.cfg.GoogleClientID)
	form.Set("client_secret", s.cfg.GoogleClientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", token.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return storedToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return storedToken{}, fmt.Errorf("%w: refresh token: %v", ErrOAuthExchange, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return storedToken{}, fmt.Errorf("%w: read refresh response", ErrOAuthExchange)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return storedToken{}, fmt.Errorf("%w: refresh returned %s", ErrOAuthExchange, resp.Status)
	}

	var refreshed googleTokenResponse
	if err := json.Unmarshal(body, &refreshed); err != nil {
		return storedToken{}, fmt.Errorf("%w: decode refresh response", ErrOAuthExchange)
	}
	if refreshed.AccessToken == "" {
		return storedToken{}, fmt.Errorf("%w: refresh missing access token", ErrOAuthExchange)
	}

	token.AccessToken = refreshed.AccessToken
	token.ExpiresAt = time.Now().UTC().Add(time.Duration(refreshed.ExpiresIn) * time.Second)
	if refreshed.TokenType != "" {
		token.TokenType = refreshed.TokenType
	}
	if refreshed.Scope != "" {
		token.Scope = refreshed.Scope
	}

	encoded, err := json.Marshal(token)
	if err != nil {
		return storedToken{}, fmt.Errorf("encode refreshed token: %w", err)
	}
	ciphertext, err := encrypt(s.key, encoded)
	if err != nil {
		return storedToken{}, fmt.Errorf("encrypt refreshed token: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		"UPDATE provider_accounts SET token_ciphertext=$1, updated_at=now() WHERE id=$2::uuid",
		ciphertext, accountID); err != nil {
		return storedToken{}, fmt.Errorf("store refreshed token: %w", err)
	}
	return token, nil
}

func (s *Service) fetchGoogleFiles(ctx context.Context, accessToken, pageToken string, pageSize int) (googleFilesResponse, error) {
	q := url.Values{}
	q.Set("q", "trashed = false")
	q.Set("spaces", "drive")
	q.Set("pageSize", strconv.Itoa(pageSize))
	q.Set("orderBy", "folder,name_natural")
	q.Set("fields", "nextPageToken,files(id,name,mimeType,parents,size,modifiedTime)")
	q.Set("supportsAllDrives", "true")
	q.Set("includeItemsFromAllDrives", "true")
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleFilesURL+"?"+q.Encode(), nil)
	if err != nil {
		return googleFilesResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return googleFilesResponse{}, fmt.Errorf("list google drive files: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return googleFilesResponse{}, fmt.Errorf("read google drive files: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return googleFilesResponse{}, fmt.Errorf("list google drive files: google returned %s", resp.Status)
	}

	var page googleFilesResponse
	if err := json.Unmarshal(body, &page); err != nil {
		return googleFilesResponse{}, fmt.Errorf("decode google drive files: %w", err)
	}
	return page, nil
}

func (s *Service) upsertGoogleFiles(ctx context.Context, userID, accountID string, files []googleFile) ([]DriveItem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin google drive sync: %w", err)
	}
	defer tx.Rollback(ctx)

	items := make([]DriveItem, 0, len(files))
	for _, file := range files {
		if strings.TrimSpace(file.ID) == "" || strings.TrimSpace(file.Name) == "" {
			continue
		}

		nodeType := "file"
		if file.MIMEType == googleFolderMIME {
			nodeType = "folder"
		}

		var size *int64
		if value, ok := parseOptionalInt64(file.Size); ok {
			size = &value
		}
		var modified *time.Time
		if value, err := time.Parse(time.RFC3339, file.ModifiedTime); err == nil {
			modified = &value
		}

		var parentItemID *string
		if len(file.Parents) > 0 && strings.TrimSpace(file.Parents[0]) != "" {
			value := file.Parents[0]
			parentItemID = &value
		}

		var nodeID string
		err := tx.QueryRow(ctx, `
			SELECT node_id::text
			FROM provider_items
			WHERE provider_account_id=$1::uuid AND provider_item_id=$2`,
			accountID, file.ID).Scan(&nodeID)

		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				INSERT INTO nodes(user_id,node_type,name,state)
				VALUES ($1::uuid,$2,$3,'active')
				RETURNING id::text`,
				userID, nodeType, file.Name).Scan(&nodeID)
			if err != nil {
				return nil, fmt.Errorf("create clusterstor node: %w", err)
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO provider_items(
					provider_account_id,node_id,provider_item_id,provider_parent_item_id,
					mime_type,size_bytes,modified_at
				) VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7)`,
				accountID, nodeID, file.ID, parentItemID, nullableString(file.MIMEType), size, modified)
			if err != nil {
				return nil, fmt.Errorf("map google drive item: %w", err)
			}
		} else if err != nil {
			return nil, fmt.Errorf("lookup google drive item: %w", err)
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE nodes
				SET node_type=$1,name=$2,state='active',updated_at=now(),deleted_at=NULL
				WHERE id=$3::uuid AND user_id=$4::uuid`,
				nodeType, file.Name, nodeID, userID)
			if err != nil {
				return nil, fmt.Errorf("update clusterstor node: %w", err)
			}
			_, err = tx.Exec(ctx, `
				UPDATE provider_items
				SET provider_parent_item_id=$1,mime_type=$2,size_bytes=$3,modified_at=$4,updated_at=now()
				WHERE provider_account_id=$5::uuid AND provider_item_id=$6`,
				parentItemID, nullableString(file.MIMEType), size, modified, accountID, file.ID)
			if err != nil {
				return nil, fmt.Errorf("update google drive item mapping: %w", err)
			}
		}

		items = append(items, DriveItem{
			NodeID: nodeID,
			Provider: "google_drive",
			ProviderItemID: file.ID,
			ParentItemID: parentItemID,
			Name: file.Name,
			NodeType: nodeType,
			MIMEType: file.MIMEType,
			SizeBytes: size,
			ModifiedAt: modified,
		})
	}

	_, err = tx.Exec(ctx, `
		UPDATE nodes n
		SET parent_id=parent_map.node_id, updated_at=now()
		FROM provider_items child_map
		JOIN provider_items parent_map
		  ON parent_map.provider_account_id=child_map.provider_account_id
		 AND parent_map.provider_item_id=child_map.provider_parent_item_id
		WHERE child_map.provider_account_id=$1::uuid
		  AND n.id=child_map.node_id
		  AND n.parent_id IS DISTINCT FROM parent_map.node_id`,
		accountID)
	if err != nil {
		return nil, fmt.Errorf("resolve google drive parents: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit google drive sync: %w", err)
	}
	return items, nil
}
