import { expect, test } from "./fixtures";

test("discovered Watcher review shows source and cadence before a decision", {
  tag:["@case:attention-09", "@feature:setup", "@concern:ui", "@concern:functional", "@profile:core"],
}, async ({page, request}) => {
  const slug = `discovered-${crypto.randomUUID()}`;
  const longURL = "https://example.com/evidence/" + "long-source-reference".repeat(20);
  const response = await request.post("/api/v1/proposals", {data:{proposal_key:slug, target_type:"watch", operation:"create", rationale_md:"Review this separately discovered source.", evidence_links:[longURL], payload:{slug, source:{kind:"fixture", locator:"Fictional support queue"}, instructions_md:"Track recurring onboarding friction.", matching_policy:"broad", interval_seconds:86400, lookback_seconds:604800}}});
  expect(response.ok(), await response.text()).toBeTruthy();
  const proposal = await response.json();
  await page.goto("/");
  await page.getByRole("link", {name:"1 proposal to review", exact:true}).click();
  const card = page.locator(".proposal-card").filter({hasText:"Review this separately discovered source."});
  for (const width of [390, 780, 1440]) {
    await page.setViewportSize({width, height:900});
    await expect.poll(() => page.locator(".workspace-list").evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
    await expect(card.getByRole("link", {name:longURL,exact:true})).toHaveAttribute("href",longURL);
  }
  await card.getByRole("button", {name:"Review proposal", exact:true}).click();
  const dialog = page.getByRole("dialog", {name:"Review proposed Watcher"});
  await expect(dialog.getByText("Fictional support queue", {exact:true})).toBeVisible();
  await expect(dialog.getByText("1 day", {exact:true})).toBeVisible();
  await expect(dialog.getByText("7 days", {exact:true})).toBeVisible();
  await expect(dialog.getByText("All active Interests", {exact:true})).toBeVisible();
  await expect(dialog.getByRole("button", {name:"Accept proposal",exact:true})).toBeEnabled();
  // Review is readable on a phone and cancellation does not apply anything.
  await page.setViewportSize({width:390,height:844});
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await expect.poll(() => dialog.locator(".proposal-review-content").evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
  await dialog.getByRole("button", {name:"Back",exact:true}).click();
  await expect(dialog).toHaveCount(0);
  const pending = await (await request.get("/api/v1/proposals?state=pending")).json();
  expect(pending.items.some((entry:{id:string}) => entry.id === proposal.id)).toBeTruthy();
  await card.getByRole("button", {name:"Reject",exact:true}).click();
  await expect(card).toHaveCount(0);
});

test(
  "Todos opens directly and survives reload",
  {
    tag: [
      "@case:attention-01",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page }) => {
    await page.goto("/todos");
    await expect(page.getByRole("heading", { name: "Todos" })).toBeVisible();
    await page.reload();
    await expect(page.getByRole("heading", { name: "Todos" })).toBeVisible();
  },
);

test(
  "Attention distinguishes incomplete setup from an uninspected plane",
  {
    tag: [
      "@case:attention-02",
      "@feature:setup",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page }) => {
    let setupDone = false;
    await page.route("**/api/v1/status", async (route) => {
      const response = await route.fetch();
      const status = await response.json();
      status.health.setup = {
        user_context_set: setupDone,
        has_interest: setupDone,
        has_watcher: setupDone,
        done: setupDone,
      };
      status.health.last_run = null;
      status.health.watches = [];
      await route.fulfill({ response, json: status });
    });
    await page.goto("/");
    await expect(page.getByText("Setup needs configuration.")).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Set up a Watcher" }),
    ).toBeVisible();
    await expect(
      page.getByText("Nothing needs attention right now."),
    ).toHaveCount(0);
    setupDone = true;
    await page.reload();
    await expect(
      page.getByText("Setup done; inspection not yet verified.", {
        exact: false,
      }),
    ).toBeVisible();
  },
);

test(
  "workspace loads only its view and Monitoring defers related Items",
  {
    tag: [
      "@case:attention-03",
      "@feature:items",
      "@feature:runs",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const slug = `load-${crypto.randomUUID().slice(0, 8)}`;
    const created = await request.post("/api/v1/interests", {
      data: { slug, title: "Load test" },
    });
    expect(created.ok()).toBeTruthy();
    const interest = await created.json();
    const itemRequests: URL[] = [];
    page.on("request", (entry) => {
      const url = new URL(entry.url());
      if (url.pathname === "/api/v1/items") itemRequests.push(url);
    });
    await page.goto("/");
    await expect(
      page.getByRole("heading", { name: "Attention" }),
    ).toBeVisible();
    await expect.poll(() => itemRequests.length).toBeGreaterThan(0);
    expect(
      itemRequests.every((url) => url.searchParams.get("view") === "attention"),
    ).toBeTruthy();

    itemRequests.length = 0;
    await page.goto("/interests");
    await expect(page.locator(`#interest-${slug}`)).toBeVisible();
    expect(itemRequests).toHaveLength(0);
    const relatedRequest = page.waitForRequest((entry) => {
      const url = new URL(entry.url());
      return (
        url.pathname === "/api/v1/items" &&
        url.searchParams.get("interest_id") === interest.id
      );
    });
    await page.locator(`#interest-${slug} summary`).click();
    const loaded = new URL((await relatedRequest).url());
    expect(loaded.searchParams.get("limit")).toBe("10");
  },
);

test(
  "user can capture a date-only Item without a link",
  {
    tag: [
      "@case:attention-04",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page }) => {
    const marker = crypto.randomUUID().slice(0, 8);
    await page.goto("/library");
    const libraryCount = page.locator(
      'nav a[href="/library"] .navigation-count',
    );
    await expect(libraryCount).not.toBeEmpty();
    const before = Number(await libraryCount.textContent());
    await page
      .getByRole("button", { name: "Add to inbox", exact: true })
      .click();
    const dialog = page.getByRole("dialog");
    await dialog
      .getByRole("textbox", { name: "Information" })
      .fill("Review this independently of source monitoring.");
    await dialog.getByText("Optional context", { exact: true }).click();
    await dialog
      .getByRole("textbox", { name: "Title (optional)" })
      .fill(`User idea ${marker}`);
    await dialog.getByLabel("Source date (optional)").fill("2026-09-27");
    await dialog
      .getByRole("button", { name: "Add to inbox", exact: true })
      .click();
    await expect(dialog).toHaveCount(0);
    // Other browser-test workers also create Items in this shared fixture database.
    await expect
      .poll(async () => Number(await libraryCount.textContent()))
      .toBeGreaterThanOrEqual(before + 1);
    await page
      .locator(".queue-row").filter({hasText:`User idea ${marker}`})
      .click();
    await expect(page.getByRole("link", { name: /Full detail/ })).toHaveCount(
      0,
    );
    await expect(page.getByText("2026-09-27", { exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: /Source date/ })).toHaveCount(
      0,
    );
  },
);

test(
  "a report survives Todo, reminder, reload, and Done",
  {
    tag: [
      "@case:attention-05",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
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
      .locator(".queue-row").filter({hasText:"Confirm the rollout sequence"})
      .click();
    await expect(page.getByRole("link", { name: /Full detail/ })).toHaveCount(
      0,
    );
    await expect(page.getByRole("heading", { name: "Options" })).toBeVisible();
    await expect(page.getByRole("table")).toBeVisible();
    const source = page.getByRole("link", { name: /Rollout discussion/ });
    await expect(source).toHaveAttribute("href", "https://example.com/thread");
    await expect(source).toHaveAttribute("target", "_blank");

    await page.getByRole("button", { name: "Add Todo" }).click();
    await expect(page.getByRole("button", { name: "Mark Done" })).toBeVisible();
    await page.getByRole("button", { name: "Remind", exact: true }).click();
    await page.getByLabel("Date").fill("2030-09-16");
    await page.getByRole("button", { name: "Set reminder" }).click();
    await expect(
      page.getByRole("button", { name: "Clear reminder" }),
    ).toBeVisible();
    await expect(page.locator(".reader-reminder")).toBeVisible();

    await page.reload();
    await page
      .locator(".queue-row").filter({hasText:"Confirm the rollout sequence"})
      .click();
    await expect(page.getByRole("button", { name: "Mark Done" })).toBeVisible();
    await expect(page.locator(".reader-reminder")).toBeVisible();
    await page.getByRole("button", { name: "Mark Done" }).click();
    await expect(
      page.getByRole("button", { name: "Reopen Todo" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Clear reminder" }),
    ).toHaveCount(0);
  },
);

test(
  "a person can accept an agent proposal and inspect its history",
  {
    tag: [
      "@case:attention-06",
      "@feature:setup",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
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
    await page.getByRole("link", {name:"1 proposal to review",exact:true}).click();
    await expect(page.getByText("Useful signal:")).toBeVisible();
    await page.getByRole("button", { name: "Review proposal" }).click();
    const review = page.getByRole("dialog", {name:"Review proposed Interest",exact:true});
    await expect(review.getByText("Gentle release watch", {exact:true})).toBeVisible();
    await expect(review.getByText("Notice meaningful release changes.", {exact:true})).toBeVisible();
    // Opening review does not apply the proposal.
    await review.getByRole("button", {name:"Back", exact:true}).click();
    await expect(review).toHaveCount(0);
    await expect(page.getByText("Useful signal:")).toBeVisible();
    await page.getByRole("button", {name:"Review proposal"}).click();
    await review.getByRole("button", {name:"Accept proposal", exact:true}).click();
    await expect(page.getByText("Useful signal:")).toHaveCount(0);
    await page.getByRole("dialog", {name:"Proposals to review",exact:true}).getByRole("button", {name:"Close",exact:true}).click();

    await page.getByRole("button", { name: "Workspace", exact: true }).click();
    await page
      .getByRole("menuitem", { name: "Monitoring", exact: true })
      .click();
    await expect(page.getByText("Gentle release watch")).toBeVisible();
    await page.getByRole("button", { name: "Workspace", exact: true }).click();
    await page.getByRole("menuitem", { name: "Activity", exact: true }).click();
    await expect(page.getByText("Human decisions")).toBeVisible();
    await expect(
      page.getByText(/this keeps release changes together/),
    ).toBeVisible();
    await expect(page.getByText("accepted")).toBeVisible();
  },
);

test(
  "owner can inspect and abandon an interrupted run",
  {
    tag: [
      "@case:attention-07",
      "@feature:runs",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const marker = crypto.randomUUID();
    const interestResponse = await request.post("/api/v1/interests", {
      data: {
        title: `Recovery ${marker}`,
        instructions_md: "Check the source",
      },
    });
    expect(interestResponse.ok()).toBeTruthy();
    const interest = await interestResponse.json();
    const watchResponse = await request.post("/api/v1/watches", {
      data: {
        matching_policy: "explicit",
        interest_ids: [interest.id],
        source: { kind: "web", locator: `https://example.com/${marker}` },
        instructions_md: "Inspect",
      },
    });
    expect(watchResponse.ok()).toBeTruthy();
    const started = await request.post("/api/v1/runs", {
      data: { runner_label: "e2e-agent" },
    });
    expect(started.ok()).toBeTruthy();
    const run = await started.json();

    await page.goto("/activity");
    await expect(
      page.getByRole("heading", { name: "Active inspection" }),
    ).toBeVisible();
    await expect(
      page.getByText(`https://example.com/${marker}`, { exact: false }),
    ).toBeVisible();
    await expect(page.getByText("No result", { exact: false })).toBeVisible();
    await page.getByRole("button", { name: "Abandon interrupted run" }).click();
    await page
      .getByRole("textbox", { name: "Reason" })
      .fill("agent exited before inspection");
    await page
      .getByRole("button", { name: "Abandon run", exact: true })
      .click();
    await expect(
      page.getByRole("heading", { name: "Active inspection" }),
    ).toHaveCount(0);
    const detail = await request.get(`/api/v1/runs/${run.run.id}`);
    expect(detail.ok()).toBeTruthy();
    expect((await detail.json()).run.summary).toContain(
      "agent exited before inspection",
    );
  },
);

test(
  "preferences keep browser defaults and expose agent context",
  {
    tag: [
      "@case:attention-08",
      "@feature:setup",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
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

    await page.getByLabel("Agent instructions").fill("a".repeat(8193));
    await expect(
      page.getByRole("button", { name: "Save preferences" }),
    ).toBeDisabled();
    await expect(
      page.getByText("8193 / 8192 bytes.", { exact: false }),
    ).toBeVisible();

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
    expect(settingsBody.agents_md).toBe(
      "# Agent rules\n\nRead the brief first.",
    );
    const brief = await request.get("/api/v1/brief");
    expect(brief.ok()).toBeTruthy();
    const body = await brief.json();
    expect(body.consistency).toBe("live");
    expect(body.contexts).toBeUndefined();
    expect(settingsBody.user_md).toBe("# Owner\n\nPrefer concise evidence.");

    await page
      .getByRole("button", { name: "Reset to default" })
      .first()
      .click();
    await expect(page.getByLabel("Agent instructions")).toHaveValue(
      settingsBody.default_agents_md,
    );
    await page.getByRole("button", { name: "Reset to default" }).last().click();
    await expect(page.getByLabel("Owner context")).toHaveValue(
      settingsBody.default_user_md,
    );
    const beforeSave = await (await request.get("/api/v1/settings")).json();
    expect(beforeSave.agents_md).toBe(settingsBody.agents_md);
    expect(beforeSave.user_md).toBe(settingsBody.user_md);
    await page.getByRole("button", { name: "Save preferences" }).click();
    await expect(page.getByText("Preferences saved.")).toBeVisible();
    const afterSave = await (await request.get("/api/v1/settings")).json();
    expect(afterSave.agents_md).toBe(settingsBody.default_agents_md);
    expect(afterSave.user_md).toBe(settingsBody.default_user_md);
  },
);
