import {
  cpSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import {
  root,
  outputRoot,
  readJSON,
  files,
  digest,
  saveJSON,
} from "./build.mjs";

const bundle = realpathSync(
  process.argv[2] ??
    readJSON(path.join(outputRoot, "latest.json"))?.bundle ??
    "",
);
if (
  path.dirname(bundle) !== realpathSync(outputRoot) ||
  !path.basename(bundle).startsWith("run-")
)
  throw new Error("Select an owned verification bundle under test-output.");
const manifest = readJSON(path.join(bundle, "manifest.json"));
if (manifest?.owner !== "aicp-verification-runner-v1")
  throw new Error("Bundle does not carry verification ownership.");
const proposal = path.join(
  outputRoot,
  "proposed-goldens-" + new Date().toISOString().replaceAll(":", "-"),
);
const observations = files(path.join(bundle, "browser")).filter((n) =>
  n.endsWith("actual.json"),
);
if (!observations.length)
  throw new Error("No journey observations in the selected bundle.");
mkdirSync(proposal, { recursive: true });
const reports = [];
for (const actualPath of observations) {
  const actual = readJSON(actualPath);
  if (!/^[a-z0-9-]+$/.test(actual.case_id))
    throw new Error("Invalid journey case ID");
  const contract = path.join(
    root,
    "tests/e2e/contracts",
    actual.case_id + ".json",
  );
  if (
    !manifest.oracles?.some(
      (o) =>
        o.case_id === actual.case_id &&
        o.digest === digest(readFileSync(contract)),
    )
  )
    throw new Error(
      `Oracle changed since execution: ${actual.case_id}; rerun before proposing.`,
    );
  const destination = path.join(proposal, actual.case_id);
  mkdirSync(destination, { recursive: true });
  cpSync(path.dirname(actualPath), destination, { recursive: true });
  cpSync(contract, path.join(destination, "expected.json"));
  reports.push(
    `- [${actual.case_id}](${actual.case_id}/report.md) — ${actual.claim}`,
  );
}
cpSync(
  path.join(root, "tests/e2e/contracts/README.md"),
  path.join(proposal, "flows.md"),
);
saveJSON(path.join(proposal, "manifest.json"), manifest);
writeFileSync(
  path.join(proposal, "README.md"),
  [
    "# Proposed journey goldens — owner review required",
    "",
    `Source execution: **${manifest.status}**. No accepted golden or executable expectation was changed.`,
    "",
    "Read the expected/observed steps and desktop/mobile screenshots. Feedback should name a case and step, for example `review-roundtrip / R3`.",
    "",
    ...reports,
    "",
    "Classify feedback as product defect, harness/environment defect, changed expectation, or inconclusive evidence. Acceptance of this concrete proposal precedes any committed golden adoption. Screenshots remain human-review evidence until comparison tolerances and platform baselines are calibrated.",
    "",
  ].join("\n"),
);
console.log(path.join(proposal, "README.md"));
