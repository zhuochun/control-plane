import path from "node:path";
import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import { performance } from "node:perf_hooks";
import {
  root,
  outputRoot,
  prepare,
  digest,
  files,
  saveJSON,
  npm,
} from "./build.mjs";
import { run } from "./process.mjs";
import { discover, browserInventory } from "./discovery.mjs";
import { features } from "./index.mjs";
import { select, reconcile } from "./selection.mjs";
import { pruneSuccessfulBundles } from "./evidence.mjs";
import { validateJourney } from "./observations.mjs";

const options = {};
for (let i = 2; i < process.argv.length; i++) {
  const key = process.argv[i].replace(/^--/, "");
  if (
    ![
      "level",
      "suite",
      "concern",
      "feature",
      "scenario",
      "profile",
      "plan",
    ].includes(key) ||
    options[key] !== undefined
  )
    throw new Error(`Unknown/duplicate argument: ${process.argv[i]}`);
  options[key] = key === "plan" ? true : process.argv[++i];
}
const started = performance.now();
const stamp = new Date().toISOString().replaceAll(":", "-");
const bundle = path.join(outputRoot, `run-${stamp}-${process.pid}`);
const manifest = {
  owner: "aicp-verification-runner-v1",
  oracle_version: 1,
  invocation: process.argv.slice(2),
  started_at: new Date().toISOString(),
  options,
  status: "not run",
  cases: [],
};
let failure;
async function sourceIdentity() {
  const revision = (
    await run("git", ["rev-parse", "HEAD"], { cwd: root })
  ).stdout.trim();
  const dirty = (await run("git", ["diff", "--binary", "HEAD"], { cwd: root }))
    .stdout;
  const untracked = (
    await run("git", ["ls-files", "--others", "--exclude-standard", "-z"], {
      cwd: root,
    })
  ).stdout
    .split("\0")
    .filter(Boolean);
  return {
    revision,
    dirty_digest: digest(
      dirty +
        untracked
          .sort()
          .map((n) => `${n}:${digest(readFileSync(path.join(root, n)))}`)
          .join("\n"),
    ),
  };
}
try {
  if (!options.plan) manifest.target = await sourceIdentity();
  const cases = await discover();
  const selection = select(cases, options, features);
  manifest.selected = selection.selected.map((c) => ({
    id: c.id,
    level: c.level,
    concerns: c.concerns,
    boundary: c.boundary,
    profile: c.profile,
  }));
  manifest.omitted = selection.omitted.map((c) => c.id);
  manifest.omitted_features = features.filter(
    (f) => !selection.selected.some((c) => c.features.includes(f)),
  );
  manifest.excluded_profiles = selection.excludedProfiles;
  const system = selection.selected.some((c) => c.runner === "browser");
  manifest.discovery_ms = performance.now() - started;
  const preparationStarted = performance.now();
  const prepared = await prepare({ system, plan: options.plan });
  manifest.build = prepared;
  manifest.preparation_ms = performance.now() - preparationStarted;
  const durations = {
    unit: 5,
    component: 10,
    system: options.feature === "reviews" && options.concern === "ui" ? 18 : 15,
  };
  console.log(
    `Selected ${selection.selected.length} cases; omitted ${selection.omitted.length}. Scope: ${options.scenario ?? options.feature ?? options.suite ?? options.level}.`,
  );
  console.log(prepared.decisions.join("\n"));
  if (options.plan) {
    console.log(
      JSON.stringify(
        {
          ...manifest,
          estimated_warm_target_seconds: options.scenario
            ? 20
            : (durations[options.level] ?? null),
          prerequisites: [
            "Go and Node/npm",
            ...(system ? [`Chromium; loopback port ${process.env.AICP_TEST_PORT ?? "7331"} free`] : []),
            ...(options.profile === "renderer"
              ? ["checksum-pinned PlantUML and compatible Java"]
              : []),
          ],
        },
        null,
        2,
      ),
    );
  } else {
    mkdirSync(bundle, { recursive: true });
    manifest.oracles = files(path.join(root, "tests/e2e/contracts"))
      .filter((n) => n.endsWith(".json"))
      .map((n) => ({
        case_id: path.basename(n, ".json"),
        digest: digest(readFileSync(n)),
      }));
    const env = {
      ...process.env,
      AICP_TEST_PROFILE: options.profile ?? "core",
      AICP_TEST_BUNDLE: bundle,
    };
    delete env.AICP_PLANTUML_JAR;
    delete env.AICP_TEST_PLANTUML_JAR;
    delete env.AICP_TEST_BINARY;
    if (options.profile === "renderer") {
      const lock = JSON.parse(
        readFileSync(path.join(root, "scripts/testing/renderer.json"), "utf8"),
      );
      const jar = process.env.AICP_TEST_PLANTUML_JAR;
      if (!jar || digest(readFileSync(jar)) !== lock.sha256)
        throw new Error(
          "Not run: renderer profile requires the pinned JAR (see scripts/testing/renderer.json).",
        );
      const java = await run("java", ["-version"], { cwd: root });
      manifest.renderer = { ...lock, java: (java.stdout + java.stderr).trim() };
      env.AICP_TEST_PLANTUML_JAR = jar;
    }
    const staticGate = !!options.suite;
    if (staticGate) {
      await npm(["run", "typecheck"], { env, echo: true });
      await run(
        "pwsh",
        ["-NoProfile", "-File", path.join(root, "scripts/check-go.ps1")],
        { cwd: root, env, echo: true },
      );
      await run("go", ["vet", "./..."], { cwd: root, env, echo: true });
      if (process.platform === "linux")
        await run("go", ["test", "-race", "./..."], {
          cwd: root,
          env,
          echo: true,
        });
    }
    const packages = new Map();
    for (const c of selection.selected.filter((c) => c.runner === "go")) {
      const pkg = "./" + path.posix.dirname(c.file);
      if (!packages.has(pkg)) packages.set(pkg, []);
      packages.get(pkg).push(c);
    }
    const selectedGo = selection.selected.filter((c) => c.runner === "go");
    const selectedIDs = new Set(selectedGo.map((c) => c.id));
    const selectedNames = new Set(selectedGo.map((c) => c.name));
    const leaks = cases.some(
      (c) =>
        c.runner === "go" &&
        packages.has("./" + path.posix.dirname(c.file)) &&
        selectedNames.has(c.name) &&
        !selectedIDs.has(c.id),
    );
    const batches = leaks
      ? [...packages].map(([pkg, selected]) => [[pkg], selected])
      : packages.size
        ? [[[...packages.keys()], selectedGo]]
        : [];
    for (const [packageNames, selected] of batches) {
      const args = [
        "test",
        "-json",
        ...(selected.some((c) => c.profile === "renderer") ? ["-count=1"] : []),
        "-run",
        `^(${selected.map((c) => c.name).join("|")})$`,
        ...packageNames,
      ];
      const result = await run("go", args, { cwd: root, env });
      writeFileSync(
        path.join(
          bundle,
          packageNames.join("_").replaceAll("/", "_") + ".jsonl",
        ),
        result.stdout,
      );
      const events = result.stdout
        .trim()
        .split("\n")
        .map((line) => JSON.parse(line));
      for (const c of selected) {
        const pkg = path.posix.dirname(c.file);
        const event = events.find(
          (e) =>
            e.Test === c.name &&
            e.Package.endsWith("/" + pkg) &&
            ["pass", "skip", "fail"].includes(e.Action),
        );
        if (!event || event.Action !== "pass")
          throw new Error(`Required Go case did not pass: ${c.id}`);
        manifest.cases.push({
          id: c.id,
          status: "pass",
          reused: events.some(
            (e) =>
              e.Package === event.Package && e.Output?.includes("(cached)"),
          ),
          duration_seconds: event.Elapsed,
        });
      }
      console.log(
        `Passed ${selected.length} Go cases in ${packageNames.length} packages; cache reuse is recorded per package.`,
      );
    }
    const nodeCases = selection.selected.filter((c) => c.runner === "node");
    if (nodeCases.length) {
      const result = await run(
        process.execPath,
        ["--test", ...nodeCases.map((c) => c.file)],
        { cwd: root, env },
      );
      writeFileSync(path.join(bundle, "harness.txt"), result.stdout);
      manifest.cases.push(
        ...nodeCases.map((c) => ({ id: c.id, status: "pass", reused: false })),
      );
    }
    if (system) {
      const playwright = path.join(root, "web/node_modules/playwright/cli.js");
      const inventory = await run(
        process.execPath,
        [playwright, "test", "--list", "--reporter=json"],
        {
          cwd: path.join(root, "web"),
          env: { ...env, AICP_TEST_PROFILE: "renderer" },
        },
      );
      reconcile(
        browserInventory(JSON.parse(inventory.stdout)),
        cases.filter((c) => c.runner === "browser"),
      );
      const grep = selection.selected
        .filter((c) => c.runner === "browser")
        .map((c) => `@case:${c.id}(?:\\s|$)`)
        .join("|");
      const result = await run(
        process.execPath,
        [playwright, "test", "--grep", grep],
        { cwd: path.join(root, "web"), env, echo: true },
      );
      writeFileSync(path.join(bundle, "browser.txt"), result.stdout);
      const browserResults = JSON.parse(
        readFileSync(path.join(bundle, "browser-outcomes.json"), "utf8"),
      );
      for (const c of selection.selected.filter(
        (c) => c.runner === "browser",
      )) {
        const result = browserResults.find((r) => r.id === c.id);
        if (!result || result.status !== "passed")
          throw new Error(`Required browser case did not pass: ${c.id}`);
        if (c.scenario) {
          const records = files(path.join(bundle, "browser"))
            .filter((n) => path.basename(n) === "actual.json")
            .map((n) => JSON.parse(readFileSync(n, "utf8")))
            .filter((r) => r.case_id === c.id);
          if (records.length !== 1)
            throw new Error(
              `Required journey record missing or duplicated: ${c.id}`,
            );
          validateJourney(
            records[0],
            JSON.parse(
              readFileSync(
                path.join(root, "tests/e2e/contracts", c.id + ".json"),
                "utf8",
              ),
            ),
          );
        }
        manifest.cases.push({ ...result, reused: false });
      }
    }
    // Packaging remains in the existing release job; this gate verifies the
    // host executable and required renderer, and never claims another host passed.
    if (
      JSON.stringify(await sourceIdentity()) !== JSON.stringify(manifest.target)
    )
      throw new Error(
        "Inconclusive: source changed during verification; rerun the stable candidate.",
      );
    manifest.status = "pass";
  }
} catch (error) {
  failure = error;
  manifest.status = /Not run:|not found|ENOENT/.test(error.message)
    ? "not run"
    : /Inconclusive:|occupied/.test(error.message)
      ? "inconclusive"
      : "fail";
  manifest.error = error.message;
  if (error.result) {
    mkdirSync(bundle, { recursive: true });
    writeFileSync(
      path.join(bundle, "failed-command.txt"),
      error.result.stdout + "\n" + error.result.stderr,
    );
  }
} finally {
  if (!options.plan) {
    const outcomesPath = path.join(bundle, "browser-outcomes.json");
    if (failure && existsSync(outcomesPath)) {
      const outcomes = JSON.parse(readFileSync(outcomesPath, "utf8"));
      for (const c of outcomes)
        if (!manifest.cases.some((existing) => existing.id === c.id))
          manifest.cases.push({
            ...c,
            status:
              c.status === "passed"
                ? "pass"
                : c.status === "skipped"
                  ? "not run"
                  : "fail",
            reused: false,
          });
    }
    for (const c of manifest.selected ?? [])
      if (!manifest.cases.some((result) => result.id === c.id))
        manifest.cases.push({
          id: c.id,
          status: "not run",
          reason: "Required execution was not reached.",
        });
    manifest.finished_at = new Date().toISOString();
    manifest.duration_seconds = (performance.now() - started) / 1000;
    saveJSON(path.join(bundle, "manifest.json"), manifest);
    const lines = [
      `# ${manifest.status.toUpperCase()} — ${options.scenario ?? options.feature ?? options.suite ?? options.level ?? "selection"}`,
      "",
      `Duration: ${manifest.duration_seconds.toFixed(2)} seconds.`,
      "",
      `Selected: ${manifest.selected?.length ?? 0}; omitted: ${manifest.omitted?.length ?? 0}.`,
      `Excluded renderer cases: ${(manifest.excluded_profiles ?? []).join(", ") || "none"}.`,
      "",
      ...(manifest.build?.decisions ?? []),
      "",
      "| Case | Outcome | Evidence |",
      "| --- | --- | --- |",
      ...manifest.cases.map(
        (c) =>
          `| ${c.id} | ${c.status} | ${c.reused ? "cache reused" : "fresh"} |`,
      ),
      "",
      manifest.error ?? "",
      "",
      "See journey report.md files under web/test-results for numbered expected/observed steps and screenshots.",
    ];
    lines.pop();
    lines.push(
      ...files(path.join(bundle, "browser"))
        .filter((n) => n.endsWith("report.md"))
        .map(
          (n) =>
            `[Journey report](${path.relative(bundle, n).replaceAll("\\", "/")})`,
        ),
    );
    writeFileSync(path.join(bundle, "summary.md"), lines.join("\n") + "\n");
    saveJSON(path.join(outputRoot, "latest.json"), {
      bundle,
      status: manifest.status,
    });
    if (manifest.status === "pass" && !process.env.CI)
      pruneSuccessfulBundles(outputRoot, bundle);
    console.log(`Evidence: ${path.join(bundle, "summary.md")}`);
  }
}
if (failure) {
  console.error(failure.message);
  process.exitCode = 1;
}
