# Agent-led onboarding and returning-use journey

- Date: 2026-09-27
- Status: Implemented; acceptance is tracked by application, browser, and package verification.
- Domain language: [glossary](../glossary.md)
- Current behavior: [Watcher–Interest specification](20260924-watcher-interest-model-spec.md)

## Outcome and boundary

A person can ask an agent to install aicp and help set up a useful local control
plane. The agent should learn the product from the distributed binary and its
packaged guidance, then work with the person to record owner context, current
Interests, and bounded Watchers. The person should be able to tell whether setup
was finished, whether inspection has ever happened, and whether it is working
now. Returning people and agents should continue from durable state without
repeating onboarding.

aicp stores configuration, context, findings, user actions, checkpoints, and
history. The external agent owns source inspection with its existing tools;
its harness and scheduler remain outside aicp. Setup, configuration reading,
and editing do not claim a Run or imply that monitoring has begun.

## First use: person and installing agent

1. The person says what they want to manage. The agent obtains the distributed
   binary, reads `aicp --help` and the packaged getting-started guide, and
   explains the four concepts in plain language: an Interest says *why* a
   finding matters; a Watcher says *which bounded source* to inspect; a Run
   records *one actual inspection*; an Item retains *a matter to review or act
   on*. Help and the guide must lead to the same model and distinguish aicp
   from the external agent and scheduler. Source-build instructions are for
   contributors, not the binary-install path.
2. The agent initializes and starts the local server, checks that it responds,
   and opens the portal when useful. aicp seeds sound `AGENTS.md` and `USER.md`
   defaults without replacing prior owner edits. The seeded `USER.md` is a
   starting prompt, not owner context already set. This confirms a working
   local plane, not agent access to sources or a completed inspection.
3. The agent asks for the person's priorities, useful background and preferred
   follow-through. It drafts `USER.md` in the person's words. It then proposes
   one or more current Interests and Watchers, starting with a concrete first
   source when available. The person can revise, approve, defer, or stop each
   part. The agent saves approved configuration directly; no heartbeat is
   started to perform configuration work. Source credentials never belong in
   `USER.md` or aicp configuration.
4. The agent connects its MCP adapter and can read a brief without claiming
   work. When a source tool is available, it performs a first Run, reports one
   truthful terminal result per selected Watcher, and finishes. The person
   reviews the resulting Items and coverage. Scheduling future agent runs is
   a separate choice after this check; a configured cadence alone is not proof
   that a scheduler is running.

An agent can save an owner-reviewed `USER.md` draft with
`aicp config user-context set --file`, MCP `set_user_context`, or the portal.
The direct command updates only owner context without starting a Run.

## The two kinds of guidance

The product's stored `AGENTS.md` is an editable operating note sent with briefs
and Run starts. Its default is useful, and a person may customize it for their
own agent workflow. `USER.md` holds the person's context and priorities.
Interests hold relevance rules; Watchers hold source scopes. The repository's
`AGENTS.md` is developer guidance and is not this stored setting.

Protocol invariants belong to aicp's commands, MCP tool contracts, validation,
and Run packet. Customizing or clearing stored `AGENTS.md` cannot disable
those rules. Help and onboarding should explain the invariant path without
making the editable note a second authority. Reset to default first changes
only the Preferences draft; Save persists it. Reset must use the current
server-provided default and must never overwrite owner text during upgrade.

## Distinguish setup from operation

aicp needs only a derived **setup done** indication, not a persisted onboarding
step, wizard progress, or completion marker. Setup is done when `USER.md` has
nonblank content different from the server's seeded default, at least one active
Interest exists, and at least one active, unexpired Watcher is configured to
assess that Interest (through broad matching or an explicit link). The portal
and agent can show which of these three pieces is missing. A seeded, empty,
or unchanged default `USER.md` does not count as owner context. A Watcher
with no applicable active Interest does not complete setup.

This indication is recomputed from current configuration. If a later edit
removes one of the pieces, the product shows what now needs configuration; it
does not erase history or replay a first-time tour. Operational status is
separate: configured but never inspected, due, running, last success, partial,
or failed.

The UI must not describe an uninspected plane as having nothing that needs
attention. If source access, MCP connection, or scheduling is unverified, say
so without claiming a successful inspection.

With all three pieces present, the product shows **setup done; inspection not
yet verified** until successful coverage arrives. A person can still use
person-created Items with setup incomplete or defer a source; the missing
piece remains visible. The first reported inspection is a separate milestone,
and a failed or partial Run does not count as successful coverage.

## Returning use and recovery

The person opens Attention for findings and actions, Monitoring for each
Watcher's due reason and last coverage, Activity for Run history and interrupted
Run recovery, and Preferences for enduring owner or agent context. Configuration
edits apply to future Run snapshots. Changes to stored guidance do not rewrite
an already active Run's captured context.

An agent returning for a scheduled or explicit inspection receives the current
stored contexts, Interests, Attention summaries, changes, and selected Watcher
snapshots. It follows continuation pages, inspects with external tools, reads an
existing Item before updating it, submits coverage for every selected Watcher,
and finishes only when all have terminal results. Direct configuration requests
remain outside this Run path. The agent can propose changes for the person to
review; it should not treat a proposal as applied configuration.

If setup is interrupted, the next visit reads current context and configuration
to show the missing pieces. If an inspection is interrupted, Activity
shows the active Run and its submitted results; the person can explicitly
abandon it with a reason. Previously committed results and checkpoints remain,
and unreported Watchers remain due.

## Acceptance

Walk a fresh binary-only installation through help, setup, owner-context
review, direct configuration, read-only brief, first inspection, and return.
Verify that the portal derives setup done from owner context, an applicable
active Interest, and an active, unexpired Watcher, independently of inspection
health; that customized contexts survive restart and upgrade; that Reset uses
the server's current default without saving until asked; and that a changed
Interest or guidance does not alter an active Run snapshot. Also walk a
no-source setup as incomplete and an interrupted first Run.

The setup indication and first-inspection guidance appear in Attention; agent
briefs and `aicp doctor` expose the same derived status. No separate onboarding
state or scheduler is introduced.
