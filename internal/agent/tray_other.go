//go:build !windows

package agent

import "context"

func RunDesktopUI(ctx context.Context,cfg *Config,onExit func()) error {
	<-ctx.Done()
	return nil
}
