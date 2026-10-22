package resourceacl

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/database"
)

// These deliberately minimal tables preserve the pinned native column types.
// The test may run only against a dedicated, loopback synthetic database.
const nativeFixtureDDL = `
CREATE TABLE public.resources (
 id varchar(36) PRIMARY KEY, handle varchar(22) NOT NULL UNIQUE, tenant_id bigint NOT NULL,
 physical_path text NOT NULL, location_hash varchar(64) NOT NULL, kind varchar(32) NOT NULL,
 original_name varchar(1024) NOT NULL DEFAULT '',
 state varchar(16) NOT NULL DEFAULT 'active', deleted_at timestamp, expires_at timestamp,
 created_at timestamp NOT NULL DEFAULT now()
);
CREATE TABLE public.knowledge_bases (
 id varchar(36) PRIMARY KEY, tenant_id bigint NOT NULL, deleted_at timestamp
);
CREATE TABLE public.knowledges (
 id varchar(36) PRIMARY KEY, tenant_id bigint NOT NULL, knowledge_base_id varchar(36) NOT NULL,
 parse_status varchar(50) NOT NULL DEFAULT 'completed', deleted_at timestamp
);
CREATE TABLE public.knowledge_processing_spans (
 id bigserial PRIMARY KEY, knowledge_id varchar(64) NOT NULL, attempt int NOT NULL,
 span_id varchar(64) NOT NULL, parent_span_id varchar(64), name varchar(255) NOT NULL,
 kind varchar(16) NOT NULL, input jsonb, UNIQUE(knowledge_id,attempt,span_id)
);
CREATE TABLE public.resource_bindings (
 id varchar(36) PRIMARY KEY,resource_id varchar(36) NOT NULL,tenant_id bigint NOT NULL,
 owner_type varchar(32),owner_id varchar(64),relation varchar(32)
);
CREATE TABLE public.chunks (
 id varchar(36) PRIMARY KEY,knowledge_id varchar(36),content text,image_info text
);
CREATE TABLE public.sessions (id varchar(36) PRIMARY KEY,tenant_id bigint NOT NULL,deleted_at timestamp);
CREATE TABLE public.messages (
 id varchar(36) PRIMARY KEY,session_id varchar(36) NOT NULL,role varchar(50) NOT NULL,
 content text,images jsonb,deleted_at timestamp
);`

func TestPinnedResourceProvenanceAndMigration(t *testing.T) {
	dsn := os.Getenv("MINDCREEK_RESOURCE_ACL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set a disposable resource ACL permission_test database DSN")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/permission_test" {
		t.Fatal("requires loopback synthetic permission_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var occupied bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.resources') IS NOT NULL`).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if occupied {
		t.Fatal("refusing an existing native resource table; use a new disposable test database")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(nativeFixtureDDL)
	defer func() {
		_, _ = db.ExecContext(context.Background(), `DROP TABLE public.messages,public.sessions,public.chunks,public.resource_bindings,public.knowledge_processing_spans,public.knowledges,public.knowledge_bases,public.resources`)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM mindcreek.kb_resource_bindings`)
	}()
	runner, err := database.NewRunner(db)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := runner.Up(ctx); err != nil {
			t.Fatal("idempotent migration", err)
		}
	}
	if err := runner.Down(ctx, 1); err != nil {
		t.Fatal("empty rollback", err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatal("forward upgrade", err)
	}
	repo := NewRepository(db)
	exec(`INSERT INTO public.knowledge_bases(id,tenant_id) VALUES('kb-a',7),('kb-b',7),('kb-other',8);
INSERT INTO public.knowledges(id,tenant_id,knowledge_base_id) VALUES('doc-a',7,'kb-a'),('doc-b',7,'kb-b'),('doc-other',8,'kb-other')`)
	for i := 1; i <= 4; i++ {
		id, handle := fmt.Sprintf("resource-%d", i), fmt.Sprintf("%022d", i)
		physical := fmt.Sprintf("local://7/exports/image-%d.png", i)
		if i == 4 {
			physical = "local://7/doc-a/source.png"
		}
		exec(`INSERT INTO public.resources(id,handle,tenant_id,physical_path,location_hash,kind) VALUES($1,$2,7,$3,$4,'image')`,
			id, handle, physical, digest(physical))
	}
	ref := func(i int) string { return fmt.Sprintf("resource://%022d", i) }
	resolve := func(i int) Resource {
		t.Helper()
		value, err := repo.Resolve(ctx, 7, ref(i))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	exec(`INSERT INTO public.knowledge_processing_spans(knowledge_id,attempt,span_id,parent_span_id,name,kind,input) VALUES
('doc-a',1,'stage-a',NULL,'multimodal','stage',NULL),
('doc-a',1,'image-a','stage-a','multimodal.image[0]','generation',jsonb_build_object('image_url',$1::text)),
('doc-b',1,'forged-parent','stage-a','multimodal.image[1]','generation',jsonb_build_object('image_url',$1::text)),
('doc-b',2,'stage-b',NULL,'multimodal','stage',NULL),
('doc-b',1,'wrong-attempt','stage-b','multimodal.image[2]','generation',jsonb_build_object('image_url',$1::text))`, ref(1))
	a := resolve(1)
	if len(a.Owners) != 1 || a.Owners[0].KnowledgeID != "doc-a" || a.Owners[0].Provenance != "processing_span" {
		t.Fatal("span lineage did not enforce knowledge and attempt", a.Owners)
	}
	alias, err := repo.Resolve(ctx, 7, a.PhysicalPath)
	if err != nil || alias.Reference != a.Reference || alias.Fingerprint != a.Fingerprint {
		t.Fatal("exact physical alias", alias, err)
	}
	if _, err := repo.Resolve(ctx, 8, ref(1)); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-tenant resource", err)
	}
	if _, err := repo.Resolve(ctx, 7, ref(4)); !errors.Is(err, ErrNotFound) {
		t.Fatal("source file exposed through image adapter", err)
	}
	exec(`INSERT INTO public.resource_bindings VALUES('forged-attachment','resource-2',7,'knowledge','doc-b','attachment');
INSERT INTO public.chunks VALUES('forged-chunk','doc-b','pasted resource://0000000000000000000002','[{"url":"resource://0000000000000000000002"}]')`)
	b := resolve(2)
	if len(b.Owners) != 0 {
		t.Fatal("editable attachment/content laundered resource", b.Owners)
	}
	exec(`UPDATE public.resources SET location_hash='bad' WHERE id='resource-1'`)
	if _, err := repo.Resolve(ctx, 7, ref(1)); !errors.Is(err, ErrNotFound) {
		t.Fatal("mismatched registry fingerprint", err)
	}
	exec(`UPDATE public.resources SET location_hash=$1 WHERE id='resource-1'`, digest(a.PhysicalPath))
	exec(`UPDATE public.knowledges SET deleted_at=now() WHERE id='doc-a'`)
	if got := resolve(1); len(got.Owners) != 0 {
		t.Fatal("deleted document retained ownership", got.Owners)
	}
	exec(`UPDATE public.knowledges SET deleted_at=NULL WHERE id='doc-a';
UPDATE public.knowledge_bases SET deleted_at=now() WHERE id='kb-a'`)
	if got := resolve(1); len(got.Owners) != 0 {
		t.Fatal("deleted KB retained ownership", got.Owners)
	}
	exec(`UPDATE public.knowledge_bases SET deleted_at=NULL WHERE id='kb-a'`)
	if err := repo.Approve(ctx, 7, ref(2), "doc-b", "stale", "synthetic review"); !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatal("stale approval", err)
	}
	if err := repo.Approve(ctx, 7, ref(2), "doc-other", b.Fingerprint, "synthetic review"); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-tenant approval", err)
	}
	if err := repo.Approve(ctx, 7, ref(2), "doc-b", b.Fingerprint, ""); !errors.Is(err, ErrInvalidApproval) {
		t.Fatal("missing review reason", err)
	}
	for i := 0; i < 2; i++ {
		if err := repo.Approve(ctx, 7, ref(2), "doc-b", b.Fingerprint, "synthetic review"); err != nil {
			t.Fatal("reviewed idempotent approval", err)
		}
	}
	if got := resolve(2); len(got.Owners) != 1 || got.Owners[0].KnowledgeID != "doc-b" || got.Owners[0].Provenance != "operator_review" {
		t.Fatal("reviewed binding missing", got.Owners)
	}
	if err := runner.Down(ctx, 1); err == nil {
		t.Fatal("rollback discarded reviewed provenance")
	}
	if got := resolve(2); len(got.Owners) != 1 {
		t.Fatal("failed rollback changed evidence")
	}
	changedPath := "local://7/exports/replaced.png"
	exec(`UPDATE public.resources SET physical_path=$1,location_hash=$2 WHERE id='resource-2'`, changedPath, digest(changedPath))
	if got := resolve(2); len(got.Owners) != 0 {
		t.Fatal("approval survived replacement", got.Owners)
	}
	exec(`UPDATE public.resources SET physical_path=$1,location_hash=$2 WHERE id='resource-2'`, b.PhysicalPath, digest(b.PhysicalPath))
	report, err := repo.AuditPage(ctx, 7, 2, "")
	if err != nil || report.Total != 4 || report.Scanned != 2 || report.Verified != 2 || report.NextCursor != "resource-2" || !report.Truncated {
		t.Fatal("audit first page", report, err)
	}
	report, err = repo.AuditPage(ctx, 7, 2, report.NextCursor)
	if err != nil || report.Scanned != 1 || report.Unverified != 1 || report.Truncated || report.Items[0].Reference != ref(3) {
		t.Fatal("audit second page/source exclusion", report, err)
	}
	exec(`INSERT INTO public.sessions(id,tenant_id) VALUES('session-a',7),('session-other',8)`)
	exec(`INSERT INTO public.messages(id,session_id,role,content,images) VALUES
('user-message','session-a','user','',jsonb_build_array(jsonb_build_object('url',$1::text))),
('assistant-message','session-a','assistant',$1::text,jsonb_build_array(jsonb_build_object('url',$1::text))),
('pasted-message','session-a','user',$1::text,'[]'),
('other-message','session-other','user','',jsonb_build_array(jsonb_build_object('url',$1::text)))`, ref(3))
	if got := resolve(3); !got.SessionScoped {
		t.Fatal("user upload not excluded from workspace manager exception")
	}
	if got := resolve(1); got.SessionScoped {
		t.Fatal("KB image incorrectly became session scoped")
	}
	for _, tc := range []struct {
		session, message, reference string
		want                        bool
	}{
		{"session-a", "user-message", ref(3), true},
		{"session-a", "user-message", "local://7/exports/image-3.png", true},
		{"session-a", "assistant-message", ref(3), false},
		{"session-a", "pasted-message", ref(3), false},
		{"session-other", "other-message", ref(3), false},
		{"session-other", "user-message", ref(3), false},
	} {
		got, err := repo.MessageImage(ctx, 7, tc.session, tc.message, tc.reference)
		if err != nil || got != tc.want {
			t.Fatal("message image check", tc, got, err)
		}
	}
	exec(`UPDATE public.sessions SET deleted_at=now() WHERE id='session-a'`)
	if got, err := repo.MessageImage(ctx, 7, "session-a", "user-message", ref(3)); err != nil || got {
		t.Fatal("deleted session image", got, err)
	}
	if got := resolve(3); !got.SessionScoped {
		t.Fatal("deleted private session lost denial-only provenance")
	}
	exec(`INSERT INTO public.resource_bindings VALUES('temporary-binding','resource-2',7,'temporary_document','temp-doc','attachment')`)
	if got := resolve(2); !got.SessionScoped || len(got.Owners) != 1 {
		t.Fatal("session exclusion must preserve independently approved KB provenance", got)
	}
	exec(`UPDATE public.resources SET state='deleted' WHERE id='resource-3'`)
	if _, err := repo.Resolve(ctx, 7, ref(3)); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted resource", err)
	}
	exec(`DELETE FROM mindcreek.kb_resource_bindings`)
	if err := runner.Down(ctx, 1); err != nil {
		t.Fatal("cleared rollback", err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatal("final forward upgrade", err)
	}
}
