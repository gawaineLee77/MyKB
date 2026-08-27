package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/capability"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	productdb "github.com/gawaineLee77/MyKB/services/gateway/internal/database"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ingestion"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/note"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/policy"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/server"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/space"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

var productVersion = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(os.Getenv("MINDCREEK_LISTEN_ADDR")); err != nil {
			log.Printf("gateway healthcheck failed: %v", err)
			os.Exit(1)
		}
		return
	}

	cfg, err := config.Load(productVersion)
	if err != nil {
		log.Fatalf("gateway configuration error: %v", err)
	}
	adapter, err := weknora.New(cfg.UpstreamURL, cfg.UpstreamVersion, cfg.UpstreamTimeout)
	if err != nil {
		log.Fatalf("WeKnora adapter compatibility error: %v", err)
	}
	db, err := productdb.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("product database error: %v", err)
	}
	defer db.Close()
	migrations, err := productdb.NewRunner(db)
	if err != nil {
		log.Fatalf("product migration configuration error: %v", err)
	}
	if err := handleMigrationCommand(context.Background(), migrations, os.Args[1:]); err != nil {
		log.Fatalf("product migration error: %v", err)
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		return
	}
	profiles, err := profile.NewRepository(db)
	if err != nil {
		log.Fatalf("profile repository error: %v", err)
	}
	accessGate, err := access.NewGate(profiles, adapter)
	if err != nil {
		log.Fatalf("access gate error: %v", err)
	}
	creationRequests, err := space.NewRequestRepository(db)
	if err != nil {
		log.Fatalf("creation request repository error: %v", err)
	}
	spaceService, err := space.NewService(creationRequests, profiles, adapter)
	if err != nil {
		log.Fatalf("knowledge-space service error: %v", err)
	}
	revisions, err := note.NewRevisionRepository(db)
	if err != nil {
		log.Fatalf("note revision repository error: %v", err)
	}
	noteService, err := note.NewService(profiles, adapter, revisions)
	if err != nil {
		log.Fatalf("notes service error: %v", err)
	}
	ingestionService, err := ingestion.NewService(profiles, adapter)
	if err != nil {
		log.Fatalf("document ingestion service error: %v", err)
	}
	capabilities, err := capability.Load(cfg.CapabilitiesFile)
	if err != nil {
		log.Fatalf("capability registry error: %v", err)
	}
	routePolicy, err := policy.Load(cfg.RoutePolicyFile, cfg.UpstreamVersion)
	if err != nil {
		log.Fatalf("route policy error: %v", err)
	}

	log.Printf("mindcreek-gateway version=%s listen=%s", cfg.ProductVersion, cfg.ListenAddr)
	dependencies := server.Dependencies{Principals: adapter, Access: accessGate, Spaces: spaceService, Notes: noteService, Ingestions: ingestionService}
	if err := http.ListenAndServe(cfg.ListenAddr, server.NewGateway(cfg, capabilities, routePolicy, dependencies)); err != nil {
		log.Fatalf("gateway stopped: %v", err)
	}
}

func handleMigrationCommand(ctx context.Context, runner *productdb.Runner, args []string) error {
	if len(args) == 0 || args[0] != "migrate" {
		return runner.Up(ctx)
	}
	action := "up"
	if len(args) > 1 {
		action = args[1]
	}
	switch action {
	case "up":
		return runner.Up(ctx)
	case "down":
		steps := 1
		if len(args) > 2 {
			parsed, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("invalid migration step count: %w", err)
			}
			steps = parsed
		}
		return runner.Down(ctx, steps)
	case "status":
		applied, err := runner.Status(ctx)
		if err != nil {
			return err
		}
		for _, migration := range applied {
			fmt.Printf("%06d %s %s\n", migration.Version, migration.Name, migration.AppliedAt.UTC().Format(time.RFC3339))
		}
		return nil
	default:
		return fmt.Errorf("usage: mindcreek-gateway migrate [up|down [steps]|status]")
	}
}

func runHealthcheck(listenAddr string) error {
	if listenAddr == "" {
		listenAddr = ":8080"
	}
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return fmt.Errorf("parse listen address: %w", err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", response.StatusCode)
	}
	return nil
}
