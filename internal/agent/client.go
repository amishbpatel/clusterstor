package agent

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
	"time"
)

type Client struct {
	baseURL string
	http *http.Client
	transfer *http.Client
}

type PairingStart struct {
	UserCode string `json:"user_code"`
	PollToken string `json:"poll_token"`
	VerificationURL string `json:"verification_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Device struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Platform string `json:"platform"`
	AgentVersion *string `json:"agent_version,omitempty"`
	Status string `json:"status"`
	PeerContributionEnabled bool `json:"peer_contribution_enabled"`
	PeerContributionBytes int64 `json:"peer_contribution_bytes"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Registration struct {
	Device Device `json:"device"`
	Secret string `json:"secret"`
}

type PairingPoll struct {
	Status string `json:"status"`
	Registration *Registration `json:"registration,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:strings.TrimRight(strings.TrimSpace(baseURL),"/"),
		http:&http.Client{Timeout:20*time.Second},
		transfer:&http.Client{},
	}
}

func (c *Client) StartPairing(ctx context.Context,name,platform,version string,peerEnabled bool,peerBytes int64) (PairingStart,error) {
	var result PairingStart
	err:=c.json(ctx,http.MethodPost,"/api/v1/device-pairings",map[string]any{
		"name":name,
		"platform":platform,
		"agent_version":version,
		"peer_contribution_enabled":peerEnabled,
		"peer_contribution_bytes":peerBytes,
	},"",&result)
	return result,err
}

func (c *Client) PollPairing(ctx context.Context,pollToken string) (PairingPoll,error) {
	var result PairingPoll
	err:=c.json(ctx,http.MethodPost,"/api/v1/device-pairings/status",map[string]any{"poll_token":pollToken},"",&result)
	return result,err
}

func (c *Client) Heartbeat(ctx context.Context,deviceID,secret,version string) (Device,error) {
	var result struct { Device Device `json:"device"` }
	auth:="Device "+deviceID+"."+secret
	err:=c.json(ctx,http.MethodPost,"/api/v1/agent/heartbeat",map[string]any{"agent_version":version},auth,&result)
	return result.Device,err
}

func (c *Client) json(ctx context.Context,method,path string,input any,authorization string,out any) error {
	if c.baseURL=="" { return errors.New("ClusterStor API URL is empty") }
	var body io.Reader
	if input!=nil {
		encoded,err:=json.Marshal(input)
		if err!=nil { return err }
		body=bytes.NewReader(encoded)
	}
	req,err:=http.NewRequestWithContext(ctx,method,c.baseURL+path,body)
	if err!=nil { return err }
	req.Header.Set("Accept","application/json")
	if input!=nil { req.Header.Set("Content-Type","application/json") }
	if authorization!="" { req.Header.Set("Authorization",authorization) }

	resp,err:=c.http.Do(req)
	if err!=nil { return err }
	defer resp.Body.Close()
	responseBody,err:=io.ReadAll(io.LimitReader(resp.Body,1<<20))
	if err!=nil { return err }
	if resp.StatusCode<200 || resp.StatusCode>=300 {
		var apiErr struct { Message string `json:"message"` }
		if json.Unmarshal(responseBody,&apiErr)==nil && strings.TrimSpace(apiErr.Message)!="" {
			return errors.New(apiErr.Message)
		}
		return fmt.Errorf("ClusterStor API returned %s",resp.Status)
	}
	if out!=nil && len(responseBody)>0 {
		if err:=json.Unmarshal(responseBody,out); err!=nil { return err }
	}
	return nil
}


type GoogleSyncItem struct {
	NodeID string `json:"node_id"`
	ProviderItemID string `json:"provider_item_id"`
	ParentNodeID *string `json:"parent_node_id,omitempty"`
	Name string `json:"name"`
	NodeType string `json:"node_type"`
	MIMEType string `json:"mime_type,omitempty"`
	SizeBytes int64 `json:"size_bytes"`
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
	VersionID string `json:"version_id,omitempty"`
	Downloadable bool `json:"downloadable"`
}

type GoogleSyncSnapshot struct {
	Provider string `json:"provider"`
	Bootstrapped bool `json:"bootstrapped"`
	Items []GoogleSyncItem `json:"items"`
}

type GoogleUploadSession struct {
	UploadURL string `json:"upload_url"`
	ParentProviderID string `json:"parent_provider_id,omitempty"`
	Name string `json:"name"`
	ContentType string `json:"content_type"`
	SizeBytes int64 `json:"size_bytes"`
}

type GoogleFinalizedUpload struct {
	NodeID string `json:"node_id"`
	VersionID string `json:"version_id"`
	ProviderItemID string `json:"provider_item_id"`
	ProviderRevisionID string `json:"provider_revision_id,omitempty"`
	Name string `json:"name"`
	SizeBytes int64 `json:"size_bytes"`
}

func deviceAuthorization(deviceID,secret string) string {
	return "Device "+strings.TrimSpace(deviceID)+"."+strings.TrimSpace(secret)
}

func (c *Client) GoogleSyncSnapshot(ctx context.Context,deviceID,secret string) (GoogleSyncSnapshot,error) {
	var result GoogleSyncSnapshot
	err:=c.json(ctx,http.MethodGet,"/api/v1/agent/sync/google/snapshot",nil,deviceAuthorization(deviceID,secret),&result)
	return result,err
}

func (c *Client) CreateGoogleSyncFolder(ctx context.Context,deviceID,secret,name string,parentNodeID *string) (GoogleSyncItem,error) {
	var raw struct {
		NodeID string `json:"node_id"`
		ProviderItemID string `json:"provider_item_id"`
		ParentItemID *string `json:"parent_item_id,omitempty"`
		Name string `json:"name"`
		NodeType string `json:"node_type"`
		MIMEType string `json:"mime_type,omitempty"`
		SizeBytes *int64 `json:"size_bytes,omitempty"`
		ModifiedAt *time.Time `json:"modified_at,omitempty"`
	}
	err:=c.json(ctx,http.MethodPost,"/api/v1/agent/sync/google/folders",map[string]any{
		"name":name,
		"parent_node_id":parentNodeID,
	},deviceAuthorization(deviceID,secret),&raw)
	if err!=nil { return GoogleSyncItem{},err }
	size:=int64(0)
	if raw.SizeBytes!=nil { size=*raw.SizeBytes }
	return GoogleSyncItem{
		NodeID:raw.NodeID,ProviderItemID:raw.ProviderItemID,Name:raw.Name,NodeType:raw.NodeType,
		MIMEType:raw.MIMEType,SizeBytes:size,ModifiedAt:raw.ModifiedAt,Downloadable:false,
	},nil
}

func (c *Client) BeginGoogleSyncUpload(ctx context.Context,deviceID,secret,nodeID,name,contentType string,sizeBytes int64,parentNodeID *string) (GoogleUploadSession,error) {
	var result GoogleUploadSession
	err:=c.json(ctx,http.MethodPost,"/api/v1/agent/sync/google/uploads",map[string]any{
		"node_id":nodeID,
		"name":name,
		"content_type":contentType,
		"size_bytes":sizeBytes,
		"parent_node_id":parentNodeID,
	},deviceAuthorization(deviceID,secret),&result)
	return result,err
}

func (c *Client) UploadGoogleSession(ctx context.Context,uploadURL,contentType string,sizeBytes int64,body io.Reader) (string,error) {
	uploadURL=strings.TrimSpace(uploadURL)
	if uploadURL=="" { return "",errors.New("google upload session URL is empty") }
	if contentType=="" { contentType="application/octet-stream" }
	req,err:=http.NewRequestWithContext(ctx,http.MethodPut,uploadURL,body)
	if err!=nil { return "",err }
	req.Header.Set("Content-Type",contentType)
	req.Header.Set("Content-Length",fmt.Sprintf("%d",sizeBytes))
	req.ContentLength=sizeBytes

	resp,err:=c.transfer.Do(req)
	if err!=nil { return "",err }
	defer resp.Body.Close()
	responseBody,err:=io.ReadAll(io.LimitReader(resp.Body,1<<20))
	if err!=nil { return "",err }
	if resp.StatusCode<200 || resp.StatusCode>=300 {
		return "",fmt.Errorf("google upload returned %s",resp.Status)
	}
	var result struct { ID string `json:"id"` }
	if err:=json.Unmarshal(responseBody,&result); err!=nil {
		return "",fmt.Errorf("decode google upload response: %w",err)
	}
	if strings.TrimSpace(result.ID)=="" { return "",errors.New("google upload response missing item id") }
	return result.ID,nil
}

func (c *Client) FinalizeGoogleSyncUpload(ctx context.Context,deviceID,secret,providerItemID string) (GoogleFinalizedUpload,error) {
	var result GoogleFinalizedUpload
	err:=c.json(ctx,http.MethodPost,"/api/v1/agent/sync/google/uploads/complete",map[string]any{
		"provider_item_id":providerItemID,
	},deviceAuthorization(deviceID,secret),&result)
	return result,err
}

func (c *Client) MutateGoogleSyncNode(ctx context.Context,deviceID,secret,nodeID,name string,parentNodeID *string) (GoogleSyncItem,error) {
	var raw struct {
		NodeID string `json:"node_id"`
		ProviderItemID string `json:"provider_item_id"`
		Name string `json:"name"`
		NodeType string `json:"node_type"`
		MIMEType string `json:"mime_type,omitempty"`
		SizeBytes *int64 `json:"size_bytes,omitempty"`
		ModifiedAt *time.Time `json:"modified_at,omitempty"`
	}
	err:=c.json(ctx,http.MethodPatch,"/api/v1/agent/sync/google/nodes/"+url.PathEscape(nodeID),map[string]any{
		"name":name,
		"parent_node_id":parentNodeID,
	},deviceAuthorization(deviceID,secret),&raw)
	if err!=nil { return GoogleSyncItem{},err }
	size:=int64(0)
	if raw.SizeBytes!=nil { size=*raw.SizeBytes }
	return GoogleSyncItem{
		NodeID:raw.NodeID,ProviderItemID:raw.ProviderItemID,Name:raw.Name,NodeType:raw.NodeType,
		MIMEType:raw.MIMEType,SizeBytes:size,ModifiedAt:raw.ModifiedAt,
	},nil
}

func (c *Client) DeleteGoogleSyncNode(ctx context.Context,deviceID,secret,nodeID string) error {
	return c.json(ctx,http.MethodDelete,"/api/v1/agent/sync/google/nodes/"+url.PathEscape(nodeID),nil,deviceAuthorization(deviceID,secret),nil)
}

func (c *Client) DownloadGoogleSyncNode(ctx context.Context,deviceID,secret,nodeID string) (*http.Response,error) {
	if c.baseURL=="" { return nil,errors.New("ClusterStor API URL is empty") }
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,c.baseURL+"/api/v1/agent/sync/google/nodes/"+url.PathEscape(nodeID)+"/download",nil)
	if err!=nil { return nil,err }
	req.Header.Set("Authorization",deviceAuthorization(deviceID,secret))
	req.Header.Set("Accept","*/*")
	resp,err:=c.transfer.Do(req)
	if err!=nil { return nil,err }
	if resp.StatusCode<200 || resp.StatusCode>=300 {
		defer resp.Body.Close()
		body,_:=io.ReadAll(io.LimitReader(resp.Body,1<<20))
		var apiErr struct { Message string `json:"message"` }
		if json.Unmarshal(body,&apiErr)==nil && strings.TrimSpace(apiErr.Message)!="" {
			return nil,errors.New(apiErr.Message)
		}
		return nil,fmt.Errorf("ClusterStor API returned %s",resp.Status)
	}
	return resp,nil
}
