import path from "node:path";
import { readFileSync } from "node:fs";
import { root, files } from "./build.mjs";
import { run } from "./process.mjs";
import {
  goFiles,
  goOverrides,
  requiredGoCases,
  requiredBrowserCases,
  features,
} from "./index.mjs";
import { reconcile } from "./selection.mjs";

export async function discover() {
  const raw = JSON.parse(
    (await run("go", ["run", "./scripts/testing/discover.go"], { cwd: root }))
      .stdout,
  );
  const discovered = raw.map((c) => ({
    id: `go:${path.posix.dirname(c.file)}:${c.name}`,
    file: c.file,
    name: c.name,
    runner: "go",
  }));
  const indexed = discovered
    .filter((c) => goFiles[c.file])
    .map((c) => ({
      ...c,
      ...goFiles[c.file],
      ...goOverrides[c.name],
      profile: goOverrides[c.name]?.profile ?? "core",
    }));
  for (const name of requiredGoCases)
    if (!raw.some((c) => c.name === name))
      throw new Error(`Declared Go case missing: ${name}`);
  for (const file of Object.keys(goFiles))
    if (!raw.some((c) => c.file === file))
      throw new Error(`Declared Go test file missing: ${file}`);
  reconcile(discovered, indexed);
  // Native Playwright tags carry case identity and classification. The executed
  // --list inventory is also reconciled before browser execution (dynamic cases).
  const browserFiles = files(path.join(root, "web/tests")).filter((n) =>
    n.endsWith(".spec.ts"),
  );
  const browser = [];
  for (const file of browserFiles) {
    const text = readFileSync(file, "utf8");
    const declarations = [
      ...text.matchAll(
        /\btest\(\s*["'`]([^"'`]+)["'`]\s*,\s*\{\s*tag:\s*\[([^\]]+)\]/g,
      ),
    ];
    const calls = [...text.matchAll(/\btest\s*\(/g)];
    if (calls.length !== declarations.length)
      throw new Error(
        `Runnable browser case without classification: ${path.relative(root, file)}`,
      );
    for (const declaration of declarations) {
      const tags = [...declaration[2].matchAll(/["']([^"']+)["']/g)].map(
        (m) => m[1],
      );
      const values = (type) =>
        tags
          .filter((t) => t.startsWith(`@${type}:`))
          .map((t) => t.split(":")[1]);
      const [id] = values("case"),
        [scenario] = values("scenario"),
        [profile = "core"] = values("profile");
      if (!id || !values("feature").length || !values("concern").length)
        throw new Error(`Incomplete browser classification: ${declaration[1]}`);
      browser.push({
        id,
        name: declaration[1],
        file: path.relative(root, file).replaceAll("\\", "/"),
        runner: "browser",
        level: "system",
        features: values("feature"),
        concerns: values("concern"),
        scenario,
        profile,
      });
    }
  }
  for (const id of requiredBrowserCases)
    if (!browser.some((c) => c.id === id))
      throw new Error(`Declared browser case missing: ${id}`);
  reconcile(browser, browser);
  const nodeFamilies = [
    {
      id: "harness-controls",
      file: "scripts/testing/harness.test.mjs",
      level: "unit",
    },
    {
      id: "harness-integration-controls",
      file: "scripts/testing/harness.integration.test.mjs",
      level: "component",
      boundary: "Go parser/compiler + process-tree termination",
    },
  ].map((c) => ({
    ...c,
    runner: "node",
    features,
    concerns: ["functional", "contract"],
    profile: "core",
  }));
  const nodeDiscovered = files(path.join(root, "scripts"))
    .filter((n) => n.endsWith(".test.mjs"))
    .map((n) => ({
      id:
        nodeFamilies.find((c) => path.resolve(root, c.file) === n)?.id ??
        path.relative(root, n),
    }));
  reconcile(nodeDiscovered, nodeFamilies);
  const all = [...indexed, ...browser, ...nodeFamilies];
  for (const c of all)
    if (c.features.some((f) => !features.includes(f)))
      throw new Error(`Unknown feature in ${c.id}`);
  return all;
}

export function browserInventory(report) {
  const cases = [];
  function visit(suite) {
    for (const spec of suite.specs ?? []) {
      const ids = (spec.tags ?? [])
        .map((tag) => tag.replace(/^@/, ""))
        .filter((tag) => tag.startsWith("case:"))
        .map((tag) => tag.slice(5));
      if (ids.length !== 1)
        throw new Error(
          `Browser inventory lacks unique case ID: ${spec.title}`,
        );
      cases.push({ id: ids[0] });
    }
    for (const child of suite.suites ?? []) visit(child);
  }
  for (const suite of report.suites ?? []) visit(suite);
  return cases;
}
