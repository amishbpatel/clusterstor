package agent

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func conflictBaseOperation(kind SyncOperationKind) PendingOperation {
	baseTime:=time.Date(2026,10,7,12,0,0,0,time.UTC)
	return PendingOperation{
		ID:"op-1",
		Kind:string(kind),
		LocalPath:filepath.Join("Docs","Report.docx"),
		OldLocalPath:filepath.Join("Docs","Report.docx"),
		NodeID:"node-1",
		BaseProviderItemID:"provider-1",
		BaseVersionID:"version-1",
		BaseLocalPath:filepath.Join("Docs","Report.docx"),
		BaseModifiedAt:&baseTime,
	}
}

func TestConflictUnchangedRemoteAppliesLocalEdit(t *testing.T) {
	op:=conflictBaseOperation(SyncOpUpsertFile)
	decision:=DecideConflict(op,RemoteSnapshot{
		Exists:true,
		ProviderItemID:"provider-1",
		VersionID:"version-1",
		LocalPath:op.BaseLocalPath,
	},"LAPTOP",time.Date(2026,10,7,9,30,0,0,time.Local))

	if decision.Action!=ConflictApplyLocal || decision.Conflict {
		t.Fatalf("expected safe local apply, got %#v",decision)
	}
}

func TestConflictConcurrentEditsPreserveBoth(t *testing.T) {
	op:=conflictBaseOperation(SyncOpUpsertFile)
	now:=time.Date(2026,10,7,9,30,15,0,time.Local)
	decision:=DecideConflict(op,RemoteSnapshot{
		Exists:true,
		ProviderItemID:"provider-1",
		VersionID:"version-2",
		LocalPath:op.BaseLocalPath,
	},"LAPTOP-SRNCLDLL",now)

	if decision.Action!=ConflictPreserveBoth || !decision.Conflict {
		t.Fatalf("expected preserve-both conflict, got %#v",decision)
	}
	want:=filepath.Join("Docs","Report (LAPTOP-SRNCLDLL conflict 2026-10-07 093015).docx")
	if decision.ConflictPath!=want {
		t.Fatalf("expected conflict path %q, got %q",want,decision.ConflictPath)
	}
}

func TestConflictDeleteSuppressedWhenRemoteChanged(t *testing.T) {
	op:=conflictBaseOperation(SyncOpDelete)
	decision:=DecideConflict(op,RemoteSnapshot{
		Exists:true,
		ProviderItemID:"provider-1",
		VersionID:"version-2",
		LocalPath:op.BaseLocalPath,
	},"LAPTOP",time.Now())

	if decision.Action!=ConflictKeepRemote || !decision.Conflict {
		t.Fatalf("expected remote-preserving delete conflict, got %#v",decision)
	}
}

func TestConflictRemoteDeleteRecoversLocalEdit(t *testing.T) {
	op:=conflictBaseOperation(SyncOpUpsertFile)
	decision:=DecideConflict(op,RemoteSnapshot{Exists:false},"LAPTOP",time.Date(2026,10,7,9,31,0,0,time.Local))

	if decision.Action!=ConflictRecoverLocalCopy || !decision.Conflict || decision.ConflictPath=="" {
		t.Fatalf("expected local recovery copy, got %#v",decision)
	}
}

func TestConflictLocalMoveCanPreserveRemoteContentEdit(t *testing.T) {
	op:=conflictBaseOperation(SyncOpMove)
	op.LocalPath=filepath.Join("Archive","Report.docx")
	decision:=DecideConflict(op,RemoteSnapshot{
		Exists:true,
		ProviderItemID:"provider-1",
		VersionID:"version-2",
		LocalPath:op.BaseLocalPath,
	},"LAPTOP",time.Now())

	if decision.Action!=ConflictApplyLocal || decision.Conflict {
		t.Fatalf("expected metadata-only local move to be safe, got %#v",decision)
	}
}

func TestConflictConcurrentMovesKeepRemoteLocation(t *testing.T) {
	op:=conflictBaseOperation(SyncOpMove)
	op.LocalPath=filepath.Join("Archive","Report.docx")
	decision:=DecideConflict(op,RemoteSnapshot{
		Exists:true,
		ProviderItemID:"provider-1",
		VersionID:"version-1",
		LocalPath:filepath.Join("Shared","Report.docx"),
	},"LAPTOP",time.Now())

	if decision.Action!=ConflictKeepRemote || !decision.Conflict {
		t.Fatalf("expected remote location to win concurrent move, got %#v",decision)
	}
}

func TestConflictNewLocalNameCollisionPreservesBoth(t *testing.T) {
	op:=PendingOperation{
		Kind:string(SyncOpUpsertFile),
		LocalPath:"notes.txt",
	}
	decision:=DecideConflict(op,RemoteSnapshot{
		Exists:true,
		ProviderItemID:"other-item",
		VersionID:"other-version",
		LocalPath:"notes.txt",
	},"DESKTOP",time.Now())

	if decision.Action!=ConflictPreserveBoth || !decision.Conflict {
		t.Fatalf("expected name collision conflict, got %#v",decision)
	}
}

func TestConflictCopyPathSanitizesDeviceAndPreservesExtension(t *testing.T) {
	got:=ConflictCopyPath(filepath.Join("Docs","Report.final.docx"),"PC:West/Desk*1",time.Date(2026,10,7,9,32,4,0,time.Local))
	if strings.ContainsAny(filepath.Base(got),":/*?\"<>|") {
		t.Fatalf("conflict filename contains Windows-reserved characters: %q",got)
	}
	if filepath.Ext(got)!=".docx" {
		t.Fatalf("expected .docx extension, got %q",got)
	}
}
