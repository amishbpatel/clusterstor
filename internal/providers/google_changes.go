package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

const googleChangesURL = "https://www.googleapis.com/drive/v3/changes"

type googleChange struct {
	FileID string `json:"fileId"`
	Removed bool `json:"removed"`
	File *googleFile `json:"file,omitempty"`
}

type googleChangesResponse struct {
	NextPageToken string `json:"nextPageToken"`
	NewStartPageToken string `json:"newStartPageToken"`
	Changes []googleChange `json:"changes"`
}

type GoogleChangeSyncResult struct {
	Processed int `json:"processed"`
	HasMore bool `json:"has_more"`
	Initialized bool `json:"initialized"`
}

func (s *Service) SyncGoogleChanges(ctx context.Context, userID string, maxPages int) (GoogleChangeSyncResult, error) {
	if maxPages <= 0 { maxPages = 4 }
	if maxPages > 20 { maxPages = 20 }

	accountID, token, err := s.googleCredential(ctx,userID)
	if err != nil { return GoogleChangeSyncResult{},err }
	token, err = s.ensureGoogleAccessToken(ctx,accountID,token)
	if err != nil { return GoogleChangeSyncResult{},err }
	if _, err := s.EnsureGoogleRoot(ctx,userID); err != nil { return GoogleChangeSyncResult{},err }

	var cursor *string
	if err := s.pool.QueryRow(ctx,"SELECT change_cursor FROM provider_accounts WHERE id=$1::uuid",accountID).Scan(&cursor); err != nil {
		return GoogleChangeSyncResult{},fmt.Errorf("load google change cursor: %w",err)
	}
	if cursor == nil || strings.TrimSpace(*cursor)=="" {
		start, err := s.googleStartPageToken(ctx,token.AccessToken)
		if err != nil { return GoogleChangeSyncResult{},err }
		if _, err := s.pool.Exec(ctx,"UPDATE provider_accounts SET change_cursor=$1,updated_at=now() WHERE id=$2::uuid",start,accountID); err != nil {
			return GoogleChangeSyncResult{},fmt.Errorf("store google start page token: %w",err)
		}
		return GoogleChangeSyncResult{Initialized:true},nil
	}

	pageToken := strings.TrimSpace(*cursor)
	processed := 0
	for pageIndex:=0; pageIndex<maxPages; pageIndex++ {
		page, err := s.fetchGoogleChanges(ctx,token.AccessToken,pageToken)
		if err != nil { return GoogleChangeSyncResult{},err }

		for _, change := range page.Changes {
			fileID := strings.TrimSpace(change.FileID)
			if fileID=="" && change.File!=nil { fileID=strings.TrimSpace(change.File.ID) }
			if fileID=="" { continue }

			if change.Removed || change.File==nil {
				if err := s.markGoogleItemUnavailable(ctx,userID,accountID,fileID,false); err != nil {
					return GoogleChangeSyncResult{},err
				}
				processed++
				continue
			}
			file := *change.File
			if file.Trashed {
				if err := s.markGoogleItemUnavailable(ctx,userID,accountID,fileID,true); err != nil {
					return GoogleChangeSyncResult{},err
				}
				processed++
				continue
			}

			inside, err := s.googleFileInsideManagedRoot(ctx,accountID,file)
			if err != nil { return GoogleChangeSyncResult{},err }
			if inside {
				if _, err := s.upsertGoogleFiles(ctx,userID,accountID,[]googleFile{file}); err != nil {
					return GoogleChangeSyncResult{},err
				}
			} else {
				if err := s.updateGoogleItemOutsideRoot(ctx,userID,accountID,file); err != nil {
					return GoogleChangeSyncResult{},err
				}
			}
			processed++
		}

		next := strings.TrimSpace(page.NextPageToken)
		if next=="" { next=strings.TrimSpace(page.NewStartPageToken) }
		if next!="" {
			if _, err := s.pool.Exec(ctx,"UPDATE provider_accounts SET change_cursor=$1,last_synced_at=now(),updated_at=now() WHERE id=$2::uuid",next,accountID); err != nil {
				return GoogleChangeSyncResult{},fmt.Errorf("store google change cursor: %w",err)
			}
		}
		if page.NextPageToken=="" {
			return GoogleChangeSyncResult{Processed:processed,HasMore:false},nil
		}
		pageToken=page.NextPageToken
	}
	return GoogleChangeSyncResult{Processed:processed,HasMore:true},nil
}

func (s *Service) googleStartPageToken(ctx context.Context, accessToken string) (string,error) {
	req, err := http.NewRequestWithContext(ctx,http.MethodGet,googleChangesURL+"/startPageToken",nil)
	if err != nil { return "",err }
	req.Header.Set("Authorization","Bearer "+accessToken)
	req.Header.Set("Accept","application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil { return "",fmt.Errorf("get google start page token: %w",err) }
	defer resp.Body.Close()
	if resp.StatusCode<200 || resp.StatusCode>=300 {
		return "",fmt.Errorf("get google start page token: google returned %s",resp.Status)
	}
	var body struct{ StartPageToken string `json:"startPageToken"` }
	if err := json.NewDecoder(io.LimitReader(resp.Body,1<<20)).Decode(&body); err != nil {
		return "",fmt.Errorf("decode google start page token: %w",err)
	}
	if strings.TrimSpace(body.StartPageToken)=="" { return "",fmt.Errorf("get google start page token: missing token") }
	return body.StartPageToken,nil
}

func (s *Service) fetchGoogleChanges(ctx context.Context, accessToken, pageToken string) (googleChangesResponse,error) {
	q:=url.Values{}
	q.Set("pageToken",pageToken)
	q.Set("pageSize","1000")
	q.Set("spaces","drive")
	q.Set("includeRemoved","true")
	q.Set("restrictToMyDrive","true")
	q.Set("fields","nextPageToken,newStartPageToken,changes(fileId,removed,file(id,name,mimeType,parents,size,modifiedTime,trashed))")

	req, err := http.NewRequestWithContext(ctx,http.MethodGet,googleChangesURL+"?"+q.Encode(),nil)
	if err != nil { return googleChangesResponse{},err }
	req.Header.Set("Authorization","Bearer "+accessToken)
	req.Header.Set("Accept","application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil { return googleChangesResponse{},fmt.Errorf("list google changes: %w",err) }
	defer resp.Body.Close()
	if resp.StatusCode<200 || resp.StatusCode>=300 {
		return googleChangesResponse{},fmt.Errorf("list google changes: google returned %s",resp.Status)
	}
	var page googleChangesResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body,8<<20)).Decode(&page); err != nil {
		return googleChangesResponse{},fmt.Errorf("decode google changes: %w",err)
	}
	return page,nil
}

func (s *Service) markGoogleItemUnavailable(ctx context.Context, userID, accountID, providerItemID string, trashed bool) error {
	var nodeID string
	err := s.pool.QueryRow(ctx,`
		SELECT n.id::text
		FROM nodes n
		JOIN provider_items pi ON pi.node_id=n.id
		WHERE n.user_id=$1::uuid
		  AND pi.provider_account_id=$2::uuid
		  AND pi.provider_item_id=$3
		LIMIT 1`,userID,accountID,providerItemID).Scan(&nodeID)
	if err!=nil {
		if err==pgx.ErrNoRows { return nil }
		return fmt.Errorf("resolve changed google item: %w",err)
	}
	if trashed {
		_, err = s.pool.Exec(ctx,"UPDATE nodes SET state='deleted',deleted_at=COALESCE(deleted_at,now()),updated_at=now() WHERE id=$1::uuid AND user_id=$2::uuid",nodeID,userID)
	} else {
		_, err = s.pool.Exec(ctx,"UPDATE nodes SET state='unavailable',deleted_at=NULL,updated_at=now() WHERE id=$1::uuid AND user_id=$2::uuid",nodeID,userID)
	}
	if err != nil { return fmt.Errorf("mark changed google item unavailable: %w",err) }
	return nil
}

func (s *Service) updateGoogleItemOutsideRoot(ctx context.Context, userID, accountID string, file googleFile) error {
	var nodeID string
	err := s.pool.QueryRow(ctx,`
		SELECT n.id::text
		FROM nodes n
		JOIN provider_items pi ON pi.node_id=n.id
		WHERE n.user_id=$1::uuid
		  AND pi.provider_account_id=$2::uuid
		  AND pi.provider_item_id=$3
		LIMIT 1`,userID,accountID,file.ID).Scan(&nodeID)
	if err!=nil {
		if err==pgx.ErrNoRows { return nil }
		return fmt.Errorf("resolve moved google item: %w",err)
	}
	var parent any
	if len(file.Parents)>0 && strings.TrimSpace(file.Parents[0])!="" { parent=file.Parents[0] }
	if _, err := s.pool.Exec(ctx,`
		UPDATE provider_items
		SET provider_parent_item_id=$1,modified_at=CASE WHEN $2='' THEN modified_at ELSE $2::timestamptz END,updated_at=now()
		WHERE provider_account_id=$3::uuid AND provider_item_id=$4`,
		parent,file.ModifiedTime,accountID,file.ID); err != nil {
		return fmt.Errorf("update moved google mapping: %w",err)
	}
	if _, err := s.pool.Exec(ctx,"UPDATE nodes SET state='unavailable',deleted_at=NULL,updated_at=now() WHERE id=$1::uuid AND user_id=$2::uuid",nodeID,userID); err != nil {
		return fmt.Errorf("mark moved google item unavailable: %w",err)
	}
	return nil
}
