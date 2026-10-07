# aicp

**A local workspace for what your AI agent finds—and what you choose to do next.**

Tell your agent what matters, give it specific places to look, and review the
results in one place. aicp keeps findings, source links, follow-ups, and your
notes together, so the next visit can build on the last one.

![aicp Attention workspace showing a launch readiness report, a queue of customer and service findings, and follow-up controls](docs/images/aicp-attention.png)

*The running app with fictional Northstar product-team data. Select a finding
on the left to read its evidence and decide on a follow-up. [Explore the demo](docs/showcase.md).*

## What you can do

- **Keep up with what matters.** Track launch blockers, customer feedback,
  project updates, or service health against your own priorities.
- **Review findings with context.** Read a report, see why it matters, and
  follow its source links. Later evidence updates the same continuing matter.
- **Choose your next step.** Acknowledge a finding, add a Todo, set a reminder,
  or leave a note. Agent updates preserve your follow-up state.
- **Keep useful knowledge.** Search retained items, revisit reports, and add
  your own notes or instructions to the inbox.
- **See what was actually checked.** Monitoring shows source coverage and
  limitations; Activity shows completed and interrupted inspections.

For example, an agent can review your project updates and customer notes,
notice that a promised feature has no owner, and save a launch-risk report.
You can mark it as a Todo and set a reminder. When the source changes, the
agent can update the report while keeping your follow-up in place.

## Get started

Download the archive for your operating system from
[Releases](https://github.com/zhuochun/control-plane/releases), verify it with
`checksums.txt`, and extract it. On Windows, run:

```powershell
.\aicp.exe init
.\aicp.exe serve
```

Keep the server terminal open and visit <http://127.0.0.1:7331>.
On macOS or Linux, use `./aicp` in place of `.\aicp.exe`.

1. In **Preferences**, describe your priorities and working context.
2. In **Monitoring**, add an **Interest** (what matters to you) and a
   **Watcher** (a specific source your agent should inspect).
3. Connect your agent and ask it to complete a first inspection.
4. Open **Attention** to review the results and choose your follow-ups.

The [getting-started guide](GETTING_STARTED.md) walks you and your agent through
setup, a first inspection, and later visits. You can also add personal items
before connecting any sources.

## Bring your agent

Your agent accesses sources through its existing tools—for example, GitHub,
Slack, email, or a browser—and saves findings through aicp's CLI or MCP interface.
Both use the same local server as the portal.

For Codex, register the executable using your installation's absolute path:

```powershell
codex mcp add aicp -- C:\absolute\path\aicp.exe mcp --server http://127.0.0.1:7331
```

Start `aicp serve` first. Ask the connected agent to read the brief, help configure
your priorities and sources, then inspect the selected sources and publish its
findings. The [agent workflow reference](docs/agent-workflow.md) explains the
run contract and [heartbeat prompt](examples/heartbeat-prompt.md).

For recurring inspections, use an external scheduler or agent automation;
[the scheduling examples](examples/scheduling.md) show how. A Watcher records
when a source is due; aicp does not launch or schedule agents itself.

## How it fits together

| In aicp | What it means for you |
| --- | --- |
| **Interest** | A priority and instructions for recognizing what matters |
| **Watcher** | A bounded source, inspection cadence, and progress checkpoint |
| **Item** | A finding, report, task, outcome, or personal note worth retaining |
| **Attention** | Items with new evidence, an open Todo, or a due reminder |
| **Run** | A recorded inspection, including coverage and limitations |
| **Proposal** | A suggested monitoring change for you to accept or reject |

One source can serve several Interests, and one Item can matter to several
priorities. See the [glossary](docs/glossary.md) for the detailed vocabulary.

## Local storage and control

aicp is one executable with a built-in web portal and SQLite database. It
listens on `127.0.0.1:7331`. Data defaults to your operating system's user
configuration directory under `control-plane`; use `AICP_DATA_DIR` or
`serve --data-dir <directory>` to choose another location.

Source credentials stay with your agent's tools. What the agent sends to aicp
is stored locally; access to external sources and any model processing follow
your agent's configuration. Reminders resurface items in the portal.

To back up, stop the server and copy the **whole data directory**. Restore it
while the server is stopped. Copying only a live `.db` file can miss SQLite WAL
changes. aicp refuses unsupported newer database schemas rather than resetting
them. [Local hostname setup](docs/local-hostname.md) covers `aicp.localhost`.

## Try the demo or build from source

The [showcase guide](docs/showcase.md) recreates the screenshot with seven
fictional findings, three priorities, and three source Watchers in a separate
workspace. No source accounts are needed.

For development, install Node.js 24 and the Go version pinned in `go.mod`:

```powershell
npm --prefix web ci
npm --prefix web run build
go build -trimpath -o dist/aicp.exe ./cmd/aicp
.\dist\aicp.exe init
.\dist\aicp.exe serve
```

The installed executable does not need Node.js. For testing, packaging, and
metric simulations, see [development and verification](docs/development.md).
Implementation owners and focused checks are in the
[architecture map](docs/architecture.md); Item-local agent assignments are
covered by the [delegation handoff guide](examples/delegation-handoff.md).
