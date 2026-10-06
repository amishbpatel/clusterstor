//go:build windows

package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"

	"github.com/gogpu/systray"
)

const trayIconBase64 = "iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAAA30lEQVR4nO1XwQ3DMAgEK7O0s7UD1bPFy6SvRFFjA0eN00jlCYg7czjBRCcbtwK3x7L0BCqZq1gHZ29gjUgaCV7DSK3AKBJJShxhTOQ7/fw6+u5PrEbJzFMP4M8YQgSSQAL35EEEkKJIvkkCq977PKsMageQYVv9YTNgKY7eBJEAqrvHoA6gp+tOIML+BK5FIOJWiAQiph4iUDOtC2iXVALaN7/mR0iYFxKP/pqEJTObJUDnodvf0FMUIbvt6Ohe+O1OuL4P4J3QAybZJkHr6RRhe6zUCowAJ/qBx+np9gYJFk49eSuMFAAAAABJRU5ErkJggg=="

func RunDesktopUI(ctx context.Context,cfg *Config,onExit func()) error {
	if cfg==nil { return fmt.Errorf("desktop config is required") }

	tray:=systray.New()
	menu:=systray.NewMenu()

	statusLabel:="Sync status: Ready"
	if cfg.SyncPaused { statusLabel="Sync status: Paused" }
	status:=menu.Add(statusLabel,func(){})
	status.SetDisabled(true)

	menu.Add("Open "+cfg.DriveName+" ("+cfg.DriveLetter+":)",func(){
		_ = exec.Command("explorer.exe",cfg.DriveLetter+":\\").Start()
	})
	menu.AddSeparator()

	pauseLabel:="Pause sync"
	if cfg.SyncPaused { pauseLabel="Resume sync" }
	var pause *systray.MenuItem
	pause=menu.Add(pauseLabel,func(){
		cfg.SyncPaused=!cfg.SyncPaused
		if cfg.SyncPaused {
			pause.SetLabel("Resume sync")
			status.SetLabel("Sync status: Paused")
		} else {
			pause.SetLabel("Pause sync")
			status.SetLabel("Sync status: Ready")
		}
		_ = SaveConfig(*cfg)
	})

	menu.Add("Activity",func(){ _ = openWebPath(cfg.WebBaseURL,"/activity") })
	menu.Add("Providers",func(){ _ = openWebPath(cfg.WebBaseURL,"/providers") })
	menu.Add("Devices",func(){ _ = openWebPath(cfg.WebBaseURL,"/devices") })
	menu.AddSeparator()
	menu.Add("Settings",func(){ _ = openWebPath(cfg.WebBaseURL,"/settings") })
	menu.Add("Manage plan...",func(){ _ = openWebPath(cfg.WebBaseURL,"/settings#billing") })
	menu.AddSeparator()
	menu.Add("Exit ClusterStor",func(){
		if onExit!=nil { onExit() }
		tray.Remove()
	})

	icon,_:=base64.StdEncoding.DecodeString(trayIconBase64)
	tray.SetIcon(icon)
	tray.SetTooltip("ClusterStor — "+statusLabel)
	tray.SetMenu(menu)
	tray.OnDoubleClick(func(){
		_ = exec.Command("explorer.exe",cfg.DriveLetter+":\\").Start()
	})
	tray.Show()

	go func(){
		<-ctx.Done()
		tray.Remove()
	}()

	return tray.Run()
}

func openWebPath(base,path string) error {
	base=strings.TrimRight(strings.TrimSpace(base),"/")
	if base=="" { base="http://localhost:5173" }
	return OpenBrowser(base+path)
}
