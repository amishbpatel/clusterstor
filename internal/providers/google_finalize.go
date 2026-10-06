package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

var ErrUploadedFileOutsideRoot = errors.New("uploaded file is outside ClusterStor root")

type FinalizeUploadInput struct {
	ProviderItemID string
}

type FinalizedUpload struct {
	NodeID string `json:"node_id"`
	VersionID string `json:"version_id"`
	ProviderItemID string `json:"provider_item_id"`
	Name string `json:"name"`
	SizeBytes int64 `json:"size_bytes"`
}

func (s *Service) FinalizeGoogleUpload(ctx context.Context, userID string, input FinalizeUploadInput) (FinalizedUpload, error) {
	providerItemID := strings.TrimSpace(input.ProviderItemID)
	if providerItemID == "" { return FinalizedUpload{}, errors.New("provider item id is required") }
	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil { return FinalizedUpload{}, err }
	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil { return FinalizedUpload{}, err }
	file, err := s.fetchGoogleFile(ctx, token.AccessToken, providerItemID)
	if err != nil { return FinalizedUpload{}, err }
	if file.MIMEType == googleFolderMIME { return FinalizedUpload{}, errors.New("uploaded item is a folder") }
	inside, err := s.googleFileInsideManagedRoot(ctx, accountID, file)
	if err != nil { return FinalizedUpload{}, err }
	if !inside { return FinalizedUpload{}, ErrUploadedFileOutsideRoot }
	items, err := s.upsertGoogleFiles(ctx, userID, accountID, []googleFile{file})
	if err != nil { return FinalizedUpload{}, err }
	if len(items) != 1 { return FinalizedUpload{}, errors.New("uploaded file mapping failed") }
	size := int64(0)
	if items[0].SizeBytes != nil { size = *items[0].SizeBytes }
	revisionID := strings.TrimSpace(file.HeadRevisionID)
	if revisionID != "" {
		if err := s.keepGoogleRevision(ctx, token.AccessToken, providerItemID, revisionID); err != nil {
			return FinalizedUpload{}, err
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil { return FinalizedUpload{}, fmt.Errorf("begin upload finalize: %w", err) }
	defer tx.Rollback(ctx)
	var nextVersion int64
	err = tx.QueryRow(ctx, "SELECT COALESCE(MAX(version_number),0)+1 FROM file_versions WHERE node_id=$1::uuid", items[0].NodeID).Scan(&nextVersion)
	if err != nil { return FinalizedUpload{}, fmt.Errorf("next file version: %w", err) }
	var versionID string
	err = tx.QueryRow(ctx, "INSERT INTO file_versions(node_id,version_number,size_bytes) VALUES ($1::uuid,$2,$3) RETURNING id::text", items[0].NodeID, nextVersion, size).Scan(&versionID)
	if err != nil { return FinalizedUpload{}, fmt.Errorf("create file version: %w", err) }
	_, err = tx.Exec(ctx, "INSERT INTO storage_objects(version_id,storage_class,provider_account_id,provider_object_id,provider_revision_id,size_bytes,encryption_mode,state,verified_at) VALUES ($1::uuid,'provider',$2::uuid,$3,$4,$5,'provider_native','available',now())", versionID, accountID, providerItemID, nullableString(revisionID), size)
	if err != nil { return FinalizedUpload{}, fmt.Errorf("create storage object: %w", err) }
	_, err = tx.Exec(ctx, "UPDATE nodes SET current_version_id=$1::uuid,state='active',updated_at=now() WHERE id=$2::uuid AND user_id=$3::uuid", versionID, items[0].NodeID, userID)
	if err != nil { return FinalizedUpload{}, fmt.Errorf("activate file version: %w", err) }
	payload, _ := json.Marshal(map[string]any{"provider":"google_drive","provider_item_id":providerItemID,"provider_revision_id":revisionID,"version_id":versionID,"size_bytes":size})
	_, err = tx.Exec(ctx, "INSERT INTO account_events(user_id,event_type,resource_type,resource_id,payload) VALUES ($1::uuid,'file.version.created','node',$2::uuid,$3::jsonb)", userID, items[0].NodeID, string(payload))
	if err != nil { return FinalizedUpload{}, fmt.Errorf("record upload event: %w", err) }
	if err := tx.Commit(ctx); err != nil { return FinalizedUpload{}, fmt.Errorf("commit upload finalize: %w", err) }
	return FinalizedUpload{NodeID: items[0].NodeID, VersionID: versionID, ProviderItemID: providerItemID, Name: items[0].Name, SizeBytes: size}, nil
}

func (s *Service) fetchGoogleFile(ctx context.Context, accessToken, itemID string) (googleFile, error) {
	fields := url.QueryEscape("id,name,mimeType,parents,size,modifiedTime,headRevisionId")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleFilesURL+"/"+url.PathEscape(itemID)+"?fields="+fields+"&supportsAllDrives=true", nil)
	if err != nil { return googleFile{}, err }
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil { return googleFile{}, fmt.Errorf("fetch uploaded google file: %w", err) }
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil { return googleFile{}, fmt.Errorf("read uploaded google file: %w", err) }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return googleFile{}, fmt.Errorf("fetch uploaded google file: google returned %s", resp.Status) }
	var file googleFile
	if err := json.Unmarshal(body, &file); err != nil { return googleFile{}, fmt.Errorf("decode uploaded google file: %w", err) }
	return file, nil
}

func (s *Service) googleFileInsideManagedRoot(ctx context.Context, accountID string, file googleFile) (bool, error) {
	var rootID *string
	if err := s.pool.QueryRow(ctx, "SELECT root_provider_item_id FROM provider_accounts WHERE id=$1::uuid", accountID).Scan(&rootID); err != nil { return false, err }
	if rootID == nil || *rootID == "" { return false, nil }
	for _, parent := range file.Parents { if parent == *rootID { return true, nil } }
	for _, parent := range file.Parents {
		var inside bool
		err := s.pool.QueryRow(ctx, "WITH RECURSIVE ancestry AS (SELECT provider_item_id,provider_parent_item_id FROM provider_items WHERE provider_account_id=$1::uuid AND provider_item_id=$2 UNION ALL SELECT p.provider_item_id,p.provider_parent_item_id FROM provider_items p JOIN ancestry a ON a.provider_parent_item_id=p.provider_item_id WHERE p.provider_account_id=$1::uuid) SELECT EXISTS(SELECT 1 FROM ancestry WHERE provider_item_id=$3 OR provider_parent_item_id=$3)", accountID, parent, *rootID).Scan(&inside)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) { return false, fmt.Errorf("validate uploaded file parent: %w", err) }
		if inside { return true, nil }
	}
	return false, nil
}

