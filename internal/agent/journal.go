package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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


func (j *Journal) QueueLocalChange(change LocalChange) (PendingOperation,bool,error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	change.LocalPath=filepath.Clean(strings.TrimSpace(change.LocalPath))
	change.OldLocalPath=filepath.Clean(strings.TrimSpace(change.OldLocalPath))
	if change.LocalPath=="" || change.LocalPath=="." {
		return PendingOperation{},false,nil
	}
	if change.ObservedAt.IsZero() { change.ObservedAt=time.Now().UTC() }

	var queued PendingOperation
	changed:=false

	switch change.Kind {
	case SyncOpUpsertFile:
		j.removePendingLocked(func(op PendingOperation) bool {
			return op.Kind==string(SyncOpUpsertFile) && journalPathEqual(op.LocalPath,change.LocalPath)
		})
		queued=j.newOperationLocked(change)
		j.state.Pending=append(j.state.Pending,queued)
		changed=true

	case SyncOpCreateFolder:
		for _,op:=range j.state.Pending {
			if op.Kind==string(SyncOpCreateFolder) && journalPathEqual(op.LocalPath,change.LocalPath) {
				return op,false,nil
			}
		}
		queued=j.newOperationLocked(change)
		j.state.Pending=append(j.state.Pending,queued)
		changed=true

	case SyncOpMove:
		if change.OldLocalPath=="" || change.OldLocalPath=="." || journalPathEqual(change.OldLocalPath,change.LocalPath) {
			return PendingOperation{},false,nil
		}

		if !j.remoteKnownLocked(change.OldLocalPath) {
			rewritten:=false
			for i:=range j.state.Pending {
				op:=&j.state.Pending[i]
				if journalPathEqual(op.LocalPath,change.OldLocalPath) &&
					(op.Kind==string(SyncOpUpsertFile) || op.Kind==string(SyncOpCreateFolder)) {
					op.LocalPath=change.LocalPath
					rewritten=true
					queued=*op
				}
				if journalPathWithin(op.LocalPath,change.OldLocalPath) {
					op.LocalPath=replaceJournalPrefix(op.LocalPath,change.OldLocalPath,change.LocalPath)
					rewritten=true
				}
				if op.OldLocalPath!="" && journalPathWithin(op.OldLocalPath,change.OldLocalPath) {
					op.OldLocalPath=replaceJournalPrefix(op.OldLocalPath,change.OldLocalPath,change.LocalPath)
					rewritten=true
				}
			}
			if rewritten {
				j.remapItemsLocked(change.OldLocalPath,change.LocalPath)
				changed=true
				break
			}
		}

		for i:=range j.state.Pending {
			op:=&j.state.Pending[i]
			if op.Kind==string(SyncOpMove) && journalPathEqual(op.LocalPath,change.OldLocalPath) {
				op.LocalPath=change.LocalPath
				queued=*op
				j.remapItemsLocked(change.OldLocalPath,change.LocalPath)
				changed=true
				break
			}
		}
		if !changed {
			queued=j.newOperationLocked(change)
			j.state.Pending=append(j.state.Pending,queued)
			j.remapItemsLocked(change.OldLocalPath,change.LocalPath)
			changed=true
		}

	case SyncOpDelete:
		remoteKnown:=j.remoteKnownTreeLocked(change.LocalPath)
		removed:=j.removePendingLocked(func(op PendingOperation) bool {
			return journalPathEqual(op.LocalPath,change.LocalPath) || journalPathWithin(op.LocalPath,change.LocalPath)
		})
		if !remoteKnown {
			if removed {
				changed=true
			} else {
				return PendingOperation{},false,nil
			}
			break
		}
		queued=j.newOperationLocked(change)
		j.state.Pending=append(j.state.Pending,queued)
		changed=true

	default:
		return PendingOperation{},false,fmt.Errorf("unsupported local change kind %q",change.Kind)
	}

	if !changed { return queued,false,nil }
	j.state.Generation++
	j.state.UpdatedAt=time.Now().UTC()
	if err:=j.persistLocked(); err!=nil { return PendingOperation{},false,err }
	return queued,true,nil
}

func (j *Journal) newOperationLocked(change LocalChange) PendingOperation {
	return PendingOperation{
		ID:newOperationID(),
		Kind:string(change.Kind),
		LocalPath:change.LocalPath,
		OldLocalPath:change.OldLocalPath,
		CreatedAt:change.ObservedAt.UTC(),
	}
}

func (j *Journal) removePendingLocked(match func(PendingOperation) bool) bool {
	if len(j.state.Pending)==0 { return false }
	out:=j.state.Pending[:0]
	removed:=false
	for _,op:=range j.state.Pending {
		if match(op) {
			removed=true
			continue
		}
		out=append(out,op)
	}
	j.state.Pending=out
	return removed
}

func (j *Journal) remoteKnownLocked(path string) bool {
	for key,item:=range j.state.Items {
		if journalPathEqual(key,path) || journalPathEqual(item.LocalPath,path) {
			return strings.TrimSpace(item.NodeID)!="" || strings.TrimSpace(item.ProviderItemID)!="" || strings.EqualFold(item.State,"synced")
		}
	}
	return false
}

func (j *Journal) remoteKnownTreeLocked(path string) bool {
	for key,item:=range j.state.Items {
		itemPath:=key
		if strings.TrimSpace(item.LocalPath)!="" { itemPath=item.LocalPath }
		if journalPathEqual(itemPath,path) || journalPathWithin(itemPath,path) {
			if strings.TrimSpace(item.NodeID)!="" || strings.TrimSpace(item.ProviderItemID)!="" || strings.EqualFold(item.State,"synced") {
				return true
			}
		}
	}
	return false
}

func (j *Journal) remapItemsLocked(oldPrefix,newPrefix string) {
	if len(j.state.Items)==0 { return }
	next:=make(map[string]JournalItem,len(j.state.Items))
	for key,item:=range j.state.Items {
		target:=key
		if journalPathEqual(key,oldPrefix) || journalPathWithin(key,oldPrefix) {
			target=replaceJournalPrefix(key,oldPrefix,newPrefix)
		}
		if item.LocalPath!="" && (journalPathEqual(item.LocalPath,oldPrefix) || journalPathWithin(item.LocalPath,oldPrefix)) {
			item.LocalPath=replaceJournalPrefix(item.LocalPath,oldPrefix,newPrefix)
		}
		next[target]=item
	}
	j.state.Items=next
}

func journalPathEqual(a,b string) bool {
	a=filepath.Clean(a)
	b=filepath.Clean(b)
	if runtime.GOOS=="windows" { return strings.EqualFold(a,b) }
	return a==b
}

func journalPathWithin(path,parent string) bool {
	path=filepath.Clean(path)
	parent=filepath.Clean(parent)
	if journalPathEqual(path,parent) { return false }
	rel,err:=filepath.Rel(parent,path)
	if err!=nil { return false }
	if rel==".." || strings.HasPrefix(rel,".."+string(filepath.Separator)) { return false }
	return rel!="."
}

func replaceJournalPrefix(path,oldPrefix,newPrefix string) string {
	path=filepath.Clean(path)
	oldPrefix=filepath.Clean(oldPrefix)
	newPrefix=filepath.Clean(newPrefix)
	if journalPathEqual(path,oldPrefix) { return newPrefix }
	rel,err:=filepath.Rel(oldPrefix,path)
	if err!=nil { return path }
	return filepath.Join(newPrefix,rel)
}

func newOperationID() string {
	var body [12]byte
	if _,err:=rand.Read(body[:]); err==nil {
		return hex.EncodeToString(body[:])
	}
	return fmt.Sprintf("%x",time.Now().UnixNano())
}

func (j *Journal) persistLocked() error {
	body,err:=json.MarshalIndent(j.state,"","  ")
	if err!=nil { return err }
	temp:=j.path+".tmp"
	if err:=os.WriteFile(temp,body,0600); err!=nil { return err }
	return os.Rename(temp,j.path)
}
