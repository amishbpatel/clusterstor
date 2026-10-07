//go:build windows

package agent

import (
	"syscall"
	"unsafe"
)

var (
	kernel32Instance = syscall.NewLazyDLL("kernel32.dll")
	createMutexW = kernel32Instance.NewProc("CreateMutexW")
	closeHandle = kernel32Instance.NewProc("CloseHandle")
)

const errorAlreadyExists syscall.Errno = 183

func AcquireSingleInstance() (func(),error) {
	name,err:=syscall.UTF16PtrFromString("Local\\ClusterStorDesktopAgent")
	if err!=nil { return nil,err }

	handle,_,callErr:=createMutexW.Call(0,0,uintptr(unsafe.Pointer(name)))
	if handle==0 { return nil,callErr }
	if callErr==errorAlreadyExists {
		closeHandle.Call(handle)
		return nil,ErrAlreadyRunning
	}
	return func(){ closeHandle.Call(handle) },nil
}
