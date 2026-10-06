//go:build !windows

package agent

func OpenBrowser(string) error { return nil }
