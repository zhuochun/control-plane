// From web/: node perf/heartbeat.mjs --binary ../dist/aicp.exe --data-dir NEW_SEEDED_DIR --output FILE
import { spawn } from "node:child_process";
import { once } from "node:events";
import { stat, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { performance } from "node:perf_hooks";

const pairs = process.argv.slice(2);
const args = Object.fromEntries(pairs.reduce((result, entry, index) => { if (index % 2 === 0) result.push([entry, pairs[index + 1]]); return result; }, []));
if (!args["--binary"] || !args["--data-dir"] || !args["--output"]) throw new Error("Provide --binary, --data-dir, and --output");
const binary = resolve(args["--binary"]);
const directory = resolve(args["--data-dir"]);
const output = resolve(args["--output"]);
await stat(binary); await stat(directory); await stat(resolve(directory, "perf-manifest.json"));
const base = "http://127.0.0.1:7331";
try {
  const response = await fetch(`${base}/healthz`, { signal: AbortSignal.timeout(1000) });
  if (response.ok) throw new Error("Port 7331 is already occupied");
} catch (error) { if (error.message?.includes("already occupied")) throw error; }
let server = spawn(binary, ["serve", "--data-dir", directory], { stdio: "ignore", windowsHide: true });
async function waitReady(child) {
  for (let i = 0; i < 100; i++) {
    if (child.exitCode !== null) throw new Error(`aicp exited with ${child.exitCode}`);
    try { if ((await fetch(`${base}/healthz`, { signal: AbortSignal.timeout(500) })).ok) return; } catch { /* starting */ }
    await new Promise((done) => setTimeout(done, 100));
  }
  throw new Error("aicp did not become ready");
}
try {
  const observations = [];
  const startupStart = performance.now();
  await waitReady(server);
  observations.push({ name: "startup_ready", milliseconds: Math.round(performance.now() - startupStart) });
  async function call(name, method, path, body) {
    const start = performance.now();
    const response = await fetch(`${base}/api/v1${path}`, { method, headers: body ? { "Content-Type": "application/json" } : undefined, body: body ? JSON.stringify(body) : undefined });
    const raw = await response.text();
    observations.push({ name, milliseconds: Math.round(performance.now() - start), bytes: Buffer.byteLength(raw), status: response.status });
    if (!response.ok) throw new Error(`${name}: ${response.status} ${raw}`);
    return JSON.parse(raw);
  }
  await call("status", "GET", "/status");
  await call("items_first_page", "GET", "/items?view=all&limit=50");
  const todos = await call("todos_first_page", "GET", "/items?view=todo&limit=50");
  if (todos.items.length === 0) throw new Error("Fixture has no Todo Items");
  await call("brief", "GET", "/brief");
  const known = new Map();
  let protectedID;
  for (let cycle = 1; cycle <= 3; cycle++) {
    const contentCycle = Math.min(cycle, 2);
    const startBody = { request_id: `perf-start-${cycle}`, runner_label: "perf-fixture" };
    if (cycle > 1) {
      startBody.watch_ids = [...known.keys()];
      startBody.force = true;
    }
    const started = await call(`start_run_${cycle}`, "POST", "/runs", startBody);
    const watches = started.context.watches;
    if (watches.length === 0) throw new Error("Fixture selected no due Watchers");
    const mixedReads = cycle === 2 ? Promise.all([0, 1].map(async (reader) => {
      for (let i = 0; i < 10; i++) await call("mixed_read", "GET", reader === 0 ? "/status" : "/items?view=attention&limit=50");
    })) : null;
    for (const watch of watches) {
      const interest = watch.interests[0]?.id;
      if (!interest) throw new Error("Selected Watcher has no Interest");
      const current = known.get(watch.id) ?? [];
      const items = Array.from({ length: 5 }, (_, index) => ({
        dedupe_key: `perf:${watch.id}:${index}`,
        expected_content_version: cycle - 1,
        kind: "note",
        title: `Measured finding ${index}`,
        summary: `Cycle ${contentCycle}: relevant source update ${index}`,
        interests: [{ id: interest, reason: "Fixture evidence" }],
        sources: [{ id: "source", url: `https://example.com/perf/${watch.id}/${index}`, label: "Fixture source", observed_at: "2026-09-28T00:00:00Z" }],
        report: { schema_version: 1, body_md: `## Finding\n\nCycle ${contentCycle} contains the source-backed update.` },
      }));
      if (cycle > 1 && current.length !== items.length) throw new Error("First cycle did not publish all Items");
      const result = await call(`submit_${cycle}`, "PUT", `/runs/${started.run.id}/watches/${watch.id}/findings`, {
        request_id: `perf-submit-${cycle}-${watch.id}`,
        expected_watch_revision: watch.revision,
        status: "success",
        coverage: { cursor_before: watch.cursor ?? null, cursor_after: { cycle }, observed_through: "2026-09-28T00:00:00Z", limitations: [] },
        items,
      });
      if (result.items.length !== 5 || result.items.some((entry) => entry.content_version !== contentCycle || (cycle === 3 && entry.changed))) throw new Error("Publication versions were incorrect");
      known.set(watch.id, result.items.map((entry) => entry.id));
    }
    await call(`finish_run_${cycle}`, "POST", `/runs/${started.run.id}/finish`, { request_id: `perf-finish-${cycle}`, summary: "Fixture findings submitted." });
    if (mixedReads) await mixedReads;
    if (cycle === 1) {
      protectedID = [...known.values()][0][0];
      await call("set_todo", "POST", `/items/${protectedID}/actions`, { request_id: "perf-todo", expected_state_version: 1, action: { type: "set_todo", state: "todo" } });
      await call("set_note", "PUT", `/items/${protectedID}/note`, { request_id: "perf-note", expected_state_version: 2, user_note: "Keep this local follow-up." });
    }
  }
  for (const ids of known.values()) {
    const item = await call("verify_item", "GET", `/items/${ids[0]}`);
    if (item.content_version !== 2 || item.sources.length !== 1) throw new Error("Item update lost version or source");
  }
  const protectedItem = await call("verify_user_state", "GET", `/items/${protectedID}`);
  if (protectedItem.todo_state !== "todo" || protectedItem.user_note !== "Keep this local follow-up." || protectedItem.state_version !== 3) throw new Error("Publication changed user-owned state");
  const interrupted = await call("start_interrupted", "POST", "/runs", { request_id: "perf-interrupted", runner_label: "perf-interrupted", watch_ids: [...known.keys()], force: true });
  server.kill();
  if (server.exitCode === null) await once(server, "exit");
  const restartStart = performance.now();
  server = spawn(binary, ["serve", "--data-dir", directory], { stdio: "ignore", windowsHide: true });
  await waitReady(server);
  observations.push({ name: "restart_ready", milliseconds: Math.round(performance.now() - restartStart) });
  const recovered = await call("recovered_status", "GET", "/status");
  if (recovered.health.active_run?.id !== interrupted.run.id) throw new Error("Restart lost active Run");
  const abandoned = await call("abandon_interrupted", "POST", `/runs/${interrupted.run.id}/abandon`, { request_id: "perf-abandon", reason: "Fixture runner stopped during inspection" });
  if (abandoned.status !== "failed") throw new Error("Interrupted Run was not abandoned");
  const finalStatus = await call("final_status", "GET", "/status");
  if (finalStatus.health.active_run !== null) throw new Error("Abandoned Run remained active");
  await writeFile(output, JSON.stringify({ binary, data_dir: directory, measured_at: new Date().toISOString(), observations }, null, 2) + "\n", "utf8");
  for (const entry of observations) console.log(`${entry.name}: ${entry.milliseconds}ms${entry.bytes === undefined ? "" : ` (${entry.bytes} bytes)`}`);
  console.log(`JSON: ${output}`);
} finally { server.kill(); }
