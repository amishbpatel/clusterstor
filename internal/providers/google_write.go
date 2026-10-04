package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const googleUploadFilesURL = "https://www.googleapis.com/upload/drive/v3/files"

var ErrInvalidProviderParent = errors.New("invalid provider parent")

type CreateFolderInput struct {
	Name string
	ParentNodeID *string
}

type UploadSessionInput struct {
	Name string
	ContentType string
	SizeBytes int64
	ParentNodeID *string
}

type UploadSession struct {
	UploadURL string `json:"upload_url"`
	ParentProviderID string `json:"parent_provider_id"`
	Name string `json:"name"`
	ContentType string `json:"content_type"`
	SizeBytes int64 `json:"size_bytes"`
}

func (s *Service) CreateGoogleFolder(ctx context.Context, userID string, input CreateFolderInput) (DriveItem, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" { return DriveItem{}, errors.New("folder name is required") }
	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil { return DriveItem{}, err }
	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil { return DriveItem{}, err }
	parentID, parentNodeID, err := s.googleManagedParent(ctx, userID, accountID, input.ParentNodeID)
	if err != nil { return DriveItem{}, err }
	payload, err := json.Marshal(map[string]any{"name": name, "mimeType": googleFolderMIME, "parents": []string{parentID}})
	if err != nil { return DriveItem{}, err }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleFilesURL+"?fields=id,name,mimeType,parents,modifiedTime", bytes.NewReader(payload))
	if err != nil { return DriveItem{}, err }
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil { return DriveItem{}, fmt.Errorf("create google folder: %w", err) }
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil { return DriveItem{}, fmt.Errorf("read google folder creation: %w", err) }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return DriveItem{}, fmt.Errorf("create google folder: google returned %s", resp.Status) }
	var file googleFile
	if err := json.Unmarshal(body, &file); err != nil { return DriveItem{}, fmt.Errorf("decode google folder creation: %w", err) }
	if strings.TrimSpace(file.ID) == "" { return DriveItem{}, errors.New("create google folder: missing folder id") }
	items, err := s.upsertGoogleFiles(ctx, userID, accountID, []googleFile{file})
	if err != nil { return DriveItem{}, err }
	if len(items) != 1 { return DriveItem{}, errors.New("create google folder: mapping failed") }
	if parentNodeID != nil {
		if _, err := s.pool.Exec(ctx, "UPDATE nodes SET parent_id=$1::uuid, updated_at=now() WHERE id=$2::uuid AND user_id=$3::uuid", *parentNodeID, items[0].NodeID, userID); err != nil {
			return DriveItem{}, fmt.Errorf("link folder parent: %w", err)
		}
	}
	return items[0], nil
}

func (s *Service) BeginGoogleUpload(ctx context.Context, userID string, input UploadSessionInput) (UploadSession, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" { return UploadSession{}, errors.New("file name is required") }
	if input.SizeBytes < 0 { return UploadSession{}, errors.New("file size must not be negative") }
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" { contentType = "application/octet-stream" }
	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil { return UploadSession{}, err }
	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil { return UploadSession{}, err }
	parentID, _, err := s.googleManagedParent(ctx, userID, accountID, input.ParentNodeID)
	if err != nil { return UploadSession{}, err }
	metadata, err := json.Marshal(map[string]any{"name": name, "parents": []string{parentID}})
	if err != nil { return UploadSession{}, err }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleUploadFilesURL+"?uploadType=resumable&fields=id,name,mimeType,parents,size,modifiedTime", bytes.NewReader(metadata))
	if err != nil { return UploadSession{}, err }
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", contentType)
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(input.SizeBytes, 10))
	resp, err := s.httpClient.Do(req)
	if err != nil { return UploadSession{}, fmt.Errorf("start google upload: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return UploadSession{}, fmt.Errorf("start google upload: google returned %s", resp.Status) }
	location := strings.TrimSpace(resp.Header.Get("Location"))
	if location == "" { return UploadSession{}, errors.New("start google upload: missing upload session URL") }
	return UploadSession{UploadURL: location, ParentProviderID: parentID, Name: name, ContentType: contentType, SizeBytes: input.SizeBytes}, nil
}

func (s *Service) googleManagedParent(ctx context.Context, userID, accountID string, parentNodeID *string) (string, *string, error) {
	rootID, err := s.EnsureGoogleRoot(ctx, userID)
	if err != nil { return "", nil, err }
	if parentNodeID == nil || strings.TrimSpace(*parentNodeID) == "" { return rootID, nil, nil }
	nodeID := strings.TrimSpace(*parentNodeID)
	var providerItemID, nodeType string
	err = s.pool.QueryRow(ctx, "SELECT pi.provider_item_id,n.node_type FROM provider_items pi JOIN nodes n ON n.id=pi.node_id WHERE pi.provider_account_id=$1::uuid AND pi.node_id=$2::uuid AND n.user_id=$3::uuid AND n.deleted_at IS NULL", accountID, nodeID, userID).Scan(&providerItemID, &nodeType)
	if err != nil || nodeType != "folder" { return "", nil, ErrInvalidProviderParent }
	var directParent *string
	err = s.pool.QueryRow(ctx, "SELECT provider_parent_item_id FROM provider_items WHERE provider_account_id=$1::uuid AND node_id=$2::uuid", accountID, nodeID).Scan(&directParent)
	if err != nil { return "", nil, ErrInvalidProviderParent }
	if directParent != nil && *directParent == rootID { return providerItemID, &nodeID, nil }
	var insideRoot bool
	err = s.pool.QueryRow(ctx, "WITH RECURSIVE ancestry AS (SELECT provider_item_id,provider_parent_item_id FROM provider_items WHERE provider_account_id=$1::uuid AND node_id=$2::uuid UNION ALL SELECT p.provider_item_id,p.provider_parent_item_id FROM provider_items p JOIN ancestry a ON a.provider_parent_item_id=p.provider_item_id WHERE p.provider_account_id=$1::uuid) SELECT EXISTS(SELECT 1 FROM ancestry WHERE provider_item_id=$3 OR provider_parent_item_id=$3)", accountID, nodeID, rootID).Scan(&insideRoot)
	if err != nil { return "", nil, fmt.Errorf("validate provider parent: %w", err) }
	if !insideRoot { return "", nil, ErrInvalidProviderParent }
	return providerItemID, &nodeID, nil
}
