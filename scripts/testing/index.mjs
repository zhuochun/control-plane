export const features = [
  "setup",
  "items",
  "runs",
  "reviews",
  "delegations",
  "diagrams",
];
const component = (
  features,
  boundary = "application + SQLite",
  concerns = ["functional", "persistence"],
) => ({ level: "component", features, boundary, concerns });
const unit = (features, concerns = ["functional"]) => ({
  level: "unit",
  features,
  boundary: "isolated rule/module with controlled dependencies",
  concerns,
});
const adapter = (features) =>
  component(features, "adapter + HTTP/application", ["contract", "functional"]);

// Homogeneous file families inherit classification. Mixed families override their
// isolated cases. Unknown files fail discovery instead of inheriting a catch-all.
export const goFiles = {
  "cmd/aicp/main_test.go": adapter(["setup", "runs"]),
  "cmd/aicp/reviews_test.go": adapter(["reviews"]),
  "cmd/aicp/item_work_test.go": adapter(["delegations", "items"]),
  "internal/client/client_test.go": unit(features, ["contract"]),
  "web/assets_test.go": unit(["items"], ["contract"]),
  "internal/store/store_test.go": component(
    features,
    "SQLite migrations and directory ownership",
  ),
  "internal/httpapi/server_test.go": adapter(["setup", "items", "runs"]),
  "internal/httpapi/reviews_test.go": adapter(["reviews", "diagrams"]),
  "internal/httpapi/item_work_test.go": adapter(["delegations", "items"]),
  "internal/mcpserver/server_test.go": adapter(["setup", "items", "runs"]),
  "internal/mcpserver/reviews_test.go": adapter(["reviews", "diagrams"]),
  "internal/app/app_test.go": component(["setup"]),
  "internal/app/config_plan_test.go": component(["setup"]),
  "internal/app/configuration_test.go": component(["setup"]),
  "internal/app/fixtures_test.go": component(
    ["items", "runs"],
    "schemas + application + SQLite",
    ["contract", "functional"],
  ),
  "internal/app/item_work_test.go": component(["delegations", "items"]),
  "internal/app/item_inputs_test.go": component(["items", "runs"], "application + SQLite", ["functional", "persistence", "recovery"]),
  "internal/app/items_test.go": component(["items"]),
  "internal/app/proposals_test.go": component(["setup"]),
  "internal/app/publication_test.go": component(
    ["items", "runs"],
    "application + SQLite",
    ["functional", "persistence", "recovery"],
  ),
  "internal/app/review_answers_test.go": component(
    ["reviews"],
    "application + SQLite",
    ["functional", "persistence", "recovery"],
  ),
  "internal/app/review_formats_test.go": component(["reviews", "diagrams"]),
  "internal/app/run_context_test.go": component(["runs", "items"]),
  "internal/app/runs_test.go": component(
    ["runs", "items"],
    "application + SQLite",
    ["functional", "persistence", "recovery"],
  ),
  "internal/app/settings_test.go": component(["setup", "runs"]),
  "internal/app/setup_test.go": component(["setup"]),
  "internal/app/watcher_interest_test.go": component([
    "setup",
    "items",
    "runs",
  ]),
};
export const goOverrides = {
  TestExitCodeContract: unit(features, ["contract"]),
  TestReportRejectsNestedInputsAndUnknownActionReferences: unit(
    ["reviews"],
    ["functional", "contract"],
  ),
  TestReminderRejectsImpossibleAndDisambiguatesRepeatedTime: unit(["items"]),
  TestAttentionProjectionBoundsProseAndPreservesDetail: unit(["items", "runs"]),
  TestRejectsNonLoopbackHost: unit(features, ["contract"]),
  TestPlantUMLUnavailableHasExplicitFailure: unit(
    ["diagrams"],
    ["functional", "recovery"],
  ),
  TestPlantUMLSetupDiscovery: unit(["diagrams"], ["contract"]),
  TestPlantUMLLocalRenderer: {
    ...component(["diagrams"], "Java + checksum-pinned PlantUML", [
      "functional",
      "visual",
      "recovery",
    ]),
    profile: "renderer",
  },
};
export const requiredGoCases = [
  "TestReviewAnswersPersistReplayAndCorrect",
  "TestReviewSubmissionFailuresWriteNothing",
  "TestReviewMaterialTracksEvidenceNotHandoff",
  "TestMixedAndPartialResultsKeepTruthfulCheckpoints",
  "TestWatchPublicationAdvancesCoverageAndPreservesHumanState",
  ...Object.keys(goOverrides),
];
export const requiredBrowserCases = [
  "attention-01",
  "attention-02",
  "attention-03",
  "attention-04",
  "attention-05",
  "attention-06",
  "attention-07",
  "attention-08",
  "reader-01",
  "reader-02",
  "reader-03",
  "reader-04",
  "reader-05",
  "reader-06",
  "reader-07",
  "reader-08",
  "review-layout",
  "review-validation",
  "review-pending",
  "review-retry",
  "review-refresh",
  "review-images",
  "review-interleaving",
  "review-stale",
  "setup-no-run",
  "inspection-two-cycles",
  "review-roundtrip",
  "recovery-answer",
  "recovery-run",
];
export const sourceScopes = {
  setup: [
    "internal/app/setup",
    "internal/app/config",
    "web/src/pages/preferences",
  ],
  items: [
    "internal/app/items",
    "internal/app/publication",
    "web/src/pages/workspace",
  ],
  runs: ["internal/app/runs", "internal/app/run_context", "scripts/demo.ps1"],
  reviews: ["internal/app/review_", "web/src/components/report-content"],
  delegations: ["internal/app/item_work"],
  diagrams: ["internal/httpapi/reviews", "web/src/components/report-content"],
};
