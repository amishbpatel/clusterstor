package devices

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidDevice = errors.New("invalid device")
	ErrNotFound = errors.New("device not found")
)

type Service struct { pool *pgxpool.Pool }

type Device struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Platform string `json:"platform"`
	AgentVersion *string `json:"agent_version,omitempty"`
	Status string `json:"status"`
	PeerContributionEnabled bool `json:"peer_contribution_enabled"`
	PeerContributionBytes int64 `json:"peer_contribution_bytes"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type Registration struct {
	Device Device `json:"device"`
	Secret string `json:"secret"`
}

type RegisterInput struct {
	Name string
	Platform string
	AgentVersion *string
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Register(ctx context.Context, userID string, input RegisterInput) (Registration, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Platform = strings.TrimSpace(input.Platform)
	if input.Name == "" || input.Platform == "" { return Registration{}, ErrInvalidDevice }

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil { return Registration{}, fmt.Errorf("create credential: %w", err) }
	secret := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(secret))

	tx, err := s.pool.Begin(ctx)
	if err != nil { return Registration{}, fmt.Errorf("begin device registration: %w", err) }
	defer tx.Rollback(ctx)

	var device Device
	err = tx.QueryRow(ctx, "INSERT INTO devices(user_id,name,platform,agent_version) VALUES ($1::uuid,$2,$3,$4) RETURNING id::text,name,platform,agent_version,status,peer_contribution_enabled,peer_contribution_bytes,last_seen_at,created_at,revoked_at",
		userID, input.Name, input.Platform, cleanString(input.AgentVersion)).
		Scan(&device.ID, &device.Name, &device.Platform, &device.AgentVersion, &device.Status, &device.PeerContributionEnabled, &device.PeerContributionBytes, &device.LastSeenAt, &device.CreatedAt, &device.RevokedAt)
	if err != nil { return Registration{}, fmt.Errorf("create device: %w", err) }

	if _, err := tx.Exec(ctx, "INSERT INTO device_credentials(device_id,secret_hash) VALUES ($1::uuid,$2)", device.ID, hash[:]); err != nil {
		return Registration{}, fmt.Errorf("store device credential: %w", err)
	}
	if err := tx.Commit(ctx); err != nil { return Registration{}, fmt.Errorf("commit device registration: %w", err) }
	return Registration{Device: device, Secret: secret}, nil
}

func (s *Service) List(ctx context.Context, userID string) ([]Device, error) {
	rows, err := s.pool.Query(ctx, "SELECT id::text,name,platform,agent_version,status,peer_contribution_enabled,peer_contribution_bytes,last_seen_at,created_at,revoked_at FROM devices WHERE user_id=$1::uuid ORDER BY created_at DESC", userID)
	if err != nil { return nil, fmt.Errorf("list devices: %w", err) }
	defer rows.Close()

	result := make([]Device, 0)
	for rows.Next() {
		var device Device
		if err := rows.Scan(&device.ID, &device.Name, &device.Platform, &device.AgentVersion, &device.Status, &device.PeerContributionEnabled, &device.PeerContributionBytes, &device.LastSeenAt, &device.CreatedAt, &device.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		result = append(result, device)
	}
	if err := rows.Err(); err != nil { return nil, fmt.Errorf("iterate devices: %w", err) }
	return result, nil
}

func (s *Service) Revoke(ctx context.Context, userID, deviceID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return fmt.Errorf("begin device revoke: %w", err) }
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, "UPDATE devices SET status='revoked', revoked_at=COALESCE(revoked_at,now()) WHERE id=$1::uuid AND user_id=$2::uuid AND revoked_at IS NULL", deviceID, userID)
	if err != nil { return fmt.Errorf("revoke device: %w", err) }
	if tag.RowsAffected() == 0 { return ErrNotFound }

	if _, err := tx.Exec(ctx, "UPDATE device_credentials SET revoked_at=COALESCE(revoked_at,now()) WHERE device_id=$1::uuid AND revoked_at IS NULL", deviceID); err != nil {
		return fmt.Errorf("revoke credentials: %w", err)
	}
	if err := tx.Commit(ctx); err != nil { return fmt.Errorf("commit device revoke: %w", err) }
	return nil
}

func cleanString(value *string) *string {
	if value == nil { return nil }
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" { return nil }
	return &cleaned
}
