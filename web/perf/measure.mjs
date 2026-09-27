// From web/: node perf/measure.mjs --binary ../dist/aicp.exe --data-dir PATH --output PATH
import { chromium } from "@playwright/test";
import { spawn } from "node:child_process";
import { readFile, writeFile, stat } from "node:fs/promises";
import { resolve } from "node:path";
import { performance } from "node:perf_hooks";

const args = Object.fromEntries(process.argv.slice(2).reduce((pairs, part, i, all) => {
  if (i % 2 === 0) pairs.push([part, all[i + 1]]);
  return pairs;
}, []));
const binary = resolve(args["--binary"] ?? "../dist/aicp.exe");
const directory = resolve(args["--data-dir"] ?? "");
const output = resolve(args["--output"] ?? "perf-results.json");
const iterations = Number(args["--iterations"] ?? 5);
if (!args["--data-dir"] || !Number.isInteger(iterations) || iterations < 1) throw new Error("Provide --data-dir and a positive --iterations");
await stat(binary);
await stat(directory);
const manifest = JSON.parse(await readFile(resolve(directory, "perf-manifest.json"), "utf8"));
const itemCount = manifest.items;
if (!Number.isInteger(itemCount) || itemCount < 1) throw new Error("Invalid fixture manifest");

const base = "http://127.0.0.1:7331";
try {
  const response = await fetch(`${base}/healthz`, { signal: AbortSignal.timeout(1000) });
  if (response.ok) throw new Error("Port 7331 is already occupied; refusing to measure another server");
} catch (error) {
  if (error.message?.includes("already occupied")) throw error;
}

const server = spawn(binary, ["serve", "--data-dir", directory], { stdio: "ignore", windowsHide: true });
let browser;
try {
  let ready = false;
  for (let i = 0; i < 100; i++) {
    if (server.exitCode !== null) throw new Error(`aicp exited with ${server.exitCode}`);
    try {
      const response = await fetch(`${base}/healthz`, { signal: AbortSignal.timeout(500) });
      if (response.ok) { ready = true; break; }
    } catch { /* not listening yet */ }
    await new Promise((done) => setTimeout(done, 100));
  }
  if (!ready) throw new Error("aicp did not become ready");
  browser = await chromium.launch({ headless: true });
  const observations = [];
  for (let iteration = 0; iteration < iterations; iteration++) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    const page = await context.newPage();
    page.setDefaultTimeout(120000);
    const failures = [];
    page.on("requestfailed", (request) => failures.push(`${request.method()} ${request.url()}: ${request.failure()?.errorText}`));
    async function measure(name, action) {
      await page.evaluate(() => performance.clearResourceTimings());
      const start = performance.now();
      try { await action(); }
      catch (error) {
        console.error(`${name} failed: ${(await page.locator("body").innerText()).slice(0, 1200)}`);
        throw error;
      }
      const milliseconds = performance.now() - start;
      const resources = await page.evaluate(() => performance.getEntriesByType("resource").filter((entry) => entry.name.includes("/api/v1/")).map((entry) => ({
        path: new URL(entry.name).pathname, duration_ms: Math.round(entry.duration), transfer_bytes: entry.transferSize,
      })));
      const heap = await page.evaluate(() => performance.memory?.usedJSHeapSize ?? null);
      observations.push({ iteration, name, milliseconds: Math.round(milliseconds), heap_bytes: heap, api: resources, failures: [...failures] });
      if (failures.length) throw new Error(failures.join("\n"));
    }
    await measure("attention", async () => {
      await page.goto(base, { waitUntil: "domcontentloaded" });
      await page.getByRole("heading", { name: "Attention" }).waitFor();
      await page.locator(".ledger-row").first().waitFor();
    });
    await measure("library", async () => {
      await page.goto(`${base}/library`, { waitUntil: "domcontentloaded" });
      await page.locator(".ledger-row").last().waitFor();
    });
    if (await page.getByRole("button", { name: "Load more items" }).isVisible()) {
      await measure("library_next_page", async () => {
        await page.getByRole("button", { name: "Load more items" }).click();
        await page.waitForFunction(() => document.querySelectorAll(".ledger-row").length > 100);
      });
    }
    await measure("library_search", async () => {
      await page.getByRole("textbox", { name: "Search items, metrics, or notes…" }).fill(`Item ${String(itemCount - 1).padStart(6, "0")}`);
      await page.locator(".workspace-count").filter({ hasText: /^1$/ }).waitFor();
    });
    await measure("kind_filter", async () => {
      await page.getByRole("textbox", { name: "Search items, metrics, or notes…" }).fill("");
      await page.getByLabel("Filter by kind").selectOption("task");
      await page.locator(".ledger-row").first().waitFor();
    });
    await measure("interest_filter", async () => {
      await page.getByLabel("Filter by kind").selectOption("");
      const interest = await page.getByLabel("Filter by Interest").locator("option").nth(1).getAttribute("value");
      if (!interest) throw new Error("Fixture has no Interest filter option");
      await page.getByLabel("Filter by Interest").selectOption(interest);
      await page.locator(".ledger-row").first().waitFor();
    });
    await measure("item_inspector", async () => {
      await page.locator(".ledger-row").first().click();
      await page.locator(".inspector-updated").waitFor();
    });
    await measure("todo", async () => {
      await page.goto(`${base}/todos`, { waitUntil: "domcontentloaded" });
      await page.locator(".ledger-row").first().waitFor({ timeout: 10000 });
    });
    await measure("monitoring", async () => {
      await page.goto(`${base}/interests`, { waitUntil: "domcontentloaded" });
      await page.locator("[id^='interest-']").first().waitFor();
    });
    await measure("activity", async () => {
      await page.goto(`${base}/activity`, { waitUntil: "domcontentloaded" });
      await page.locator(".activity-panel .run-row").first().waitFor();
    });
    await context.close();
  }
  const report = { binary, data_dir: directory, iterations, measured_at: new Date().toISOString(), observations };
  await writeFile(output, JSON.stringify(report, null, 2) + "\n", "utf8");
  for (const name of [...new Set(observations.map((entry) => entry.name))]) {
    const values = observations.filter((entry) => entry.name === name).map((entry) => entry.milliseconds).sort((a, b) => a - b);
    const pick = (p) => values[Math.min(values.length - 1, Math.ceil(values.length * p) - 1)];
    console.log(`${name}: p50=${pick(.5)}ms p95=${pick(.95)}ms n=${values.length}`);
  }
  console.log(`JSON: ${output}`);
} finally {
  if (browser) await browser.close();
  server.kill();
}
