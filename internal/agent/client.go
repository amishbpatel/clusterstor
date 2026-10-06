package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http *http.Client
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
