# Vim-style keyboard shortcuts

- Date: 2026-10-09
- Status: Implemented; Windows core verification and independent code review passed.
- Authority: The owner requested Vim-style shortcuts and then a specification.
  The owner subsequently authorized implementation, verification, and independent
  code review. Deferred features remain outside that authority.
- Related contracts: [glossary](../glossary.md),
  [user input processing](20261008-user-input-processing-spec.md), and
  [review answers](20261004-item-review-formats-and-user-answers-spec.md).

## Outcome and scope

A keyboard user can read a queue, add context, acknowledge evidence, and manage
follow-up without repeatedly moving between the mouse and keyboard. The primary
flow is `j/k` -> read -> `i` -> edit -> `Esc` -> `r` -> `j`.

The first slice covers Attention, Todos, and All items, including their reader,
search, Add to inbox, reminder dialog, shortcut help, and a shortcut preference.
It includes the same Item actions on the standalone Item detail route, except
queue navigation, which has no queue there. Mouse controls and ordinary keyboard
access remain available with the mode on or off.

The owner's requested anchors are `j/k` for next/previous Item and `i` followed
by `Esc` for editing and saving a note. `r` was suggested for Mark seen. The
remaining bindings, opt-in default, and detailed semantics below were proposed
for owner review and accepted through the subsequent implementation request.

## Current behavior and evidence

Code inspected on 2026-10-09:

- [Workspace](../../web/src/workspace.tsx) owns filtered, sorted, grouped,
  paginated queues, selection, and Item-local note drafts. It keeps a selected
  report open when acknowledgement removes its row from Attention.
- [Reader](../../web/src/item-reader.tsx) and
  [standalone detail](../../web/src/items.tsx) expose note saving, Mark seen,
  Todo changes, and reminders through existing mutations.
- [Portal](../../web/src/main.tsx) owns navigation, search, and Add to inbox.
- [Preferences](../../web/src/preferences.tsx) is the existing settings surface.

The working tree contains ongoing user-input-processing changes. This document
does not certify their completion or treat the related specification's older
current-behavior table as a fresh runtime observation. No rendered UI or live
keyboard session was inspected for this specification.

Preserve the existing domain rules: Mark seen acknowledges the viewed content
version; it does not clear Todos or reminders. Todo and reminder operations also
acknowledge viewed content according to their existing contracts. Done clears
the reminder. Saving a note does not imply Mark seen, Done, review submission,
or successful agent processing.

## Key map

Bindings apply only when Vim shortcuts are enabled and the context permits them.

| Key | Context | Behavior |
| --- | --- | --- |
| `j` / `k` | Item workspace | Open next / previous Item in displayed queue order. |
| `i` | Open, loaded Item | Focus Your note and place the cursor at the end without replacing text. |
| `Esc` | Your note | Save changed text and leave editing after success; unchanged text exits immediately. |
| `Ctrl+Enter` / `Cmd+Enter` | Your note | Save changed text while keeping editing focus. |
| `r` | Open, loaded Item | Mark the displayed content version seen; already seen is a no-op. |
| `t` | Open, loaded Item | Add Todo when absent, or reopen Done; an existing open Todo is a no-op. |
| `d` | Open, loaded Item with an open Todo | Mark Done, including existing reminder-clearing semantics. Otherwise no-op. |
| `s` | Open, loaded Item | Open the existing reminder dialog; do not set a time immediately. |
| `/` | Supported workspace or Item detail | Focus search and select its text; search runs on ordinary form submission. |
| `n` | Supported workspace or Item detail | Open the existing Add to inbox dialog. |
| `?` | Supported workspace or Item detail | Open shortcut help. |
| `Esc` | Menu or dialog | Close the topmost dismissible overlay using its existing pending/dirty-data rules; return focus to its opener. |
| `Esc` | Search | Leave search and return focus to the originating workspace or reader without submitting or changing the query. |
| `Esc` | Reader on a narrow layout | Return to the queue and focus the selected row, or queue heading if that row is gone. |

On a wide layout, `Esc` in ordinary reader content is a no-op. It must not close
the report or discard a draft. Note editing and overlays take precedence over
the narrow-layout Back behavior: one press performs one contextual action.

The note save bindings are active only in this opt-in mode in the first slice.
Save note remains usable normally. Other editable forms, including structured
review answers and inbox capture, retain their existing behavior; `Esc` must
not submit them. Deferred bindings include `g a/g t/g l`, `h/l`, `o`, bulk
actions, proposal decisions, arbitrary report actions, and undo. No Vim counts,
operators, key sequences, or text-editing emulation are required.

## Queue navigation and continuity

`j/k` uses rendered Item rows in visual order after filtering, sorting, and
grouping. Group headings, proposals, and other controls are not Items. Only
expanded groups participate; navigation does not silently expand groups.
With no selection, `j` opens the first rendered Item and `k` the last. An empty
queue is a no-op. There is no wraparound.

Selection opens the report, resets its content scroll to the start, and keeps
the selected row visible. Keyboard focus goes to the reader's focusable content
region, with a visible focus indicator. Navigation never marks an Item seen,
saves notes, submits reviews, or changes Todo/reminder state. Narrow layouts
open the reader using the same selected Item and focus behavior.

At the last loaded row, `j` does not silently fetch another page. If more pages
exist, show a nonintrusive status: "More items available. Load more items to
continue." The existing Load more control remains keyboard accessible. `k` at
the beginning is a no-op. Holding a navigation key may repeat within loaded rows.

Background refresh must not replace the selected Item. If the selected row
disappears, keep its report and remember its last rendered order for continuation.
For each direction, choose the nearest still-visible Item in that remembered
order. If none survives in that direction, stay on the current report. Newly
arrived rows do not displace that continuation. After selecting a visible row,
subsequent navigation uses the current rendered order. Explicit changes to
search, filters, sort, or workspace reset this remembered order and use the
resulting queue's normal selection rules.

Example: rows A, B, C; B is open. `r` removes B from Attention without closing
it. `j` opens C; `k` from the retained B opens A. If C also disappears before
`j`, select the next surviving successor from the remembered order, or stay if
there is none. Never guess a replacement solely from B's old numeric index.

Item note drafts remain attached to Item identity within the mounted workspace,
including across `j/k` and narrow-layout Back. This change does not introduce
draft persistence across reloads or route unmounts. Dirty notes are saved only
by an explicit save action. No automatic save on selection, blur, or navigation.

## Note editing, saving, and recovery

The keyboard path invokes the same note operation as Save note, preserving
version checks, request identity/replay semantics, and input-processing rules.
It does not introduce a second submission or overwrite path.

| State / event | Required result |
| --- | --- |
| `i` with no dirty draft | Edit the current saved note. |
| `i` with an Item-local draft | Resume that draft, without refreshing over it. |
| Save with text identical to the current saved note | No mutation or new input revision. `Esc` exits; Ctrl/Cmd+Enter stays. |
| Changed text, including clearing a nonempty note | Submit the exact draft through the existing note operation. |
| Save pending | Show Saving; suppress duplicate saves and Item mutations for this Item; do not navigate via shortcuts. In Vim mode keep the submitted text visible and prevent editing until this attempt resolves. With Vim mode off, preserve existing ordinary editing during save; completion clears only the submitted draft and retains any newer text. |
| `Esc` save succeeds | Show Note saved, clear only the submitted draft, and return focus to this Item's reader content. Do not move to the next Item. |
| Ctrl/Cmd+Enter save succeeds | Show Note saved and retain note focus. |
| Save fails or conflicts | Retain the draft, keep note focus, show the existing error/recovery controls. No success message or navigation. |
| Refresh reveals another saved note while a draft exists | Retain the local draft; rely on the owning note conflict contract, without weakening version checks or automatically overwriting the remote note. |

Pointer navigation, Tab navigation, and route changes remain available during
a save. The focus outcomes above apply only while the originating Item and
note editor remain active; completion must never steal focus from another Item,
route, or control the user has since focused. Resolve the request against its
originating Item only. Within the mounted workspace, success clears only that
Item's submitted draft and failure retains it for re-entry. Route unmounts keep
the existing draft-lifetime limits; this slice adds no durable draft storage.

IME composition has priority: `Esc` and Enter used to compose or dismiss an
input-method candidate must not trigger note saving or navigation. Pending
requests cannot be cancelled by `Esc`. A refresh or uncertain response must
not cause automatic resubmission; retry retains the owning request-replay rules.

## Activation, focus, and discoverability

Provide a Preferences control named **Vim keyboard shortcuts**, off by default,
with a brief description and help entry. Persist this browser-local preference
across reloads; it does not belong to agent instructions, server-wide settings,
or other browsers. If local preference storage is unavailable, the choice works
for the current page session. The off state disables the entire shortcut layer,
including note-specific save bindings; existing control behavior remains intact.

With the mode enabled, workspace/Item commands work when focus is outside
editable controls. Ignore events from inputs, textareas, selects, contenteditable
regions, and composite controls that own their keys, except search `Esc` and
the explicit note save bindings. Search keeps native behavior for all other
keys. An open overlay suspends underlying commands;
native/component keyboard behavior remains authoritative inside it.

Ignore composition events and Ctrl/Meta/Alt combinations except the defined
note-save chord. Do not intercept browser shortcuts, ordinary Tab/Shift+Tab,
Enter/Space activation of controls, or native reader scrolling. The `?` binding
uses the resulting character, allowing Shift where needed to produce it.
Suppress repeated keydown for mutations, editing entry, and dialog opening;
holding `r`, `t`, `d`, `s`, `i`, or `n` produces at most one action until release.

An Item command targets the Item actually displayed, not a merely focused row
or a new Item awaiting loading. It is disabled while detail is loading, failed,
or unavailable, and while an Item mutation or note save is pending. Act on the
displayed version; stale-version rejection retains existing refresh/retry
behavior. Shortcut dispatch must not add automatic mutation retries.

Help lists applicable bindings, mode status, and the note-specific meaning of
`Esc`. Keep help accessible by a visible control even when shortcuts are off.
Show key hints beside relevant actions while enabled, including "Esc: save and
exit" by Your note. Announce save and mutation results without requiring color
or a focus jump. Existing buttons remain the accessible alternative.

The off switch addresses character-shortcut activation concerns described in
[W3C character key shortcut guidance](https://www.w3.org/WAI/WCAG22/Understanding/character-key-shortcuts.html).
This design does not claim accessibility conformance or observed usability.

## Acceptance claims

| ID | Observable claim and representative boundary |
| --- | --- |
| CHG-01 — Explicit mode | New browser starts with shortcuts off; enabling survives reload; disabling restores ordinary key behavior. |
| CHG-02 — Ordered navigation | `j/k` follows visible rows across expanded groups, never wraps, fetches implicitly, or acknowledges an Item. |
| CHG-03 — Retained selection | Mark seen or a refresh can remove B from A/B/C while B remains readable; continuation reaches surviving A/C in the requested direction. |
| CHG-04 — Save and exit | `i`, edit, `Esc` produces one normal note submission and returns focus only after success; unchanged text produces none. |
| CHG-05 — Draft recovery | Failed/conflicting saves preserve the exact draft and focus; moving among Items preserves Item-local drafts within the mounted workspace. |
| CHG-06 — Intentional mutations | `r` marks viewed evidence seen; `t` never completes Todo; `d` only completes an open Todo and follows reminder clearing; `s` only opens a dialog. |
| CHG-07 — Input isolation | Typing shortcuts in search, review forms, inbox, or IME composition causes no underlying Item command; note Esc does not close the narrow reader. |
| CHG-08 — Pending and repeat isolation | Held mutation keys, duplicate save presses, and loading/pending detail cannot create extra mutations or target a different Item. |
| CHG-09 — Equivalent state contracts | Keyboard and visible controls share version fencing, note/input retention, request replay, and existing error behavior across workspace and standalone detail. |
| CHG-10 — Focus and discoverability | Help is reachable while off; selection, successful save, dialog dismissal, and narrow Back leave visible, predictable focus; ordinary keyboard access works in both modes. |

## Affected surfaces and readiness

Portal navigation/search/capture, workspace selection and drafts, both Item
reading surfaces, reminders, Preferences, and help are affected. HTTP/CLI/MCP
domain semantics remain owned by their existing contracts; this slice adds no
new agent command, processing obligation, or source execution permission.

The smallest delivery is the full first-slice key map with activation, focus,
help, pending isolation, and note recovery. Deferred bindings are independent
follow-ons. No custom remapping, Vim editor, bulk approval, new undo mechanism,
or cross-browser settings synchronization is included.

The implementation uses one portal-level shortcut dispatcher. The workspace
owns selection and remembered visible order. A mounted workspace or standalone
detail owns Item-keyed note-edit state: draft, base note/version, pending save,
and exact retry request. Reader remounts do not discard that state. Keyboard and
visible controls share Item mutation operations and version/replay semantics.

Disposition: implemented and verified on Windows on 2026-10-09. The core gate,
`scripts/verify.ps1` with isolated `AICP_TEST_PORT=17331`, passed 137 indexed
cases: 87 Go cases, 48 browser tests, and quality/type checks. Go package cache
reuse is identified in the runner evidence; browser tests executed fresh.
The browser set includes 11 shortcut regression cases. The two renderer-profile
cases are outside the core gate and were not run. Independent code review passed
after fixes for live announcements, narrow search visibility, and ordinary-mode
pending editing. Keyboard saving preserves the committed input processing
contract. Automated checks do not establish observed usability or
assistive-technology conformance. No release or deployment was performed.
