package store

import (
	"context"
	"database/sql"
	_ "embed"
	"strings"
)

// Keep this version's template immutable when adding later guidance migrations.
//
//go:embed defaults/agents-v12.md
var agentGuidanceV12Raw string

// Embedded guidance has identical bytes on LF and CRLF checkouts.
var agentGuidanceV12 = strings.ReplaceAll(agentGuidanceV12Raw, "\r\n", "\n")

func migrateConsolidatedGuidance(ctx context.Context, tx *sql.Tx) error {
	var previous string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='default_agents_md'").Scan(&previous); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='agents_md' AND value=?", agentGuidanceV12, previous); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='default_agents_md'", agentGuidanceV12)
	return err
}
