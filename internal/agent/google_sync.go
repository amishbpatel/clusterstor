package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const DefaultGoogleSyncInterval = 20 * time.Second

type resolvedGoogleSnapshot struct {
	ItemsByNode map[string]GoogleSyncItem
	ItemsByProvider map[string]GoogleSyncItem
	ItemsByPath map[string]GoogleSyncItem
	PathsByNode map[string]string
	Unsupported []GoogleSyncItem
}

func RunGoogleSyncLoop(ctx context.Context,client *Client,cfg Config,secret string,journal *Journal) {
	run:=func() {
		current,err:=LoadConfig()
		if err==nil && current.SyncPaused { return }
		if err:=SyncGoogleOnce(ctx,client,cfg,secret,journal); err!=nil && ctx.Err()==nil {
			log.Printf("google desktop sync waiting: %v",err)
		}
	}
	run()

	ticker:=time.NewTicker(DefaultGoogleSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func SyncGoogleOnce(ctx context.Context,client *Client,cfg Config,secret string,journal *Journal) error {
	if client==nil || journal==nil { return errors.New("google sync requires client and journal") }
	snapshot,err:=client.GoogleSyncSnapshot(ctx,cfg.DeviceID,secret)
	if err!=nil { return err }
	resolved:=resolveGoogleSnapshot(snapshot)
	for _,item:=range resolved.Unsupported {
		log.Printf("google sync skipped unsupported local filename: %q",item.Name)
	}

	if err:=processGooglePending(ctx,client,cfg,secret,journal,resolved); err!=nil {
		return err
	}

	// Local operations may have changed remote paths/versions. Refresh before
	// applying the remote state back to this device.
	snapshot,err=client.GoogleSyncSnapshot(ctx,cfg.DeviceID,secret)
	if err!=nil { return err }
	resolved=resolveGoogleSnapshot(snapshot)
	if err:=reconcileGoogleRemote(ctx,client,cfg,secret,journal,resolved); err!=nil {
		return err
	}
	return nil
}

func resolveGoogleSnapshot(snapshot GoogleSyncSnapshot) resolvedGoogleSnapshot {
	result:=resolvedGoogleSnapshot{
		ItemsByNode:map[string]GoogleSyncItem{},
		ItemsByProvider:map[string]GoogleSyncItem{},
		ItemsByPath:map[string]GoogleSyncItem{},
		PathsByNode:map[string]string{},
	}
	for _,item:=range snapshot.Items {
		result.ItemsByNode[item.NodeID]=item
		if item.ProviderItemID!="" { result.ItemsByProvider[item.ProviderItemID]=item }
	}

	remaining:=append([]GoogleSyncItem(nil),snapshot.Items...)
	for len(remaining)>0 {
		progress:=false
		next:=remaining[:0]
		for _,item:=range remaining {
			if !localSyncNameSupported(item.Name) {
				result.Unsupported=append(result.Unsupported,item)
				continue
			}
			var rel string
			if item.ParentNodeID==nil || strings.TrimSpace(*item.ParentNodeID)=="" {
				rel=filepath.Clean(item.Name)
			} else {
				parent,ok:=result.PathsByNode[*item.ParentNodeID]
				if !ok {
					next=append(next,item)
					continue
				}
				rel=filepath.Join(parent,item.Name)
			}
			result.PathsByNode[item.NodeID]=rel
			result.ItemsByPath[pathKey(rel)]=item
			progress=true
		}
		if !progress {
			for _,item:=range next { result.Unsupported=append(result.Unsupported,item) }
			break
		}
		remaining=next
	}
	return result
}

func localSyncNameSupported(name string) bool {
	if name=="" || name=="." || name==".." || strings.ContainsAny(name,"/\\") { return false }
	if runtime.GOOS!="windows" { return true }
	if strings.ContainsAny(name,`<>:"/\|?*`) || strings.HasSuffix(name," ") || strings.HasSuffix(name,".") { return false }
	base:=strings.ToUpper(strings.TrimSuffix(name,filepath.Ext(name)))
	switch base {
	case "CON","PRN","AUX","NUL","COM1","COM2","COM3","COM4","COM5","COM6","COM7","COM8","COM9",
		"LPT1","LPT2","LPT3","LPT4","LPT5","LPT6","LPT7","LPT8","LPT9":
		return false
	}
	return true
}

func processGooglePending(ctx context.Context,client *Client,cfg Config,secret string,journal *Journal,remote resolvedGoogleSnapshot) error {
	ops:=journal.PendingOperations()
	sort.SliceStable(ops,func(i,j int) bool {
		ri,rj:=syncOperationRank(ops[i]),syncOperationRank(ops[j])
		if ri!=rj { return ri<rj }
		di,dj:=pathDepth(ops[i].LocalPath),pathDepth(ops[j].LocalPath)
		if ops[i].Kind==string(SyncOpDelete) { return di>dj }
		return di<dj
	})

	for _,op:=range ops {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err:=processGoogleOperation(ctx,client,cfg,secret,journal,remote,op); err!=nil {
			return fmt.Errorf("%s %s: %w",op.Kind,op.LocalPath,err)
		}
	}
	return nil
}

func syncOperationRank(op PendingOperation) int {
	switch SyncOperationKind(op.Kind) {
	case SyncOpCreateFolder: return 0
	case SyncOpUpsertFile: return 1
	case SyncOpMove: return 2
	case SyncOpDelete: return 3
	default: return 4
	}
}

func pathDepth(path string) int {
	path=filepath.Clean(path)
	if path=="." || path=="" { return 0 }
	return len(strings.FieldsFunc(path,func(r rune) bool { return r=='/' || r=='\\' }))
}

func processGoogleOperation(ctx context.Context,client *Client,cfg Config,secret string,journal *Journal,remote resolvedGoogleSnapshot,op PendingOperation) error {
	item,remoteState,exists:=remoteForOperation(op,remote)
	decision:=DecideConflict(op,remoteState,cfg.DeviceName,time.Now())

	switch SyncOperationKind(op.Kind) {
	case SyncOpCreateFolder:
		if exists && item.NodeType=="folder" && strings.TrimSpace(op.NodeID)=="" {
			return journal.AcknowledgeOperation(op.ID,op.LocalPath,journalItemFromGoogle(item,op.LocalPath,journal))
		}
		if decision.Action!=ConflictApplyLocal {
			return fmt.Errorf("folder conflict requires a non-colliding local name: %s",decision.Reason)
		}
		parent,err:=googleParentNodeID(journal,remote,op.LocalPath)
		if err!=nil { return err }
		created,err:=client.CreateGoogleSyncFolder(ctx,cfg.DeviceID,secret,filepath.Base(op.LocalPath),parent)
		if err!=nil { return err }
		log.Printf("synced local folder to Google Drive: %s",op.LocalPath)
		return journal.AcknowledgeOperation(op.ID,op.LocalPath,journalItemFromGoogle(created,op.LocalPath,journal))

	case SyncOpUpsertFile:
		switch decision.Action {
		case ConflictPreserveBoth,ConflictRecoverLocalCopy:
			return preserveGoogleConflictFile(ctx,client,cfg,secret,journal,remote,op,decision)
		case ConflictKeepRemote,ConflictNoop:
			log.Printf("resolved local file conflict by preserving remote: %s (%s)",op.LocalPath,decision.Reason)
			return journal.CompleteOperation(op.ID)
		case ConflictApplyLocal:
			return uploadGoogleLocalFile(ctx,client,cfg,secret,journal,remote,op,item,exists)
		default:
			return fmt.Errorf("unsupported conflict action %q",decision.Action)
		}

	case SyncOpMove:
		switch decision.Action {
		case ConflictKeepRemote,ConflictNoop:
			log.Printf("resolved local move conflict by preserving remote location: %s (%s)",op.LocalPath,decision.Reason)
			return journal.CompleteOperation(op.ID)
		case ConflictApplyLocal:
			nodeID:=op.NodeID
			if nodeID=="" && exists { nodeID=item.NodeID }
			if nodeID=="" { return errors.New("remote identity missing for move") }
			parent,err:=googleParentNodeID(journal,remote,op.LocalPath)
			if err!=nil { return err }
			updated,err:=client.MutateGoogleSyncNode(ctx,cfg.DeviceID,secret,nodeID,filepath.Base(op.LocalPath),parent)
			if err!=nil { return err }
			if exists {
				updated.VersionID=item.VersionID
				updated.ModifiedAt=item.ModifiedAt
			}
			log.Printf("synced local move to Google Drive: %s -> %s",op.OldLocalPath,op.LocalPath)
			return journal.AcknowledgeOperation(op.ID,op.LocalPath,journalItemFromGoogle(updated,op.LocalPath,journal))
		default:
			return fmt.Errorf("unsupported move conflict action %q",decision.Action)
		}

	case SyncOpDelete:
		switch decision.Action {
		case ConflictNoop:
			if op.NodeID!="" { _=journal.RemoveRemoteItem(op.NodeID,op.BaseProviderItemID) }
			return journal.CompleteOperation(op.ID)
		case ConflictKeepRemote:
			log.Printf("suppressed local delete because remote changed: %s",op.LocalPath)
			return journal.CompleteOperation(op.ID)
		case ConflictApplyLocal:
			nodeID:=op.NodeID
			if nodeID=="" && exists { nodeID=item.NodeID }
			if nodeID=="" { return journal.CompleteOperation(op.ID) }
			if err:=client.DeleteGoogleSyncNode(ctx,cfg.DeviceID,secret,nodeID); err!=nil { return err }
			if err:=journal.CompleteOperation(op.ID); err!=nil { return err }
			if err:=journal.RemoveRemoteItem(nodeID,op.BaseProviderItemID); err!=nil { return err }
			log.Printf("synced local delete to Google Drive: %s",op.LocalPath)
			return nil
		default:
			return fmt.Errorf("unsupported delete conflict action %q",decision.Action)
		}
	}
	return fmt.Errorf("unsupported sync operation %q",op.Kind)
}

func remoteForOperation(op PendingOperation,remote resolvedGoogleSnapshot) (GoogleSyncItem,RemoteSnapshot,bool) {
	var item GoogleSyncItem
	var ok bool
	if strings.TrimSpace(op.NodeID)!="" {
		item,ok=remote.ItemsByNode[op.NodeID]
	}
	if !ok && strings.TrimSpace(op.BaseProviderItemID)!="" {
		item,ok=remote.ItemsByProvider[op.BaseProviderItemID]
	}
	if !ok {
		item,ok=remote.ItemsByPath[pathKey(op.LocalPath)]
	}
	if !ok {
		return GoogleSyncItem{},RemoteSnapshot{Exists:false},false
	}
	path:=remote.PathsByNode[item.NodeID]
	return item,RemoteSnapshot{
		Exists:true,
		NodeID:item.NodeID,
		ProviderItemID:item.ProviderItemID,
		VersionID:item.VersionID,
		LocalPath:path,
		ModifiedAt:item.ModifiedAt,
	},true
}

func googleParentNodeID(journal *Journal,remote resolvedGoogleSnapshot,path string) (*string,error) {
	parent:=filepath.Dir(filepath.Clean(path))
	if parent=="." || parent=="" { return nil,nil }
	if item,ok:=journal.ItemByPath(parent); ok && strings.TrimSpace(item.NodeID)!="" {
		value:=item.NodeID
		return &value,nil
	}
	if item,ok:=remote.ItemsByPath[pathKey(parent)]; ok && strings.TrimSpace(item.NodeID)!="" {
		value:=item.NodeID
		return &value,nil
	}
	return nil,fmt.Errorf("parent folder has not synced yet: %s",parent)
}

func uploadGoogleLocalFile(ctx context.Context,client *Client,cfg Config,secret string,journal *Journal,remote resolvedGoogleSnapshot,op PendingOperation,remoteItem GoogleSyncItem,remoteExists bool) error {
	full:=filepath.Join(cfg.SyncRoot,op.LocalPath)
	info,err:=os.Stat(full)
	if err!=nil { return err }
	if info.IsDir() { return errors.New("file upload target is a directory") }

	parent,err:=googleParentNodeID(journal,remote,op.LocalPath)
	if err!=nil { return err }
	contentType:=mime.TypeByExtension(filepath.Ext(full))
	if contentType=="" { contentType="application/octet-stream" }

	nodeID:=""
	if remoteExists && strings.TrimSpace(remoteItem.NodeID)!="" {
		nodeID=remoteItem.NodeID
	}
	session,err:=client.BeginGoogleSyncUpload(
		ctx,cfg.DeviceID,secret,nodeID,filepath.Base(op.LocalPath),contentType,info.Size(),parent,
	)
	if err!=nil { return err }

	file,err:=os.Open(full)
	if err!=nil { return err }
	providerItemID,uploadErr:=client.UploadGoogleSession(ctx,session.UploadURL,contentType,info.Size(),file)
	closeErr:=file.Close()
	if uploadErr!=nil { return uploadErr }
	if closeErr!=nil { return closeErr }

	finalized,err:=client.FinalizeGoogleSyncUpload(ctx,cfg.DeviceID,secret,providerItemID)
	if err!=nil { return err }
	modified:=info.ModTime().UTC()
	item:=JournalItem{
		LocalPath:op.LocalPath,
		NodeID:finalized.NodeID,
		Provider:"google_drive",
		ProviderItemID:finalized.ProviderItemID,
		VersionID:finalized.ProviderRevisionID,
		SizeBytes:finalized.SizeBytes,
		ModifiedAt:&modified,
		State:"synced",
		Availability:AvailabilityAutomatic,
		SyncScope:SyncScopeIncluded,
		LocalContentState:LocalContentResident,
		RemoteVerified:true,
	}
	if existing,ok:=journal.ItemByPath(op.LocalPath); ok {
		item.Availability=existing.Availability
		item.SyncScope=existing.SyncScope
	}
	if err:=journal.AcknowledgeOperation(op.ID,op.LocalPath,item); err!=nil { return err }
	log.Printf("synced local file to Google Drive: %s",op.LocalPath)
	return nil
}

func preserveGoogleConflictFile(ctx context.Context,client *Client,cfg Config,secret string,journal *Journal,remote resolvedGoogleSnapshot,op PendingOperation,decision ConflictDecision) error {
	source:=filepath.Join(cfg.SyncRoot,op.LocalPath)
	info,err:=os.Stat(source)
	if err!=nil { return err }
	if info.IsDir() { return errors.New("folder conflict copies are not yet supported") }

	conflictRel:=uniqueConflictRelativePath(cfg.SyncRoot,decision.ConflictPath)
	conflictFull:=filepath.Join(cfg.SyncRoot,conflictRel)
	if err:=os.MkdirAll(filepath.Dir(conflictFull),0700); err!=nil { return err }

	journal.SuppressLocalPath(op.LocalPath,15*time.Second)
	journal.SuppressLocalPath(conflictRel,15*time.Second)
	if err:=os.Rename(source,conflictFull); err!=nil { return fmt.Errorf("preserve local conflict copy: %w",err) }

	conflictOp:=op
	conflictOp.NodeID=""
	conflictOp.BaseProviderItemID=""
	conflictOp.BaseVersionID=""
	conflictOp.BaseLocalPath=conflictRel
	conflictOp.LocalPath=conflictRel
	conflictOp.OldLocalPath=""
	if err:=uploadGoogleLocalFile(ctx,client,cfg,secret,journal,remote,conflictOp,GoogleSyncItem{},false); err!=nil {
		_ = os.Rename(conflictFull,source)
		return err
	}
	// uploadGoogleLocalFile acknowledged the original operation ID while storing
	// the newly-created conflict item at conflictRel.
	if op.NodeID!="" && !decision.Conflict {
		_ = journal.RemoveRemoteItem(op.NodeID,op.BaseProviderItemID)
	}
	log.Printf("preserved local conflict copy: %s",conflictRel)
	return nil
}

func uniqueConflictRelativePath(root,rel string) string {
	rel=filepath.Clean(rel)
	if _,err:=os.Stat(filepath.Join(root,rel)); errors.Is(err,os.ErrNotExist) { return rel }
	dir:=filepath.Dir(rel)
	name:=filepath.Base(rel)
	ext:=filepath.Ext(name)
	stem:=strings.TrimSuffix(name,ext)
	for i:=2;i<1000;i++ {
		candidate:=filepath.Join(dir,fmt.Sprintf("%s (%d)%s",stem,i,ext))
		if _,err:=os.Stat(filepath.Join(root,candidate)); errors.Is(err,os.ErrNotExist) { return candidate }
	}
	return filepath.Join(dir,fmt.Sprintf("%s (%d)%s",stem,time.Now().UnixNano(),ext))
}

func reconcileGoogleRemote(ctx context.Context,client *Client,cfg Config,secret string,journal *Journal,remote resolvedGoogleSnapshot) error {
	items:=make([]GoogleSyncItem,0,len(remote.PathsByNode))
	for nodeID:=range remote.PathsByNode {
		items=append(items,remote.ItemsByNode[nodeID])
	}
	sort.Slice(items,func(i,j int) bool {
		pi,pj:=remote.PathsByNode[items[i].NodeID],remote.PathsByNode[items[j].NodeID]
		di,dj:=pathDepth(pi),pathDepth(pj)
		if di!=dj { return di<dj }
		if items[i].NodeType!=items[j].NodeType { return items[i].NodeType=="folder" }
		return pi<pj
	})

	remoteNodes:=map[string]bool{}
	for _,item:=range items {
		remoteNodes[item.NodeID]=true
		rel:=remote.PathsByNode[item.NodeID]
		if rel=="" { continue }

		oldPath,oldItem,known:=journal.ItemByNodeID(item.NodeID)
		if journal.HasPendingPath(rel) || (known && journal.HasPendingPath(oldPath)) {
			continue
		}

		full:=filepath.Join(cfg.SyncRoot,rel)
		if known && !journalPathEqual(oldPath,rel) {
			oldFull:=filepath.Join(cfg.SyncRoot,oldPath)
			if _,err:=os.Stat(oldFull); err==nil {
				if _,destErr:=os.Stat(full); errors.Is(destErr,os.ErrNotExist) {
					if err:=os.MkdirAll(filepath.Dir(full),0700); err!=nil { return err }
					journal.SuppressLocalPath(oldPath,15*time.Second)
					journal.SuppressLocalPath(rel,15*time.Second)
					if err:=os.Rename(oldFull,full); err!=nil {
						return fmt.Errorf("apply remote move %s -> %s: %w",oldPath,rel,err)
					}
				} else if destErr==nil {
					return fmt.Errorf("remote move destination already exists locally: %s",rel)
				}
			}
		}

		if item.NodeType=="folder" {
			journal.SuppressLocalPath(rel,15*time.Second)
			if err:=os.MkdirAll(full,0700); err!=nil { return err }
			if err:=journal.RecordRemoteItem(rel,journalItemFromGoogle(item,rel,journal)); err!=nil { return err }
			continue
		}

		if !item.Downloadable {
			meta:=journalItemFromGoogle(item,rel,journal)
			meta.LocalContentState=LocalContentUnavailable
			if err:=journal.RecordRemoteItem(rel,meta); err!=nil { return err }
			continue
		}

		needDownload:=true
		if info,err:=os.Stat(full); err==nil && !info.IsDir() && known &&
			oldItem.VersionID==item.VersionID && oldItem.SizeBytes==item.SizeBytes {
			needDownload=false
		}
		if needDownload {
			if err:=downloadGoogleRemoteFile(ctx,client,cfg,secret,journal,item,rel); err!=nil { return err }
			log.Printf("applied Google Drive file locally: %s",rel)
		}
		if err:=journal.RecordRemoteItem(rel,journalItemFromGoogle(item,rel,journal)); err!=nil { return err }
	}

	state:=journal.Snapshot()
	for _,item:=range state.Items {
		if item.Provider!="google_drive" || strings.TrimSpace(item.NodeID)=="" { continue }
		if remoteNodes[item.NodeID] { continue }
		if journal.HasPendingPath(item.LocalPath) { continue }
		full:=filepath.Join(cfg.SyncRoot,item.LocalPath)
		journal.SuppressLocalPath(item.LocalPath,15*time.Second)
		if err:=os.RemoveAll(full); err!=nil && !errors.Is(err,os.ErrNotExist) {
			return fmt.Errorf("apply remote delete %s: %w",item.LocalPath,err)
		}
		if err:=journal.RemoveRemoteItem(item.NodeID,item.ProviderItemID); err!=nil { return err }
		log.Printf("applied Google Drive delete locally: %s",item.LocalPath)
	}
	return nil
}

func downloadGoogleRemoteFile(ctx context.Context,client *Client,cfg Config,secret string,journal *Journal,item GoogleSyncItem,rel string) error {
	target:=filepath.Join(cfg.SyncRoot,rel)
	if err:=os.MkdirAll(filepath.Dir(target),0700); err!=nil { return err }
	resp,err:=client.DownloadGoogleSyncNode(ctx,cfg.DeviceID,secret,item.NodeID)
	if err!=nil { return err }
	defer resp.Body.Close()

	temp,err:=os.CreateTemp(filepath.Dir(target),".clusterstor-download-*")
	if err!=nil { return err }
	tempName:=temp.Name()
	cleanup:=func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	if _,err:=io.Copy(temp,resp.Body); err!=nil {
		cleanup()
		return err
	}
	if err:=temp.Sync(); err!=nil {
		cleanup()
		return err
	}
	if err:=temp.Close(); err!=nil {
		_ = os.Remove(tempName)
		return err
	}
	if info,err:=os.Stat(tempName); err!=nil {
		_ = os.Remove(tempName)
		return err
	} else if item.SizeBytes>=0 && info.Size()!=item.SizeBytes {
		_ = os.Remove(tempName)
		return fmt.Errorf("download size mismatch for %s: got %d expected %d",rel,info.Size(),item.SizeBytes)
	}

	journal.SuppressLocalPath(rel,15*time.Second)
	if err:=replaceLocalFile(tempName,target); err!=nil {
		_ = os.Remove(tempName)
		return err
	}
	if item.ModifiedAt!=nil {
		_ = os.Chtimes(target,*item.ModifiedAt,*item.ModifiedAt)
	}
	return nil
}

func replaceLocalFile(temp,target string) error {
	backup:=""
	if info,err:=os.Stat(target); err==nil {
		if info.IsDir() { return fmt.Errorf("local path is a directory: %s",target) }
		backup=target+".clusterstor-old-"+fmt.Sprintf("%d",time.Now().UnixNano())
		if err:=os.Rename(target,backup); err!=nil { return err }
	} else if !errors.Is(err,os.ErrNotExist) {
		return err
	}
	if err:=os.Rename(temp,target); err!=nil {
		if backup!="" { _=os.Rename(backup,target) }
		return err
	}
	if backup!="" { _=os.Remove(backup) }
	return nil
}

func journalItemFromGoogle(item GoogleSyncItem,path string,journal *Journal) JournalItem {
	result:=JournalItem{
		LocalPath:path,
		NodeID:item.NodeID,
		Provider:"google_drive",
		ProviderItemID:item.ProviderItemID,
		VersionID:item.VersionID,
		SizeBytes:item.SizeBytes,
		ModifiedAt:item.ModifiedAt,
		State:"synced",
		Availability:AvailabilityAutomatic,
		SyncScope:SyncScopeIncluded,
		LocalContentState:LocalContentResident,
		RemoteVerified:true,
	}
	if existing,ok:=journal.ItemByPath(path); ok {
		if existing.Availability!="" { result.Availability=existing.Availability }
		if existing.SyncScope!="" { result.SyncScope=existing.SyncScope }
		if existing.LocalContentState!="" { result.LocalContentState=existing.LocalContentState }
	}
	return result
}
