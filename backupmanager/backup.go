package backupmanager

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Backup creates a compressed tar.gz dump of the database.
// storageName selects the destination storage ("" = default).
// Returns the archive filename on success.
func (m *Manager) Backup(cfg Config, storageName string) (string, error) {
	dir, err := m.resolvePath(storageName)
	if err != nil {
		return "", err
	}

	baseName := archiveName(cfg.DBName)
	dumpFile := baseName + ".sql"
	archiveFile := baseName + ".tar.gz"
	archivePath := filepath.Join(dir, archiveFile)

	m.logger.Printf("backup start: db=%s storage=%s archive=%s", cfg.DBName, storageName, archiveFile)

	// Build mysqldump command
	dumpCmd := exec.Command(
		"mysqldump",
		"--host="+cfg.Host,
		"--port="+cfg.Port,
		"--user="+cfg.User,
		"--password="+cfg.Password,
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		"--skip-column-statistics",
		"--no-tablespaces",
		cfg.DBName,
	)

	// Open archive for writing
	archiveF, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("create archive file: %w", err)
	}
	defer archiveF.Close()

	gzWriter := gzip.NewWriter(archiveF)
	defer gzWriter.Close()

	// Write tar archive manually (single-file tar)
	tw, err := writeTarHeader(gzWriter, dumpFile)
	if err != nil {
		return "", err
	}

	// Pipe mysqldump stdout → tar content
	dumpCmd.Stdout = tw
	dumpCmd.Stderr = os.Stderr

	if err := dumpCmd.Run(); err != nil {
		_ = os.Remove(archivePath)
		m.logger.Printf("ERROR backup db=%s: %v", cfg.DBName, err)
		return "", fmt.Errorf("mysqldump failed: %w", err)
	}

	// Flush tar + gzip
	if err := tw.Close(); err != nil {
		return "", fmt.Errorf("close tar: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return "", fmt.Errorf("close gzip: %w", err)
	}

	m.logger.Printf("backup done: db=%s archive=%s", cfg.DBName, archiveFile)
	return archiveFile, nil
}

// writeTarHeader returns a writer that streams content for a single tar entry.
// We use a streaming approach: we write a TAR header with size=0 then the
// actual payload. Because mysqldump size is unknown ahead of time we use GNU
// tar extension with unknown size (-1), which is widely supported.
func writeTarHeader(w io.Writer, filename string) (io.WriteCloser, error) {
	return newStreamingTar(w, filename)
}
