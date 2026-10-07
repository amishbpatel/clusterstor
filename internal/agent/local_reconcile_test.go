package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartupReconcileQueuesFileCreatedWhileAgentStopped(t *testing.T) {
	root:=t.TempDir()
	j:=newTestJournal(t)
	j.state.SyncRoot=root

	path:=filepath.Join(root,"offline.txt")
	if err:=os.WriteFile(path,[]byte("offline"),0600); err!=nil { t.Fatal(err) }

	count,err:=ReconcileLocalJournal(root,j)
	if err!=nil { t.Fatal(err) }
	if count!=1 { t.Fatalf("expected one queued change, got %d",count) }
	pending:=j.PendingOperations()
	if len(pending)!=1 || pending[0].Kind!=string(SyncOpUpsertFile) || pending[0].LocalPath!="offline.txt" {
		t.Fatalf("unexpected pending operations %#v",pending)
	}
}

func TestStartupReconcileQueuesEditAgainstKnownBase(t *testing.T) {
	root:=t.TempDir()
	j:=newTestJournal(t)
	j.state.SyncRoot=root

	path:=filepath.Join(root,"report.txt")
	if err:=os.WriteFile(path,[]byte("new bytes"),0600); err!=nil { t.Fatal(err) }
	old:=time.Now().Add(-time.Hour).UTC()
	j.state.Items["report.txt"]=JournalItem{
		LocalPath:"report.txt",
		NodeID:"node-1",
		Provider:"google_drive",
		ProviderItemID:"provider-1",
		VersionID:"rev-1",
		SizeBytes:3,
		ModifiedAt:&old,
		State:"synced",
		LocalContentState:LocalContentResident,
		RemoteVerified:true,
	}

	count,err:=ReconcileLocalJournal(root,j)
	if err!=nil { t.Fatal(err) }
	if count!=1 { t.Fatalf("expected one queued edit, got %d",count) }
	op:=j.PendingOperations()[0]
	if op.BaseVersionID!="rev-1" || op.NodeID!="node-1" {
		t.Fatalf("offline edit lost base metadata: %#v",op)
	}
}

func TestStartupReconcileQueuesDeleteWhileAgentStopped(t *testing.T) {
	root:=t.TempDir()
	j:=newTestJournal(t)
	j.state.SyncRoot=root
	now:=time.Now().UTC()
	j.state.Items["gone.txt"]=JournalItem{
		LocalPath:"gone.txt",
		NodeID:"node-1",
		Provider:"google_drive",
		ProviderItemID:"provider-1",
		VersionID:"rev-1",
		SizeBytes:10,
		ModifiedAt:&now,
		State:"synced",
		LocalContentState:LocalContentResident,
		RemoteVerified:true,
	}

	count,err:=ReconcileLocalJournal(root,j)
	if err!=nil { t.Fatal(err) }
	if count!=1 { t.Fatalf("expected one queued delete, got %d",count) }
	op:=j.PendingOperations()[0]
	if op.Kind!=string(SyncOpDelete) || op.LocalPath!="gone.txt" {
		t.Fatalf("unexpected delete operation %#v",op)
	}
}

func TestStartupReconcileDoesNotDeleteUnavailableRemoteOnlyItem(t *testing.T) {
	root:=t.TempDir()
	j:=newTestJournal(t)
	j.state.SyncRoot=root
	j.state.Items["Google Doc"]=JournalItem{
		LocalPath:"Google Doc",
		NodeID:"node-doc",
		Provider:"google_drive",
		ProviderItemID:"provider-doc",
		State:"synced",
		LocalContentState:LocalContentUnavailable,
		RemoteVerified:true,
	}

	count,err:=ReconcileLocalJournal(root,j)
	if err!=nil { t.Fatal(err) }
	if count!=0 { t.Fatalf("expected no queued deletion, got %d",count) }
	if len(j.PendingOperations())!=0 { t.Fatal("remote-only item must not be treated as a local deletion") }
}

func TestStartupReconcileIgnoresClusterStorTransferTemps(t *testing.T) {
	root:=t.TempDir()
	j:=newTestJournal(t)
	j.state.SyncRoot=root
	if err:=os.WriteFile(filepath.Join(root,"~clusterstor-download-123.tmp"),[]byte("partial"),0600); err!=nil {
		t.Fatal(err)
	}

	count,err:=ReconcileLocalJournal(root,j)
	if err!=nil { t.Fatal(err) }
	if count!=0 { t.Fatalf("temporary transfer should not queue sync work, got %d",count) }
}
