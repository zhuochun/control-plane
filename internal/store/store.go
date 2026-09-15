// Package store owns the SQLite connection, directory lock, and migrations.
// Only the server opens a Store; adapters communicate through HTTP.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	DB   *sql.DB
	lock *flock.Flock
}

func Open(ctx context.Context, directory string) (*Store, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(absolute, "server.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock data directory: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("data directory is in use by another aicp server: %s", absolute)
	}

	// DSN pragmas apply to every replacement connection, not just the first.
	filename := filepath.ToSlash(filepath.Join(absolute, "aicp.db"))
	if len(filename) > 1 && filename[1] == ':' {
		filename = "/" + filename
	}
	uri := url.URL{Scheme: "file", Path: filename}
	params := url.Values{}
	for _, pragma := range []string{"foreign_keys(1)", "busy_timeout(5000)", "journal_mode(WAL)", "synchronous(FULL)"} {
		params.Add("_pragma", pragma)
	}
	uri.RawQuery = params.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{DB: db, lock: lock}
	if err := s.migrate(ctx); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	source, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, s.DB, source)
	if err != nil {
		return err
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > 2 {
		return fmt.Errorf("database schema %d is newer than this aicp supports; upgrade aicp", version)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	return errors.Join(s.DB.Close(), s.lock.Close())
}
