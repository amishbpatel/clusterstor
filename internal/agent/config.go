package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	APIBaseURL string `json:"api_base_url"`
	WebBaseURL string `json:"web_base_url"`
	DeviceID string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Platform string `json:"platform"`
	AgentVersion string `json:"agent_version"`
	SyncRoot string `json:"sync_root"`
	DriveName string `json:"drive_name"`
	DriveLetter string `json:"drive_letter"`
	SyncPaused bool `json:"sync_paused"`
	DefaultAvailability AvailabilityMode `json:"default_availability"`
	AvailabilityRules []AvailabilityRule `json:"availability_rules,omitempty"`
	FreeSpaceReserveBytes int64 `json:"free_space_reserve_bytes,omitempty"`
	FreeSpaceReservePercent int `json:"free_space_reserve_percent,omitempty"`
	PeerContributionEnabled bool `json:"peer_contribution_enabled"`
	PeerContributionBytes int64 `json:"peer_contribution_bytes"`
}

func StateDir() (string,error) {
	if runtime.GOOS=="windows" {
		if root:=strings.TrimSpace(os.Getenv("LOCALAPPDATA")); root!="" {
			return filepath.Join(root,"ClusterStor"),nil
		}
	}
	root,err:=os.UserConfigDir()
	if err!=nil { return "",err }
	if strings.TrimSpace(root)=="" { return "",errors.New("user config directory is unavailable") }
	return filepath.Join(root,"clusterstor"),nil
}

func LoadConfig() (Config,error) {
	dir,err:=StateDir()
	if err!=nil { return Config{},err }
	body,err:=os.ReadFile(filepath.Join(dir,"agent.json"))
	if errors.Is(err,os.ErrNotExist) { return Config{},nil }
	if err!=nil { return Config{},err }
	var cfg Config
	if err:=json.Unmarshal(body,&cfg); err!=nil { return Config{},err }
	ApplyLocalStorageDefaults(&cfg)
	return cfg,nil
}

func SaveConfig(cfg Config) error {
	ApplyLocalStorageDefaults(&cfg)
	dir,err:=StateDir()
	if err!=nil { return err }
	if err:=os.MkdirAll(dir,0700); err!=nil { return err }
	body,err:=json.MarshalIndent(cfg,"","  ")
	if err!=nil { return err }
	path:=filepath.Join(dir,"agent.json")
	temp:=path+".tmp"
	if err:=os.WriteFile(temp,body,0600); err!=nil { return err }
	return os.Rename(temp,path)
}
