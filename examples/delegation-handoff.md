# Delegation handoff

The primary agent records a pending delegation on an existing Item, launches
the executor through its own tools, and saves the returned reference in
`external_ref`. Keep its external system and continuation instructions in
delegation context, for example: "Codex thread; use read_thread and
send_message_to_thread with external_ref." A record alone does not prove that
launch succeeded. The executor may begin before this reference is saved.

## Short executor assignment

Replace the bracketed values and provide available Item read/update tools:

> Work on Item [item ID], delegation [delegation ID]. Deliver [outcome] within
> [constraints]. In aicp, read only this Item using get_item. Choose and explain
> reasonable defaults for reversible details; surface only material decisions,
> missing authority, or unresolved blockers, with evidence and a recommendation.
> Use update_item_work to preserve existing conclusions and add your result.
> Record the input version and requirements actually used. Preserve continuation
> context and leave the delegation pending for primary-agent review. Omit other
> delegation fields. On content_conflict, read again, merge, and retry with the
> current version and a new request ID. No source Run or global inbox read is
> needed.

When MCP is unavailable, provide `item get <item-id> --json` and
`item work <item-id> --file work-update.json`, or the equivalent HTTP endpoints
`GET /api/v1/items/{id}` and `PATCH /api/v1/items/{id}/work`. If the executor has
no aicp access, ask it to return the result through its original channel for
the primary to import.

## Work update

MCP `update_item_work` accepts this envelope; CLI/HTTP accepts just `update`:

```json
{
  "item_id": "ITEM_ID",
  "update": {
    "request_id": "UNIQUE_WRITE_ID",
    "expected_content_version": 3,
    "report": {
      "schema_version": 1,
      "body_md": "Preserved current report followed by the new result."
    },
    "delegations": [{
      "id": "DELEGATION_ID",
      "status": "pending",
      "context_md": "Preserved continuation instructions. Input v2: assessed the three Copy requirements; default to brief button feedback; clipboard availability remains an assumption to validate."
    }]
  }
}
```

Read before constructing this payload; replace its IDs, versions, and text.
Report and context Markdown replace complete fields, so merge current text
before writing. A supplied report replaces the entire report object: retain
its existing `actions` as well as its Markdown. Delegation patches merge by ID;
omitting `external_ref`, executor, and instructions preserves them even if the
primary changed them.
Omit report or context if no change is needed. Do not set the user's Todo.

On a version conflict, rebuild from the current Item rather than just replacing
the version number. On an uncertain network outcome, retry the identical
payload with the same request ID; a changed payload gets a new request ID.

## Primary-agent continuation

Use `list_items` with `delegation_status: "pending,blocked"`, paging through
the bounded results. Read the matching Item and delegation, check the returned
evidence, and request repair through its persisted continuation reference if
needed. Reopen a closed delegation with the same ID before assigning rework;
ask for bounded additions that preserve earlier conclusions. If the original
session is unavailable, explain why and retain its executor/reference in
context before transferring. Close only after checking and persisting the
supported result; closing delegated work is independent of the user's Todo.
