package server

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runSeeds(ctx context.Context, db *sql.DB, seedsPath string) error {
	if strings.TrimSpace(seedsPath) == "" {
		return fmt.Errorf("SEEDS_PATH must not be empty")
	}

	entries, err := os.ReadDir(seedsPath)
	if err != nil {
		return fmt.Errorf("read seed files: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		files = append(files, entry.Name())
	}

	if len(files) == 0 {
		return nil
	}

	sort.Strings(files)

	for _, fileName := range files {
		path := filepath.Join(seedsPath, fileName)
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read seed file %s: %w", path, err)
		}

		query := strings.TrimSpace(string(contents))
		if query == "" {
			continue
		}

		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("run seed file %s: %w", path, err)
		}
	}

	return nil
}
