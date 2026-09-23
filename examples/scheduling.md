# External heartbeat scheduling

Keep `aicp serve` running independently, then invoke one checked-in wrapper every
two hours. The wrapper uses an OS file lock, so overlap is a successful skip.
Redirect standard output and error in the scheduler to retain agent and setup
failures. A source or harness failure must be reported as a failed run, never as
an empty successful scan.

On Windows, create a Task Scheduler action using absolute paths:

```text
powershell.exe -NoProfile -ExecutionPolicy Bypass -File C:\path\to\aicp\examples\heartbeat.ps1
```

Set its **Start in** directory to the repository and redirect output through a
small site-specific wrapper if persistent logs are needed. The account running
the task must already have a working noninteractive `codex` configuration.

On a system with `flock`, a cron entry can be:

```cron
0 */2 * * * /absolute/path/to/aicp/examples/heartbeat.sh >>$HOME/.local/state/aicp-heartbeat.log 2>&1
```

The wrappers accept configurable absolute paths through parameters on Windows
and `AICP_PROJECT_DIR`, `AICP_PROMPT_PATH`, and `CODEX_BIN` on Unix.
