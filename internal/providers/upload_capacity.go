package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrInsufficientProviderSpace = errors.New("insufficient provider space")
var ErrUnsupportedProvider = errors.New("unsupported provider")

type UploadCapacity struct {
	Provider string `json:"provider"`
	RequestedBytes int64 `json:"requested_bytes"`
	TotalBytes *int64 `json:"total_bytes,omitempty"`
	UsedBytes *int64 `json:"used_bytes,omitempty"`
	FreeBytes *int64 `json:"free_bytes,omitempty"`
	Allowed bool `json:"allowed"`
}

func (s *Service) CheckUploadCapacity(ctx context.Context, userID, provider string, requestedBytes int64) (UploadCapacity, error) {
	if requestedBytes < 0 { return UploadCapacity{}, errors.New("requested bytes must not be negative") }
	switch strings.TrimSpace(provider) {
	case "google_drive":
		return s.checkGoogleUploadCapacity(ctx,userID,requestedBytes)
	default:
		return UploadCapacity{}, ErrUnsupportedProvider
	}
}

func (s *Service) checkGoogleUploadCapacity(ctx context.Context, userID string, requestedBytes int64) (UploadCapacity, error) {
	accountID, token, err := s.googleCredential(ctx,userID)
	if err != nil { return UploadCapacity{},err }
	token, err = s.ensureGoogleAccessToken(ctx,accountID,token)
	if err != nil { return UploadCapacity{},err }

	about, err := s.fetchGoogleAbout(ctx,token.AccessToken)
	if err != nil { return UploadCapacity{},err }

	result := UploadCapacity{Provider:"google_drive",RequestedBytes:requestedBytes,Allowed:true}
	if value, ok := parseOptionalInt64(about.StorageQuota.Limit); ok { result.TotalBytes=&value }
	if value, ok := parseOptionalInt64(about.StorageQuota.Usage); ok { result.UsedBytes=&value }
	if result.TotalBytes != nil && result.UsedBytes != nil {
		free := *result.TotalBytes - *result.UsedBytes
		if free < 0 { free=0 }
		result.FreeBytes=&free
		if requestedBytes > free { result.Allowed=false }
	}

	_, updateErr := s.pool.Exec(ctx, `
		UPDATE provider_accounts
		SET quota_total_bytes=$1,quota_used_bytes=$2,quota_free_bytes=$3,last_synced_at=now(),updated_at=now()
		WHERE id=$4::uuid`,
		result.TotalBytes,result.UsedBytes,result.FreeBytes,accountID)
	if updateErr != nil { return UploadCapacity{},fmt.Errorf("update google quota: %w",updateErr) }

	if !result.Allowed { return result,ErrInsufficientProviderSpace }
	return result,nil
}
