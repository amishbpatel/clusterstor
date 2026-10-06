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

const agentVersion = "0.1.0-dev"

func main() {
	defaultAPI:=strings.TrimSpace(os.Getenv("CLUSTERSTOR_API_BASE_URL"))
	if defaultAPI=="" { defaultAPI="http://localhost:8080" }

	apiBase:=flag.String("api",defaultAPI,"ClusterStor API base URL")
	once:=flag.Bool("once",false,"send one heartbeat and exit after registration")
	flag.Parse()

	ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM)
	defer stop()

	cfg,err:=agent.LoadConfig()
	if err!=nil { log.Fatal(err) }
	if cfg.APIBaseURL=="" { cfg.APIBaseURL=*apiBase }

	client:=agent.NewClient(cfg.APIBaseURL)

	if strings.TrimSpace(cfg.DeviceID)=="" {
		cfg,err=pairDevice(ctx,client,cfg)
		if err!=nil { log.Fatal(err) }
	}

	secret,err:=agent.LoadDeviceSecret()
	if err!=nil { log.Fatalf("load device credential: %v",err) }

	if err:=heartbeat(ctx,client,cfg,secret); err!=nil {
		log.Fatalf("initial heartbeat failed: %v",err)
	}
	log.Printf("ClusterStor agent connected as %q (%s)",cfg.DeviceName,cfg.Platform)
	if *once { return }

	ticker:=time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("ClusterStor agent stopping")
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
			DeviceID:status.Registration.Device.ID,
			DeviceName:status.Registration.Device.Name,
			Platform:status.Registration.Device.Platform,
			AgentVersion:agentVersion,
			PeerContributionEnabled:status.Registration.Device.PeerContributionEnabled,
			PeerContributionBytes:status.Registration.Device.PeerContributionBytes,
		}
		if err:=agent.SaveConfig(cfg); err!=nil { return agent.Config{},fmt.Errorf("store agent configuration: %w",err) }
		fmt.Println("This computer is now registered with ClusterStor.")
		return cfg,nil
	}
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
