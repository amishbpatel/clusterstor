//go:build !windows

package agent

import (
	"fmt"
	"os"
	"syscall"
)

func fileIdentity(path string) (string,error) {
	info,err:=os.Stat(path)
	if err!=nil { return "",err }
	stat,ok:=info.Sys().(*syscall.Stat_t)
	if !ok { return "",fmt.Errorf("file identity unavailable") }
	return fmt.Sprintf("%d:%d",uint64(stat.Dev),uint64(stat.Ino)),nil
}
