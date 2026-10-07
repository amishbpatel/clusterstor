package agent

import (
	"path/filepath"
	"strings"
	"time"
)


const (
	DefaultFreeSpaceReserveBytes int64 = 10 * 1024 * 1024 * 1024
	DefaultFreeSpaceReservePercent = 10
)

func ApplyLocalStorageDefaults(cfg *Config) {
	if cfg==nil { return }
	cfg.DefaultAvailability=NormalizeAvailabilityMode(cfg.DefaultAvailability)
	if cfg.FreeSpaceReserveBytes<=0 {
		cfg.FreeSpaceReserveBytes=DefaultFreeSpaceReserveBytes
	}
	if cfg.FreeSpaceReservePercent<=0 || cfg.FreeSpaceReservePercent>90 {
		cfg.FreeSpaceReservePercent=DefaultFreeSpaceReservePercent
	}
	for i:=range cfg.AvailabilityRules {
		cfg.AvailabilityRules[i].Path=filepath.Clean(strings.TrimSpace(cfg.AvailabilityRules[i].Path))
		cfg.AvailabilityRules[i].Mode=NormalizeAvailabilityMode(cfg.AvailabilityRules[i].Mode)
		cfg.AvailabilityRules[i].Scope=NormalizeSyncScope(cfg.AvailabilityRules[i].Scope)
	}
}

func IsDiskPressure(totalBytes,freeBytes,reserveBytes int64,reservePercent int) bool {
	if totalBytes<=0 || freeBytes<0 { return false }
	if reserveBytes<=0 { reserveBytes=DefaultFreeSpaceReserveBytes }
	if reservePercent<=0 || reservePercent>90 { reservePercent=DefaultFreeSpaceReservePercent }

	percentFloor:=totalBytes*int64(reservePercent)/100
	floor:=reserveBytes
	if percentFloor>floor { floor=percentFloor }
	return freeBytes<floor
}

type AvailabilityMode string

const (
	AvailabilityAutomatic   AvailabilityMode = "automatic"
	AvailabilityAlwaysLocal AvailabilityMode = "always_local"
	AvailabilityOnlineOnly  AvailabilityMode = "online_only"
)

type SyncScope string

const (
	SyncScopeIncluded SyncScope = "included"
	SyncScopeExcluded SyncScope = "excluded"
)

type LocalContentState string

const (
	LocalContentResident    LocalContentState = "resident"
	LocalContentPlaceholder LocalContentState = "placeholder"
	LocalContentUnavailable LocalContentState = "unavailable"
)

type LocalStorageAction string

const (
	LocalStorageKeepLocal   LocalStorageAction = "keep_local"
	LocalStorageHydrate     LocalStorageAction = "hydrate"
	LocalStorageDehydrate   LocalStorageAction = "dehydrate"
	LocalStorageHide        LocalStorageAction = "hide"
	LocalStorageNoop        LocalStorageAction = "noop"
)

type LocalStoragePolicyInput struct {
	Path               string
	Mode               AvailabilityMode
	Scope              SyncScope
	State              LocalContentState
	RemoteVerified     bool
	PendingLocalChange bool
	Conflict           bool
	IsOpen             bool
	RequestedOpen      bool
	DiskPressure       bool
	LastAccessedAt     *time.Time
}

type LocalStorageDecision struct {
	Action LocalStorageAction
	Reason string
}

func NormalizeAvailabilityMode(value AvailabilityMode) AvailabilityMode {
	switch value {
	case AvailabilityAlwaysLocal, AvailabilityOnlineOnly:
		return value
	default:
		return AvailabilityAutomatic
	}
}

func NormalizeSyncScope(value SyncScope) SyncScope {
	if value==SyncScopeExcluded { return SyncScopeExcluded }
	return SyncScopeIncluded
}

func DecideLocalStorage(input LocalStoragePolicyInput) LocalStorageDecision {
	input.Mode=NormalizeAvailabilityMode(input.Mode)
	input.Scope=NormalizeSyncScope(input.Scope)

	if input.Scope==SyncScopeExcluded {
		if input.PendingLocalChange || input.Conflict {
			return LocalStorageDecision{
				Action:LocalStorageKeepLocal,
				Reason:"excluded content still has unsynchronized or conflicted local work",
			}
		}
		return LocalStorageDecision{
			Action:LocalStorageHide,
			Reason:"path is excluded from sync on this device",
		}
	}

	if input.RequestedOpen && input.State==LocalContentPlaceholder {
		return LocalStorageDecision{
			Action:LocalStorageHydrate,
			Reason:"user opened an online-only placeholder",
		}
	}

	if input.PendingLocalChange {
		return LocalStorageDecision{
			Action:LocalStorageKeepLocal,
			Reason:"pending local work must remain resident until remote sync succeeds",
		}
	}

	if input.Conflict {
		return LocalStorageDecision{
			Action:LocalStorageKeepLocal,
			Reason:"conflicted content must remain resident until both copies are preserved",
		}
	}

	if input.IsOpen {
		return LocalStorageDecision{
			Action:LocalStorageKeepLocal,
			Reason:"open files cannot be dehydrated",
		}
	}

	switch input.Mode {
	case AvailabilityAlwaysLocal:
		if input.State==LocalContentPlaceholder {
			return LocalStorageDecision{
				Action:LocalStorageHydrate,
				Reason:"always-available content must be resident on this device",
			}
		}
		return LocalStorageDecision{
			Action:LocalStorageKeepLocal,
			Reason:"always-available content stays resident on this device",
		}

	case AvailabilityOnlineOnly:
		if input.State==LocalContentResident {
			if !input.RemoteVerified {
				return LocalStorageDecision{
					Action:LocalStorageKeepLocal,
					Reason:"local bytes cannot be removed until the remote copy is verified",
				}
			}
			return LocalStorageDecision{
				Action:LocalStorageDehydrate,
				Reason:"online-only content has a verified remote copy",
			}
		}
		return LocalStorageDecision{
			Action:LocalStorageNoop,
			Reason:"online-only content is already non-resident",
		}

	default: // automatic
		if input.State==LocalContentPlaceholder {
			return LocalStorageDecision{
				Action:LocalStorageNoop,
				Reason:"automatic placeholder remains online-only until opened",
			}
		}
		if !input.DiskPressure {
			return LocalStorageDecision{
				Action:LocalStorageKeepLocal,
				Reason:"automatic mode keeps resident content while local space is healthy",
			}
		}
		if !input.RemoteVerified {
			return LocalStorageDecision{
				Action:LocalStorageKeepLocal,
				Reason:"automatic cleanup requires a verified remote copy",
			}
		}
		return LocalStorageDecision{
			Action:LocalStorageDehydrate,
			Reason:"automatic mode may reclaim verified resident bytes under disk pressure",
		}
	}
}

type AvailabilityRule struct {
	Path string `json:"path"`
	Mode AvailabilityMode `json:"mode"`
	Scope SyncScope `json:"scope"`
}

func ResolveAvailabilityRule(path string,defaultMode AvailabilityMode,rules []AvailabilityRule) AvailabilityRule {
	path=filepath.Clean(strings.TrimSpace(path))
	best:=AvailabilityRule{Mode:NormalizeAvailabilityMode(defaultMode),Scope:SyncScopeIncluded}
	bestLen:=-1

	for _,rule:=range rules {
		rulePath:=filepath.Clean(strings.TrimSpace(rule.Path))
		if rulePath=="" || rulePath=="." { continue }
		if !journalPathEqual(path,rulePath) && !journalPathWithin(path,rulePath) { continue }
		if len(rulePath)<=bestLen { continue }
		bestLen=len(rulePath)
		best=AvailabilityRule{
			Path:rulePath,
			Mode:NormalizeAvailabilityMode(rule.Mode),
			Scope:NormalizeSyncScope(rule.Scope),
		}
	}
	return best
}
