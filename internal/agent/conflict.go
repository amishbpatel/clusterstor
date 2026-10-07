package agent

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type ConflictAction string

const (
	ConflictApplyLocal       ConflictAction = "apply_local"
	ConflictKeepRemote       ConflictAction = "keep_remote"
	ConflictPreserveBoth     ConflictAction = "preserve_both"
	ConflictRecoverLocalCopy ConflictAction = "recover_local_copy"
	ConflictNoop             ConflictAction = "noop"
)

type RemoteSnapshot struct {
	Exists         bool       `json:"exists"`
	NodeID         string     `json:"node_id,omitempty"`
	ProviderItemID string     `json:"provider_item_id,omitempty"`
	VersionID      string     `json:"version_id,omitempty"`
	LocalPath      string     `json:"local_path,omitempty"`
	ModifiedAt     *time.Time `json:"modified_at,omitempty"`
}

type ConflictDecision struct {
	Action       ConflictAction `json:"action"`
	Conflict     bool           `json:"conflict"`
	ConflictPath string         `json:"conflict_path,omitempty"`
	Reason       string         `json:"reason"`
}

func DecideConflict(op PendingOperation,remote RemoteSnapshot,deviceName string,now time.Time) ConflictDecision {
	if now.IsZero() { now=time.Now() }

	knownBase:=strings.TrimSpace(op.NodeID)!="" ||
		strings.TrimSpace(op.BaseProviderItemID)!="" ||
		strings.TrimSpace(op.BaseVersionID)!=""

	if !knownBase {
		if !remote.Exists {
			return ConflictDecision{Action:ConflictApplyLocal,Reason:"new local item has no remote collision"}
		}
		return ConflictDecision{
			Action:ConflictPreserveBoth,
			Conflict:true,
			ConflictPath:ConflictCopyPath(op.LocalPath,deviceName,now),
			Reason:"new local item collides with an existing remote item",
		}
	}

	if !remote.Exists {
		if op.Kind==string(SyncOpDelete) {
			return ConflictDecision{Action:ConflictNoop,Reason:"remote item is already deleted"}
		}
		return ConflictDecision{
			Action:ConflictRecoverLocalCopy,
			Conflict:true,
			ConflictPath:ConflictCopyPath(op.LocalPath,deviceName,now),
			Reason:"remote item was deleted after the local base version",
		}
	}

	if baseID:=strings.TrimSpace(op.BaseProviderItemID); baseID!="" &&
		strings.TrimSpace(remote.ProviderItemID)!="" &&
		baseID!=strings.TrimSpace(remote.ProviderItemID) {
		return ConflictDecision{
			Action:ConflictPreserveBoth,
			Conflict:true,
			ConflictPath:ConflictCopyPath(op.LocalPath,deviceName,now),
			Reason:"remote path now refers to a different provider item",
		}
	}

	remoteContentChanged:=false
	if baseVersion:=strings.TrimSpace(op.BaseVersionID); baseVersion!="" &&
		strings.TrimSpace(remote.VersionID)!="" {
		remoteContentChanged=baseVersion!=strings.TrimSpace(remote.VersionID)
	} else if op.BaseModifiedAt!=nil && remote.ModifiedAt!=nil {
		remoteContentChanged=!op.BaseModifiedAt.Equal(*remote.ModifiedAt)
	}

	basePath:=strings.TrimSpace(op.BaseLocalPath)
	if basePath=="" {
		if strings.TrimSpace(op.OldLocalPath)!="" { basePath=op.OldLocalPath } else { basePath=op.LocalPath }
	}
	remotePathChanged:=strings.TrimSpace(remote.LocalPath)!="" && !journalPathEqual(basePath,remote.LocalPath)

	switch SyncOperationKind(op.Kind) {
	case SyncOpUpsertFile:
		if remoteContentChanged {
			return ConflictDecision{
				Action:ConflictPreserveBoth,
				Conflict:true,
				ConflictPath:ConflictCopyPath(op.LocalPath,deviceName,now),
				Reason:"local and remote file content both changed from the same base",
			}
		}
		// A remote rename with unchanged content is safe: the future worker can
		// upload local content to the same provider item at its remote path.
		return ConflictDecision{Action:ConflictApplyLocal,Reason:"remote content is unchanged from the local base"}

	case SyncOpMove:
		if remotePathChanged {
			return ConflictDecision{
				Action:ConflictKeepRemote,
				Conflict:true,
				Reason:"local and remote locations both changed from the same base",
			}
		}
		// A remote content edit does not conflict with a local move because the
		// move changes only metadata/path and can preserve the new remote bytes.
		return ConflictDecision{Action:ConflictApplyLocal,Reason:"local move does not overwrite remote content"}

	case SyncOpDelete:
		if remoteContentChanged || remotePathChanged {
			return ConflictDecision{
				Action:ConflictKeepRemote,
				Conflict:true,
				Reason:"remote item changed after the local base, so delete is suppressed",
			}
		}
		return ConflictDecision{Action:ConflictApplyLocal,Reason:"remote item is unchanged from the local base"}

	case SyncOpCreateFolder:
		if remote.Exists {
			return ConflictDecision{
				Action:ConflictKeepRemote,
				Conflict:true,
				Reason:"folder create collides with an existing remote item",
			}
		}
		return ConflictDecision{Action:ConflictApplyLocal,Reason:"folder path is available remotely"}

	default:
		return ConflictDecision{Action:ConflictKeepRemote,Conflict:true,Reason:"unknown local operation is not safe to apply automatically"}
	}
}

var conflictDeviceCleaner=regexp.MustCompile(`[\\/:*?"<>|\x00-\x1F]+`)

func ConflictCopyPath(path,deviceName string,now time.Time) string {
	path=filepath.Clean(strings.TrimSpace(path))
	dir:=filepath.Dir(path)
	name:=filepath.Base(path)
	ext:=filepath.Ext(name)
	stem:=strings.TrimSuffix(name,ext)
	if stem=="" { stem=name; ext="" }

	device:=strings.TrimSpace(conflictDeviceCleaner.ReplaceAllString(deviceName,"-"))
	device=strings.Trim(device," .-")
	if device=="" { device="device" }
	if len(device)>40 { device=device[:40] }

	stamp:=now.Format("2006-01-02 150405")
	conflictName:=stem+" ("+device+" conflict "+stamp+")"+ext
	if dir=="." || dir=="" { return conflictName }
	return filepath.Join(dir,conflictName)
}
