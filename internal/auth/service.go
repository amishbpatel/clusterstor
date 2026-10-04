package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	passwordIterations = 600000
	sessionTTL = 30 * 24 * time.Hour
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailInUse = errors.New("email already in use")
	ErrInvalidEmail = errors.New("invalid email")
	ErrWeakPassword = errors.New("password must be at least 12 characters")
	ErrInvalidSession = errors.New("invalid session")
)

type Service struct { pool *pgxpool.Pool }

type User struct {
	ID string `json:"id"`
	Email string `json:"email"`
	DisplayName *string `json:"display_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	Token string `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User User `json:"user"`
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Signup(ctx context.Context, email, password string, displayName *string) (Session, error) {
	email = normalizeEmail(email)
	if !validEmail(email) { return Session{}, ErrInvalidEmail }
	if len(password) < 12 { return Session{}, ErrWeakPassword }

	passwordHash, err := hashPassword(password)
	if err != nil { return Session{}, fmt.Errorf("hash password: %w", err) }

	tx, err := s.pool.Begin(ctx)
	if err != nil { return Session{}, fmt.Errorf("begin signup: %w", err) }
	defer tx.Rollback(ctx)

	var user User
	err = tx.QueryRow(ctx, "INSERT INTO users(email, display_name, password_hash) VALUES ($1,$2,$3) RETURNING id::text,email,display_name,created_at",
		email, cleanDisplayName(displayName), passwordHash).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { return Session{}, ErrEmailInUse }
		return Session{}, fmt.Errorf("create user: %w", err)
	}

	session, err := createSession(ctx, tx, user)
	if err != nil { return Session{}, err }
	if err := tx.Commit(ctx); err != nil { return Session{}, fmt.Errorf("commit signup: %w", err) }
	return session, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	var user User
	var passwordHash *string
	err := s.pool.QueryRow(ctx, "SELECT id::text,email,display_name,created_at,password_hash FROM users WHERE lower(email)=lower($1)", normalizeEmail(email)).
		Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) || passwordHash == nil { return Session{}, ErrInvalidCredentials }
	if err != nil { return Session{}, fmt.Errorf("load user: %w", err) }

	ok, err := verifyPassword(password, *passwordHash)
	if err != nil || !ok { return Session{}, ErrInvalidCredentials }
	return createSession(ctx, s.pool, user)
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" { return User{}, ErrInvalidSession }
	hash := sha256.Sum256([]byte(token))
	var user User
	err := s.pool.QueryRow(ctx, "SELECT u.id::text,u.email,u.display_name,u.created_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now()", hash[:]).
		Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) { return User{}, ErrInvalidSession }
	if err != nil { return User{}, fmt.Errorf("authenticate session: %w", err) }
	return user, nil
}

func (s *Service) RevokeSession(ctx context.Context, token string) error {
	hash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	tag, err := s.pool.Exec(ctx, "UPDATE sessions SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL", hash[:])
	if err != nil { return fmt.Errorf("revoke session: %w", err) }
	if tag.RowsAffected() == 0 { return ErrInvalidSession }
	return nil
}

type queryRower interface { QueryRow(context.Context, string, ...any) pgx.Row }

func createSession(ctx context.Context, q queryRower, user User) (Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil { return Session{}, err }
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	expiresAt := time.Now().UTC().Add(sessionTTL)
	var id string
	if err := q.QueryRow(ctx, "INSERT INTO sessions(user_id,token_hash,expires_at) VALUES ($1::uuid,$2,$3) RETURNING id::text", user.ID, hash[:], expiresAt).Scan(&id); err != nil {
		return Session{}, fmt.Errorf("store session: %w", err)
	}
	return Session{Token: token, ExpiresAt: expiresAt, User: user}, nil
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func validEmail(email string) bool {
	at := strings.LastIndex(email, "@")
	return at > 0 && at < len(email)-1 && strings.Contains(email[at+1:], ".")
}

func cleanDisplayName(value *string) *string {
	if value == nil { return nil }
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" { return nil }
	return &cleaned
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil { return "", err }
	key := pbkdf2SHA256([]byte(password), salt, passwordIterations, 32)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" { return false, errors.New("unsupported password hash") }
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 { return false, errors.New("invalid password hash") }
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil { return false, err }
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil { return false, err }
	actual := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hashLen := sha256.Size
	blocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, blocks*hashLen)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t { t[j] ^= u[j] }
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
