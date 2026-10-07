package agent

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type watcherEntry struct {
	ID        string
	IsDir     bool
	Size      int64
	Modified  int64
}

type pendingRemoval struct {
	Path  string
	ID    string
	IsDir bool
	Due   time.Time
}

type pendingWrite struct {
	Path       string
	Due        time.Time
	HasSample  bool
	SampleSize int64
	SampleMod  int64
}

type filesystemWatcher struct {
	root       string
	native     *fsnotify.Watcher
	journal    *Journal
	entries    map[string]watcherEntry
	removals   map[string]pendingRemoval
	writes     map[string]pendingWrite
	onQueued   func(PendingOperation)
}

func StartFilesystemWatcher(root string,journal *Journal,onQueued func(PendingOperation)) (*filesystemWatcher,error) {
	if journal==nil { return nil,errors.New("sync journal is required") }
	root=filepath.Clean(strings.TrimSpace(root))
	if root=="" { return nil,errors.New("watch root is required") }

	native,err:=fsnotify.NewWatcher()
	if err!=nil { return nil,err }

	w:=&filesystemWatcher{
		root:root,
		native:native,
		journal:journal,
		entries:map[string]watcherEntry{},
		removals:map[string]pendingRemoval{},
		writes:map[string]pendingWrite{},
		onQueued:onQueued,
	}
	log.Printf("Filesystem watcher initializing: %s",root)
	if err:=w.scanTree(root,false); err!=nil {
		_ = native.Close()
		return nil,fmt.Errorf("initial filesystem scan: %w",err)
	}
	log.Printf("Filesystem watcher active: %s (%d indexed paths)",root,len(w.entries))
	return w,nil
}

func (w *filesystemWatcher) Run(ctx context.Context) error {
	if w==nil || w.native==nil { return errors.New("filesystem watcher is not initialized") }
	defer w.native.Close()

	ticker:=time.NewTicker(250*time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err,ok:=<-w.native.Errors:
			if !ok { return nil }
			log.Printf("filesystem watcher warning: %v",err)
		case event,ok:=<-w.native.Events:
			if !ok { return nil }
			if err:=w.handleEvent(event); err!=nil {
				log.Printf("filesystem event warning for %s: %v",event.Name,err)
			}
		case now:=<-ticker.C:
			w.flushRemovals(now)
			w.flushWrites(now)
		}
	}
}

func RunFilesystemWatcher(ctx context.Context,root string,journal *Journal,onQueued func(PendingOperation)) error {
	w,err:=StartFilesystemWatcher(root,journal,onQueued)
	if err!=nil { return err }
	return w.Run(ctx)
}

func (w *filesystemWatcher) handleEvent(event fsnotify.Event) error {
	rel,ok:=NormalizeRelativePath(w.root,event.Name)
	if !ok { return nil }
	if w.journal.IsLocalPathSuppressed(rel) {
		w.cancelWritesUnder(rel)
		w.cancelRemovalsUnder(rel)
		return nil
	}

	if event.Op&fsnotify.Create!=0 {
		info,err:=os.Stat(event.Name)
		if err==nil {
			id,_:=fileIdentity(event.Name)
			if removalKey,removal,matched:=w.removalForID(id); matched {
				delete(w.removals,removalKey)
				w.cancelRemovalsUnder(removal.Path)
				w.remapWrites(removal.Path,rel)
				if _,changed,err:=w.journal.QueueLocalChange(LocalChange{
					Kind:SyncOpMove,
					LocalPath:rel,
					OldLocalPath:removal.Path,
					ObservedAt:time.Now().UTC(),
				}); err!=nil {
					return err
				} else if changed {
					log.Printf("journaled local move: %s -> %s",removal.Path,rel)
				}
				w.removeSnapshot(removal.Path)
				if removal.IsDir {
					_ = w.native.Remove(filepath.Join(w.root,removal.Path))
					return w.scanTree(event.Name,false)
				}
				w.rememberPath(rel,event.Name,info,id)
				return nil
			}

			if info.IsDir() {
				return w.scanTree(event.Name,true)
			}
			w.rememberPath(rel,event.Name,info,id)
			if !IsTransientWriteName(rel) {
				w.scheduleWrite(rel)
			}
		}
	}

	if event.Op&fsnotify.Write!=0 {
		info,err:=os.Stat(event.Name)
		if err==nil && !info.IsDir() {
			id,_:=fileIdentity(event.Name)
			w.rememberPath(rel,event.Name,info,id)
			if !IsTransientWriteName(rel) {
				w.scheduleWrite(rel)
			}
		}
	}

	if event.Op&(fsnotify.Rename|fsnotify.Remove)!=0 {
		if w.underPendingRemoval(rel) { return nil }
		entry,exists:=w.entries[pathKey(rel)]
		removal:=pendingRemoval{
			Path:rel,
			Due:time.Now().Add(DefaultRenameGrace),
		}
		if exists {
			removal.ID=entry.ID
			removal.IsDir=entry.IsDir
		}
		key:="path:"+pathKey(rel)
		if removal.ID!="" { key="id:"+removal.ID }
		w.removals[key]=removal
	}

	return nil
}

func (w *filesystemWatcher) scanTree(start string,queueNew bool) error {
	return filepath.WalkDir(start,func(path string,d fs.DirEntry,walkErr error) error {
		if walkErr!=nil {
			if errors.Is(walkErr,os.ErrNotExist) { return nil }
			return walkErr
		}
		if filepath.Clean(path)==filepath.Clean(w.root) {
			if d.IsDir() { return w.native.Add(path) }
			return nil
		}

		rel,ok:=NormalizeRelativePath(w.root,path)
		if !ok {
			if d.IsDir() { return filepath.SkipDir }
			return nil
		}
		if ShouldIgnoreRelativePath(rel) {
			if d.IsDir() { return filepath.SkipDir }
			return nil
		}

		info,err:=d.Info()
		if err!=nil {
			if errors.Is(err,os.ErrNotExist) { return nil }
			return err
		}
		id,_:=fileIdentity(path)
		w.rememberPath(rel,path,info,id)

		if info.IsDir() {
			if err:=w.native.Add(path); err!=nil { return err }
			if queueNew {
				if _,changed,err:=w.journal.QueueLocalChange(LocalChange{
					Kind:SyncOpCreateFolder,
					LocalPath:rel,
					ObservedAt:time.Now().UTC(),
				}); err!=nil {
					return err
				} else if changed {
					log.Printf("journaled local folder create: %s",rel)
				}
			}
			return nil
		}

		if queueNew && !IsTransientWriteName(rel) {
			w.scheduleWrite(rel)
		}
		return nil
	})
}

func (w *filesystemWatcher) rememberPath(rel,absolute string,info os.FileInfo,id string) {
	w.entries[pathKey(rel)]=watcherEntry{
		ID:id,
		IsDir:info.IsDir(),
		Size:info.Size(),
		Modified:info.ModTime().UnixNano(),
	}
}

func (w *filesystemWatcher) scheduleWrite(rel string) {
	key:=pathKey(rel)
	w.writes[key]=pendingWrite{
		Path:rel,
		Due:time.Now().Add(DefaultWriteDebounce),
	}
}

func (w *filesystemWatcher) flushWrites(now time.Time) {
	for key,pending:=range w.writes {
		if now.Before(pending.Due) { continue }
		if w.journal.IsLocalPathSuppressed(pending.Path) {
			delete(w.writes,key)
			continue
		}

		absolute:=filepath.Join(w.root,pending.Path)
		info,err:=os.Stat(absolute)
		if err!=nil {
			if errors.Is(err,os.ErrNotExist) {
				delete(w.writes,key)
			}
			continue
		}
		if info.IsDir() {
			delete(w.writes,key)
			continue
		}
		if IsTransientWriteName(pending.Path) {
			delete(w.writes,key)
			continue
		}

		size:=info.Size()
		modified:=info.ModTime().UnixNano()
		if !pending.HasSample {
			pending.HasSample=true
			pending.SampleSize=size
			pending.SampleMod=modified
			pending.Due=now.Add(DefaultStableWindow)
			w.writes[key]=pending
			continue
		}
		if pending.SampleSize!=size || pending.SampleMod!=modified {
			pending.SampleSize=size
			pending.SampleMod=modified
			pending.Due=now.Add(DefaultStableWindow)
			w.writes[key]=pending
			continue
		}

		id,_:=fileIdentity(absolute)
		w.rememberPath(pending.Path,absolute,info,id)
		op,changed,queueErr:=w.journal.QueueLocalChange(LocalChange{
			Kind:SyncOpUpsertFile,
			LocalPath:pending.Path,
			ObservedAt:now.UTC(),
		})
		if queueErr!=nil {
			log.Printf("journal local file change failed for %s: %v",pending.Path,queueErr)
			pending.Due=now.Add(time.Second)
			w.writes[key]=pending
			continue
		}
		delete(w.writes,key)
		if changed {
			log.Printf("journaled local file upsert: %s",pending.Path)
			if op.ID!="" && w.onQueued!=nil { w.onQueued(op) }
		}
	}
}

func (w *filesystemWatcher) flushRemovals(now time.Time) {
	for key,removal:=range w.removals {
		if now.Before(removal.Due) { continue }
		if w.journal.IsLocalPathSuppressed(removal.Path) {
			delete(w.removals,key)
			w.cancelWritesUnder(removal.Path)
			continue
		}

		op,changed,err:=w.journal.QueueLocalChange(LocalChange{
			Kind:SyncOpDelete,
			LocalPath:removal.Path,
			ObservedAt:now.UTC(),
		})
		if err!=nil {
			log.Printf("journal local delete failed for %s: %v",removal.Path,err)
			removal.Due=now.Add(time.Second)
			w.removals[key]=removal
			continue
		}
		delete(w.removals,key)
		w.cancelWritesUnder(removal.Path)
		w.removeSnapshot(removal.Path)
		if changed {
			if op.ID!="" {
				log.Printf("journaled local delete: %s",removal.Path)
				if w.onQueued!=nil { w.onQueued(op) }
			} else {
				log.Printf("coalesced unsynced local delete: %s",removal.Path)
			}
		}
	}
}

func (w *filesystemWatcher) removalForID(id string) (string,pendingRemoval,bool) {
	if strings.TrimSpace(id)=="" { return "",pendingRemoval{},false }
	key:="id:"+id
	removal,ok:=w.removals[key]
	return key,removal,ok
}

func (w *filesystemWatcher) underPendingRemoval(rel string) bool {
	for _,removal:=range w.removals {
		if journalPathEqual(rel,removal.Path) || journalPathWithin(rel,removal.Path) {
			return true
		}
	}
	return false
}

func (w *filesystemWatcher) cancelRemovalsUnder(parent string) {
	for key,removal:=range w.removals {
		if journalPathEqual(removal.Path,parent) || journalPathWithin(removal.Path,parent) {
			delete(w.removals,key)
		}
	}
}

func (w *filesystemWatcher) cancelWritesUnder(parent string) {
	for key,pending:=range w.writes {
		if journalPathEqual(pending.Path,parent) || journalPathWithin(pending.Path,parent) {
			delete(w.writes,key)
		}
	}
}

func (w *filesystemWatcher) remapWrites(oldPrefix,newPrefix string) {
	next:=make(map[string]pendingWrite,len(w.writes))
	for _,pending:=range w.writes {
		if journalPathEqual(pending.Path,oldPrefix) || journalPathWithin(pending.Path,oldPrefix) {
			pending.Path=replaceJournalPrefix(pending.Path,oldPrefix,newPrefix)
		}
		next[pathKey(pending.Path)]=pending
	}
	w.writes=next
}

func (w *filesystemWatcher) removeSnapshot(parent string) {
	for key:=range w.entries {
		path:=key
		if journalPathEqual(path,parent) || journalPathWithin(path,parent) {
			delete(w.entries,key)
		}
	}
}

func pathKey(path string) string {
	path=filepath.Clean(path)
	if runtime.GOOS=="windows" { return strings.ToLower(path) }
	return path
}
