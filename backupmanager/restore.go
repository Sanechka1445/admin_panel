package backupmanager

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Restore extracts a backup archive and restores the database.
// The target database name is extracted from the SQL dump inside the archive.
// If the database does not exist, an error is returned with a clear message.
func (m *Manager) Restore(cfg Config, storageName, archiveFile string) error {
	dir, err := m.resolvePath(storageName)
	if err != nil {
		return err
	}

	archivePath := filepath.Join(dir, archiveFile)
	m.logger.Printf("restore start: archive=%s storage=%s", archiveFile, storageName)

	// Open and decompress archive
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	hdr, err := tr.Next()
	if err != nil {
		return fmt.Errorf("read tar entry: %w", err)
	}
	_ = hdr // we have the dump stream in tr now

	// Peek at beginning of dump to extract DB name
	// mysqldump writes: -- Current Database: `dbname`
	dbName, peekReader, err := extractDBName(tr)
	if err != nil {
		return fmt.Errorf("extract db name from dump: %w", err)
	}

	// Verify database exists
	if err := ensureDBExists(cfg, dbName); err != nil {
		m.logger.Printf("ERROR restore: db check failed db=%s: %v", dbName, err)
		return err
	}

	// Pipe dump into mysql
	mysqlCmd := exec.Command(
		"mysql",
		"--host="+cfg.Host,
		"--port="+cfg.Port,
		"--user="+cfg.User,
		"--password="+cfg.Password,
		dbName,
	)
	mysqlCmd.Stdin = peekReader
	mysqlCmd.Stdout = os.Stdout
	mysqlCmd.Stderr = os.Stderr

	if err := mysqlCmd.Run(); err != nil {
		m.logger.Printf("ERROR restore failed db=%s: %v", dbName, err)
		return fmt.Errorf("mysql restore failed: %w", err)
	}

	m.logger.Printf("restore done: db=%s archive=%s", dbName, archiveFile)
	return nil
}

// extractDBName reads a mysqldump stream looking for the database name comment.
// It returns the db name and an io.Reader that replays the already-read bytes
// followed by the rest of the original reader.
func extractDBName(r io.Reader) (string, io.Reader, error) {
	const peekSize = 8192
	buf := make([]byte, peekSize)
	n, err := r.Read(buf)
	if err != nil && n == 0 {
		return "", nil, fmt.Errorf("empty dump: %w", err)
	}
	buf = buf[:n]

	dbName := parseDBName(string(buf))
	if dbName == "" {
		return "", nil, fmt.Errorf("could not find database name in dump header")
	}

	combined := io.MultiReader(strings.NewReader(string(buf)), r)
	return dbName, combined, nil
}

// parseDBName scans the dump text for the current database comment.
func parseDBName(text string) string {
	const marker = "-- Current Database: `"
	idx := strings.Index(text, marker)
	if idx == -1 {
		// Also try USE `dbname`;
		const use = "USE `"
		idx2 := strings.Index(text, use)
		if idx2 == -1 {
			return ""
		}
		rest := text[idx2+len(use):]
		end := strings.Index(rest, "`")
		if end == -1 {
			return ""
		}
		return rest[:end]
	}
	rest := text[idx+len(marker):]
	end := strings.Index(rest, "`")
	if end == -1 {
		return ""
	}
	return rest[:end]
}

// ensureDBExists checks that the target database exists via a simple query.
func ensureDBExists(cfg Config, dbName string) error {
	// We connect without specifying a DB and run a quick check
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/", cfg.User, cfg.Password, cfg.Host, cfg.Port)

	// Use mysql CLI for simplicity (avoids import cycle with driver)
	out, err := exec.Command(
		"mysql",
		"--host="+cfg.Host,
		"--port="+cfg.Port,
		"--user="+cfg.User,
		"--password="+cfg.Password,
		"--execute=SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='"+dbName+"';",
		"information_schema",
		"--skip-column-names",
		"--silent",
	).Output()
	_ = dsn
	if err != nil {
		return fmt.Errorf("check database existence: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("database %q does not exist; create it before restoring", dbName)
	}
	return nil
}
