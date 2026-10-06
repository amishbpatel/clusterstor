package agent

import (
	"path/filepath"
	"testing"
)

func TestNormalizeRelativePath(t *testing.T) {
	root:=filepath.Join("C:","Users","test","ClusterStor")
	path:=filepath.Join(root,"Projects","Report.docx")
	got,ok:=NormalizeRelativePath(root,path)
	if !ok { t.Fatal("expected path inside root to be accepted") }
	if got!=filepath.Join("Projects","Report.docx") {
		t.Fatalf("unexpected relative path: %q",got)
	}
}

func TestNormalizeRelativePathRejectsOutsideRoot(t *testing.T) {
	root:=filepath.Join("C:","Users","test","ClusterStor")
	path:=filepath.Join("C:","Users","test","Elsewhere","Report.docx")
	if _,ok:=NormalizeRelativePath(root,path); ok {
		t.Fatal("expected path outside root to be rejected")
	}
}

func TestIgnoreWindowsMetadataAndOfficeLocks(t *testing.T) {
	for _,path:=range []string{
		"desktop.ini",
		filepath.Join("Folder","Thumbs.db"),
		filepath.Join("Folder","~$Report.docx"),
		filepath.Join("$RECYCLE.BIN","item"),
	} {
		if !ShouldIgnoreRelativePath(path) {
			t.Fatalf("expected %q to be ignored",path)
		}
	}
}

func TestNormalDotfilesAreNotIgnored(t *testing.T) {
	if ShouldIgnoreRelativePath(filepath.Join("Project",".env.example")) {
		t.Fatal("normal dotfiles should remain syncable")
	}
}

func TestTransientDownloadNames(t *testing.T) {
	for _,name:=range []string{"movie.crdownload","archive.download","big.partial","~abc.tmp"} {
		if !IsTransientWriteName(name) { t.Fatalf("expected %q transient",name) }
	}
	if IsTransientWriteName("notes.tmp") {
		t.Fatal("ordinary .tmp filename should not be ignored solely by extension")
	}
}
