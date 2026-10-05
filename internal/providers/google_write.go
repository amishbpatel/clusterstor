package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
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
	eventPayload, _ := json.Marshal(map[string]any{"provider":"google_drive","provider_item_id":items[0].ProviderItemID,"name":items[0].Name})
	if _, err := s.pool.Exec(ctx, "INSERT INTO account_events(user_id,event_type,resource_type,resource_id,payload) VALUES ($1::uuid,'folder.created','node',$2::uuid,$3::jsonb)", userID, items[0].NodeID, string(eventPayload)); err != nil {
		return DriveItem{}, fmt.Errorf("record folder event: %w", err)
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


var ErrDeleteNotFound = errors.New("delete target not found")

type deleteTarget struct {
	NodeID string
	ProviderItemID string
	Depth int
}

func (s *Service) DeleteGoogleNode(ctx context.Context, userID, nodeID string) (int, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" { return 0, ErrDeleteNotFound }

	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil { return 0, err }
	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil { return 0, err }

	var rootID *string
	if err := s.pool.QueryRow(ctx, "SELECT root_provider_item_id FROM provider_accounts WHERE id=$1::uuid", accountID).Scan(&rootID); err != nil {
		return 0, fmt.Errorf("load provider root: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT n.id, pi.provider_item_id, 0 AS depth
			FROM nodes n
			JOIN provider_items pi ON pi.node_id=n.id
			WHERE n.id=$1::uuid
			  AND n.user_id=$2::uuid
			  AND pi.provider_account_id=$3::uuid
			  AND n.deleted_at IS NULL
			UNION ALL
			SELECT child.id, child_pi.provider_item_id, subtree.depth + 1
			FROM subtree
			JOIN nodes child ON child.parent_id=subtree.id
			JOIN provider_items child_pi
			  ON child_pi.node_id=child.id
			 AND child_pi.provider_account_id=$3::uuid
			WHERE child.user_id=$2::uuid
			  AND child.deleted_at IS NULL
		)
		SELECT id::text, provider_item_id, depth
		FROM subtree
		ORDER BY depth DESC`, nodeID, userID, accountID)
	if err != nil { return 0, fmt.Errorf("load delete subtree: %w", err) }
	defer rows.Close()

	targets := make([]deleteTarget, 0)
	for rows.Next() {
		var target deleteTarget
		if err := rows.Scan(&target.NodeID, &target.ProviderItemID, &target.Depth); err != nil {
			return 0, fmt.Errorf("scan delete subtree: %w", err)
		}
		if rootID != nil && target.ProviderItemID == *rootID {
			return 0, ErrInvalidProviderParent
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil { return 0, fmt.Errorf("iterate delete subtree: %w", err) }
	if len(targets) == 0 { return 0, ErrDeleteNotFound }

	for _, target := range targets {
		payload := bytes.NewBufferString(`{"trashed":true}`)
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
			googleFilesURL+"/"+url.PathEscape(target.ProviderItemID)+"?supportsAllDrives=true",
			payload)
		if err != nil { return 0, err }
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := s.httpClient.Do(req)
		if err != nil { return 0, fmt.Errorf("trash google drive item: %w", err) }
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return 0, fmt.Errorf("trash google drive item: google returned %s", resp.Status)
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil { return 0, fmt.Errorf("begin delete: %w", err) }
	defer tx.Rollback(ctx)

	for _, target := range targets {
		if _, err := tx.Exec(ctx,
			"UPDATE nodes SET state='deleted', deleted_at=now(), updated_at=now() WHERE id=$1::uuid AND user_id=$2::uuid",
			target.NodeID, userID); err != nil {
			return 0, fmt.Errorf("mark node deleted: %w", err)
		}
	}

	eventPayload, _ := json.Marshal(map[string]any{
		"provider":"google_drive",
		"deleted_count":len(targets),
	})
	if _, err := tx.Exec(ctx,
		"INSERT INTO account_events(user_id,event_type,resource_type,resource_id,payload) VALUES ($1::uuid,'node.deleted','node',$2::uuid,$3::jsonb)",
		userID, nodeID, string(eventPayload)); err != nil {
		return 0, fmt.Errorf("record delete event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil { return 0, fmt.Errorf("commit delete: %w", err) }
	return len(targets), nil
}


func (s *Service) RenameGoogleNode(ctx context.Context, userID, nodeID, name string) (DriveItem, error) {
	nodeID = strings.TrimSpace(nodeID)
	name = strings.TrimSpace(name)
	if nodeID == "" || name == "" { return DriveItem{}, ErrDeleteNotFound }

	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil { return DriveItem{}, err }
	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil { return DriveItem{}, err }

	var providerItemID, nodeType string
	err = s.pool.QueryRow(ctx, `
		SELECT pi.provider_item_id, n.node_type
		FROM nodes n
		JOIN provider_items pi ON pi.node_id=n.id AND pi.provider_account_id=$3::uuid
		WHERE n.id=$1::uuid AND n.user_id=$2::uuid AND n.deleted_at IS NULL`,
		nodeID, userID, accountID).Scan(&providerItemID, &nodeType)
	if errors.Is(err, pgx.ErrNoRows) { return DriveItem{}, ErrDeleteNotFound }
	if err != nil { return DriveItem{}, fmt.Errorf("resolve rename target: %w", err) }

	payload, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		googleFilesURL+"/"+url.PathEscape(providerItemID)+"?fields=id,name,mimeType,parents,size,modifiedTime&supportsAllDrives=true",
		bytes.NewReader(payload))
	if err != nil { return DriveItem{}, err }
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil { return DriveItem{}, fmt.Errorf("rename google drive item: %w", err) }
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil { return DriveItem{}, err }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DriveItem{}, fmt.Errorf("rename google drive item: google returned %s", resp.Status)
	}
	var file googleFile
	if err := json.Unmarshal(body, &file); err != nil { return DriveItem{}, fmt.Errorf("decode renamed google item: %w", err) }

	items, err := s.upsertGoogleFiles(ctx, userID, accountID, []googleFile{file})
	if err != nil { return DriveItem{}, err }
	if len(items) != 1 { return DriveItem{}, errors.New("rename google drive item: mapping failed") }

	eventPayload, _ := json.Marshal(map[string]any{"provider":"google_drive","name":name})
	_, _ = s.pool.Exec(ctx,
		"INSERT INTO account_events(user_id,event_type,resource_type,resource_id,payload) VALUES ($1::uuid,'node.renamed','node',$2::uuid,$3::jsonb)",
		userID, nodeID, string(eventPayload))
	_ = nodeType
	return items[0], nil
}

func (s *Service) MoveGoogleNode(ctx context.Context, userID, nodeID string, parentNodeID *string) (DriveItem, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" { return DriveItem{}, ErrDeleteNotFound }

	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil { return DriveItem{}, err }
	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil { return DriveItem{}, err }

	var providerItemID, currentParent, nodeType string
	err = s.pool.QueryRow(ctx, `
		SELECT pi.provider_item_id, COALESCE(pi.provider_parent_item_id,''), n.node_type
		FROM nodes n
		JOIN provider_items pi ON pi.node_id=n.id AND pi.provider_account_id=$3::uuid
		WHERE n.id=$1::uuid AND n.user_id=$2::uuid AND n.deleted_at IS NULL`,
		nodeID, userID, accountID).Scan(&providerItemID, &currentParent, &nodeType)
	if errors.Is(err, pgx.ErrNoRows) { return DriveItem{}, ErrDeleteNotFound }
	if err != nil { return DriveItem{}, fmt.Errorf("resolve move target: %w", err) }

	newParentProviderID, newParentNodeID, err := s.googleManagedParent(ctx, userID, accountID, parentNodeID)
	if err != nil { return DriveItem{}, err }

	if newParentNodeID != nil {
		if *newParentNodeID == nodeID { return DriveItem{}, ErrInvalidProviderParent }
		if nodeType == "folder" {
			var inside bool
			err := s.pool.QueryRow(ctx, `
				WITH RECURSIVE descendants AS (
					SELECT id FROM nodes WHERE id=$1::uuid AND user_id=$2::uuid
					UNION ALL
					SELECT n.id FROM nodes n JOIN descendants d ON n.parent_id=d.id
					WHERE n.user_id=$2::uuid AND n.deleted_at IS NULL
				)
				SELECT EXISTS(SELECT 1 FROM descendants WHERE id=$3::uuid)`,
				nodeID, userID, *newParentNodeID).Scan(&inside)
			if err != nil { return DriveItem{}, fmt.Errorf("validate move destination: %w", err) }
			if inside { return DriveItem{}, ErrInvalidProviderParent }
		}
	}

	q := url.Values{}
	q.Set("addParents", newParentProviderID)
	if currentParent != "" { q.Set("removeParents", currentParent) }
	q.Set("fields", "id,name,mimeType,parents,size,modifiedTime")
	q.Set("supportsAllDrives", "true")
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		googleFilesURL+"/"+url.PathEscape(providerItemID)+"?"+q.Encode(),
		bytes.NewBufferString("{}"))
	if err != nil { return DriveItem{}, err }
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil { return DriveItem{}, fmt.Errorf("move google drive item: %w", err) }
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil { return DriveItem{}, err }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DriveItem{}, fmt.Errorf("move google drive item: google returned %s", resp.Status)
	}
	var file googleFile
	if err := json.Unmarshal(body, &file); err != nil { return DriveItem{}, fmt.Errorf("decode moved google item: %w", err) }

	items, err := s.upsertGoogleFiles(ctx, userID, accountID, []googleFile{file})
	if err != nil { return DriveItem{}, err }
	if len(items) != 1 { return DriveItem{}, errors.New("move google drive item: mapping failed") }

	if newParentNodeID == nil {
		_, err = s.pool.Exec(ctx, "UPDATE nodes SET parent_id=NULL,updated_at=now() WHERE id=$1::uuid AND user_id=$2::uuid", nodeID, userID)
	} else {
		_, err = s.pool.Exec(ctx, "UPDATE nodes SET parent_id=$1::uuid,updated_at=now() WHERE id=$2::uuid AND user_id=$3::uuid", *newParentNodeID, nodeID, userID)
	}
	if err != nil { return DriveItem{}, fmt.Errorf("update moved node parent: %w", err) }

	eventPayload, _ := json.Marshal(map[string]any{"provider":"google_drive","parent_provider_item_id":newParentProviderID})
	_, _ = s.pool.Exec(ctx,
		"INSERT INTO account_events(user_id,event_type,resource_type,resource_id,payload) VALUES ($1::uuid,'node.moved','node',$2::uuid,$3::jsonb)",
		userID, nodeID, string(eventPayload))
	return items[0], nil
}

func (s *Service) ListGoogleTrash(ctx context.Context, userID string) ([]DriveItem, error) {
	accountID, _, err := s.googleCredential(ctx, userID)
	if err != nil { return nil, err }

	rows, err := s.pool.Query(ctx, `
		SELECT n.id::text, pi.provider_item_id, pi.provider_parent_item_id,
		       n.name, n.node_type, COALESCE(pi.mime_type,''), pi.size_bytes, pi.modified_at
		FROM nodes n
		JOIN provider_items pi ON pi.node_id=n.id AND pi.provider_account_id=$2::uuid
		LEFT JOIN nodes parent ON parent.id=n.parent_id
		WHERE n.user_id=$1::uuid
		  AND n.deleted_at IS NOT NULL
		  AND (n.parent_id IS NULL OR parent.deleted_at IS NULL)
		ORDER BY n.deleted_at DESC, n.name`, userID, accountID)
	if err != nil { return nil, fmt.Errorf("list trash: %w", err) }
	defer rows.Close()

	items := make([]DriveItem, 0)
	for rows.Next() {
		var item DriveItem
		item.Provider = "google_drive"
		if err := rows.Scan(&item.NodeID, &item.ProviderItemID, &item.ParentItemID, &item.Name, &item.NodeType, &item.MIMEType, &item.SizeBytes, &item.ModifiedAt); err != nil {
			return nil, fmt.Errorf("scan trash item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return items, nil
}

func (s *Service) RestoreGoogleNode(ctx context.Context, userID, nodeID string) (int, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" { return 0, ErrDeleteNotFound }

	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil { return 0, err }
	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil { return 0, err }

	rows, err := s.pool.Query(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT n.id, pi.provider_item_id, 0 AS depth
			FROM nodes n
			JOIN provider_items pi ON pi.node_id=n.id AND pi.provider_account_id=$3::uuid
			WHERE n.id=$1::uuid AND n.user_id=$2::uuid AND n.deleted_at IS NOT NULL
			UNION ALL
			SELECT child.id, child_pi.provider_item_id, subtree.depth + 1
			FROM subtree
			JOIN nodes child ON child.parent_id=subtree.id
			JOIN provider_items child_pi ON child_pi.node_id=child.id AND child_pi.provider_account_id=$3::uuid
			WHERE child.user_id=$2::uuid AND child.deleted_at IS NOT NULL
		)
		SELECT id::text, provider_item_id, depth FROM subtree ORDER BY depth ASC`,
		nodeID, userID, accountID)
	if err != nil { return 0, fmt.Errorf("load restore subtree: %w", err) }
	defer rows.Close()

	targets := make([]deleteTarget, 0)
	for rows.Next() {
		var target deleteTarget
		if err := rows.Scan(&target.NodeID, &target.ProviderItemID, &target.Depth); err != nil { return 0, err }
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil { return 0, err }
	if len(targets) == 0 { return 0, ErrDeleteNotFound }

	for _, target := range targets {
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
			googleFilesURL+"/"+url.PathEscape(target.ProviderItemID)+"?supportsAllDrives=true",
			bytes.NewBufferString(`{"trashed":false}`))
		if err != nil { return 0, err }
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.httpClient.Do(req)
		if err != nil { return 0, fmt.Errorf("restore google drive item: %w", err) }
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return 0, fmt.Errorf("restore google drive item: google returned %s", resp.Status)
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil { return 0, err }
	defer tx.Rollback(ctx)
	for _, target := range targets {
		if _, err := tx.Exec(ctx, "UPDATE nodes SET state='active',deleted_at=NULL,updated_at=now() WHERE id=$1::uuid AND user_id=$2::uuid", target.NodeID, userID); err != nil {
			return 0, err
		}
	}
	eventPayload, _ := json.Marshal(map[string]any{"provider":"google_drive","restored_count":len(targets)})
	_, _ = tx.Exec(ctx,
		"INSERT INTO account_events(user_id,event_type,resource_type,resource_id,payload) VALUES ($1::uuid,'node.restored','node',$2::uuid,$3::jsonb)",
		userID, nodeID, string(eventPayload))
	if err := tx.Commit(ctx); err != nil { return 0, err }
	return len(targets), nil
}


func (s *Service) RecoverGoogleUpload(ctx context.Context, userID string, input UploadSessionInput) (FinalizedUpload, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" { return FinalizedUpload{}, errors.New("file name is required") }
	if input.SizeBytes < 0 { return FinalizedUpload{}, errors.New("file size must not be negative") }

	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil { return FinalizedUpload{}, err }
	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil { return FinalizedUpload{}, err }
	parentID, _, err := s.googleManagedParent(ctx, userID, accountID, input.ParentNodeID)
	if err != nil { return FinalizedUpload{}, err }

	q := url.Values{}
	q.Set("q", fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false",
		strings.ReplaceAll(parentID, "'", "\\'"),
		strings.ReplaceAll(name, "'", "\\'")))
	q.Set("pageSize", "20")
	q.Set("orderBy", "modifiedTime desc")
	q.Set("fields", "files(id,name,mimeType,parents,size,modifiedTime)")
	q.Set("supportsAllDrives", "true")
	q.Set("includeItemsFromAllDrives", "true")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleFilesURL+"?"+q.Encode(), nil)
	if err != nil { return FinalizedUpload{}, err }
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil { return FinalizedUpload{}, fmt.Errorf("recover google upload: %w", err) }
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil { return FinalizedUpload{}, fmt.Errorf("read google upload recovery: %w", err) }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FinalizedUpload{}, fmt.Errorf("recover google upload: google returned %s", resp.Status)
	}
	var page googleFilesResponse
	if err := json.Unmarshal(body, &page); err != nil {
		return FinalizedUpload{}, fmt.Errorf("decode google upload recovery: %w", err)
	}
	for _, file := range page.Files {
		size, ok := parseOptionalInt64(file.Size)
		if !ok || size != input.SizeBytes { continue }
		return s.FinalizeGoogleUpload(ctx, userID, FinalizeUploadInput{ProviderItemID: file.ID})
	}
	return FinalizedUpload{}, ErrDownloadNotFound
}
