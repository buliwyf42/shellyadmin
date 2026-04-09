package db

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BackupDatabaseFile creates a timestamped copy of the SQLite file if it exists.
// It is intended for last-chance preservation when startup migration/open fails.
func BackupDatabaseFile(dataDir string) (string, error) {
	source := filepath.Join(dataDir, "shellyctl.db")
	if _, err := os.Stat(source); err != nil {
		return "", err
	}
	target := filepath.Join(dataDir, fmt.Sprintf("shellyctl.db.backup-%s", time.Now().UTC().Format("20060102T150405Z")))

	srcFile, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(target)
	if err != nil {
		return "", err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return "", err
	}
	return target, nil
}
