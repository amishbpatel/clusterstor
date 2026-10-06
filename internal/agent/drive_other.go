//go:build !windows

package agent

import "errors"

func NormalizeDriveLetter(string) (string,error) {
	return "",errors.New("drive letters are only supported on Windows")
}

func ValidateDriveName(value string) (string,error) {
	if value=="" { return "",errors.New("drive name cannot be empty") }
	return value,nil
}

func DriveLetterAvailable(string) (bool,error) { return false,nil }
func PreferredDriveLetter() string { return "" }
func EnsureDriveMapping(string,string,string) error { return nil }
