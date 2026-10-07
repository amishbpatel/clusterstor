//go:build windows

package agent

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func fileIdentity(path string) (string,error) {
	f,err:=os.Open(path)
	if err!=nil { return "",err }
	defer f.Close()

	var info windows.ByHandleFileInformation
	if err:=windows.GetFileInformationByHandle(windows.Handle(f.Fd()),&info); err!=nil {
		return "",err
	}
	return fmt.Sprintf("%08x:%08x%08x",info.VolumeSerialNumber,info.FileIndexHigh,info.FileIndexLow),nil
}
