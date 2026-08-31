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
	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentaudit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentscope"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/audit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/capability"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/catalog"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	productdb "github.com/gawaineLee77/MyKB/services/gateway/internal/database"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/grant"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ingestion"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/library"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/managedmodel"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/mcp"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/note"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/observability"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ownership"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/policy"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/revision"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/routeaction"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/server"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/sessionscope"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/space"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/subscription"
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
	grantRepository, err := grant.NewRepository(db)
	if err != nil {
		log.Fatalf("grant repository error: %v", err)
	}
	auditRepository, err := audit.NewRepository(db)
	if err != nil {
		log.Fatalf("access audit repository error: %v", err)
	}
	agentAuditRepository, err := agentaudit.NewRepository(db)
	if err != nil {
		log.Fatalf("agent operation audit repository error: %v", err)
	}
	sessionScopes, err := sessionscope.NewRepository(db)
	if err != nil {
		log.Fatalf("session-scope repository error: %v", err)
	}
	ownerResolver, err := ownership.NewResolver(profiles, adapter)
	if err != nil {
		log.Fatalf("ownership resolver error: %v", err)
	}
	revisionRepository, err := revision.NewRepository(db)
	if err != nil {
		log.Fatalf("content revision repository error: %v", err)
	}
	publicationRepository, err := publication.NewRepository(db)
	if err != nil {
		log.Fatalf("publication repository error: %v", err)
	}
	subscriptionRepository, err := subscription.NewRepository(db)
	if err != nil {
		log.Fatalf("subscription repository error: %v", err)
	}
	subscriptionService, err := subscription.NewService(subscriptionRepository, publicationRepository, revisionRepository,
		subscription.WithAuditRecorder(auditRepository))
	if err != nil {
		log.Fatalf("subscription service error: %v", err)
	}
	publicationService, err := publication.NewService(publicationRepository, ownerResolver, revisionRepository,
		publication.WithSubscriptionInvalidator(subscriptionRepository), publication.WithAuditRecorder(auditRepository))
	if err != nil {
		log.Fatalf("publication service error: %v", err)
	}
	authorizationService, err := authorization.NewService(ownerResolver, grantRepository, nil,
		authorization.WithPublicationAccess(publicationRepository, subscriptionService))
	if err != nil {
		log.Fatalf("authorization service error: %v", err)
	}
	catalogService, err := catalog.NewService(publicationRepository, subscriptionService, revisionRepository)
	if err != nil {
		log.Fatalf("publication catalog error: %v", err)
	}
	grantService, err := grant.NewService(grantRepository, ownerResolver, grant.WithAuditRecorder(auditRepository))
	if err != nil {
		log.Fatalf("grant service error: %v", err)
	}
	libraryService, err := library.NewService(adapter, authorizationService, library.WithSubscriptions(subscriptionService))
	if err != nil {
		log.Fatalf("authorized library error: %v", err)
	}
	agentScopeResolver, err := agentscope.NewResolver(libraryService, authorizationService)
	if err != nil {
		log.Fatalf("authorized agent scope error: %v", err)
	}
	mcpService, err := mcp.NewService(agentScopeResolver, libraryService, catalogService, subscriptionService, adapter, agentAuditRepository)
	if err != nil {
		log.Fatalf("MCP service error: %v", err)
	}
	mcpLimiter, err := mcp.NewFixedWindowLimiter(60, time.Minute)
	if err != nil {
		log.Fatalf("MCP rate limiter error: %v", err)
	}
	mcpHandler, err := mcp.NewHandler(adapter, mcpService, mcpLimiter, cfg.ProductVersion)
	if err != nil {
		log.Fatalf("MCP transport error: %v", err)
	}
	routeActions, err := routeaction.Load(cfg.RouteActionsFile, cfg.UpstreamVersion)
	if err != nil {
		log.Fatalf("route-action policy error: %v", err)
	}
	accessGate, err := access.NewPhase4Gate(profiles, adapter, routeActions, authorizationService, sessionScopes, auditRepository,
		revisionRepository, publicationService, agentScopeResolver, agentAuditRepository)
	if err != nil {
		log.Fatalf("access gate error: %v", err)
	}
	creationRequests, err := space.NewRequestRepository(db)
	if err != nil {
		log.Fatalf("creation request repository error: %v", err)
	}
	spaceService, err := space.NewService(creationRequests, profiles, adapter, authorizationService)
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
	ingestionService, err := ingestion.NewPhase3Service(profiles, adapter, authorizationService, revisionRepository)
	if err != nil {
		log.Fatalf("document ingestion service error: %v", err)
	}
	capabilities, err := capability.Load(cfg.CapabilitiesFile)
	if err != nil {
		log.Fatalf("capability registry error: %v", err)
	}
	overridesEnabled := capabilities.Document(cfg.ProductVersion, cfg.UpstreamVersion).Capabilities["user_model_overrides"]
	if overridesEnabled != cfg.ModelOverridesEnabled {
		log.Fatalf("model override capability and MINDCREEK_USER_MODEL_OVERRIDES must agree")
	}
	var modelService *managedmodel.Service
	if capabilities.Document(cfg.ProductVersion, cfg.UpstreamVersion).Capabilities["managed_models"] {
		modelService, err = managedmodel.NewService(adapter, agentAuditRepository, managedmodel.Policy{
			OverridesEnabled: overridesEnabled,
			AllowedProviders: cfg.ModelOverrideProviders,
			AllowedHosts:     cfg.ModelOverrideHosts,
			AllowHTTP:        cfg.ModelOverrideAllowHTTP,
		})
		if err != nil {
			log.Fatalf("managed model service error: %v", err)
		}
	}
	routePolicy, err := policy.Load(cfg.RoutePolicyFile, cfg.UpstreamVersion)
	if err != nil {
		log.Fatalf("route policy error: %v", err)
	}
	var hostedMCP http.Handler
	if capabilities.Document(cfg.ProductVersion, cfg.UpstreamVersion).Capabilities["mcp"] {
		hostedMCP = mcpHandler
	}
	var identityBroker http.Handler
	var identityGate server.CorporateIdentityGate
	var identityAdmin server.IdentityAdminService
	if cfg.Identity.Enabled {
		identityRepository, err := identity.NewRepository(db)
		if err != nil {
			log.Fatalf("corporate identity repository error: %v", err)
		}
		corporateProvider, err := identity.NewCorporateProvider(cfg.Identity, nil)
		if err != nil {
			log.Fatalf("corporate identity provider error: %v", err)
		}
		identityGate, err = identity.NewGate(identityRepository, cfg.Identity.BreakGlassUserIDs)
		if err != nil {
			log.Fatalf("corporate identity gate error: %v", err)
		}
		identityAdmin, err = identity.NewAdminService(identityRepository)
		if err != nil {
			log.Fatalf("corporate identity administration error: %v", err)
		}
		broker, err := identity.NewBroker(cfg.Identity, corporateProvider, identityRepository)
		if err != nil {
			log.Fatalf("corporate identity broker error: %v", err)
		}
		identityBroker = broker
		log.Printf("corporate identity enabled provider=%q registration=closed", cfg.Identity.ProviderName)
	}

	log.Printf("mindcreek-gateway version=%s listen=%s", cfg.ProductVersion, cfg.ListenAddr)
	telemetry := observability.NewRecorder(log.New(os.Stdout, "", 0))
	dependencies := server.Dependencies{
		Principals: adapter, Access: accessGate, Spaces: spaceService, Notes: noteService,
		Ingestions: ingestionService, Library: libraryService, Grants: grantService, Directory: adapter,
		Decisions: authorizationService, Publications: publicationService, Catalog: catalogService,
		Subscriptions: subscriptionService, AgentScopes: agentScopeResolver, Models: modelService, MCP: hostedMCP,
		IdentityBroker: identityBroker, IdentityGate: identityGate, IdentityAdmin: identityAdmin,
		Observability: telemetry,
	}
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
