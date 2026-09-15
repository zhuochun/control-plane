#!/bin/sh
set -eu

project_dir=${AICP_PROJECT_DIR:-"$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"}
prompt_path=${AICP_PROMPT_PATH:-"$project_dir/examples/heartbeat-prompt.md"}
codex_bin=${CODEX_BIN:-codex}
lock_path=${AICP_HEARTBEAT_LOCK:-"${TMPDIR:-/tmp}/aicp-heartbeat.lock"}

cd "$project_dir"
exec 9>"$lock_path"
if ! flock -n 9; then
  echo 'aicp heartbeat already running; skipped'
  exit 0
fi
"$codex_bin" exec - < "$prompt_path"
