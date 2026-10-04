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
	"strings"
)

const clusterStorRootName = "ClusterStor"

type googleCreatedFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MIMEType string `json:"mimeType"`
}

func (s *Service) EnsureGoogleRoot(ctx context.Context, userID string) (string, error) {
	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil {
		return "", err
	}

	var existing *string
	err = s.pool.QueryRow(ctx,
		"SELECT root_provider_item_id FROM provider_accounts WHERE id=$1::uuid AND user_id=$2::uuid",
		accountID, userID).Scan(&existing)
	if err != nil {
		return "", fmt.Errorf("load provider root: %w", err)
	}
	if existing != nil && strings.TrimSpace(*existing) != "" {
		return *existing, nil
	}

	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil {
		return "", err
	}

	rootID, err := s.findGoogleRoot(ctx, token.AccessToken)
	if err != nil {
		return "", err
	}
	if rootID == "" {
		rootID, err = s.createGoogleRoot(ctx, token.AccessToken)
		if err != nil {
			return "", err
		}
	}

	_, err = s.pool.Exec(ctx,
		"UPDATE provider_accounts SET root_provider_item_id=$1, updated_at=now() WHERE id=$2::uuid AND user_id=$3::uuid",
		rootID, accountID, userID)
	if err != nil {
		return "", fmt.Errorf("store provider root: %w", err)
	}
	return rootID, nil
}

func (s *Service) findGoogleRoot(ctx context.Context, accessToken string) (string, error) {
	q := url.Values{}
	q.Set("q", "name = '"+escapeDriveQuery(clusterStorRootName)+"' and mimeType = '"+googleFolderMIME+"' and 'root' in parents and trashed = false")
	q.Set("spaces", "drive")
	q.Set("pageSize", "10")
	q.Set("fields", "files(id,name,mimeType)")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleFilesURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("find google root: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read google root search: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("find google root: google returned %s", resp.Status)
	}

	var result struct {
		Files []googleCreatedFile `json:"files"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode google root search: %w", err)
	}
	if len(result.Files) == 0 {
		return "", nil
	}
	return result.Files[0].ID, nil
}

func (s *Service) createGoogleRoot(ctx context.Context, accessToken string) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"name": clusterStorRootName,
		"mimeType": googleFolderMIME,
		"parents": []string{"root"},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleFilesURL+"?fields=id,name,mimeType", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create google root: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read google root creation: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("create google root: google returned %s", resp.Status)
	}

	var created googleCreatedFile
	if err := json.Unmarshal(body, &created); err != nil {
		return "", fmt.Errorf("decode google root creation: %w", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		return "", errors.New("create google root: missing folder id")
	}
	return created.ID, nil
}

func escapeDriveQuery(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "'", "\\'")
	return value
}
