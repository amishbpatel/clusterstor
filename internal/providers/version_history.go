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
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrVersionHistoryNotFound = errors.New("version history not found")
var ErrVersionHistoryUnavailable = errors.New("version history unavailable")

type FileVersion struct {
	ID string `json:"id"`
	VersionNumber int64 `json:"version_number"`
	SizeBytes int64 `json:"size_bytes"`
	Provider string `json:"provider"`
	CreatedAt time.Time `json:"created_at"`
	IsCurrent bool `json:"is_current"`
	Downloadable bool `json:"downloadable"`
}

func (s *Service) ensureCurrentGoogleRevisionCaptured(ctx context.Context, userID, nodeID string) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" { return ErrVersionHistoryNotFound }

	var accountID, providerItemID, name string
	var sizeBytes int64
	var currentVersionID, currentRevisionID string
	err := s.pool.QueryRow(ctx, `
		SELECT pa.id::text,pi.provider_item_id,n.name,COALESCE(pi.size_bytes,0),
		       COALESCE(n.current_version_id::text,''),
		       COALESCE(so.provider_revision_id,'')
		FROM nodes n
		JOIN provider_items pi ON pi.node_id=n.id
		JOIN provider_accounts pa ON pa.id=pi.provider_account_id
		LEFT JOIN storage_objects so
		  ON so.version_id=n.current_version_id
		 AND so.provider_account_id=pa.id
		 AND so.state='available'
		 AND so.deleted_at IS NULL
		WHERE n.id=$1::uuid
		  AND n.user_id=$2::uuid
		  AND n.node_type='file'
		  AND n.deleted_at IS NULL
		  AND pa.provider='google_drive'
		  AND pa.status='connected'
		  AND pa.disconnected_at IS NULL
		ORDER BY so.verified_at DESC NULLS LAST,so.created_at DESC
		LIMIT 1`,nodeID,userID).Scan(&accountID,&providerItemID,&name,&sizeBytes,&currentVersionID,&currentRevisionID)
	if errors.Is(err,pgx.ErrNoRows) { return ErrVersionHistoryNotFound }
	if err != nil { return fmt.Errorf("resolve current google revision: %w",err) }

	var ciphertext []byte
	if err := s.pool.QueryRow(ctx,"SELECT token_ciphertext FROM provider_accounts WHERE id=$1::uuid",accountID).Scan(&ciphertext); err != nil {
		return fmt.Errorf("load google version credentials: %w",err)
	}
	if len(ciphertext)==0 || len(s.key)!=32 { return ErrProviderNotConfigured }
	plaintext, err := decrypt(s.key,ciphertext)
	if err != nil { return fmt.Errorf("decrypt google version credentials: %w",err) }
	var token storedToken
	if err := json.Unmarshal(plaintext,&token); err != nil { return fmt.Errorf("decode google version credentials: %w",err) }
	token, err = s.ensureGoogleAccessToken(ctx,accountID,token)
	if err != nil { return err }

	if currentRevisionID=="" {
		file, err := s.fetchGoogleFile(ctx,token.AccessToken,providerItemID)
		if err != nil { return err }
		currentRevisionID = strings.TrimSpace(file.HeadRevisionID)
		if currentRevisionID=="" { return ErrVersionHistoryUnavailable }

		if currentVersionID=="" {
			tx, err := s.pool.Begin(ctx)
			if err != nil { return fmt.Errorf("begin baseline version: %w",err) }
			defer tx.Rollback(ctx)

			var nextVersion int64
			if err := tx.QueryRow(ctx,"SELECT COALESCE(MAX(version_number),0)+1 FROM file_versions WHERE node_id=$1::uuid",nodeID).Scan(&nextVersion); err != nil {
				return fmt.Errorf("next baseline version: %w",err)
			}
			if err := tx.QueryRow(ctx,
				"INSERT INTO file_versions(node_id,version_number,size_bytes) VALUES ($1::uuid,$2,$3) RETURNING id::text",
				nodeID,nextVersion,sizeBytes).Scan(&currentVersionID); err != nil {
				return fmt.Errorf("create baseline version: %w",err)
			}
			if _, err := tx.Exec(ctx,`
				INSERT INTO storage_objects(
					version_id,storage_class,provider_account_id,provider_object_id,provider_revision_id,
					size_bytes,encryption_mode,state,verified_at
				) VALUES ($1::uuid,'provider',$2::uuid,$3,$4,$5,'provider_native','available',now())`,
				currentVersionID,accountID,providerItemID,currentRevisionID,sizeBytes); err != nil {
				return fmt.Errorf("create baseline storage object: %w",err)
			}
			if _, err := tx.Exec(ctx,
				"UPDATE nodes SET current_version_id=$1::uuid,updated_at=now() WHERE id=$2::uuid AND user_id=$3::uuid",
				currentVersionID,nodeID,userID); err != nil {
				return fmt.Errorf("activate baseline version: %w",err)
			}
			payload,_ := json.Marshal(map[string]any{
				"provider":"google_drive","provider_item_id":providerItemID,
				"provider_revision_id":currentRevisionID,"size_bytes":sizeBytes,"baseline":true,
			})
			if _, err := tx.Exec(ctx,
				"INSERT INTO account_events(user_id,event_type,resource_type,resource_id,payload) VALUES ($1::uuid,'file.version.baseline','node',$2::uuid,$3::jsonb)",
				userID,nodeID,string(payload)); err != nil {
				return fmt.Errorf("record baseline version event: %w",err)
			}
			if err := tx.Commit(ctx); err != nil { return fmt.Errorf("commit baseline version: %w",err) }
		} else {
			if _, err := s.pool.Exec(ctx,`
				UPDATE storage_objects
				SET provider_revision_id=$1
				WHERE version_id=$2::uuid
				  AND provider_account_id=$3::uuid
				  AND provider_object_id=$4
				  AND state='available'
				  AND deleted_at IS NULL`,
				currentRevisionID,currentVersionID,accountID,providerItemID); err != nil {
				return fmt.Errorf("capture current google revision: %w",err)
			}
		}
	}

	if err := s.keepGoogleRevision(ctx,token.AccessToken,providerItemID,currentRevisionID); err != nil {
		return err
	}
	_ = name
	return nil
}

func (s *Service) keepGoogleRevision(ctx context.Context, accessToken, fileID, revisionID string) error {
	fileID = strings.TrimSpace(fileID)
	revisionID = strings.TrimSpace(revisionID)
	if fileID=="" || revisionID=="" { return ErrVersionHistoryUnavailable }

	payload := bytes.NewBufferString(`{"keepForever":true}`)
	req, err := http.NewRequestWithContext(ctx,http.MethodPatch,
		googleFilesURL+"/"+url.PathEscape(fileID)+"/revisions/"+url.PathEscape(revisionID),
		payload)
	if err != nil { return err }
	req.Header.Set("Authorization","Bearer "+accessToken)
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("Accept","application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil { return fmt.Errorf("preserve google revision: %w",err) }
	defer resp.Body.Close()
	io.Copy(io.Discard,io.LimitReader(resp.Body,1<<20))
	if resp.StatusCode<200 || resp.StatusCode>=300 {
		return fmt.Errorf("preserve google revision: google returned %s",resp.Status)
	}
	return nil
}

func (s *Service) BeginGoogleVersionUpload(ctx context.Context, userID, nodeID, contentType string, sizeBytes int64) (UploadSession,error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID=="" { return UploadSession{},ErrVersionHistoryNotFound }
	if sizeBytes<0 { return UploadSession{},errors.New("file size must not be negative") }
	if _, err := s.CheckUploadCapacity(ctx,userID,"google_drive",sizeBytes); err != nil {
		return UploadSession{},err
	}
	if err := s.ensureCurrentGoogleRevisionCaptured(ctx,userID,nodeID); err != nil {
		return UploadSession{},err
	}

	accountID, token, err := s.googleCredential(ctx,userID)
	if err != nil { return UploadSession{},err }
	token, err = s.ensureGoogleAccessToken(ctx,accountID,token)
	if err != nil { return UploadSession{},err }

	var providerItemID,name string
	err = s.pool.QueryRow(ctx,`
		SELECT pi.provider_item_id,n.name
		FROM nodes n
		JOIN provider_items pi ON pi.node_id=n.id AND pi.provider_account_id=$3::uuid
		WHERE n.id=$1::uuid AND n.user_id=$2::uuid AND n.node_type='file' AND n.deleted_at IS NULL`,
		nodeID,userID,accountID).Scan(&providerItemID,&name)
	if errors.Is(err,pgx.ErrNoRows) { return UploadSession{},ErrVersionHistoryNotFound }
	if err != nil { return UploadSession{},fmt.Errorf("resolve version upload target: %w",err) }

	contentType = strings.TrimSpace(contentType)
	if contentType=="" { contentType="application/octet-stream" }
	uploadURL := googleUploadFilesURL+"/"+url.PathEscape(providerItemID)+"?uploadType=resumable&fields="+url.QueryEscape("id,name,mimeType,parents,size,modifiedTime,headRevisionId")
	req, err := http.NewRequestWithContext(ctx,http.MethodPatch,uploadURL,bytes.NewBufferString("{}"))
	if err != nil { return UploadSession{},err }
	req.Header.Set("Authorization","Bearer "+token.AccessToken)
	req.Header.Set("Content-Type","application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type",contentType)
	req.Header.Set("X-Upload-Content-Length",strconv.FormatInt(sizeBytes,10))
	resp, err := s.httpClient.Do(req)
	if err != nil { return UploadSession{},fmt.Errorf("start google version upload: %w",err) }
	defer resp.Body.Close()
	io.Copy(io.Discard,io.LimitReader(resp.Body,1<<20))
	if resp.StatusCode<200 || resp.StatusCode>=300 {
		return UploadSession{},fmt.Errorf("start google version upload: google returned %s",resp.Status)
	}
	location := strings.TrimSpace(resp.Header.Get("Location"))
	if location=="" { return UploadSession{},errors.New("start google version upload: missing upload session URL") }
	return UploadSession{
		UploadURL:location,Name:name,ContentType:contentType,SizeBytes:sizeBytes,
	},nil
}

func (s *Service) ListFileVersions(ctx context.Context, userID, nodeID string) ([]FileVersion,error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID=="" { return nil,ErrVersionHistoryNotFound }
	if err := s.ensureCurrentGoogleRevisionCaptured(ctx,userID,nodeID); err != nil {
		return nil,err
	}

	var nodeType string
	if err := s.pool.QueryRow(ctx,
		"SELECT node_type FROM nodes WHERE id=$1::uuid AND user_id=$2::uuid AND deleted_at IS NULL",
		nodeID,userID).Scan(&nodeType); errors.Is(err,pgx.ErrNoRows) {
		return nil,ErrVersionHistoryNotFound
	} else if err != nil {
		return nil,fmt.Errorf("resolve version history node: %w",err)
	}
	if nodeType!="file" { return nil,ErrVersionHistoryNotFound }

	rows, err := s.pool.Query(ctx,`
		SELECT fv.id::text,fv.version_number,fv.size_bytes,COALESCE(pa.provider,''),
		       fv.created_at,(n.current_version_id=fv.id),
		       EXISTS(
		         SELECT 1 FROM storage_objects available
		         WHERE available.version_id=fv.id
		           AND available.state='available'
		           AND available.deleted_at IS NULL
		           AND available.provider_object_id IS NOT NULL
		       ) AS downloadable
		FROM file_versions fv
		JOIN nodes n ON n.id=fv.node_id
		LEFT JOIN storage_objects so ON so.version_id=fv.id AND so.state='available' AND so.deleted_at IS NULL
		LEFT JOIN provider_accounts pa ON pa.id=so.provider_account_id
		WHERE fv.node_id=$1::uuid AND n.user_id=$2::uuid
		GROUP BY fv.id,fv.version_number,fv.size_bytes,fv.created_at,n.current_version_id,pa.provider
		ORDER BY fv.version_number DESC`,nodeID,userID)
	if err != nil { return nil,fmt.Errorf("list file versions: %w",err) }
	defer rows.Close()

	versions:=make([]FileVersion,0)
	for rows.Next() {
		var item FileVersion
		if err := rows.Scan(&item.ID,&item.VersionNumber,&item.SizeBytes,&item.Provider,&item.CreatedAt,&item.IsCurrent,&item.Downloadable); err != nil {
			return nil,fmt.Errorf("scan file version: %w",err)
		}
		versions=append(versions,item)
	}
	if err := rows.Err(); err != nil { return nil,err }
	return versions,nil
}

func (s *Service) OpenGoogleVersionDownload(ctx context.Context, userID,nodeID,versionID,rangeHeader string) (DownloadStream,error) {
	nodeID=strings.TrimSpace(nodeID)
	versionID=strings.TrimSpace(versionID)
	if nodeID=="" || versionID=="" { return DownloadStream{},ErrVersionHistoryNotFound }

	var accountID,providerItemID,revisionID,name string
	var sizeBytes int64
	var current bool
	err:=s.pool.QueryRow(ctx,`
		SELECT pa.id::text,so.provider_object_id,COALESCE(so.provider_revision_id,''),n.name,
		       fv.size_bytes,(n.current_version_id=fv.id)
		FROM nodes n
		JOIN file_versions fv ON fv.node_id=n.id
		JOIN storage_objects so ON so.version_id=fv.id AND so.state='available' AND so.deleted_at IS NULL
		JOIN provider_accounts pa ON pa.id=so.provider_account_id
		WHERE n.id=$1::uuid
		  AND n.user_id=$2::uuid
		  AND fv.id=$3::uuid
		  AND n.node_type='file'
		  AND pa.provider='google_drive'
		  AND pa.status='connected'
		  AND pa.disconnected_at IS NULL
		LIMIT 1`,nodeID,userID,versionID).Scan(&accountID,&providerItemID,&revisionID,&name,&sizeBytes,&current)
	if errors.Is(err,pgx.ErrNoRows) { return DownloadStream{},ErrVersionHistoryNotFound }
	if err != nil { return DownloadStream{},fmt.Errorf("resolve version download: %w",err) }

	var ciphertext []byte
	if err:=s.pool.QueryRow(ctx,"SELECT token_ciphertext FROM provider_accounts WHERE id=$1::uuid",accountID).Scan(&ciphertext); err!=nil {
		return DownloadStream{},fmt.Errorf("load version download credentials: %w",err)
	}
	if len(ciphertext)==0 || len(s.key)!=32 { return DownloadStream{},ErrProviderNotConfigured }
	plaintext,err:=decrypt(s.key,ciphertext)
	if err!=nil { return DownloadStream{},fmt.Errorf("decrypt version credentials: %w",err) }
	var token storedToken
	if err:=json.Unmarshal(plaintext,&token); err!=nil { return DownloadStream{},fmt.Errorf("decode version credentials: %w",err) }
	token,err=s.ensureGoogleAccessToken(ctx,accountID,token)
	if err!=nil { return DownloadStream{},err }

	var downloadURL string
	if revisionID!="" {
		downloadURL=googleFilesURL+"/"+url.PathEscape(providerItemID)+"/revisions/"+url.PathEscape(revisionID)+"?alt=media"
	} else if current {
		downloadURL=googleFilesURL+"/"+url.PathEscape(providerItemID)+"?alt=media&supportsAllDrives=true"
	} else {
		return DownloadStream{},ErrVersionHistoryUnavailable
	}

	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,downloadURL,nil)
	if err!=nil { return DownloadStream{},err }
	req.Header.Set("Authorization","Bearer "+token.AccessToken)
	req.Header.Set("Accept","*/*")
	if strings.TrimSpace(rangeHeader)!="" { req.Header.Set("Range",rangeHeader) }
	resp,err:=s.httpClient.Do(req)
	if err!=nil { return DownloadStream{},fmt.Errorf("open google version download: %w",err) }
	if resp.StatusCode!=http.StatusOK && resp.StatusCode!=http.StatusPartialContent {
		resp.Body.Close()
		return DownloadStream{},fmt.Errorf("open google version download: google returned %s",resp.Status)
	}
	return DownloadStream{Response:resp,Name:name,SizeBytes:sizeBytes},nil
}
