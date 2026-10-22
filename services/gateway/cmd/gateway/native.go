package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentaudit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/agenticon"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/assistant"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/branding"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/capability"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	productdb "github.com/gawaineLee77/MyKB/services/gateway/internal/database"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/graphview"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbacl"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/managedmodel"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/mcp"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/observability"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/server"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/workspaceadmin"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	neo4jconfig "github.com/neo4j/neo4j-go-driver/v6/neo4j/config"
)

// nativeDependencies is separate from the historical composition: no product
// profile, ownership, grant, library, note, publication or subscription service.
func nativeDependencies(cfg config.Config, db, lockDB *sql.DB, adapter *weknora.Client, installation *enterprise.Service) (server.Dependencies, *capability.Registry, error) {
	caps, err := capability.Load(cfg.CapabilitiesFile)
	if err != nil {
		return server.Dependencies{}, nil, err
	}
	if caps.Phase != "r3" || !cfg.EnterpriseEnabled || cfg.ModelOverridesEnabled != caps.Capabilities["user_model_overrides"] || cfg.GraphEnabled != caps.Capabilities["rag_graph"] {
		return server.Dependencies{}, nil, fmt.Errorf("native workspace requires consistent R3/enterprise capabilities")
	}
	routes, err := nativeaccess.LoadRoutes(cfg.NativeRoutesFile)
	if err != nil {
		return server.Dependencies{}, nil, err
	}
	auditor, err := agentaudit.NewRepository(db)
	if err != nil {
		return server.Dependencies{}, nil, err
	}
	models, err := managedmodel.NewService(adapter, auditor, managedmodel.Policy{OverridesEnabled: cfg.ModelOverridesEnabled, AllowedProviders: cfg.ModelOverrideProviders, AllowedHosts: cfg.ModelOverrideHosts, AllowHTTP: cfg.ModelOverrideAllowHTTP})
	if err != nil {
		return server.Dependencies{}, nil, err
	}
	d := server.Dependencies{Principals: adapter, Enterprise: installation, Models: models, NativeAccess: &nativeaccess.Gate{Routes: routes, Principals: adapter}, Observability: observability.NewRecorder(log.New(os.Stdout, "", 0))}
	d.Branding = &branding.Service{Store: &branding.Repository{DB: db}, Installations: installation.Store}
	scopes := &nativeaccess.ScopeService{Upstream: adapter, Store: &nativeaccess.Repository{DB: db}}
	d.NativeScopes = scopes
	var delegationSecret []byte
	if cfg.WorkspaceAdminSecretFile != "" {
		encoded, err := enterprise.ReadSecret(cfg.WorkspaceAdminSecretFile)
		if err != nil {
			return d, nil, fmt.Errorf("workspace delegation secret unavailable")
		}
		delegationSecret, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(delegationSecret) != 32 {
			return d, nil, fmt.Errorf("workspace delegation secret must contain a base64-encoded 32-byte key")
		}
	}
	managers, err := workspaceadmin.NewService(workspaceadmin.NewRepository(db, lockDB), adapter, delegationSecret)
	if err != nil {
		return d, nil, err
	}
	identityRepository, err := identity.NewRepository(db)
	if err != nil {
		return d, nil, err
	}
	managers.IdentityAllowed = func(ctx context.Context, userID string) error {
		i, err := installation.Store.Installation(ctx)
		if err != nil {
			return &workspaceadmin.Error{Status: 503, Code: "identity.unavailable"}
		}
		if userID == i.AdminUserID && userID != "" {
			return nil
		}
		err = identityRepository.CheckLocalUser(ctx, userID)
		if errors.Is(err, identity.ErrNotFound) || errors.Is(err, identity.ErrSuspended) || errors.Is(err, identity.ErrInvalid) {
			return &workspaceadmin.Error{Status: 403, Code: "identity.inactive"}
		}
		if err != nil {
			return &workspaceadmin.Error{Status: 503, Code: "identity.unavailable"}
		}
		return nil
	}
	managers.ValidateKeyScope = func(ctx context.Context, p weknora.Principal, h http.Header, ids []string) error {
		if p.User == nil || p.Tenant == nil {
			return &workspaceadmin.Error{Status: 403, Code: "workspace.denied"}
		}
		_, err := scopes.Resolve(ctx, ids, true, "", nativeaccess.Actor{Kind: "human", ID: p.User.ID, TenantID: p.Tenant.ID}, h)
		return err
	}
	managers.ProtectedKey = func(ctx context.Context, tenant, keyID uint64) (bool, error) {
		i, err := installation.Store.Installation(ctx)
		if err != nil {
			return false, err
		}
		return tenant == i.DefaultTenantID && strconv.FormatUint(keyID, 10) == i.MemberKeyID, nil
	}
	d.WorkspaceAdmin = managers
	graph := &graphview.Service{Enabled: cfg.GraphEnabled, Upstream: adapter}
	if cfg.GraphEnabled && cfg.GraphViewURI != "" {
		driver, err := neo4j.NewDriver(cfg.GraphViewURI, neo4j.BasicAuth(cfg.GraphViewUsername, cfg.GraphViewPassword, ""), func(c *neo4jconfig.Config) {
			c.MaxConnectionPoolSize = 8
			c.SocketConnectTimeout = 3 * time.Second
			c.ConnectionAcquisitionTimeout = 3 * time.Second
			c.MaxTransactionRetryTime = 0
		})
		if err != nil {
			return d, nil, fmt.Errorf("invalid graph reader configuration")
		}
		graph.Reader = &graphview.Neo4j{Driver: driver, Database: cfg.GraphViewDatabase}
	}
	d.GraphView = graph
	knowledgePermissions := wireKnowledgePermissions(db, adapter, managers, scopes, graph, &d)
	d.AgentIcons = &agenticon.Service{Upstream: adapter, Store: agenticon.NewRepository(db), IdentityAllowed: managers.IdentityAllowed, AgentAvatarForRead: scopes.AgentAvatarForRead}
	scopes.AgentAvatarCheck = d.AgentIcons.ValidateReference
	modelPolicy := &nativeaccess.ModelPolicy{Upstream: adapter, GraphEnabled: cfg.GraphEnabled, MaxFileBytes: cfg.MaxFileSizeMB << 20}
	d.NativeAccess.ScopeCheck = scopes.Check
	d.NativeAccess.ModelCheck = modelPolicy.Check
	d.NativeAccess.ResponseCheck = scopes.FilterResponse
	d.NativeAccess.LocalRequest = scopes.Local
	origin := ""
	if cfg.Identity.ExternalOrigin != nil {
		origin = cfg.Identity.ExternalOrigin.String()
	}
	d.Assistant = &assistant.Service{Enabled: cfg.AssistantEnabled, Origin: origin, Principals: adapter, Scopes: scopes, Models: modelPolicy, Store: &assistant.Repository{DB: db}}
	d.Assistant.Employee = func(ctx context.Context, h http.Header) error {
		p, err := adapter.CurrentPrincipal(ctx, h)
		if err != nil {
			return err
		}
		i, err := installation.Store.Installation(ctx)
		if err != nil {
			return err
		}
		if i.IsAdmin(p) {
			return &nativeaccess.Error{Status: 403, Code: "assistant.employee_required"}
		}
		_, err = installation.CheckEmployee(ctx, p)
		return err
	}
	scopes.SessionGuard = d.Assistant.GuardSession
	limiter, err := mcp.NewFixedWindowLimiter(30, time.Minute)
	if err != nil {
		return d, nil, err
	}
	d.MCP, err = mcp.NewNativeHandler(adapter, &mcp.NativeService{Scopes: scopes, Models: modelPolicy, Knowledge: adapter}, limiter, cfg.ProductVersion)
	if err != nil {
		return d, nil, err
	}
	if cfg.Identity.Enabled {
		repository, err := identity.NewRepository(db)
		if err != nil {
			return d, nil, err
		}
		provider, err := identity.NewProvider(cfg.Identity, nil)
		if err != nil {
			return d, nil, err
		}
		d.IdentityGate, err = identity.NewGate(repository, cfg.Identity.BreakGlassUserIDs)
		if err != nil {
			return d, nil, err
		}
		identityAdmin, err := identity.NewAdminService(repository)
		if err != nil {
			return d, nil, err
		}
		identityAdmin.DisableKeys = managers.DisableCreatorKeys
		identityAdmin.RevokeKnowledgeAccess = func(ctx context.Context, user, actor string) error {
			return knowledgePermissions.RevokeIdentity(kbacl.WithAuditActor(ctx, actor), user)
		}
		identityAdmin.WithStatusLock = func(ctx context.Context, subject string, fn func() error) error {
			person, err := repository.GetByBrokerSubject(ctx, subject)
			if err != nil {
				return err
			}
			keys := []string{"identity-status:" + subject}
			if person.LocalUserID != "" {
				keys = append(keys, "identity-user:"+person.LocalUserID)
			}
			return workspaceadmin.NewRepository(db, lockDB).WithLocks(ctx, keys, func(workspaceadmin.Store) error { return fn() })
		}
		d.IdentityAdmin = identityAdmin
		d.IdentityBroker, err = identity.NewBroker(cfg.Identity, provider, repository)
		if err != nil {
			return d, nil, err
		}
	}
	return d, caps, nil
}

func runNativeGateway(cfg config.Config, db *sql.DB, adapter *weknora.Client, installation *enterprise.Service) error {
	// Lock waiters must not consume the query pool needed by the lock holder's
	// identity, installation and authorization lookups.
	lockDB, err := productdb.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer lockDB.Close()
	d, caps, err := nativeDependencies(cfg, db, lockDB, adapter, installation)
	if err != nil {
		return err
	}
	log.Printf("mindcreek-gateway version=%s authorization=native-workspace listen=%s", cfg.ProductVersion, cfg.ListenAddr)
	return http.ListenAndServe(cfg.ListenAddr, server.NewGateway(cfg, caps, nil, d))
}
