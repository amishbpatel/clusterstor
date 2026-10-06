package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func DefaultSyncRoot() (string,error) {
	dir,err:=StateDir()
	if err!=nil { return "",err }
	dir=strings.TrimSpace(dir)
	if dir=="" { return "",errors.New("ClusterStor state directory is unavailable") }
	return filepath.Join(dir,"DriveRoot"),nil
}


func UpgradeLegacySyncRoot(path string) (string,error) {
	path=strings.TrimSpace(path)
	if path=="" { return "",nil }

	home,err:=os.UserHomeDir()
	if err!=nil { return path,nil }
	legacy:=filepath.Join(home,"ClusterStor")
	if !strings.EqualFold(filepath.Clean(path),filepath.Clean(legacy)) {
		return path,nil
	}

	entries,err:=os.ReadDir(legacy)
	switch {
	case errors.Is(err,os.ErrNotExist):
		return "",nil
	case err!=nil:
		return path,err
	case len(entries)>0:
		// Never move or delete user data automatically.
		return path,nil
	default:
		if err:=os.Remove(legacy); err!=nil && !errors.Is(err,os.ErrNotExist) {
			return path,err
		}
		return "",nil
	}
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
