package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentaudit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/assistant"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/capability"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/managedmodel"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/mcp"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/observability"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/server"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

// nativeDependencies is separate from the historical composition: no product
// profile, ownership, grant, library, note, publication or subscription service.
func nativeDependencies(cfg config.Config, db *sql.DB, adapter *weknora.Client, installation *enterprise.Service) (server.Dependencies, *capability.Registry, error) {
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
	scopes := &nativeaccess.ScopeService{Upstream: adapter, Store: &nativeaccess.Repository{DB: db}}
	d.NativeScopes = scopes
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
		d.IdentityAdmin, err = identity.NewAdminService(repository)
		if err != nil {
			return d, nil, err
		}
		d.IdentityBroker, err = identity.NewBroker(cfg.Identity, provider, repository)
		if err != nil {
			return d, nil, err
		}
	}
	return d, caps, nil
}

func runNativeGateway(cfg config.Config, db *sql.DB, adapter *weknora.Client, installation *enterprise.Service) error {
	d, caps, err := nativeDependencies(cfg, db, adapter, installation)
	if err != nil {
		return err
	}
	log.Printf("mindcreek-gateway version=%s authorization=native-workspace listen=%s", cfg.ProductVersion, cfg.ListenAddr)
	return http.ListenAndServe(cfg.ListenAddr, server.NewGateway(cfg, caps, nil, d))
}
