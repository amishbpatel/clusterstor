//go:build windows

package agent

import "os/exec"

func OpenBrowser(url string) error {
	return exec.Command("rundll32","url.dll,FileProtocolHandler",url).Start()
}
