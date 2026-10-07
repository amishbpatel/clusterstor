package agent

import (
	"path/filepath"
	"testing"
)

func TestAutomaticKeepsLocalWhenSpaceHealthy(t *testing.T) {
	decision:=DecideLocalStorage(LocalStoragePolicyInput{
		Mode:AvailabilityAutomatic,
		Scope:SyncScopeIncluded,
		State:LocalContentResident,
		RemoteVerified:true,
		DiskPressure:false,
	})
	if decision.Action!=LocalStorageKeepLocal {
		t.Fatalf("expected resident content to stay local, got %#v",decision)
	}
}

func TestAutomaticDehydratesOnlyVerifiedContentUnderPressure(t *testing.T) {
	decision:=DecideLocalStorage(LocalStoragePolicyInput{
		Mode:AvailabilityAutomatic,
		Scope:SyncScopeIncluded,
		State:LocalContentResident,
		RemoteVerified:true,
		DiskPressure:true,
	})
	if decision.Action!=LocalStorageDehydrate {
		t.Fatalf("expected verified content to be eligible for dehydration, got %#v",decision)
	}

	decision=DecideLocalStorage(LocalStoragePolicyInput{
		Mode:AvailabilityAutomatic,
		Scope:SyncScopeIncluded,
		State:LocalContentResident,
		RemoteVerified:false,
		DiskPressure:true,
	})
	if decision.Action!=LocalStorageKeepLocal {
		t.Fatalf("expected unverified content to remain local, got %#v",decision)
	}
}

func TestPendingOrConflictContentNeverDehydrates(t *testing.T) {
	for _,input:=range []LocalStoragePolicyInput{
		{
			Mode:AvailabilityOnlineOnly,
			Scope:SyncScopeIncluded,
			State:LocalContentResident,
			RemoteVerified:true,
			PendingLocalChange:true,
		},
		{
			Mode:AvailabilityOnlineOnly,
			Scope:SyncScopeIncluded,
			State:LocalContentResident,
			RemoteVerified:true,
			Conflict:true,
		},
	} {
		decision:=DecideLocalStorage(input)
		if decision.Action!=LocalStorageKeepLocal {
			t.Fatalf("expected protected content to remain local, got %#v",decision)
		}
	}
}

func TestAlwaysLocalHydratesPlaceholder(t *testing.T) {
	decision:=DecideLocalStorage(LocalStoragePolicyInput{
		Mode:AvailabilityAlwaysLocal,
		Scope:SyncScopeIncluded,
		State:LocalContentPlaceholder,
		RemoteVerified:true,
	})
	if decision.Action!=LocalStorageHydrate {
		t.Fatalf("expected always-local placeholder to hydrate, got %#v",decision)
	}
}

func TestOpeningPlaceholderHydrates(t *testing.T) {
	decision:=DecideLocalStorage(LocalStoragePolicyInput{
		Mode:AvailabilityOnlineOnly,
		Scope:SyncScopeIncluded,
		State:LocalContentPlaceholder,
		RemoteVerified:true,
		RequestedOpen:true,
	})
	if decision.Action!=LocalStorageHydrate {
		t.Fatalf("expected opened placeholder to hydrate, got %#v",decision)
	}
}

func TestExcludedPendingWorkStaysVisibleUntilSafe(t *testing.T) {
	decision:=DecideLocalStorage(LocalStoragePolicyInput{
		Mode:AvailabilityAutomatic,
		Scope:SyncScopeExcluded,
		State:LocalContentResident,
		PendingLocalChange:true,
	})
	if decision.Action!=LocalStorageKeepLocal {
		t.Fatalf("expected excluded pending work to remain protected, got %#v",decision)
	}

	decision=DecideLocalStorage(LocalStoragePolicyInput{
		Mode:AvailabilityAutomatic,
		Scope:SyncScopeExcluded,
		State:LocalContentResident,
		RemoteVerified:true,
	})
	if decision.Action!=LocalStorageHide {
		t.Fatalf("expected safe excluded content to hide on this device, got %#v",decision)
	}
}

func TestNestedAvailabilityRuleUsesMostSpecificPath(t *testing.T) {
	rules:=[]AvailabilityRule{
		{Path:"Projects",Mode:AvailabilityAlwaysLocal,Scope:SyncScopeIncluded},
		{Path:filepath.Join("Projects","Archive"),Mode:AvailabilityOnlineOnly,Scope:SyncScopeIncluded},
	}
	got:=ResolveAvailabilityRule(filepath.Join("Projects","Archive","old.zip"),AvailabilityAutomatic,rules)
	if got.Mode!=AvailabilityOnlineOnly {
		t.Fatalf("expected nested rule to win, got %#v",got)
	}

	got=ResolveAvailabilityRule(filepath.Join("Projects","current.txt"),AvailabilityAutomatic,rules)
	if got.Mode!=AvailabilityAlwaysLocal {
		t.Fatalf("expected parent rule, got %#v",got)
	}
}

func TestDiskPressureUsesLargerReserve(t *testing.T) {
	const total int64=500*1024*1024*1024
	const fortyGB int64=40*1024*1024*1024
	const sixtyGB int64=60*1024*1024*1024

	if !IsDiskPressure(total,fortyGB,10*1024*1024*1024,10) {
		t.Fatal("expected pressure below 10 percent reserve")
	}
	if IsDiskPressure(total,sixtyGB,10*1024*1024*1024,10) {
		t.Fatal("did not expect pressure above 10 percent reserve")
	}
}

func TestLocalStorageDefaults(t *testing.T) {
	cfg:=Config{}
	ApplyLocalStorageDefaults(&cfg)
	if cfg.DefaultAvailability!=AvailabilityAutomatic {
		t.Fatalf("expected automatic default, got %q",cfg.DefaultAvailability)
	}
	if cfg.FreeSpaceReserveBytes!=DefaultFreeSpaceReserveBytes {
		t.Fatalf("unexpected reserve bytes %d",cfg.FreeSpaceReserveBytes)
	}
	if cfg.FreeSpaceReservePercent!=DefaultFreeSpaceReservePercent {
		t.Fatalf("unexpected reserve percent %d",cfg.FreeSpaceReservePercent)
	}
}
