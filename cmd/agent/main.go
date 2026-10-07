package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/amishbpatel/clusterstor/internal/agent"
)

const agentVersion = "0.5.0-dev"

func main() {
	defaultAPI:=strings.TrimSpace(os.Getenv("CLUSTERSTOR_API_BASE_URL"))
	if defaultAPI=="" { defaultAPI="http://localhost:8080" }
	defaultWeb:=strings.TrimSpace(os.Getenv("CLUSTERSTOR_PUBLIC_BASE_URL"))
	if defaultWeb=="" { defaultWeb="http://localhost:5173" }

	apiBase:=flag.String("api",defaultAPI,"ClusterStor API base URL")
	webBase:=flag.String("web",defaultWeb,"ClusterStor web base URL")
	syncRootFlag:=flag.String("sync-root","","internal ClusterStor drive backing folder")
	driveNameFlag:=flag.String("drive-name","","Windows drive display name")
	driveLetterFlag:=flag.String("drive-letter","","Windows drive letter")
	once:=flag.Bool("once",false,"send one heartbeat and exit after registration")
	flag.Parse()

	releaseInstance,err:=agent.AcquireSingleInstance()
	if errors.Is(err,agent.ErrAlreadyRunning) {
		log.Fatal("ClusterStor desktop agent is already running")
	}
	if err!=nil { log.Fatalf("acquire desktop agent instance: %v",err) }
	defer releaseInstance()

	ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM)
	defer stop()

	cfg,err:=agent.LoadConfig()
	if err!=nil { log.Fatal(err) }
	if cfg.APIBaseURL=="" { cfg.APIBaseURL=*apiBase }
	if cfg.WebBaseURL=="" { cfg.WebBaseURL=*webBase }

	client:=agent.NewClient(cfg.APIBaseURL)

	if strings.TrimSpace(cfg.DeviceID)=="" {
		cfg,err=pairDevice(ctx,client,cfg)
		if err!=nil { log.Fatal(err) }
	}

	if strings.TrimSpace(*syncRootFlag)!="" {
		cfg.SyncRoot=*syncRootFlag
	} else if runtime.GOOS=="windows" {
		cfg.SyncRoot,err=agent.UpgradeLegacySyncRoot(cfg.SyncRoot)
		if err!=nil { log.Fatalf("upgrade legacy ClusterStor backing folder: %v",err) }
		if strings.TrimSpace(cfg.SyncRoot)=="" {
			cfg.SyncRoot,err=agent.DefaultSyncRoot()
			if err!=nil { log.Fatalf("resolve ClusterStor backing folder: %v",err) }
		}
	}
	if strings.TrimSpace(*driveNameFlag)!="" {
		cfg.DriveName=*driveNameFlag
	}
	if strings.TrimSpace(*driveLetterFlag)!="" {
		cfg.DriveLetter=*driveLetterFlag
	}

	if runtime.GOOS=="windows" && (strings.TrimSpace(cfg.DriveName)=="" || strings.TrimSpace(cfg.DriveLetter)=="") {
		reader:=bufio.NewReader(os.Stdin)
		cfg,err=promptDrive(reader,cfg)
		if err!=nil { log.Fatalf("configure ClusterStor drive: %v",err) }
	}

	syncRoot,err:=agent.EnsureSyncRoot(cfg.SyncRoot)
	if err!=nil { log.Fatalf("prepare ClusterStor drive backing folder: %v",err) }
	cfg.SyncRoot=syncRoot
	cfg.AgentVersion=agentVersion

	if runtime.GOOS=="windows" {
		name,err:=agent.ValidateDriveName(cfg.DriveName)
		if err!=nil { log.Fatalf("invalid ClusterStor drive name: %v",err) }
		letter,err:=agent.NormalizeDriveLetter(cfg.DriveLetter)
		if err!=nil { log.Fatalf("invalid ClusterStor drive letter: %v",err) }
		cfg.DriveName=name
		cfg.DriveLetter=letter
		if err:=agent.EnsureDriveMapping(letter,name,syncRoot); err!=nil {
			log.Fatalf("mount ClusterStor drive: %v",err)
		}
	}

	if err:=agent.SaveConfig(cfg); err!=nil { log.Fatalf("store agent configuration: %v",err) }

	journal,err:=agent.OpenJournal(cfg.DeviceID,syncRoot)
	if err!=nil { log.Fatalf("open sync journal: %v",err) }
	snapshot:=journal.Snapshot()
	if runtime.GOOS=="windows" {
		log.Printf("ClusterStor drive ready: %s (%s:)",cfg.DriveName,cfg.DriveLetter)
	} else {
		log.Printf("ClusterStor sync root ready: %s",syncRoot)
	}
	log.Printf("Sync journal ready: generation=%d items=%d pending=%d",snapshot.Generation,len(snapshot.Items),len(snapshot.Pending))

	go func() {
		if err:=agent.RunFilesystemWatcher(ctx,syncRoot,journal,nil); err!=nil && ctx.Err()==nil {
			log.Printf("filesystem watcher stopped: %v",err)
			stop()
		}
	}()

	secret,err:=agent.LoadDeviceSecret()
	if err!=nil { log.Fatalf("load device credential: %v",err) }

	if err:=heartbeat(ctx,client,cfg,secret); err!=nil {
		log.Fatalf("initial heartbeat failed: %v",err)
	}
	log.Printf("ClusterStor agent connected as %q (%s)",cfg.DeviceName,cfg.Platform)
	if *once { return }

	go runHeartbeatLoop(ctx,client,cfg,secret)
	if runtime.GOOS=="windows" {
		if err:=agent.RunDesktopUI(ctx,&cfg,stop); err!=nil && ctx.Err()==nil {
			log.Fatalf("run ClusterStor tray: %v",err)
		}
		return
	}
	<-ctx.Done()
	log.Println("ClusterStor agent stopping")
}

func runHeartbeatLoop(ctx context.Context,client *agent.Client,cfg agent.Config,secret string) {
	ticker:=time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err:=heartbeat(ctx,client,cfg,secret); err!=nil {
				log.Printf("heartbeat failed: %v",err)
			}
		}
	}
}

func pairDevice(ctx context.Context,client *agent.Client,cfg agent.Config) (agent.Config,error) {
	name,err:=os.Hostname()
	if err!=nil || strings.TrimSpace(name)=="" { name="Windows PC" }

	reader:=bufio.NewReader(os.Stdin)
	peerEnabled,peerBytes,err:=promptPeer(reader)
	if err!=nil { return agent.Config{},err }

	start,err:=client.StartPairing(ctx,name,runtime.GOOS,agentVersion,peerEnabled,peerBytes)
	if err!=nil { return agent.Config{},fmt.Errorf("start device pairing: %w",err) }

	fmt.Println()
	fmt.Println("Pair this computer with ClusterStor:")
	fmt.Printf("  Code: %s\n",start.UserCode)
	fmt.Printf("  Open: %s\n",start.VerificationURL)
	fmt.Println()
	if err:=agent.OpenBrowser(start.VerificationURL); err!=nil {
		log.Printf("unable to open browser automatically: %v",err)
	}
	fmt.Println("Approve the device in your browser. This code expires in 10 minutes.")

	for {
		if time.Now().After(start.ExpiresAt) { return agent.Config{},errors.New("device pairing expired") }
		select {
		case <-ctx.Done():
			return agent.Config{},ctx.Err()
		case <-time.After(2*time.Second):
		}
		status,err:=client.PollPairing(ctx,start.PollToken)
		if err!=nil { return agent.Config{},fmt.Errorf("check device pairing: %w",err) }
		if status.Status!="approved" || status.Registration==nil { continue }

		if err:=agent.SaveDeviceSecret(status.Registration.Secret); err!=nil {
			return agent.Config{},fmt.Errorf("store device credential: %w",err)
		}
		cfg=agent.Config{
			APIBaseURL:cfg.APIBaseURL,
			WebBaseURL:cfg.WebBaseURL,
			DeviceID:status.Registration.Device.ID,
			DeviceName:status.Registration.Device.Name,
			Platform:status.Registration.Device.Platform,
			AgentVersion:agentVersion,
			SyncRoot:cfg.SyncRoot,
			DriveName:cfg.DriveName,
			DriveLetter:cfg.DriveLetter,
			PeerContributionEnabled:status.Registration.Device.PeerContributionEnabled,
			PeerContributionBytes:status.Registration.Device.PeerContributionBytes,
		}
		if err:=agent.SaveConfig(cfg); err!=nil { return agent.Config{},fmt.Errorf("store agent configuration: %w",err) }
		fmt.Println("This computer is now registered with ClusterStor.")
		return cfg,nil
	}
}

func promptDrive(reader *bufio.Reader,cfg agent.Config) (agent.Config,error) {
	defaultName:=strings.TrimSpace(cfg.DriveName)
	if defaultName=="" { defaultName="ClusterStor" }

	defaultLetter:=strings.TrimSpace(cfg.DriveLetter)
	if defaultLetter=="" { defaultLetter=agent.PreferredDriveLetter() }
	if defaultLetter=="" { return cfg,errors.New("no preferred drive letter is available") }

	for {
		fmt.Printf("Drive name [%s]: ",defaultName)
		raw,err:=reader.ReadString('\n')
		if err!=nil && len(raw)==0 { return cfg,err }
		value:=strings.TrimSpace(raw)
		if value=="" { value=defaultName }
		name,validateErr:=agent.ValidateDriveName(value)
		if validateErr!=nil {
			fmt.Printf("Invalid drive name: %v\n",validateErr)
			continue
		}
		cfg.DriveName=name
		break
	}

	for {
		fmt.Printf("Drive letter [%s]: ",defaultLetter)
		raw,err:=reader.ReadString('\n')
		if err!=nil && len(raw)==0 { return cfg,err }
		value:=strings.TrimSpace(raw)
		if value=="" { value=defaultLetter }
		letter,normalizeErr:=agent.NormalizeDriveLetter(value)
		if normalizeErr!=nil {
			fmt.Printf("Invalid drive letter: %v\n",normalizeErr)
			continue
		}
		available,checkErr:=agent.DriveLetterAvailableOrOwned(letter,cfg.SyncRoot)
		if checkErr!=nil { return cfg,checkErr }
		if !available {
			fmt.Printf("Drive %s: is already in use by another drive. Choose another letter.\n",letter)
			continue
		}
		cfg.DriveLetter=letter
		break
	}
	return cfg,nil
}

func promptPeer(reader *bufio.Reader) (bool,int64,error) {
	fmt.Println("Peer Storage participation")
	fmt.Println("ClusterStor Peer Storage is optional and is OFF by default.")
	fmt.Print("Do you want this computer to contribute local disk space to Peer Storage? [y/N]: ")
	answer,err:=reader.ReadString('\n')
	if err!=nil && !errors.Is(err,os.ErrClosed) && len(answer)==0 { return false,0,err }
	answer=strings.ToLower(strings.TrimSpace(answer))
	if answer!="y" && answer!="yes" {
		fmt.Println("Peer Storage contribution: OFF")
		return false,0,nil
	}

	for {
		fmt.Print("How many GB of local disk space do you want to contribute? ")
		raw,err:=reader.ReadString('\n')
		if err!=nil && len(raw)==0 { return false,0,err }
		gb,parseErr:=strconv.ParseInt(strings.TrimSpace(raw),10,64)
		if parseErr!=nil || gb<=0 {
			fmt.Println("Enter a positive whole number of GB.")
			continue
		}
		const gib int64=1024*1024*1024
		if gb > (1<<63-1)/gib { return false,0,errors.New("peer contribution amount is too large") }
		bytes:=gb*gib
		fmt.Printf("Peer Storage contribution: ON (%d GB)\n",gb)
		return true,bytes,nil
	}
}

func heartbeat(ctx context.Context,client *agent.Client,cfg agent.Config,secret string) error {
	_,err:=client.Heartbeat(ctx,cfg.DeviceID,secret,agentVersion)
	return err
}
