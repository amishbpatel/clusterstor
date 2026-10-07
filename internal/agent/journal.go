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
	Availability AvailabilityMode `json:"availability,omitempty"`
	SyncScope SyncScope `json:"sync_scope,omitempty"`
	LocalContentState LocalContentState `json:"local_content_state,omitempty"`
	RemoteVerified bool `json:"remote_verified,omitempty"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
}

type PendingOperation struct {
	ID string `json:"id"`
	Kind string `json:"kind"`
	LocalPath string `json:"local_path"`
	OldLocalPath string `json:"old_local_path,omitempty"`
	NodeID string `json:"node_id,omitempty"`
	BaseProviderItemID string `json:"base_provider_item_id,omitempty"`
	BaseVersionID string `json:"base_version_id,omitempty"`
	BaseLocalPath string `json:"base_local_path,omitempty"`
	BaseModifiedAt *time.Time `json:"base_modified_at,omitempty"`
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
	suppressed map[string]time.Time
}

func OpenJournal(deviceID,syncRoot string) (*Journal,error) {
	dir,err:=StateDir()
	if err!=nil { return nil,err }
	if err:=os.MkdirAll(dir,0700); err!=nil { return nil,err }
	path:=filepath.Join(dir,"sync-journal.json")
	j:=&Journal{path:path,suppressed:map[string]time.Time{}}
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
	if j.isSuppressedLocked(change.LocalPath) || (change.OldLocalPath!="." && j.isSuppressedLocked(change.OldLocalPath)) {
		return PendingOperation{},false,nil
	}
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


func (j *Journal) PendingOperations() []PendingOperation {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]PendingOperation(nil),j.state.Pending...)
}

func (j *Journal) HasPendingPath(path string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _,op:=range j.state.Pending {
		if journalPathEqual(op.LocalPath,path) || journalPathWithin(op.LocalPath,path) ||
			(op.OldLocalPath!="" && (journalPathEqual(op.OldLocalPath,path) || journalPathWithin(op.OldLocalPath,path))) {
			return true
		}
	}
	return false
}

func (j *Journal) ItemByPath(path string) (JournalItem,bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	item:=j.itemForPathLocked(path)
	if strings.TrimSpace(item.LocalPath)=="" && strings.TrimSpace(item.NodeID)=="" && strings.TrimSpace(item.ProviderItemID)=="" {
		return JournalItem{},false
	}
	return item,true
}

func (j *Journal) ItemByNodeID(nodeID string) (string,JournalItem,bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	nodeID=strings.TrimSpace(nodeID)
	if nodeID=="" { return "",JournalItem{},false }
	for key,item:=range j.state.Items {
		if item.NodeID==nodeID { return key,item,true }
	}
	return "",JournalItem{},false
}

func (j *Journal) AcknowledgeOperation(operationID,path string,item JournalItem) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	operationID=strings.TrimSpace(operationID)
	path=filepath.Clean(strings.TrimSpace(path))
	j.removePendingLocked(func(op PendingOperation) bool { return operationID!="" && op.ID==operationID })

	if path!="" && path!="." {
		item.LocalPath=path
		if item.State=="" { item.State="synced" }
		if item.Availability=="" { item.Availability=AvailabilityAutomatic }
		if item.SyncScope=="" { item.SyncScope=SyncScopeIncluded }
		if item.LocalContentState=="" { item.LocalContentState=LocalContentResident }

		for key,current:=range j.state.Items {
			if key==path { continue }
			if item.NodeID!="" && current.NodeID==item.NodeID {
				delete(j.state.Items,key)
				continue
			}
			if item.ProviderItemID!="" && current.ProviderItemID==item.ProviderItemID {
				delete(j.state.Items,key)
			}
		}
		j.state.Items[path]=item

		// A newer local event may have replaced the operation while this upload
		// was in flight. Rebase that newer work onto the revision we just
		// committed so sequential edits on one device are not false conflicts.
		for i:=range j.state.Pending {
			op:=&j.state.Pending[i]
			if !journalPathEqual(op.LocalPath,path) { continue }
			op.NodeID=item.NodeID
			op.BaseProviderItemID=item.ProviderItemID
			op.BaseVersionID=item.VersionID
			op.BaseLocalPath=path
			op.BaseModifiedAt=item.ModifiedAt
		}
	}

	j.state.Generation++
	j.state.UpdatedAt=time.Now().UTC()
	return j.persistLocked()
}

func (j *Journal) CompleteOperation(operationID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	operationID=strings.TrimSpace(operationID)
	if operationID=="" { return nil }
	removed:=j.removePendingLocked(func(op PendingOperation) bool { return op.ID==operationID })
	if !removed { return nil }
	j.state.Generation++
	j.state.UpdatedAt=time.Now().UTC()
	return j.persistLocked()
}

func (j *Journal) RecordRemoteItem(path string,item JournalItem) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	path=filepath.Clean(strings.TrimSpace(path))
	if path=="" || path=="." { return nil }
	item.LocalPath=path
	if item.State=="" { item.State="synced" }
	if item.Availability=="" { item.Availability=AvailabilityAutomatic }
	if item.SyncScope=="" { item.SyncScope=SyncScopeIncluded }
	if item.LocalContentState=="" { item.LocalContentState=LocalContentResident }

	duplicate:=false
	for key,current:=range j.state.Items {
		if key==path { continue }
		if (item.NodeID!="" && current.NodeID==item.NodeID) ||
			(item.ProviderItemID!="" && current.ProviderItemID==item.ProviderItemID) {
			duplicate=true
			break
		}
	}
	if current,ok:=j.state.Items[path]; ok && !duplicate && journalItemsEqual(current,item) {
		return nil
	}

	for key,current:=range j.state.Items {
		if key==path { continue }
		if item.NodeID!="" && current.NodeID==item.NodeID {
			delete(j.state.Items,key)
			continue
		}
		if item.ProviderItemID!="" && current.ProviderItemID==item.ProviderItemID {
			delete(j.state.Items,key)
		}
	}
	j.state.Items[path]=item
	j.state.Generation++
	j.state.UpdatedAt=time.Now().UTC()
	return j.persistLocked()
}

func journalItemsEqual(a,b JournalItem) bool {
	return a.LocalPath==b.LocalPath &&
		a.NodeID==b.NodeID &&
		a.Provider==b.Provider &&
		a.ProviderItemID==b.ProviderItemID &&
		a.VersionID==b.VersionID &&
		a.SizeBytes==b.SizeBytes &&
		timePointersEqual(a.ModifiedAt,b.ModifiedAt) &&
		a.State==b.State &&
		a.Availability==b.Availability &&
		a.SyncScope==b.SyncScope &&
		a.LocalContentState==b.LocalContentState &&
		a.RemoteVerified==b.RemoteVerified &&
		timePointersEqual(a.LastAccessedAt,b.LastAccessedAt)
}

func timePointersEqual(a,b *time.Time) bool {
	if a==nil || b==nil { return a==nil && b==nil }
	return a.Equal(*b)
}

func (j *Journal) RemoveRemoteItem(nodeID,providerItemID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	changed:=false
	for key,item:=range j.state.Items {
		if (nodeID!="" && item.NodeID==nodeID) || (providerItemID!="" && item.ProviderItemID==providerItemID) {
			delete(j.state.Items,key)
			changed=true
		}
	}
	if !changed { return nil }
	j.state.Generation++
	j.state.UpdatedAt=time.Now().UTC()
	return j.persistLocked()
}

func (j *Journal) SuppressLocalPath(path string,duration time.Duration) {
	j.mu.Lock()
	defer j.mu.Unlock()
	path=filepath.Clean(strings.TrimSpace(path))
	if path=="" || path=="." { return }
	if duration<=0 { duration=10*time.Second }
	if j.suppressed==nil { j.suppressed=map[string]time.Time{} }
	j.suppressed[path]=time.Now().Add(duration)
}

func (j *Journal) isSuppressedLocked(path string) bool {
	path=filepath.Clean(strings.TrimSpace(path))
	if path=="" || path=="." || len(j.suppressed)==0 { return false }
	now:=time.Now()
	for key,until:=range j.suppressed {
		if now.After(until) {
			delete(j.suppressed,key)
			continue
		}
		if journalPathEqual(path,key) || journalPathWithin(path,key) { return true }
	}
	return false
}

func (j *Journal) newOperationLocked(change LocalChange) PendingOperation {
	basePath:=change.LocalPath
	if change.Kind==SyncOpMove && change.OldLocalPath!="" {
		basePath=change.OldLocalPath
	}
	base:=j.itemForPathLocked(basePath)

	return PendingOperation{
		ID:newOperationID(),
		Kind:string(change.Kind),
		LocalPath:change.LocalPath,
		OldLocalPath:change.OldLocalPath,
		NodeID:base.NodeID,
		BaseProviderItemID:base.ProviderItemID,
		BaseVersionID:base.VersionID,
		BaseLocalPath:basePath,
		BaseModifiedAt:base.ModifiedAt,
		CreatedAt:change.ObservedAt.UTC(),
	}
}

func (j *Journal) itemForPathLocked(path string) JournalItem {
	for key,item:=range j.state.Items {
		if journalPathEqual(key,path) || journalPathEqual(item.LocalPath,path) {
			return item
		}
	}
	return JournalItem{}
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
