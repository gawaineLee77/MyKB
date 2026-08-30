package database

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadMigrations(t *testing.T) {
	files := fstest.MapFS{
		"migrations/000002_second.up.sql":   {Data: []byte("SELECT 2")},
		"migrations/000002_second.down.sql": {Data: []byte("SELECT -2")},
		"migrations/000001_first.up.sql":    {Data: []byte("SELECT 1")},
		"migrations/000001_first.down.sql":  {Data: []byte("SELECT -1")},
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("unexpected migrations: %+v", migrations)
	}
	if migrations[0].Checksum == "" {
		t.Fatal("migration checksum is empty")
	}
}

func TestLoadMigrationsRequiresDownFile(t *testing.T) {
	files := fstest.MapFS{"migrations/000001_first.up.sql": {Data: []byte("SELECT 1")}}
	if _, err := loadMigrations(files); err == nil {
		t.Fatal("loadMigrations accepted a migration without rollback SQL")
	}
}

func TestEmbeddedMigrationsIncludeCurrentProductSchema(t *testing.T) {
	migrations, err := loadMigrations(migrationFiles)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 9 {
		t.Fatalf("embedded migration count = %d, want 9", len(migrations))
	}
	grantMigration := migrations[5]
	if grantMigration.Version != 6 || grantMigration.Name != "kb_access_grants" {
		t.Fatalf("last migration = %+v", grantMigration)
	}
	for _, required := range []string{
		"subject_type IN ('user', 'group', 'workspace')",
		"permission IN ('viewer', 'editor')",
		"kb_access_grants_active_subject_unique",
		"last_audit_correlation_id",
	} {
		if !strings.Contains(grantMigration.UpSQL, required) {
			t.Errorf("grant migration is missing %q", required)
		}
	}
	securityMigration := migrations[6]
	if securityMigration.Version != 7 || securityMigration.Name != "phase2_security_records" ||
		!strings.Contains(securityMigration.UpSQL, "session_kb_scopes") ||
		!strings.Contains(securityMigration.UpSQL, "kb_access_audit_events") {
		t.Fatalf("Phase 2 security migration = %+v", securityMigration)
	}
	phase3Migration := migrations[7]
	if phase3Migration.Version != 8 || phase3Migration.Name != "phase3_publications" {
		t.Fatalf("Phase 3 publication migration = %+v", phase3Migration)
	}
	for _, required := range []string{
		"kb_publications", "organization_public", "kb_subscriptions",
		"kb_content_revisions", "kb_activity_events", "last_seen_revision",
	} {
		if !strings.Contains(phase3Migration.UpSQL, required) {
			t.Errorf("Phase 3 publication migration is missing %q", required)
		}
	}
	phase4Migration := migrations[8]
	if phase4Migration.Version != 9 || phase4Migration.Name != "phase4_agent_operations" {
		t.Fatalf("Phase 4 agent-operation migration = %+v", phase4Migration)
	}
	for _, required := range []string{"agent_operation_audit_events", "knowledge_base_ids", "duration_ms", "client_kind IN ('web', 'mcp')"} {
		if !strings.Contains(phase4Migration.UpSQL, required) {
			t.Errorf("Phase 4 agent-operation migration is missing %q", required)
		}
	}
}
