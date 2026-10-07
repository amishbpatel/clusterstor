package agent

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const localReconcileModifiedTolerance = 2 * time.Second

func ReconcileLocalJournal(root string,journal *Journal) (int,error) {
	if journal==nil { return 0,errors.New("sync journal is required") }
	root=filepath.Clean(strings.TrimSpace(root))
	if root=="" { return 0,errors.New("sync root is required") }

	state:=journal.Snapshot()
	seen:=map[string]bool{}
	queued:=0

	err:=filepath.WalkDir(root,func(path string,d fs.DirEntry,walkErr error) error {
		if walkErr!=nil {
			if errors.Is(walkErr,os.ErrNotExist) { return nil }
			return walkErr
		}
		if filepath.Clean(path)==root { return nil }

		rel,ok:=NormalizeRelativePath(root,path)
		if !ok {
			if d.IsDir() { return filepath.SkipDir }
			return nil
		}
		if d.IsDir() {
			seen[pathKey(rel)]=true
			if _,exists:=findJournalItemByPath(state,rel); !exists {
				_,changed,err:=journal.QueueLocalChange(LocalChange{
					Kind:SyncOpCreateFolder,
					LocalPath:rel,
					ObservedAt:time.Now().UTC(),
				})
				if err!=nil { return err }
				if changed { queued++ }
			}
			return nil
		}
		if IsTransientWriteName(rel) { return nil }

		info,err:=d.Info()
		if err!=nil {
			if errors.Is(err,os.ErrNotExist) { return nil }
			return err
		}
		seen[pathKey(rel)]=true
		item,exists:=findJournalItemByPath(state,rel)
		changed:=!exists
		if exists && item.LocalContentState!=LocalContentResident && item.LocalContentState!="" {
			return nil
		}
		if exists {
			if item.SizeBytes!=info.Size() {
				changed=true
			} else if item.ModifiedAt!=nil {
				delta:=info.ModTime().Sub(*item.ModifiedAt)
				if delta<0 { delta=-delta }
				if delta>localReconcileModifiedTolerance { changed=true }
			}
		}
		if changed {
			_,didQueue,err:=journal.QueueLocalChange(LocalChange{
				Kind:SyncOpUpsertFile,
				LocalPath:rel,
				ObservedAt:time.Now().UTC(),
			})
			if err!=nil { return err }
			if didQueue { queued++ }
		}
		return nil
	})
	if err!=nil { return queued,err }

	latest:=journal.Snapshot()
	for _,item:=range latest.Items {
		if strings.TrimSpace(item.ProviderItemID)=="" && strings.TrimSpace(item.NodeID)=="" { continue }
		if item.SyncScope==SyncScopeExcluded { continue }
		if item.LocalContentState!="" && item.LocalContentState!=LocalContentResident { continue }
		if seen[pathKey(item.LocalPath)] { continue }

		_,changed,err:=journal.QueueLocalChange(LocalChange{
			Kind:SyncOpDelete,
			LocalPath:item.LocalPath,
			ObservedAt:time.Now().UTC(),
		})
		if err!=nil { return queued,err }
		if changed { queued++ }
	}
	return queued,nil
}

func findJournalItemByPath(state JournalState,path string) (JournalItem,bool) {
	for key,item:=range state.Items {
		if journalPathEqual(key,path) || journalPathEqual(item.LocalPath,path) {
			return item,true
		}
	}
	return JournalItem{},false
}
