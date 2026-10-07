# aicp

**Your agent runs the loop. You review what matters.**

aicp is an agent-first control plane for ongoing monitoring, research, and
delegated work. Tell your agent your interests and the sources to look after.
It sets up the workspace, inspects new evidence, investigates or delegates
follow-through, and brings you reports and decisions to review.

![aicp Attention workspace showing a launch readiness report, a queue of customer and service findings, and follow-up controls](docs/images/aicp-attention.png)

*The agent's output, ready for review: source-backed findings, tables, diagrams,
and decisions. The Northstar demo uses fictional source material. [Explore the demo](docs/showcase.md).*

## Get started: ask your agent

Copy this into your agent:

> Install and set up aicp from https://github.com/zhuochun/control-plane.
> Follow its GETTING_STARTED.md, connect yourself through CLI or MCP, and open
> the review portal when ready.

Then give it a purpose:

> Track our October launch and customer onboarding. Watch Slack
> #launch-readiness and customer email from the pilot accounts using your
> existing tools. Keep findings in aicp, investigate and delegate useful
> follow-ups within my authority, and bring me decisions with evidence.

The agent handles installation, owner context, Interests, and Watchers. It
checks source access and completes a first inspection before setting up recurring
checks through its available scheduler. The [getting-started contract](GETTING_STARTED.md)
and [copyable prompts](examples/install-prompt.md) give it the details.

## The agent's working loop

1. **Keep your interests current.** The agent turns your priorities into durable
   instructions and bounded source Watchers.
2. **Inspect and reconcile.** It reads selected Slack channels, email, GitHub,
   documents, or other sources through its existing tools, then updates the same
   continuing Items with evidence and relevance reasons.
3. **Advance the work.** Within your authority, it investigates or delegates
   bounded work. Handoffs, continuation references, results, and remaining
   questions stay with the Item.
4. **Deliver something worth reviewing.** It publishes reports with Markdown,
   tables, charts, diagrams, and structured questions. Material decisions come
   with context and a recommendation.
5. **Continue from your response.** Your answers and notes persist for later
   agent work; your Todo, reminder, and acknowledgement state survives updates.

For example, Slack says SSO is ready, while a customer email makes audit-log
export a pilot condition. The agent joins those facts in a launch Item, delegates
an assessment of the options, and brings you a recommendation, dependency
diagram, and scope decision. You review the work instead of reconstructing it
from several conversations.

## Review the work

Open <http://127.0.0.1:7331> when the agent has results for you. **Attention**
surfaces new evidence and follow-ups. Outcome Items show metric trends;
reports can combine tables, diagrams, and review controls. Filter or search the
retained work, check source links, save an answer, or leave instructions for
the next visit. Monitoring and Activity expose coverage and Run history.

## How the agent connects

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
