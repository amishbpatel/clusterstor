package agent

import (
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultWriteDebounce = 2 * time.Second
	DefaultRenameGrace   = 3 * time.Second
	DefaultStableWindow  = 1 * time.Second
)

type SyncOperationKind string

const (
	SyncOpCreateFolder SyncOperationKind = "create_folder"
	SyncOpUpsertFile   SyncOperationKind = "upsert_file"
	SyncOpMove         SyncOperationKind = "move"
	SyncOpDelete       SyncOperationKind = "delete"
)

type LocalChange struct {
	Kind        SyncOperationKind `json:"kind"`
	LocalPath   string            `json:"local_path"`
	OldLocalPath string           `json:"old_local_path,omitempty"`
	ObservedAt  time.Time         `json:"observed_at"`
}

func NormalizeRelativePath(root,path string) (string,bool) {
	root=filepath.Clean(strings.TrimSpace(root))
	path=filepath.Clean(strings.TrimSpace(path))
	if root=="" || path=="" { return "",false }

	rel,err:=filepath.Rel(root,path)
	if err!=nil { return "",false }
	if rel=="." { return "",false }
	if rel==".." || strings.HasPrefix(rel,".."+string(filepath.Separator)) {
		return "",false
	}
	rel=filepath.Clean(rel)
	if ShouldIgnoreRelativePath(rel) { return "",false }
	return rel,true
}

func ShouldIgnoreRelativePath(rel string) bool {
	rel=filepath.Clean(strings.TrimSpace(rel))
	if rel=="" || rel=="." { return true }

	parts:=strings.FieldsFunc(rel,func(r rune) bool { return r=='/' || r=='\\' })
	for _,part:=range parts {
		lower:=strings.ToLower(part)
		switch lower {
		case "desktop.ini","thumbs.db",".ds_store","system volume information","$recycle.bin":
			return true
		}
		if strings.HasPrefix(lower,"~$") {
			// Office lock/owner files are transient and should not become cloud files.
			return true
		}
	}
	return false
}

func IsTransientWriteName(path string) bool {
	name:=strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	if name=="" { return false }

	if strings.HasPrefix(name,"~$") { return true }
	for _,suffix:=range []string{".crdownload",".download",".partial"} {
		if strings.HasSuffix(name,suffix) { return true }
	}
	return strings.HasSuffix(name,".tmp") && strings.HasPrefix(name,"~")
}
