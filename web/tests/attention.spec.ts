import { expect, test } from "@playwright/test";

test("a report survives Todo, reminder, reload, and Done", async ({
  page,
  request,
}) => {
  const requestId = crypto.randomUUID();
  const created = await request.post("/api/v1/items", {
    data: {
      request_id: requestId,
      dedupe_key: `browser:${requestId}`,
      expected_content_version: 0,
      kind: "report",
      title: "Confirm the rollout sequence",
      summary: "One decision needs attention.",
      sources: [
        {
          id: "thread",
          url: "https://example.com/thread",
          label: "Rollout discussion",
          observed_at: "2026-09-15T01:00:00Z",
        },
      ],
      report: {
        schema_version: 1,
        body_md:
          "## Options\n\n| Choice | Effect |\n| --- | --- |\n| Small cohort | Easier validation |",
      },
    },
  });
  expect(created.ok()).toBeTruthy();

  await page.goto("http://127.0.0.1:7331/");
  await page
    .getByRole("button", { name: /Confirm the rollout sequence/ })
    .click();
  await page.getByRole("link", { name: /Full detail/ }).click();
  await expect(page.getByRole("heading", { name: "Options" })).toBeVisible();
  await expect(page.getByRole("table")).toBeVisible();
  const source = page.getByRole("link", { name: /Rollout discussion/ });
  await expect(source).toHaveAttribute("href", "https://example.com/thread");
  await expect(source).toHaveAttribute("target", "_blank");

  await page.getByRole("button", { name: "Set Todo" }).click();
  await expect(page.getByRole("button", { name: "Mark Done" })).toBeVisible();
  await page.getByRole("button", { name: "Remind me" }).click();
  await page.getByLabel("Date").fill("2030-09-16");
  await page.getByRole("button", { name: "Set reminder" }).click();
  await expect(
    page.getByRole("button", { name: "Clear reminder" }),
  ).toBeVisible();
  await expect(page.getByText(/Reminder/)).toBeVisible();

  await page.reload();
  await expect(page.getByRole("button", { name: "Mark Done" })).toBeVisible();
  await expect(page.getByText(/Reminder/)).toBeVisible();
  await page.getByRole("button", { name: "Mark Done" }).click();
  await expect(page.getByRole("button", { name: "Reopen Todo" })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Clear reminder" }),
  ).toHaveCount(0);
});

test("a person can accept an agent proposal and inspect its history", async ({
  page,
  request,
}) => {
  const requestId = crypto.randomUUID();
  const response = await request.post("/api/v1/proposals", {
    data: {
      request_id: requestId,
      proposal_key: `browser:${requestId}`,
      target_type: "interest",
      operation: "create",
      payload: {
        title: "Gentle release watch",
        instructions_md: "Notice meaningful release changes.",
      },
      rationale_md: "**Useful signal:** this keeps release changes together.",
    },
  });
  expect(response.ok()).toBeTruthy();

  await page.goto("http://127.0.0.1:7331/");
  await expect(page.getByText("Useful signal:")).toBeVisible();
  await page.getByRole("button", { name: "Accept" }).click();
  await expect(page.getByText("Useful signal:")).toHaveCount(0);

  await page.getByRole("link", { name: "Monitoring" }).click();
  await expect(page.getByText("Gentle release watch")).toBeVisible();
  await page.getByRole("link", { name: "Activity" }).click();
  await expect(page.getByText("Human decisions")).toBeVisible();
  await expect(
    page.getByText(/this keeps release changes together/),
  ).toBeVisible();
  await expect(page.getByText("accepted")).toBeVisible();
});

test("owner can inspect and abandon an interrupted run", async ({ page, request }) => {
  const marker = crypto.randomUUID();
  const interestResponse = await request.post("/api/v1/interests", {
    data: { title: `Recovery ${marker}`, instructions_md: "Check the source" },
  });
  expect(interestResponse.ok()).toBeTruthy();
  const interest = await interestResponse.json();
  const watchResponse = await request.post("/api/v1/watches", {
    data: { interest_id: interest.id, source: { kind: "web", locator: `https://example.com/${marker}` }, instructions_md: "Inspect" },
  });
  expect(watchResponse.ok()).toBeTruthy();
  const started = await request.post("/api/v1/runs", { data: { runner_label: "e2e-agent" } });
  expect(started.ok()).toBeTruthy();
  const run = await started.json();

  await page.goto("/activity");
  await expect(page.getByRole("heading", { name: "Active inspection" })).toBeVisible();
  await expect(page.getByText(`https://example.com/${marker}`, { exact: false })).toBeVisible();
  await expect(page.getByText("No result", { exact: false })).toBeVisible();
  await page.getByRole("button", { name: "Abandon interrupted run" }).click();
  await page.getByRole("textbox", { name: "Reason" }).fill("agent exited before inspection");
  await page.getByRole("button", { name: "Abandon run", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Active inspection" })).toHaveCount(0);
  const detail = await request.get(`/api/v1/runs/${run.run.id}`);
  expect(detail.ok()).toBeTruthy();
  expect((await detail.json()).run.summary).toContain("agent exited before inspection");
});

test("preferences keep browser defaults and expose agent context", async ({
  page,
  request,
}) => {
  await page.goto("http://127.0.0.1:7331/preferences");
  await expect(
    page.getByRole("heading", { name: "Preferences" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Use browser default" }),
  ).toBeVisible();
  await expect(page.getByLabel("Agent instructions")).toHaveValue(
    /Working with aicp/,
  );
  await expect(page.getByLabel("Owner context")).toHaveValue(/Owner context/);

  await page
    .getByLabel("Agent instructions")
    .fill("# Agent rules\n\nRead the brief first.");
  await page
    .getByLabel("Owner context")
    .fill("# Owner\n\nPrefer concise evidence.");
  await page.getByRole("button", { name: "Save preferences" }).click();
  await expect(page.getByText("Preferences saved.")).toBeVisible();

  const settings = await request.get("/api/v1/settings");
  expect(settings.ok()).toBeTruthy();
  const settingsBody = await settings.json();
  expect(settingsBody.agents_md).toBe("# Agent rules\n\nRead the brief first.");
  const brief = await request.get("/api/v1/brief");
  expect(brief.ok()).toBeTruthy();
  const body = await brief.json();
  expect(body.contexts["AGENTS.md"]).toBe(
    "# Agent rules\n\nRead the brief first.",
  );
  expect(body.contexts["USER.md"]).toBe("# Owner\n\nPrefer concise evidence.");
});
