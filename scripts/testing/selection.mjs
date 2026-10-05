export const levels = ["unit", "component", "system"];
export const concerns = [
  "functional",
  "contract",
  "ui",
  "visual",
  "accessibility",
  "persistence",
  "recovery",
  "performance",
];
export const suites = ["change", "main", "release"];

export function select(cases, options, features) {
  const {
    level,
    suite,
    concern,
    feature,
    scenario,
    profile = "core",
  } = options;
  if (!!level === !!suite)
    throw new Error("Select exactly one of -Level or -Suite.");
  for (const [value, allowed, name] of [
    [level, levels, "level"],
    [suite, suites, "suite"],
    [concern, concerns, "concern"],
    [feature, features, "feature"],
  ]) {
    if (value && !allowed.includes(value))
      throw new Error(`Unknown ${name}: ${value}`);
  }
  if (!["core", "renderer"].includes(profile))
    throw new Error(`Unknown profile: ${profile}`);
  if (concern && !level) throw new Error("-Concern requires -Level.");
  if (scenario && (level !== "system" || feature || concern))
    throw new Error("-Scenario requires system and excludes Feature/Concern.");
  if (["main", "release"].includes(suite) && (feature || concern || scenario))
    throw new Error("Main/release cannot be narrowed.");
  if (suite === "release" && profile !== "renderer")
    throw new Error("Release requires -Profile renderer.");
  if (scenario && !cases.some((c) => c.scenario === scenario))
    throw new Error(`Unknown scenario: ${scenario}`);
  const matched = cases.filter(
    (c) =>
      (!level || c.level === level) &&
      (!concern || c.concerns.includes(concern)) &&
      (!feature || c.features.includes(feature)) &&
      (!scenario || c.scenario === scenario),
  );
  const selected = matched.filter(
    (c) => c.profile !== "renderer" || profile === "renderer",
  );
  if (!selected.length)
    throw new Error("Empty selection; no pass is possible.");
  return {
    selected,
    omitted: cases.filter((c) => !selected.includes(c)),
    excludedProfiles: matched
      .filter((c) => !selected.includes(c))
      .map((c) => c.id),
  };
}

export function reconcile(discovered, indexed) {
  const discoveredIDs = new Set();
  for (const entry of discovered) {
    if (discoveredIDs.has(entry.id))
      throw new Error(`Duplicate runnable case: ${entry.id}`);
    discoveredIDs.add(entry.id);
  }
  const ids = new Set();
  for (const entry of indexed) {
    if (ids.has(entry.id)) throw new Error(`Duplicate case: ${entry.id}`);
    ids.add(entry.id);
    if (
      !levels.includes(entry.level) ||
      !entry.concerns.length ||
      entry.concerns.some((c) => !concerns.includes(c))
    )
      throw new Error(`Invalid classification: ${entry.id}`);
    if (!discovered.some((c) => c.id === entry.id))
      throw new Error(`Declared case missing: ${entry.id}`);
  }
  for (const entry of discovered)
    if (!ids.has(entry.id))
      throw new Error(`Runnable case unindexed: ${entry.id}`);
  return indexed;
}
