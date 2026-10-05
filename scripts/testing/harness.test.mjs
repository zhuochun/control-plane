import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { select, reconcile } from "./selection.mjs";
import {
  currentArtifact,
  digest,
  binarySources,
  fingerprint,
  dependenciesCurrent,
} from "./build.mjs";
import {
  compareObservation,
  JourneyRecord,
  validateJourney,
} from "./observations.mjs";
import { readFileSync } from "node:fs";

const cases = [
  {
    id: "rule",
    level: "unit",
    features: ["reviews"],
    concerns: ["functional"],
  },
  {
    id: "ui",
    level: "system",
    features: ["reviews"],
    concerns: ["ui"],
    profile: "core",
  },
  {
    id: "roundtrip",
    level: "system",
    features: ["reviews"],
    concerns: ["ui", "persistence"],
    scenario: "review-roundtrip",
  },
  {
    id: "renderer",
    level: "system",
    features: ["diagrams"],
    concerns: ["visual"],
    profile: "renderer",
  },
];
test("journey completion rejects omitted, duplicate, failed and reordered steps", () => {
  const directory = mkdtempSync(path.join(tmpdir(), "aicp-journey-control-"));
  try {
    const expected = { R1: { count: 1 }, R2: { count: 2 } };
    const record = new JourneyRecord(
      "control",
      "completion",
      expected,
      directory,
    );
    const actual = () =>
      JSON.parse(readFileSync(path.join(directory, "actual.json"), "utf8"));
    record.check("R1", "first", expected.R1);
    assert.throws(
      () => validateJourney(actual(), expected),
      /every frozen step/,
    );
    record.check("R2", "second", expected.R2);
    validateJourney(actual(), expected);
    const complete = actual();
    assert.throws(() =>
      validateJourney(
        { ...complete, steps: complete.steps.toReversed() },
        expected,
      ),
    );
    assert.throws(() =>
      validateJourney(
        { ...complete, steps: [...complete.steps, complete.steps[0]] },
        expected,
      ),
    );
    assert.throws(() =>
      validateJourney(
        {
          ...complete,
          steps: complete.steps.map((s) => ({ ...s, outcome: "fail" })),
        },
        expected,
      ),
    );
  } finally {
    rmSync(directory, { recursive: true });
  }
});
test("targeted selectors preserve level, concern and scenario boundaries", () => {
  assert.deepEqual(
    select(cases, { level: "unit", feature: "reviews" }, [
      "reviews",
    ]).selected.map((c) => c.id),
    ["rule"],
  );
  assert.deepEqual(
    select(cases, { level: "system", scenario: "review-roundtrip" }, [
      "reviews",
    ]).selected.map((c) => c.id),
    ["roundtrip"],
  );
  assert.equal(
    select(cases, { suite: "main" }, ["reviews"]).excludedProfiles.length,
    1,
  );
  for (const options of [
    { suite: "main", feature: "reviews" },
    { suite: "main", concern: "ui" },
    { level: "system", scenario: "unknown" },
    { level: "unit", concern: "visual" },
    { suite: "release" },
    { level: "system", scenario: "review-roundtrip", feature: "reviews" },
  ])
    assert.throws(() => select(cases, options, ["reviews"]));
});
test("discovery rejects missing declared and new unindexed runnable cases", () => {
  assert.throws(
    () => reconcile(cases.slice(1), cases),
    /Declared case missing/,
  );
  assert.throws(
    () => reconcile([...cases, { id: "new-test" }], cases),
    /Runnable case unindexed/,
  );
  assert.throws(() => reconcile(cases, [...cases, cases[0]]), /Duplicate/);
  assert.throws(
    () => reconcile([...cases, cases[0]], cases),
    /Duplicate runnable/,
  );
});
test("stale input, changed binary and missing identity force a rebuild", () => {
  const directory = mkdtempSync(path.join(tmpdir(), "aicp-cache-control-"));
  try {
    const artifact = path.join(directory, "binary");
    writeFileSync(artifact, "old");
    const manifest = { inputs: "source-1", digest: digest("old") };
    assert.equal(currentArtifact("source-1", artifact, manifest), true);
    assert.equal(currentArtifact("source-2", artifact, manifest), false);
    writeFileSync(artifact, "tampered");
    assert.equal(currentArtifact("source-1", artifact, manifest), false);
    assert.equal(currentArtifact("source-1", artifact, undefined), false);
  } finally {
    rmSync(directory, { recursive: true });
  }
});
test("an intentionally wrong golden observation is rejected", () => {
  const expected = { answer_count: 1, answer_event_count: 1 };
  assert.throws(
    () =>
      compareObservation({ answer_count: 2, answer_event_count: 1 }, expected),
    assert.AssertionError,
  );
});
test("migration resources invalidate executable identity and lock identity cannot advance without install", () => {
  const directory = mkdtempSync(path.join(tmpdir(), "aicp-build-control-"));
  try {
    mkdirSync(path.join(directory, "internal/store/migrations"), {
      recursive: true,
    });
    const migration = path.join(directory, "internal/store/migrations/001.sql");
    writeFileSync(migration, "CREATE TABLE before (id INT);");
    const before = fingerprint(binarySources(directory));
    writeFileSync(migration, "CREATE TABLE after (id INT);");
    assert.notEqual(fingerprint(binarySources(directory)), before);
    const manifest = { dependencies: "lock-1" };
    assert.equal(dependenciesCurrent("lock-2", true, manifest), false);
    assert.equal(manifest.dependencies, "lock-1");
    assert.equal(
      dependenciesCurrent("lock-2", true, { dependencies: "lock-2" }),
      true,
    );
  } finally {
    rmSync(directory, { recursive: true });
  }
});
