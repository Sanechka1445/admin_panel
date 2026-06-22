// Package backupmanager provides orchestration of MySQL/MariaDB database backups.
// It supports multiple local storage locations, streaming compression, and timestamped archives.
package backupmanager

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Storage represents a named directory for storing backups.
type Storage struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Config holds connection parameters for the database.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

// Manager orchestrates backup operations and manages multiple storages.
type Manager struct {
	mu           sync.RWMutex
	storages     map[string]*Storage
	defaultStore string
	configPath   string
	logger       *log.Logger
}

// storagesFile is the JSON structure persisted to disk.
type storagesFile struct {
	Default  string              `json:"default"`
	Storages map[string]*Storage `json:"storages"`
}

// New creates a Manager, loading storages from configPath.
// If configPath does not exist it is created with the default storage at defaultDir.
func New(configPath, defaultDir string) (*Manager, error) {
	m := &Manager{
		storages:   make(map[string]*Storage),
		configPath: configPath,
		logger:     log.New(os.Stdout, "[backupmanager] ", log.LstdFlags),
	}

	if err := m.load(defaultDir); err != nil {
		return nil, err
	}
	return m, nil
}

// load reads the config file or initialises defaults.
func (m *Manager) load(defaultDir string) error {
	data, err := os.ReadFile(m.configPath)
	if os.IsNotExist(err) {
		// Bootstrap default storage
		if err2 := os.MkdirAll(defaultDir, 0o755); err2 != nil {
			return fmt.Errorf("create default storage dir: %w", err2)
		}
		m.storages["default"] = &Storage{Name: "default", Path: defaultDir}
		m.defaultStore = "default"
		return m.save()
	}
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var sf storagesFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	m.storages = sf.Storages
	m.defaultStore = sf.Default
	return nil
}

// save persists current storages to disk (caller must hold m.mu or be single-threaded).
func (m *Manager) save() error {
	sf := storagesFile{Default: m.defaultStore, Storages: m.storages}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath, data, 0o644)
}

// AddStorage registers a new storage directory and persists the change.
func (m *Manager) AddStorage(name, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.storages[name]; exists {
		return fmt.Errorf("storage %q already exists", name)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}
	m.storages[name] = &Storage{Name: name, Path: path}
	m.logger.Printf("storage added: name=%s path=%s", name, path)
	return m.save()
}

// RemoveStorage removes a storage registration (directory is NOT deleted).
func (m *Manager) RemoveStorage(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if name == m.defaultStore {
		return fmt.Errorf("cannot remove the default storage")
	}
	if _, exists := m.storages[name]; !exists {
		return fmt.Errorf("storage %q not found", name)
	}
	delete(m.storages, name)
	m.logger.Printf("storage removed: name=%s", name)
	return m.save()
}

// ListStorages returns a snapshot of all active storages.
func (m *Manager) ListStorages() []*Storage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Storage, 0, len(m.storages))
	for _, s := range m.storages {
		result = append(result, &Storage{Name: s.Name, Path: s.Path})
	}
	return result
}

// resolvePath returns the directory for the given storage name, or the default.
func (m *Manager) resolvePath(storageName string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name := storageName
	if name == "" {
		name = m.defaultStore
	}
	s, ok := m.storages[name]
	if !ok {
		return "", fmt.Errorf("storage %q not found", name)
	}
	return s.Path, nil
}

// archiveName generates a unique filename for a backup archive.
// Format: <dbname>_<timestamp>_<rand>.tar.gz
func archiveName(dbName string) string {
	ts := time.Now().UTC().Format("20060102T150405")
	rnd := rand.Intn(999999) //nolint:gosec // non-crypto random is fine for naming
	return fmt.Sprintf("%s_%s_%06d", dbName, ts, rnd)
}

// DeleteBackup removes a backup archive from the specified storage.
func (m *Manager) DeleteBackup(storageName, filename string) error {
	dir, err := m.resolvePath(storageName)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, filename)
	if err := os.Remove(target); err != nil {
		m.logger.Printf("ERROR delete backup %s: %v", filename, err)
		return err
	}
	m.logger.Printf("backup deleted: storage=%s file=%s", storageName, filename)
	return nil
}

// ListBackups returns all .tar.gz files in the given storage.
func (m *Manager) ListBackups(storageName string) ([]string, error) {
	dir, err := m.resolvePath(storageName)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read storage dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".gz" {
			files = append(files, e.Name())
		}
	}
	return files, nil
}
