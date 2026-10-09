# Working with aicp

aicp stores configuration, Items, intake, and inspection results. You inspect sources and launch agents through your own tools. Act within existing authority. Choose reasonable defaults for low-risk, reversible details; escalate material decisions, missing authority, or unresolved blockers with evidence and a recommendation.

## Read and handle input

For scheduled inspection/intake, start one Run. Read captured AGENTS.md, USER.md, applicable Interest instructions, and every changes and user_inputs page. Attention is optional, not an execution queue. A live brief is orientation, not captured context. Direct edits, configuration, and follow-through need no source Run.

Read user inputs chronologically, then affected current Items and relevant prior input history. Reconcile corrections before acting. Use process_item_input to save a visible response, incorporated information, or durable follow-up for each exact input. Reading and acknowledgement do not process input. Preserve originals; successful handling clears only the captured note. Inbox handling may update or archive its capture. Newer input belongs to the next Run. Never clear newer notes or infer authority from capture.

Configuration/reference changes use their owning commands; apply explicit owner requests and use propose_change for other configuration suggestions. Retain result references and check receipts before retrying. Report failures, partial effects, and the next step on the Item. Failed input stays pending and permits truthful failed/partial Run closure; recovered success controls its final disposition.

## Write to the right place

Inspect only selected Watchers using their captured scope and Interest IDs/revisions; later configuration applies to later Runs. Reuse existing Items through bounded lookup and stable dedupe keys. Source findings need concrete references and relevance reasons.

Use submit_watch_findings for source publication, upsert_item for Interest-level findings without Watcher coverage, and update_item_work for authorized corrections/follow-through without a Run. Preserve Item identity, origin, provenance, evidence, and user state. Work edits neither acknowledge nor process input. Credit mutations to their actual actor; owner-facing CLI/API writes by agents identify actor=agent. Agent status belongs in report/context, not user notes.

Read before writing. On content_conflict, reread and merge; changed payloads need a new request_id. Reuse IDs only for identical uncertain retries. Omitted work fields retain values; empty delegation lists delete nothing. Preserve earlier conclusions, report actions, and continuation context.

## Continue work

Recover pending/blocked delegations even when no Watcher is due. Before handoff, record a pending delegation with its stable ID, executor, outcome, constraints, and input version/source basis. After launch, save external_ref and how to continue that session. Give executors only assigned-Item evidence, constraints, and read/update tools; no global inbox or Run is required. Executors omit external_ref from patches and record the input version actually used separately from the current write version.

Executors leave delivery pending; the primary checks evidence and requests repair before closing work. Reuse the delegation ID and external session for repair; explain unavailable sessions and retain old references on transfer. Scan publication preserves delegated report/context; reconcile new evidence through update_item_work. Closing delegated work does not mark the user's Todo done.

## Finish truthfully

Every selected Watcher needs a truthful terminal result, including successful no-change scans; only successful coverage advances its checkpoint. Every captured input needs a handling disposition or visible failed attempt. Summarize coverage, intake, limitations, and follow-up. Acknowledge only the captured change range fully consumed and durably reflected. Recorded status does not prove external execution, completion, authority, or successful resume.
