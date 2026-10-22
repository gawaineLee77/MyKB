package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/diagnostics"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/graphview"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbacl"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbguard"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/resourceacl"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/server"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/workspaceadmin"
)

func wireKnowledgePermissions(db *sql.DB, adapter *weknora.Client, managers *workspaceadmin.Service, scopes *nativeaccess.ScopeService, graph *graphview.Service, d *server.Dependencies) *kbacl.Service {
	managers.KnowledgeStore = &workspaceadmin.KnowledgeRepository{DB: db}
	repository := kbacl.NewRepository(db)
	acl := &kbacl.Service{Upstream: adapter, Store: repository, IdentityAllowed: managers.IdentityAllowed, WithAuthorizationLock: managers.WithKnowledgeLock}
	acl.MachinePermissions = func(ctx context.Context, h http.Header, kb string) (kbacl.Permissions, error) {
		p, err := adapter.CurrentPrincipal(ctx, h)
		if err != nil {
			return kbacl.Permissions{}, err
		}
		if p.Tenant == nil {
			return kbacl.Permissions{}, &kbacl.Error{Status: 403, Code: "kb_acl.workspace_required"}
		}
		policy, err := managers.MachineKnowledgePolicy(ctx, h, p.Tenant.ID, kb)
		if err != nil {
			return kbacl.Permissions{}, err
		}
		return kbacl.Permissions{Visible: true, Operations: policy.Operations}, nil
	}
	check := func(ctx context.Context, kb string, h http.Header) error {
		_, err := acl.Authorize(ctx, h, kb)
		return err
	}
	scopes.KnowledgeAccess = check
	scopes.PlainChatAllowed = func(ctx context.Context, h http.Header) error {
		p, err := adapter.CurrentPrincipal(ctx, h)
		if err != nil {
			return err
		}
		if p.Tenant == nil || p.Tenant.ID == 0 {
			return &nativeaccess.Error{Status: 503, Code: "kb_acl.memory_verification_unavailable"}
		}
		if p.Tenant.MemoryConfig != nil && p.Tenant.MemoryConfig.Enabled {
			return &nativeaccess.Error{Status: 403, Code: "kb_acl.workspace_memory_disabled_required"}
		}
		return nil
	}
	scopes.KnowledgeManage = func(ctx context.Context, kb string, h http.Header) error {
		_, err := acl.Authorize(ctx, h, kb, "manage")
		return err
	}
	graph.KnowledgeAccess = check
	managers.RevokeKnowledgeMember = func(ctx context.Context, tenant uint64, user string, remove bool, actor string) error {
		return acl.RevokeMember(kbacl.WithAuditActor(ctx, actor), tenant, user, remove)
	}
	d.KBPermissions = acl
	d.KBGuard = &kbguard.Guard{ACL: acl, Upstream: adapter, WithLock: managers.WithKnowledgeLock, WithCollaborator: managers.WithKnowledgeExecutor, BindCreated: acl.BindCreated, MarkDeleted: acl.MarkDeleted}
	resources := &resourceacl.Service{Catalog: resourceacl.NewRepository(db), Access: acl}
	d.KBGuard.AuthorizeResource = resources.AuthorizeKB
	d.KBGuard.RewriteResources = resources.RewriteResources
	scopes.MessageResource = resources.AuthorizeMessage
	scopes.RewriteResources = resources.RewriteResources
	scopes.StreamResources = func(ctx context.Context, h http.Header) nativeaccess.ResourceStream {
		return resources.NewStreamRewriter(ctx, h)
	}
	d.KBGuard.BeforeCreate = func(ctx context.Context, h http.Header, key, hash string) error {
		_, err := acl.BeginCreate(ctx, h, key, hash)
		return err
	}
	d.KBGuard.AfterCreate = acl.CompleteCreate
	d.KBGuard.Audit = func(ctx context.Context, h http.Header, event kbguard.AuditEvent) error {
		actor, err := nativeaccess.ResolveActor(ctx, adapter, h)
		if err != nil {
			return err
		}
		actorID := actor.ID
		if actor.Kind == "api_key" {
			actorID = "api-key:" + actorID
		}
		after, err := json.Marshal(struct {
			Actions []string `json:"actions"`
			Targets []string `json:"targets"`
			Method  string   `json:"method"`
			Route   string   `json:"route"`
			Status  int      `json:"status"`
		}{event.Actions, event.Targets, event.Method, event.Route, event.Status})
		if err != nil {
			return err
		}
		for _, kb := range event.KBIDs {
			if err := repository.RecordEvent(ctx, kbacl.Event{TenantID: actor.TenantID, KBID: kb, ActorUserID: actorID, Action: "content.operation", Revision: event.Revisions[kb], After: after, Outcome: event.Outcome, RequestID: diagnostics.RequestID(ctx)}); err != nil {
				return err
			}
		}
		return nil
	}
	return acl
}
