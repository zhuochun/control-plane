import { createHash } from "node:crypto";
import {
  existsSync,
  readFileSync,
  readdirSync,
  mkdirSync,
  writeFileSync,
  realpathSync,
  statSync,
} from "node:fs";
import { pathToFileURL } from "node:url";
import path from "node:path";
import { run } from "./process.mjs";

export const root = path.resolve(import.meta.dirname, "../..");
export const outputRoot = path.join(root, "test-output");
export const binary = path.join(
  root,
  "dist",
  process.platform === "win32" ? "aicp.exe" : "aicp",
);
export const digest = (data) => createHash("sha256").update(data).digest("hex");
export function files(directory) {
  if (!existsSync(directory)) return [];
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    if (
      ["node_modules", ".git", "dist", "test-output", "test-results"].includes(
        entry.name,
      )
    )
      return [];
    const name = path.join(directory, entry.name);
    return entry.isDirectory() ? files(name) : [name];
  });
}
export function fingerprint(names) {
  return digest(
    names
      .sort()
      .map(
        (name) => `${path.relative(root, name)}:${digest(readFileSync(name))}`,
      )
      .join("\n"),
  );
}
export function currentArtifact(inputs, artifact, manifest) {
  return (
    existsSync(artifact) &&
    manifest?.inputs === inputs &&
    manifest.digest === digest(readFileSync(artifact))
  );
}
export function binarySources(base = root) {
  return ["cmd", "internal"]
    .flatMap((dir) => files(path.join(base, dir)))
    .filter((n) => !n.endsWith("_test.go"));
}
export function dependenciesCurrent(inputs, installed, manifest) {
  return installed && manifest?.dependencies === inputs;
}
export function readJSON(name) {
  try {
    return JSON.parse(readFileSync(name, "utf8"));
  } catch {
    return null;
  }
}
export function saveJSON(name, value) {
  mkdirSync(path.dirname(name), { recursive: true });
  writeFileSync(name, JSON.stringify(value, null, 2) + "\n");
}
export function npmCLI() {
  const locations = [
    path.dirname(process.execPath),
    ...process.env.PATH.split(path.delimiter),
  ];
  for (const directory of locations) {
    for (const candidate of [
      path.join(directory, "node_modules/npm/bin/npm-cli.js"),
      path.join(directory, "../lib/node_modules/npm/bin/npm-cli.js"),
      path.join(directory, "npm"),
    ]) {
      if (!existsSync(candidate)) continue;
      const resolved = realpathSync(candidate);
      if (resolved.endsWith("npm-cli.js")) return resolved;
    }
  }
  throw new Error("npm CLI not found; install Node with npm.");
}
export async function npm(args, options = {}) {
  return run(
    process.execPath,
    [npmCLI(), "--prefix", path.join(root, "web"), ...args],
    { cwd: root, ...options },
  );
}

export async function prepare({ system = false, plan = false } = {}) {
  const goVersion = (await run("go", ["version"], { cwd: root })).stdout.trim();
  const goConfig = (
    await run("go", ["env", "GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS"], {
      cwd: root,
    })
  ).stdout.trim();
  const toolVersions = {
    node: process.version,
    go: goVersion,
    goConfig,
    platform: process.platform,
    arch: process.arch,
  };
  const manifestPath = path.join(outputRoot, "build.json");
  const old = readJSON(manifestPath) ?? {};
  const decisions = [];
  const depsInputs =
    fingerprint([
      path.join(root, "web/package.json"),
      path.join(root, "web/package-lock.json"),
    ]) + process.version;
  const assetInputs =
    fingerprint(
      files(path.join(root, "web")).filter(
        (name) =>
          !name.includes(`${path.sep}tests${path.sep}`) &&
          !name.endsWith("_test.go") &&
          !name.endsWith("playwright.config.ts"),
      ),
    ) + process.version;
  const assets = path.join(root, "web/dist/index.html");
  // Validate all assets, not just the entry point: chunks are embedded into the executable.
  const assetDigest = () =>
    fingerprint(
      existsSync(assets)
        ? readdirSync(path.join(root, "web/dist"), { recursive: true })
            .map((n) => path.join(root, "web/dist", n))
            .filter((n) => statSync(n).isFile())
        : [],
    );
  const validAssets =
    existsSync(assets) &&
    old.assets?.inputs === assetInputs &&
    old.assets.digest === assetDigest();
  if (
    (system || !validAssets) &&
    !dependenciesCurrent(
      depsInputs,
      existsSync(path.join(root, "web/node_modules/@playwright/test")),
      old,
    )
  ) {
    decisions.push(
      "Install locked web dependencies (missing or changed identity).",
    );
    if (!plan) {
      await npm(["ci"], { echo: true });
      old.dependencies = depsInputs;
      saveJSON(manifestPath, old);
    }
  }
  decisions.push(
    validAssets
      ? "Reuse frontend assets (verified inputs and outputs)."
      : "Build frontend assets (missing or stale).",
  );
  if (!plan && !validAssets) {
    await npm(["run", "build"], { echo: true });
    old.assets = { inputs: assetInputs, digest: assetDigest() };
    saveJSON(manifestPath, old);
  }
  const sourceFiles = binarySources();
  sourceFiles.push(
    path.join(root, "go.mod"),
    path.join(root, "go.sum"),
    path.join(root, "web/assets.go"),
  );
  const binaryInputs =
    fingerprint(sourceFiles) +
    (old.assets?.digest ?? "missing-assets") +
    JSON.stringify(toolVersions) +
    "-trimpath";
  if (system) {
    const validBinary =
      validAssets && currentArtifact(binaryInputs, binary, old.binary);
    decisions.push(
      validBinary
        ? "Reuse executable (verified inputs and binary digest)."
        : "Build executable (missing or stale identity).",
    );
    if (!plan && !validBinary) {
      mkdirSync(path.dirname(binary), { recursive: true });
      await run("go", ["build", "-trimpath", "-o", binary, "./cmd/aicp"], {
        cwd: root,
        echo: true,
      });
      old.binary = {
        inputs: binaryInputs,
        digest: digest(readFileSync(binary)),
      };
      saveJSON(manifestPath, old);
    }
    if (!plan) {
      const { chromium } = await import(
        pathToFileURL(
          path.join(root, "web/node_modules/@playwright/test/index.mjs"),
        ).href
      );
      toolVersions.playwright = readJSON(
        path.join(root, "web/node_modules/playwright/package.json"),
      ).version;
      toolVersions.chromium_executable = chromium.executablePath();
      if (!existsSync(chromium.executablePath())) {
        decisions.push("Install missing Chromium.");
        await run(
          process.execPath,
          [
            path.join(root, "web/node_modules/playwright/cli.js"),
            "install",
            ...(process.env.CI && process.platform === "linux"
              ? ["--with-deps"]
              : []),
            "chromium",
          ],
          { cwd: root, echo: true },
        );
      }
    }
  }
  if (!plan) saveJSON(manifestPath, old);
  return {
    decisions,
    tools: toolVersions,
    binary: old.binary,
    assets: old.assets,
  };
}
