package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type FileItem = DriveItem

type FilePage struct {
	Items []FileItem `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func (s *Service) ListManagedFiles(ctx context.Context, userID string, parentNodeID *string, cursor string, pageSize int) (FilePage, error) {
	if pageSize <= 0 { pageSize = defaultDrivePageSize }
	if pageSize > maxDrivePageSize { pageSize = maxDrivePageSize }

	accountID, token, err := s.googleCredential(ctx, userID)
	if err != nil { return FilePage{}, err }
	token, err = s.ensureGoogleAccessToken(ctx, accountID, token)
	if err != nil { return FilePage{}, err }

	parentProviderID, _, err := s.googleManagedParent(ctx, userID, accountID, parentNodeID)
	if err != nil { return FilePage{}, err }

	page, err := s.fetchGoogleChildren(ctx, token.AccessToken, parentProviderID, cursor, pageSize)
	if err != nil { return FilePage{}, err }
	items, err := s.upsertGoogleFiles(ctx, userID, accountID, page.Files)
	if err != nil { return FilePage{}, err }

	if _, err := s.pool.Exec(ctx,
		"UPDATE provider_accounts SET last_synced_at=now(),updated_at=now() WHERE id=$1::uuid",
		accountID); err != nil {
		return FilePage{}, fmt.Errorf("update managed folder sync time: %w", err)
	}

	return FilePage{Items:items,NextCursor:page.NextPageToken},nil
}

func (s *Service) fetchGoogleChildren(ctx context.Context, accessToken, parentProviderID, cursor string, pageSize int) (googleFilesResponse, error) {
	q := url.Values{}
	q.Set("q", "'"+escapeDriveQuery(parentProviderID)+"' in parents and trashed = false")
	q.Set("spaces", "drive")
	q.Set("pageSize", strconv.Itoa(pageSize))
	q.Set("orderBy", "folder,name_natural")
	q.Set("fields", "nextPageToken,files(id,name,mimeType,parents,size,modifiedTime)")
	q.Set("supportsAllDrives", "true")
	q.Set("includeItemsFromAllDrives", "true")
	if cursor != "" { q.Set("pageToken", cursor) }

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleFilesURL+"?"+q.Encode(), nil)
	if err != nil { return googleFilesResponse{}, err }
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil { return googleFilesResponse{}, fmt.Errorf("list managed google folder: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return googleFilesResponse{}, fmt.Errorf("list managed google folder: google returned %s", resp.Status)
	}

	var page googleFilesResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&page); err != nil {
		return googleFilesResponse{}, fmt.Errorf("decode managed google folder: %w", err)
	}
	return page,nil
}
