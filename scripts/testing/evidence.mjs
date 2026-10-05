import { readdirSync, realpathSync, rmSync } from "node:fs";
import path from "node:path";
import { readJSON } from "./build.mjs";

export function pruneSuccessfulBundles(outputRoot, current) {
  const resolvedRoot = realpathSync(outputRoot);
  for (const entry of readdirSync(outputRoot, { withFileTypes: true })) {
    if (!entry.isDirectory() || !entry.name.startsWith("run-")) continue;
    const directory = path.join(outputRoot, entry.name);
    if (directory === current) continue;
    const manifest = readJSON(path.join(directory, "manifest.json"));
    if (
      manifest?.owner !== "aicp-verification-runner-v1" ||
      manifest.status !== "pass"
    )
      continue;
    const resolved = realpathSync(directory);
    if (path.dirname(resolved) !== resolvedRoot)
      throw new Error(`Unsafe evidence cleanup target: ${resolved}`);
    rmSync(resolved, { recursive: true });
  }
}
