package database

import (
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
