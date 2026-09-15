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
    .getByRole("link", { name: /Confirm the rollout sequence/ })
    .click();
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
  await expect(page.getByText(/UTC/)).toBeVisible();

  await page.reload();
  await expect(page.getByRole("button", { name: "Mark Done" })).toBeVisible();
  await expect(page.getByText(/UTC/)).toBeVisible();
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

  await page.getByRole("link", { name: "Interests" }).click();
  await expect(page.getByText("Gentle release watch")).toBeVisible();
  await page.getByRole("link", { name: "Activity" }).click();
  await expect(page.getByText("Human decisions")).toBeVisible();
  await expect(
    page.getByText(/this keeps release changes together/),
  ).toBeVisible();
  await expect(page.getByText("accepted")).toBeVisible();
});
