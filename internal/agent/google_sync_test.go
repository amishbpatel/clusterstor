package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveGoogleSnapshotBuildsNestedPaths(t *testing.T) {
	parent:="folder-1"
	snapshot:=GoogleSyncSnapshot{Items:[]GoogleSyncItem{
		{NodeID:"file-1",ProviderItemID:"provider-file",ParentNodeID:&parent,Name:"report.txt",NodeType:"file",Downloadable:true},
		{NodeID:"folder-1",ProviderItemID:"provider-folder",Name:"Docs",NodeType:"folder"},
	}}
	resolved:=resolveGoogleSnapshot(snapshot)
	if got:=resolved.PathsByNode["folder-1"]; got!="Docs" {
		t.Fatalf("unexpected folder path %q",got)
	}
	if got:=resolved.PathsByNode["file-1"]; got!=filepath.Join("Docs","report.txt") {
		t.Fatalf("unexpected file path %q",got)
	}
}

func TestResolveGoogleSnapshotLeavesMissingParentUnsupported(t *testing.T) {
	parent:="missing"
	resolved:=resolveGoogleSnapshot(GoogleSyncSnapshot{Items:[]GoogleSyncItem{
		{NodeID:"file-1",ParentNodeID:&parent,Name:"report.txt",NodeType:"file"},
	}})
	if len(resolved.Unsupported)!=1 {
		t.Fatalf("expected one unsupported unresolved item, got %d",len(resolved.Unsupported))
	}
}

func TestSyncOperationOrdering(t *testing.T) {
	if syncOperationRank(PendingOperation{Kind:string(SyncOpCreateFolder)}) >= syncOperationRank(PendingOperation{Kind:string(SyncOpUpsertFile)}) {
		t.Fatal("folder creates must run before file uploads")
	}
	if syncOperationRank(PendingOperation{Kind:string(SyncOpDelete)}) <= syncOperationRank(PendingOperation{Kind:string(SyncOpMove)}) {
		t.Fatal("deletes must run after moves")
	}
}

func TestAcknowledgeRebasesNewerPendingEdit(t *testing.T) {
	j:=newTestJournal(t)
	first,_,err:=j.QueueLocalChange(LocalChange{Kind:SyncOpUpsertFile,LocalPath:"report.txt",ObservedAt:time.Now().UTC()})
	if err!=nil { t.Fatal(err) }

	// Simulate a newer watcher event replacing the in-flight operation.
	second,_,err:=j.QueueLocalChange(LocalChange{Kind:SyncOpUpsertFile,LocalPath:"report.txt",ObservedAt:time.Now().UTC().Add(time.Second)})
	if err!=nil { t.Fatal(err) }
	if first.ID==second.ID { t.Fatal("expected newer operation id") }

	modified:=time.Now().UTC()
	if err:=j.AcknowledgeOperation(first.ID,"report.txt",JournalItem{
		LocalPath:"report.txt",
		NodeID:"node-1",
		Provider:"google_drive",
		ProviderItemID:"provider-1",
		VersionID:"revision-2",
		ModifiedAt:&modified,
		State:"synced",
		RemoteVerified:true,
	}); err!=nil { t.Fatal(err) }

	pending:=j.PendingOperations()
	if len(pending)!=1 { t.Fatalf("expected newer edit to remain pending, got %d",len(pending)) }
	if pending[0].ID!=second.ID || pending[0].BaseVersionID!="revision-2" || pending[0].NodeID!="node-1" {
		t.Fatalf("newer edit was not rebased: %#v",pending[0])
	}
}

func TestWatcherSuppressionDropsClusterStorAppliedEvent(t *testing.T) {
	j:=newTestJournal(t)
	j.SuppressLocalPath(filepath.Join("Docs","remote.txt"),time.Minute)
	_,changed,err:=j.QueueLocalChange(LocalChange{
		Kind:SyncOpUpsertFile,
		LocalPath:filepath.Join("Docs","remote.txt"),
		ObservedAt:time.Now().UTC(),
	})
	if err!=nil { t.Fatal(err) }
	if changed { t.Fatal("expected ClusterStor-applied event to be suppressed") }
	if len(j.PendingOperations())!=0 { t.Fatal("suppressed event must not enter pending queue") }
}

func TestUniqueConflictRelativePathPreservesExtension(t *testing.T) {
	root:=t.TempDir()
	existing:=filepath.Join(root,"Report (PC conflict 2026-10-07 101500).docx")
	if err:=os.WriteFile(existing,[]byte("x"),0600); err!=nil { t.Fatal(err) }
	got:=uniqueConflictRelativePath(root,filepath.Base(existing))
	if got==filepath.Base(existing) { t.Fatal("expected a non-colliding path") }
	if filepath.Ext(got)!=".docx" { t.Fatalf("extension changed: %q",got) }
}


func TestResolveGoogleSnapshotKeepsDuplicateGoogleNames(t *testing.T) {
	snapshot:=GoogleSyncSnapshot{Items:[]GoogleSyncItem{
		{NodeID:"file-a",ProviderItemID:"aaa111",Name:"same.txt",NodeType:"file",Downloadable:true},
		{NodeID:"file-b",ProviderItemID:"bbb222",Name:"same.txt",NodeType:"file",Downloadable:true},
	}}
	resolved:=resolveGoogleSnapshot(snapshot)
	a:=resolved.PathsByNode["file-a"]
	b:=resolved.PathsByNode["file-b"]
	if a=="" || b=="" || a==b {
		t.Fatalf("expected distinct local paths for duplicate Drive names: %q %q",a,b)
	}
	if filepath.Ext(a)!=".txt" || filepath.Ext(b)!=".txt" {
		t.Fatalf("duplicate mapping changed extensions: %q %q",a,b)
	}
}


func TestSuppressedPathDoesNotQueueWatcherWrite(t *testing.T) {
	root:=t.TempDir()
	j:=newTestJournal(t)
	j.state.SyncRoot=root
	path:=filepath.Join(root,"conflict.txt")
	if err:=os.WriteFile(path,[]byte("internal sync"),0600); err!=nil { t.Fatal(err) }

	w,err:=StartFilesystemWatcher(root,j,nil)
	if err!=nil { t.Fatal(err) }
	defer w.native.Close()

	j.SuppressLocalPath("conflict.txt",time.Minute)
	w.scheduleWrite("conflict.txt")
	w.flushWrites(time.Now().Add(10*time.Second))

	if got:=len(j.Snapshot().Pending); got!=0 {
		t.Fatalf("expected suppressed internal sync write to be ignored, got %d pending operations",got)
	}
}
