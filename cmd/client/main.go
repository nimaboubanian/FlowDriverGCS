package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/NullLatency/flow-driver/internal/config"
	"github.com/NullLatency/flow-driver/internal/httpclient"
	"github.com/NullLatency/flow-driver/internal/storage"
	"github.com/NullLatency/flow-driver/internal/transport"
	"github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

func generateSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type rawResolver struct{}

func (rawResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	// Defends comprehensively against Local DNS leaks by doing absolutely nothing.
	return ctx, nil, nil
}

func main() {
	var configPath, gcPath string
	flag.StringVar(&configPath, "c", "config.json", "Path to config file")
	flag.StringVar(&gcPath, "gc", "credentials.json", "Path to Google Service Account JSON")
	flag.Parse()

	log.Println("Starting Flow Client...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	appCfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	var backend storage.Backend
	switch appCfg.StorageType {
	case "google":
		customHttpClient := httpclient.NewCustomClient(appCfg.Transport)
		backend = storage.NewGoogleBackend(customHttpClient, gcPath, appCfg.GoogleFolderID)
	case "gcs":
		customHttpClient := httpclient.NewCustomClient(appCfg.Transport)
		backend = storage.NewGCSBackend(customHttpClient, gcPath, appCfg.GCSBucket)
	default:
		backend, err = storage.NewLocalBackend(appCfg.LocalDir)
		if err != nil {
			log.Fatalf("Failed to init local storage: %v", err)
		}
	}
	if err := backend.Login(ctx); err != nil {
		log.Fatalf("Backend login failed: %v", err)
	}

	// AUTOMATION: If folder ID is missing, find or create it (Google Drive)
	if appCfg.StorageType == "google" && appCfg.GoogleFolderID == "" {
		log.Println("Zero-Config: Searching for existing Google Drive folder 'Flow-Data'...")
		folderID, err := backend.FindFolder(ctx, "Flow-Data")
		if err != nil {
			log.Fatalf("Failed to search for folder: %v", err)
		}

		if folderID == "" {
			log.Println("Zero-Config: 'Flow-Data' not found. Creating new folder...")
			folderID, err = backend.CreateFolder(ctx, "Flow-Data")
			if err != nil {
				log.Fatalf("Failed to auto-create folder: %v", err)
			}
		} else {
			log.Printf("Zero-Config: Found existing folder with ID %s", folderID)
		}

		appCfg.GoogleFolderID = folderID
		if err := appCfg.Save(configPath); err != nil {
			log.Printf("Warning: Failed to save folder ID to %s: %v", configPath, err)
		} else {
			log.Printf("Zero-Config: Config updated with folder ID %s", folderID)
		}
	}

	// GCS: Verify bucket exists and is accessible
	if appCfg.StorageType == "gcs" && appCfg.GCSBucket != "" {
		bucketID, err := backend.FindFolder(ctx, appCfg.GCSBucket)
		if err != nil {
			log.Fatalf("Failed to verify GCS bucket: %v", err)
		}
		if bucketID == "" {
			log.Fatalf("GCS bucket '%s' not found or not accessible. Please create it in the Google Cloud Console.", appCfg.GCSBucket)
		}
		log.Printf("GCS: Verified bucket '%s' is accessible", appCfg.GCSBucket)
	}

	cid := appCfg.ClientID
	if cid == "" {
		cid = generateSessionID()[:8] // Short random ID as fallback
	}
	engine := transport.NewEngine(backend, true, cid)
	if appCfg.RefreshRateMs > 0 {
		engine.SetPollRate(appCfg.RefreshRateMs)
	}
	if appCfg.FlushRateMs > 0 {
		engine.SetFlushRate(appCfg.FlushRateMs)
	}
	engine.Start(ctx)

	listenAddr := appCfg.ListenAddr
	if listenAddr == "" {
		listenAddr = "127.0.0.1:1080"
	}

	// Create the library SOCKS5 server wrapping our custom Google Drive Engine tunnel
	server := socks5.NewServer(
		socks5.WithDial(func(dc context.Context, network, addr string) (net.Conn, error) {
			sessionID := generateSessionID()

			// Intelligently parse the address string to warn users if their browser is natively leaking DNS
			host, port, err := net.SplitHostPort(addr)
			if err == nil {
				if net.ParseIP(host) != nil {
					log.Printf("New covert session %s targeting RAW IP %s:%s (Warning: Local DNS Leak?)", sessionID, host, port)
				} else {
					log.Printf("New covert session %s targeting SECURE DOMAIN %s:%s", sessionID, host, port)
				}
			} else {
				log.Printf("New covert session %s targeting %s", sessionID, addr)
			}

			session := transport.NewSession(sessionID)
			session.TargetAddr = addr
			engine.AddSession(session)

			// Instantly ping a blank payload so the remote end opens the actual TCP destination
			session.EnqueueTx(nil)

			return transport.NewVirtualConn(session, engine), nil
		}),
		socks5.WithAssociateHandle(func(ctx context.Context, w io.Writer, req *socks5.Request) error {
			// Explicitly block UDP routing to confidently prevent ISP endpoint leakage
			socks5.SendReply(w, statute.RepCommandNotSupported, nil)
			return fmt.Errorf("covert UDP not supported")
		}),
		// DEFEND AGAINST LOCAL DNS LEAKS:
		// The library natively performs system DNS lookups for all FQDNs before proxying!
		// We explicitly override the resolver with a NoOp dummy to force raw strings into the pipe.
		socks5.WithResolver(rawResolver{}),
	)

	log.Printf("Listening for SOCKS5 on %s...", listenAddr)

	go func() {
		if err := server.ListenAndServe("tcp", listenAddr); err != nil {
			log.Fatalf("SOCKS5 server failed: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down client...")
	cancel()
}
