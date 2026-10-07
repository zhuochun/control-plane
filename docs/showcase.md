# A product-team demo

This demo uses **fictional Northstar data** to show a typical return visit:
review a launch decision, scan customer feedback and service changes, then
choose what to follow up. Names, metrics, source links, and inspection results
are illustrative. No external systems or accounts are accessed.

![Attention queue and launch readiness report in aicp](images/aicp-attention.png)

The workspace contains seven Items across three Interests:

| Interest | Findings |
| --- | --- |
| Launch readiness | Launch review with an unresolved owner; rollback rehearsal |
| Customer onboarding | Repeated setup friction; activation trend; pilot check-in |
| Service health & cost | Search latency recovery; cost per active workspace |

Three Items have open Todos, and the launch review has a next-day reminder.
The selected report includes a recommendation, workstream table, decision,
source reference, and Interest reason. The Watchers and Activity record a
completed fixture-backed inspection with an explicit demo limitation.

## Recreate the workspace

Build the executable as described in the [README](../README.md#try-the-demo-or-build-from-source).
Stop your existing server first: aicp uses the fixed loopback port 7331.
From the repository root, start a separate workspace in one terminal:

```powershell
.\dist\aicp.exe serve --data-dir .\test-output\showcase
```

In a second terminal:

```powershell
.\scripts\showcase.ps1
```

The seed script requires an empty workspace and refuses to modify one that
already contains Interests, Watchers, Items, Runs, or Proposals. For a second
fresh demo, choose a different data-directory path. The database stays in the
ignored `test-output/` directory and remains available for later demos.

Open <http://127.0.0.1:7331> and select **October launch: one decision before
rollout** in Attention. Visit Todos, All items, Monitoring, and Activity to
explore the same data. Source URLs use `example.com` as placeholders.

## Refresh the screenshot

Install Playwright's browser if it is not available, then capture the running
demo at a 1440 × 1280 viewport:

```powershell
npx --prefix web playwright install chromium
node scripts/capture-showcase.mjs
```

The capture script selects the launch report, checks that the seven fictional
Items are present, and saves `docs/images/aicp-attention.png`. It captures the
rendered application without changing its styling or substituting content.
Check the saved image before using it in a presentation. Stop the demo server
with Ctrl+C when finished; restart your normal server with its usual data path.
