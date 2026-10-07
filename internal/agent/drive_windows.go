//go:build windows

package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var driveNamePattern = regexp.MustCompile(`^[^\\/:*?"<>|\x00-\x1F]{1,32}$`)

func NormalizeDriveLetter(value string) (string,error) {
	value=strings.ToUpper(strings.TrimSpace(value))
	value=strings.TrimSuffix(value,":")
	if len(value)!=1 || value[0]<'A' || value[0]>'Z' {
		return "",errors.New("drive letter must be a single letter from A to Z")
	}
	return value,nil
}

func ValidateDriveName(value string) (string,error) {
	value=strings.TrimSpace(value)
	if value=="" { return "",errors.New("drive name cannot be empty") }
	if !driveNamePattern.MatchString(value) {
		return "",errors.New("drive name must be 1-32 characters and cannot contain Windows reserved characters")
	}
	return value,nil
}

func DriveLetterAvailable(letter string) (bool,error) {
	letter,err:=NormalizeDriveLetter(letter)
	if err!=nil { return false,err }
	if _,ok:=currentSubstTarget(letter); ok {
		return false,nil
	}
	_,err=os.Stat(letter+":\\")
	switch {
	case err==nil:
		return false,nil
	case errors.Is(err,os.ErrNotExist):
		return true,nil
	default:
		return false,err
	}
}

func DriveLetterAvailableOrOwned(letter,backingRoot string) (bool,error) {
	letter,err:=NormalizeDriveLetter(letter)
	if err!=nil { return false,err }
	if current,ok:=currentSubstTarget(letter); ok {
		return samePath(current,backingRoot),nil
	}
	return DriveLetterAvailable(letter)
}

func PreferredDriveLetter() string {
	for _,letter:=range []string{"S","T","U","V","W","X","Y","Z","R","Q","P"} {
		available,err:=DriveLetterAvailable(letter)
		if err==nil && available { return letter }
	}
	return ""
}

func EnsureDriveMapping(letter,name,backingRoot string) error {
	letter,err:=NormalizeDriveLetter(letter)
	if err!=nil { return err }
	name,err=ValidateDriveName(name)
	if err!=nil { return err }
	backingRoot,err=filepath.Abs(strings.TrimSpace(backingRoot))
	if err!=nil { return err }
	if backingRoot=="" { return errors.New("drive backing root is empty") }

	current,ok:=currentSubstTarget(letter)
	if ok {
		if samePath(current,backingRoot) {
			return setDriveLabel(letter,name)
		}
		return fmt.Errorf("drive %s: is already mapped to %s",letter,current)
	}
	if _,err:=os.Stat(letter+":\\"); err==nil {
		return fmt.Errorf("drive %s: is already in use",letter)
	} else if !errors.Is(err,os.ErrNotExist) {
		return fmt.Errorf("check drive %s: %w",letter,err)
	}

	out,err:=exec.Command("subst",letter+":",backingRoot).CombinedOutput()
	if err!=nil {
		return fmt.Errorf("map drive %s: %w: %s",letter,err,strings.TrimSpace(string(out)))
	}
	if err:=setDriveLabel(letter,name); err!=nil {
		_ = exec.Command("subst",letter+":","/D").Run()
		return err
	}
	return nil
}

func ReleaseDriveMapping(letter,backingRoot string) error {
	letter,err:=NormalizeDriveLetter(letter)
	if err!=nil { return err }
	backingRoot,err=filepath.Abs(strings.TrimSpace(backingRoot))
	if err!=nil { return err }

	current,ok:=currentSubstTarget(letter)
	if !ok || !samePath(current,backingRoot) {
		return nil
	}

	out,err:=exec.Command("subst",letter+":","/D").CombinedOutput()
	if err!=nil {
		return fmt.Errorf("unmap drive %s: %w: %s",letter,err,strings.TrimSpace(string(out)))
	}

	key:=`HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\DriveIcons\`+letter
	_,_ = exec.Command("reg","delete",key,"/f").CombinedOutput()
	return nil
}

func currentSubstTarget(letter string) (string,bool) {
	letter,err:=NormalizeDriveLetter(letter)
	if err!=nil { return "",false }

	out,err:=exec.Command("subst").CombinedOutput()
	if err!=nil { return "",false }

	for _,line:=range strings.Split(string(out),"\n") {
		line=strings.TrimSpace(line)
		if line=="" { continue }

		parts:=strings.SplitN(line,"=>",2)
		if len(parts)!=2 { continue }

		left:=strings.ToUpper(strings.TrimSpace(parts[0]))
		left=strings.TrimSuffix(left,"\\")
		left=strings.TrimSuffix(left,":")
		left=strings.TrimSuffix(left,"\\")
		left=strings.TrimSuffix(left,":")
		if left!=letter { continue }

		target:=strings.TrimSpace(parts[1])
		if target=="" { return "",false }
		return filepath.Clean(target),true
	}
	return "",false
}

func samePath(a,b string) bool {
	a=filepath.Clean(strings.TrimSpace(a))
	b=filepath.Clean(strings.TrimSpace(b))
	return strings.EqualFold(a,b)
}

func setDriveLabel(letter,name string) error {
	key:=`HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\DriveIcons\`+letter+`\DefaultLabel`
	out,err:=exec.Command("reg","add",key,"/ve","/t","REG_SZ","/d",name,"/f").CombinedOutput()
	if err!=nil {
		return fmt.Errorf("set drive label: %w: %s",err,strings.TrimSpace(string(out)))
	}
	return nil
}
