package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

var ErrDownloadNotFound = errors.New("download not found")

type DownloadStream struct {
	Response *http.Response
	Name string
	SizeBytes int64
}

func (s *Service) OpenGoogleDownload(ctx context.Context, userID, nodeID, rangeHeader string) (DownloadStream, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" { return DownloadStream{}, ErrDownloadNotFound }

	var accountID, providerItemID, name string
	var sizeBytes int64
	err := s.pool.QueryRow(ctx, `
		SELECT pa.id::text, so.provider_object_id, n.name, fv.size_bytes
		FROM nodes n
		JOIN file_versions fv ON fv.id=n.current_version_id
		JOIN storage_objects so ON so.version_id=fv.id AND so.state='available' AND so.deleted_at IS NULL
		JOIN provider_accounts pa ON pa.id=so.provider_account_id
		WHERE n.id=$1::uuid
		  AND n.user_id=$2::uuid
		  AND n.node_type='file'
		  AND n.deleted_at IS NULL
		  AND pa.provider='google_drive'
		  AND pa.status='connected'
		  AND pa.disconnected_at IS NULL
		ORDER BY so.verified_at DESC NULLS LAST, so.created_at DESC
		LIMIT 1`, nodeID, userID).Scan(&accountID, &providerItemID, &name, &sizeBytes)

	if errors.Is(err, pgx.ErrNoRows) {
		// Provider-backed files discovered during Drive sync may not have a
		// ClusterStor file_version/storage_object yet. They are still valid
		// downloadable files as long as the provider mapping belongs to this user.
		err = s.pool.QueryRow(ctx, `
			SELECT pa.id::text, pi.provider_item_id, n.name, COALESCE(pi.size_bytes,0)
			FROM nodes n
			JOIN provider_items pi ON pi.node_id=n.id
			JOIN provider_accounts pa ON pa.id=pi.provider_account_id
			WHERE n.id=$1::uuid
			  AND n.user_id=$2::uuid
			  AND n.node_type='file'
			  AND n.deleted_at IS NULL
			  AND pa.user_id=$2::uuid
			  AND pa.provider='google_drive'
			  AND pa.status='connected'
			  AND pa.disconnected_at IS NULL
			LIMIT 1`, nodeID, userID).Scan(&accountID, &providerItemID, &name, &sizeBytes)
	}
	if errors.Is(err, pgx.ErrNoRows) { return DownloadStream{}, ErrDownloadNotFound }
	if err != nil { return DownloadStream{}, fmt.Errorf("resolve google download: %w", err) }

	var ciphertext []byte
	err = s.pool.QueryRow(ctx, "SELECT token_ciphertext FROM provider_accounts WHERE id=$1::uuid", accountID).Scan(&ciphertext)
	if err != nil { return DownloadStream{}, fmt.Errorf("load google download credentials: %w", err) }
	if len(ciphertext) == 0 || len(s.key) != 32 { return DownloadStream{}, ErrProviderNotConfigured }
	plaintext, err := decrypt(s.key, ciphertext)
	if err != nil { return DownloadStream{}, fmt.Errorf("decrypt google download credentials: %w", err) }
	var token storedToken
	if err := json.Unmarshal(plaintext, &token); err != nil { return DownloadStream{}, fmt.Errorf("decode google download credentials: %w", err) }
	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil { return DownloadStream{}, err }

	downloadURL := googleFilesURL+"/"+url.PathEscape(providerItemID)+"?alt=media&supportsAllDrives=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil { return DownloadStream{}, err }
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "*/*")
	if strings.TrimSpace(rangeHeader) != "" { req.Header.Set("Range", rangeHeader) }

	resp, err := s.httpClient.Do(req)
	if err != nil { return DownloadStream{}, fmt.Errorf("open google download: %w", err) }
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return DownloadStream{}, fmt.Errorf("open google download: google returned %s", resp.Status)
	}
	return DownloadStream{Response: resp, Name: name, SizeBytes: sizeBytes}, nil
}
