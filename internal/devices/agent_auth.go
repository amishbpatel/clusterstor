package devices

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Heartbeat(ctx context.Context, deviceID, secret string, agentVersion *string) (Device,error) {
	deviceID=strings.TrimSpace(deviceID)
	secret=strings.TrimSpace(secret)
	if deviceID=="" || secret=="" { return Device{},ErrNotFound }
	hash:=sha256.Sum256([]byte(secret))

	var device Device
	err:=s.pool.QueryRow(ctx,`
		UPDATE devices d
		SET last_seen_at=now(),
		    agent_version=COALESCE($3, d.agent_version)
		FROM device_credentials dc
		WHERE d.id=$1::uuid
		  AND dc.device_id=d.id
		  AND dc.secret_hash=$2
		  AND d.revoked_at IS NULL
		  AND dc.revoked_at IS NULL
		RETURNING d.id::text,d.name,d.platform,d.agent_version,d.status,
		          d.peer_contribution_enabled,d.peer_contribution_bytes,d.last_seen_at,d.created_at,d.revoked_at`,
		deviceID,hash[:],cleanString(agentVersion)).
		Scan(&device.ID,&device.Name,&device.Platform,&device.AgentVersion,&device.Status,
			&device.PeerContributionEnabled,&device.PeerContributionBytes,&device.LastSeenAt,&device.CreatedAt,&device.RevokedAt)
	if errors.Is(err,pgx.ErrNoRows) { return Device{},ErrNotFound }
	if err!=nil { return Device{},fmt.Errorf("device heartbeat: %w",err) }
	return device,nil
}
