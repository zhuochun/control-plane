package store

import (
	"context"
	"database/sql"
)

// Item content is already versioned JSON. Only default agent guidance needs a
// versioned upgrade; owner-edited guidance remains untouched.
func migrateDelegationGuidance(ctx context.Context, tx *sql.Tx) error {
	var previous string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='default_agents_md'").Scan(&previous); err != nil {
		return err
	}
	next := previous + "\n\n## Autonomous follow-through\n\nWithin the authorized goal and scope, investigate, delegate, collect results, check evidence, and request repair without routine human dispatch or collection. Ask the person for material decisions, missing authority, or blockers you cannot resolve. Record pending Item-local delegations before external handoff, then retain executor and external continuation references. aicp records work; you choose and launch agents through your own tools.\n\nOn continuation, use list_items with delegation_status pending,blocked even when no Watcher is due; no dummy source Run is needed. Read current applicable guidance. Give executors only the assigned Item ID, delegation ID, necessary evidence and constraints. They can use get_item and update_item_work without reading the global inbox or starting a Run. Keep the input Item version and source/instruction basis actually used in result context; current write versions alone do not establish research freshness.\n\nupdate_item_work preserves Item identity, origin, Watcher and Interest relevance, and user-owned state. Omitted fields and delegation entries retain values; merge delegation changes by ID. Empty lists delete nothing; close follow-up using closed. Executor delivery stays pending for primary-agent judgment; repair reuses the delegation and external reference. Retain original executor/references in context on transfer. Persist supported results and artifact references before closing follow-up. Scan publication retains delegated reports/context: reconcile new source evidence with research via update_item_work after publication. Never infer external execution, completion, authority, or successful resume from a recorded status.\n"
	if _, err := tx.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='agents_md' AND value=?", next, previous); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='default_agents_md'", next)
	return err
}
