package store

import (
	"context"
	"database/sql"
)

// Upgrade defaults from the observed handoff experience without replacing
// owner-edited instructions or changing existing Item content.
func migrateDelegationHandoffGuidance(ctx context.Context, tx *sql.Tx) error {
	var previous string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='default_agents_md'").Scan(&previous); err != nil {
		return err
	}
	next := previous + `

## Practical delegation handoff

Choose reasonable defaults for low-risk, reversible details within the authorized scope and explain them in the result. Do not require a human question in every task. Escalate material decisions, missing authority, or unresolved blockers with evidence and a recommendation.

Give the executor a short assignment: Item ID, delegation ID, desired outcome, constraints, and the available read/update tools. Ask for supported conclusions, assumptions, and only necessary decisions. Prefer get_item and update_item_work when available; otherwise provide the equivalent Item-scoped CLI or HTTP route. Do not require unrelated inbox or configuration reads.

After launching, save external_ref and retain the external system and how to continue that session in delegation context. The executor may already be working while you save this reference. On content_conflict, re-read the Item, merge your intended changes with current content, and retry using the current expected_content_version and a new request_id for the changed payload. Reuse a request_id only for an identical retry whose outcome is uncertain. Never replay stale whole-report text or overwrite another agent's continuation reference.

For rework, retrieve the assigned delegation's external_ref from the current Item and continue the same session when available. Explain an unavailable session before transferring work, and retain its old executor and reference in context. Preserve prior conclusions and report actions, append bounded repair results, and retain continuation instructions when updating context_md. Report the input version actually used separately from the current write version. Executors leave delivery pending; the primary agent checks results and closes the delegation, independently of the user's Todo.
`
	if _, err := tx.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='agents_md' AND value=?", next, previous); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='default_agents_md'", next)
	return err
}
