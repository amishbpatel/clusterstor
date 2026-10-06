package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func DefaultSyncRoot() (string,error) {
	home,err:=os.UserHomeDir()
	if err!=nil { return "",err }
	home=strings.TrimSpace(home)
	if home=="" { return "",errors.New("user home directory is unavailable") }
	return filepath.Join(home,"ClusterStor"),nil
}

func EnsureSyncRoot(path string) (string,error) {
	path=strings.TrimSpace(path)
	if path=="" {
		var err error
		path,err=DefaultSyncRoot()
		if err!=nil { return "",err }
	}
	absolute,err:=filepath.Abs(path)
	if err!=nil { return "",err }
	if err:=os.MkdirAll(absolute,0700); err!=nil { return "",err }
	info,err:=os.Stat(absolute)
	if err!=nil { return "",err }
	if !info.IsDir() { return "",errors.New("ClusterStor sync root is not a directory") }
	return filepath.Clean(absolute),nil
}
