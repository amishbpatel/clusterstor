package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const journalSchemaVersion = 1

type JournalItem struct {
	LocalPath string `json:"local_path"`
	NodeID string `json:"node_id,omitempty"`
	Provider string `json:"provider,omitempty"`
	ProviderItemID string `json:"provider_item_id,omitempty"`
	VersionID string `json:"version_id,omitempty"`
	SizeBytes int64 `json:"size_bytes,omitempty"`
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
	State string `json:"state"`
}

type PendingOperation struct {
	ID string `json:"id"`
	Kind string `json:"kind"`
	LocalPath string `json:"local_path"`
	OldLocalPath string `json:"old_local_path,omitempty"`
	NodeID string `json:"node_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Attempts int `json:"attempts"`
	LastError string `json:"last_error,omitempty"`
}

type JournalState struct {
	SchemaVersion int `json:"schema_version"`
	DeviceID string `json:"device_id"`
	SyncRoot string `json:"sync_root"`
	Generation int64 `json:"generation"`
	Items map[string]JournalItem `json:"items"`
	Pending []PendingOperation `json:"pending"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Journal struct {
	mu sync.Mutex
	path string
	state JournalState
}

func OpenJournal(deviceID,syncRoot string) (*Journal,error) {
	dir,err:=StateDir()
	if err!=nil { return nil,err }
	if err:=os.MkdirAll(dir,0700); err!=nil { return nil,err }
	path:=filepath.Join(dir,"sync-journal.json")
	j:=&Journal{path:path}
	body,err:=os.ReadFile(path)
	switch {
	case errors.Is(err,os.ErrNotExist):
		j.state=JournalState{
			SchemaVersion:journalSchemaVersion,
			DeviceID:deviceID,
			SyncRoot:syncRoot,
			Items:map[string]JournalItem{},
			Pending:[]PendingOperation{},
			UpdatedAt:time.Now().UTC(),
		}
		if err:=j.persistLocked(); err!=nil { return nil,err }
	case err!=nil:
		return nil,err
	default:
		if err:=json.Unmarshal(body,&j.state); err!=nil { return nil,err }
		if j.state.SchemaVersion!=journalSchemaVersion {
			return nil,errors.New("unsupported ClusterStor sync journal version")
		}
		if j.state.Items==nil { j.state.Items=map[string]JournalItem{} }
		if j.state.Pending==nil { j.state.Pending=[]PendingOperation{} }
		changed:=false
		if j.state.DeviceID!=deviceID { j.state.DeviceID=deviceID; changed=true }
		if j.state.SyncRoot!=syncRoot { j.state.SyncRoot=syncRoot; changed=true }
		if changed {
			j.state.Generation++
			j.state.UpdatedAt=time.Now().UTC()
			if err:=j.persistLocked(); err!=nil { return nil,err }
		}
	}
	return j,nil
}

func (j *Journal) Snapshot() JournalState {
	j.mu.Lock()
	defer j.mu.Unlock()
	state:=j.state
	state.Items=make(map[string]JournalItem,len(j.state.Items))
	for key,item:=range j.state.Items { state.Items[key]=item }
	state.Pending=append([]PendingOperation(nil),j.state.Pending...)
	return state
}

func (j *Journal) persistLocked() error {
	body,err:=json.MarshalIndent(j.state,"","  ")
	if err!=nil { return err }
	temp:=j.path+".tmp"
	if err:=os.WriteFile(temp,body,0600); err!=nil { return err }
	return os.Rename(temp,j.path)
}
