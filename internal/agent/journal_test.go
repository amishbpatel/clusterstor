package agent

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestJournal(t *testing.T) *Journal {
	t.Helper()
	return &Journal{
		path:filepath.Join(t.TempDir(),"journal.json"),
		state:JournalState{
			SchemaVersion:journalSchemaVersion,
			DeviceID:"device-test",
			SyncRoot:t.TempDir(),
			Items:map[string]JournalItem{},
			Pending:[]PendingOperation{},
			UpdatedAt:time.Now().UTC(),
		},
	}
}

func TestJournalCoalescesRepeatedFileWrites(t *testing.T) {
	j:=newTestJournal(t)
	for i:=0;i<3;i++ {
		if _,_,err:=j.QueueLocalChange(LocalChange{
			Kind:SyncOpUpsertFile,
			LocalPath:"Docs/report.txt",
			ObservedAt:time.Now().UTC(),
		}); err!=nil { t.Fatal(err) }
	}
	state:=j.Snapshot()
	if len(state.Pending)!=1 { t.Fatalf("expected 1 pending operation, got %d",len(state.Pending)) }
	if state.Pending[0].Kind!=string(SyncOpUpsertFile) { t.Fatalf("unexpected operation: %#v",state.Pending[0]) }
}

func TestJournalDropsCreateThenDeleteBeforeRemoteSync(t *testing.T) {
	j:=newTestJournal(t)
	if _,_,err:=j.QueueLocalChange(LocalChange{
		Kind:SyncOpUpsertFile,
		LocalPath:"draft.txt",
	}); err!=nil { t.Fatal(err) }

	if _,_,err:=j.QueueLocalChange(LocalChange{
		Kind:SyncOpDelete,
		LocalPath:"draft.txt",
	}); err!=nil { t.Fatal(err) }

	if got:=len(j.Snapshot().Pending); got!=0 {
		t.Fatalf("expected no remote work after unsynced create+delete, got %d operations",got)
	}
}

func TestJournalRewritesUnsyncedRenameToFinalPath(t *testing.T) {
	j:=newTestJournal(t)
	if _,_,err:=j.QueueLocalChange(LocalChange{
		Kind:SyncOpUpsertFile,
		LocalPath:"old.txt",
	}); err!=nil { t.Fatal(err) }

	if _,_,err:=j.QueueLocalChange(LocalChange{
		Kind:SyncOpMove,
		OldLocalPath:"old.txt",
		LocalPath:"new.txt",
	}); err!=nil { t.Fatal(err) }

	state:=j.Snapshot()
	if len(state.Pending)!=1 { t.Fatalf("expected 1 pending operation, got %d",len(state.Pending)) }
	if state.Pending[0].Kind!=string(SyncOpUpsertFile) || state.Pending[0].LocalPath!="new.txt" {
		t.Fatalf("expected one upsert at final path, got %#v",state.Pending[0])
	}
}

func TestJournalKeepsMoveForKnownRemoteFile(t *testing.T) {
	j:=newTestJournal(t)
	j.state.Items["old.txt"]=JournalItem{
		LocalPath:"old.txt",
		NodeID:"node-1",
		ProviderItemID:"provider-1",
		State:"synced",
	}
	if _,_,err:=j.QueueLocalChange(LocalChange{
		Kind:SyncOpMove,
		OldLocalPath:"old.txt",
		LocalPath:"new.txt",
	}); err!=nil { t.Fatal(err) }

	state:=j.Snapshot()
	if len(state.Pending)!=1 { t.Fatalf("expected 1 pending operation, got %d",len(state.Pending)) }
	if state.Pending[0].Kind!=string(SyncOpMove) || state.Pending[0].OldLocalPath!="old.txt" || state.Pending[0].LocalPath!="new.txt" {
		t.Fatalf("unexpected move operation: %#v",state.Pending[0])
	}
}


func TestJournalCapturesBaseVersionForConflictCheck(t *testing.T) {
	j:=newTestJournal(t)
	modified:=time.Date(2026,10,7,12,0,0,0,time.UTC)
	j.state.Items["Docs/report.txt"]=JournalItem{
		LocalPath:"Docs/report.txt",
		NodeID:"node-7",
		ProviderItemID:"provider-7",
		VersionID:"version-7",
		ModifiedAt:&modified,
		State:"synced",
	}

	op,changed,err:=j.QueueLocalChange(LocalChange{
		Kind:SyncOpUpsertFile,
		LocalPath:"Docs/report.txt",
		ObservedAt:time.Now().UTC(),
	})
	if err!=nil { t.Fatal(err) }
	if !changed { t.Fatal("expected local operation to be queued") }
	if op.NodeID!="node-7" || op.BaseProviderItemID!="provider-7" || op.BaseVersionID!="version-7" {
		t.Fatalf("missing base identity/version metadata: %#v",op)
	}
	if op.BaseLocalPath!=filepath.Clean("Docs/report.txt") {
		t.Fatalf("unexpected base path %q",op.BaseLocalPath)
	}
	if op.BaseModifiedAt==nil || !op.BaseModifiedAt.Equal(modified) {
		t.Fatalf("unexpected base modified time %#v",op.BaseModifiedAt)
	}
}
