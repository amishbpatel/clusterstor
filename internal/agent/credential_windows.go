//go:build windows

package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	crypt32 = syscall.NewLazyDLL("crypt32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	cryptProtectData = crypt32.NewProc("CryptProtectData")
	cryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	localFree = kernel32.NewProc("LocalFree")
)

func SaveDeviceSecret(secret string) error {
	secret=strings.TrimSpace(secret)
	if secret=="" { return errors.New("device secret is empty") }
	protected,err:=protectData([]byte(secret))
	if err!=nil { return err }
	dir,err:=StateDir()
	if err!=nil { return err }
	if err:=os.MkdirAll(dir,0700); err!=nil { return err }
	return os.WriteFile(filepath.Join(dir,"credential.bin"),protected,0600)
}

func LoadDeviceSecret() (string,error) {
	dir,err:=StateDir()
	if err!=nil { return "",err }
	protected,err:=os.ReadFile(filepath.Join(dir,"credential.bin"))
	if err!=nil { return "",err }
	plain,err:=unprotectData(protected)
	if err!=nil { return "",err }
	secret:=strings.TrimSpace(string(plain))
	if secret=="" { return "",errors.New("device secret is empty") }
	return secret,nil
}

func protectData(data []byte) ([]byte,error) {
	if len(data)==0 { return nil,errors.New("cannot protect empty data") }
	in:=dataBlob{cbData:uint32(len(data)),pbData:&data[0]}
	var out dataBlob
	ret,_,callErr:=cryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),0,0,0,0,0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret==0 { return nil,callErr }
	defer localFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	result:=make([]byte,out.cbData)
	copy(result,unsafe.Slice(out.pbData,int(out.cbData)))
	return result,nil
}

func unprotectData(data []byte) ([]byte,error) {
	if len(data)==0 { return nil,errors.New("cannot unprotect empty data") }
	in:=dataBlob{cbData:uint32(len(data)),pbData:&data[0]}
	var out dataBlob
	ret,_,callErr:=cryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),0,0,0,0,0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret==0 { return nil,callErr }
	defer localFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	result:=make([]byte,out.cbData)
	copy(result,unsafe.Slice(out.pbData,int(out.cbData)))
	return result,nil
}
