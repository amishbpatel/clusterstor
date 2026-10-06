//go:build !windows

package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func SaveDeviceSecret(secret string) error {
	secret=strings.TrimSpace(secret)
	if secret=="" { return errors.New("device secret is empty") }
	dir,err:=StateDir()
	if err!=nil { return err }
	if err:=os.MkdirAll(dir,0700); err!=nil { return err }
	return os.WriteFile(filepath.Join(dir,"device.secret"),[]byte(secret),0600)
}

func LoadDeviceSecret() (string,error) {
	dir,err:=StateDir()
	if err!=nil { return "",err }
	body,err:=os.ReadFile(filepath.Join(dir,"device.secret"))
	if err!=nil { return "",err }
	secret:=strings.TrimSpace(string(body))
	if secret=="" { return "",errors.New("device secret is empty") }
	return secret,nil
}
