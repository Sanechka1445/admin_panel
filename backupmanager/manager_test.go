package backupmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupManager creates a Manager with a temp config and default storage.
func setupManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "storages.json")
	defaultDir := filepath.Join(dir, "default")

	m, err := New(cfgPath, defaultDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m, dir
}

func TestNew_CreatesDefaultStorage(t *testing.T) {
	m, dir := setupManager(t)
	defaultDir := filepath.Join(dir, "default")

	stores := m.ListStorages()
	if len(stores) != 1 {
		t.Fatalf("expected 1 storage, got %d", len(stores))
	}
	if stores[0].Name != "default" {
		t.Errorf("expected name=default, got %s", stores[0].Name)
	}
	if stores[0].Path != defaultDir {
		t.Errorf("unexpected path %s", stores[0].Path)
	}
}

func TestAddStorage(t *testing.T) {
	m, dir := setupManager(t)
	newPath := filepath.Join(dir, "scheduled")

	if err := m.AddStorage("scheduled", newPath); err != nil {
		t.Fatalf("AddStorage: %v", err)
	}

	stores := m.ListStorages()
	if len(stores) != 2 {
		t.Errorf("expected 2 storages, got %d", len(stores))
	}

	// Duplicate should fail
	if err := m.AddStorage("scheduled", newPath); err == nil {
		t.Error("expected error for duplicate storage name")
	}
}

func TestRemoveStorage(t *testing.T) {
	m, dir := setupManager(t)
	newPath := filepath.Join(dir, "manual")
	_ = m.AddStorage("manual", newPath)

	if err := m.RemoveStorage("manual"); err != nil {
		t.Fatalf("RemoveStorage: %v", err)
	}

	// Directory should still exist (not deleted)
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		t.Error("directory was deleted, should be preserved")
	}

	// Cannot remove default
	if err := m.RemoveStorage("default"); err == nil {
		t.Error("expected error removing default storage")
	}
}

func TestRemoveStorage_NotFound(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.RemoveStorage("nonexistent"); err == nil {
		t.Error("expected error for nonexistent storage")
	}
}

func TestListBackups_Empty(t *testing.T) {
	m, _ := setupManager(t)
	files, err := m.ListBackups("default")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty list, got %v", files)
	}
}

func TestListBackups_WithFiles(t *testing.T) {
	m, dir := setupManager(t)
	defaultDir := filepath.Join(dir, "default")

	// Create fake backup files
	for _, name := range []string{"test_20240101T120000_001234.tar.gz", "other.txt"} {
		_ = os.WriteFile(filepath.Join(defaultDir, name), []byte("x"), 0o644)
	}

	files, err := m.ListBackups("default")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 .gz file, got %d: %v", len(files), files)
	}
	if files[0] != "test_20240101T120000_001234.tar.gz" {
		t.Errorf("unexpected file name: %s", files[0])
	}
}

func TestDeleteBackup(t *testing.T) {
	m, dir := setupManager(t)
	defaultDir := filepath.Join(dir, "default")
	fname := "testdb_20240101T120000_001234.tar.gz"
	fpath := filepath.Join(defaultDir, fname)
	_ = os.WriteFile(fpath, []byte("data"), 0o644)

	if err := m.DeleteBackup("default", fname); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	if _, err := os.Stat(fpath); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}

	// Deleting non-existent should error
	if err := m.DeleteBackup("default", fname); err == nil {
		t.Error("expected error deleting nonexistent file")
	}
}

func TestDeleteBackup_UnknownStorage(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.DeleteBackup("ghost", "something.tar.gz"); err == nil {
		t.Error("expected error for unknown storage")
	}
}

func TestArchiveName_Format(t *testing.T) {
	name := archiveName("mydb")
	if !strings.HasPrefix(name, "mydb_") {
		t.Errorf("expected prefix mydb_, got %s", name)
	}
	if !strings.Contains(name, "T") {
		t.Errorf("expected timestamp with T, got %s", name)
	}
}

func TestParseDBName_CurrentDatabase(t *testing.T) {
	dump := `-- MySQL dump\n-- Current Database: ` + "`testdb`" + `\n-- more content`
	got := parseDBName(dump)
	if got != "testdb" {
		t.Errorf("expected testdb, got %q", got)
	}
}

func TestParseDBName_UseStatement(t *testing.T) {
	dump := "-- header\nUSE `myschema`;\nCREATE TABLE..."
	got := parseDBName(dump)
	if got != "myschema" {
		t.Errorf("expected myschema, got %q", got)
	}
}

func TestParseDBName_NotFound(t *testing.T) {
	got := parseDBName("-- some random content without db name")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("DB_HOST", "myhost")
	t.Setenv("DB_PORT", "3307")
	t.Setenv("DB_USER", "bob")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "shop")

	cfg := ConfigFromEnv()
	if cfg.Host != "myhost" {
		t.Errorf("host: got %s", cfg.Host)
	}
	if cfg.Port != "3307" {
		t.Errorf("port: got %s", cfg.Port)
	}
	if cfg.User != "bob" {
		t.Errorf("user: got %s", cfg.User)
	}
	if cfg.Password != "secret" {
		t.Errorf("password: got %s", cfg.Password)
	}
	if cfg.DBName != "shop" {
		t.Errorf("dbname: got %s", cfg.DBName)
	}
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_PORT", "")
	t.Setenv("DB_USER", "")

	cfg := ConfigFromEnv()
	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected default host, got %s", cfg.Host)
	}
	if cfg.Port != "3306" {
		t.Errorf("expected default port, got %s", cfg.Port)
	}
	if cfg.User != "root" {
		t.Errorf("expected default user, got %s", cfg.User)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "storages.json")
	defaultDir := filepath.Join(dir, "default")

	m1, err := New(cfgPath, defaultDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = m1.AddStorage("extra", filepath.Join(dir, "extra"))

	// Reload
	m2, err := New(cfgPath, defaultDir)
	if err != nil {
		t.Fatal(err)
	}
	stores := m2.ListStorages()
	if len(stores) != 2 {
		t.Errorf("expected 2 storages after reload, got %d", len(stores))
	}
}

func TestResolvePath_Default(t *testing.T) {
	m, dir := setupManager(t)
	defaultDir := filepath.Join(dir, "default")

	path, err := m.resolvePath("")
	if err != nil {
		t.Fatalf("resolvePath: %v", err)
	}
	if path != defaultDir {
		t.Errorf("expected %s, got %s", defaultDir, path)
	}
}

func TestResolvePath_Named(t *testing.T) {
	m, dir := setupManager(t)
	namedPath := filepath.Join(dir, "named")
	_ = m.AddStorage("named", namedPath)

	path, err := m.resolvePath("named")
	if err != nil {
		t.Fatalf("resolvePath: %v", err)
	}
	if path != namedPath {
		t.Errorf("expected %s, got %s", namedPath, path)
	}
}

func TestResolvePath_Unknown(t *testing.T) {
	m, _ := setupManager(t)
	_, err := m.resolvePath("ghost")
	if err == nil {
		t.Error("expected error for unknown storage")
	}
}
