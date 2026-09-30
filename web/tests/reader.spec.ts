import { expect, test } from "@playwright/test";

test("inbox capture accepts one piece of information and offers it to the next run", async ({ page, request }) => {
  const content = "Inbox context " + crypto.randomUUID() + "\n\nPlease assess https://example.com/update in the next inspection.";
  await page.goto("/library");
  await expect(page.getByRole("navigation", { name: "Queue views" })).toHaveCount(0);
  await page.getByRole("button", { name: "Add to inbox", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox", { name: "Information" }).fill(content);
  const capture = page.waitForResponse(response => response.url().endsWith("/api/v1/items") && response.request().method() === "POST");
  await dialog.getByRole("button", { name: "Add to inbox", exact: true }).click();
  const response = await capture;
  expect(response.ok()).toBeTruthy();
  const item = await response.json();
  expect(item.kind).toBe("note");
  expect(item.todo_state).toBe("none");
  expect(item.report.body_md).toBe(content);
  await expect(dialog).toHaveCount(0);
  const brief = await (await request.get("/api/v1/brief")).json();
  expect(brief.consistency).toBe("live");
  expect(brief.pending_changes.count).toBeGreaterThan(0);
  let cursor: string | undefined;
  let found = false;
  do {
    const params = new URLSearchParams({ after_seq: String(brief.pending_changes.after_seq), through_seq: String(brief.pending_changes.through_seq) });
    if (cursor) params.set("cursor", cursor);
    const changes = await (await request.get(`/api/v1/changes?${params}`)).json();
    found ||= changes.items.some((event: { entity_id: string; change_type: string; actor: string }) => event.entity_id === item.id && event.change_type === "item.created" && event.actor === "user");
    cursor = changes.next_cursor;
  } while (cursor);
  expect(found).toBeTruthy();
});

test("queue and Workspace pages fit phone, tablet, and desktop widths", async ({ page }) => {
  for (const width of [390, 780, 1100, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    for (const path of ["/library", "/interests", "/activity", "/preferences"]) {
      await page.goto(path);
      await expect(page.getByRole("banner").getByRole("button", { name: "Add to inbox", exact: true })).toBeVisible();
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      if (path === "/library") {
        await expect.poll(() => page.evaluate(() => document.documentElement.scrollHeight <= innerHeight + 1)).toBe(true);
      }
    }
  }
});

test("short reports place the note inline and pin it only when the viewport requires scrolling", async ({ page, request }) => {
  const title = "Inline note " + crypto.randomUUID();
  const response = await request.post("/api/v1/items", { data: {
    dedupe_key: title, kind: "note", title, summary: "A short finding.",
    report: { schema_version: 1, body_md: "## Evidence\n\nReview this short finding." },
  } });
  expect(response.ok()).toBeTruthy();
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/?q=" + encodeURIComponent(title));
  const note = page.getByRole("textbox", { name: "Your note", exact: true });
  await expect(note).toBeVisible();
  const layout = () => page.evaluate(() => {
    const content = document.querySelector(".reader-scroll")!;
    const editor = document.querySelector(".reader-note")!;
    return { scrolls: content.scrollHeight > content.clientHeight + 1, noteTop: editor.getBoundingClientRect().top, contentBottom: content.getBoundingClientRect().bottom, noteBottom: editor.getBoundingClientRect().bottom };
  });
  await expect.poll(async () => (await layout()).scrolls).toBe(false);
  expect((await layout()).noteBottom).toBeLessThan(900);
  expect(Math.abs((await layout()).noteTop - (await layout()).contentBottom)).toBeLessThan(2);
  await note.fill("Keep my draft through resizing");
  await page.setViewportSize({ width: 1440, height: 360 });
  await expect.poll(async () => (await layout()).scrolls).toBe(true);
  await expect.poll(async () => (await layout()).noteBottom).toBe(360);
  await expect(note).toBeInViewport();
  await page.setViewportSize({ width: 1440, height: 1000 });
  await expect.poll(async () => (await layout()).scrolls).toBe(false);
  await expect(note).toHaveValue("Keep my draft through resizing");
  await expect(page.getByRole("banner").getByRole("button", { name: "Add to inbox", exact: true })).toBeVisible();
  await expect(page.locator(".workspace-toolbar")).toHaveCount(0);
  await page.getByText("Filters", { exact: true }).click();
  const interest = (await page.getByLabel("Filter by Interest", { exact: true }).boundingBox())!;
  const kind = (await page.getByLabel("Filter by kind", { exact: true }).boundingBox())!;
  const panel = (await page.locator(".queue-filters").boundingBox())!;
  expect(kind.x - (interest.x + interest.width)).toBeGreaterThanOrEqual(8);
  expect(kind.x + kind.width).toBeLessThanOrEqual(panel.x + panel.width - 10);
});

test("metric chart uses the explicit target instead of the distance above target", async ({ page, request }) => {
  const title = "Metric target " + crypto.randomUUID();
  const response = await request.post("/api/v1/items", {
    data: {
      dedupe_key: title, kind: "report", title,
      summary: "Cost fell from $100 to $82.",
      report: { schema_version: 1, body_md: "The latest cost remains $2 above target.\n\n| Week | Cost |\n| --- | --- |\n| Sep 2 | $100 |\n| Sep 30 | $82 |\n\nTarget: $80\n\nMetric attention: Automation cost" },
    },
  });
  expect(response.ok()).toBeTruthy();
  await page.goto("/?q=" + encodeURIComponent(title));
  await expect(page.getByRole("img", { name: "Automation cost: Sep 2 $100, Sep 30 $82; target $80", exact: true })).toBeVisible();
  await expect(page.locator(".reader-metrics")).toContainText("$80");
  await expect(page.locator(".reader-chart-heading")).toHaveText("Automation costTarget $80");
  const noTargetTitle = title + " without explicit target";
  const noTarget = await request.post("/api/v1/items", {
    data: { dedupe_key: noTargetTitle, kind: "report", title: noTargetTitle,
      summary: "Cost fell from $100 to $82.",
      report: { schema_version: 1, body_md: "The latest cost remains $2 above target.\n\nMetric attention: Automation cost" } },
  });
  expect(noTarget.ok()).toBeTruthy();
  await page.goto("/?q=" + encodeURIComponent(noTargetTitle));
  await expect(page.getByRole("img", { name: "Automation cost: $100, $82", exact: true })).toBeVisible();
  await expect(page.locator(".reader-chart-heading")).toHaveText("Automation cost");
});

test("acknowledging an automatically opened item keeps its report and note draft available", async ({
  page,
  request,
}) => {
  const title = "Auto-selected " + crypto.randomUUID();
  const response = await request.post("/api/v1/items", {
    data: {
      dedupe_key: title,
      kind: "note",
      title,
      summary: "New evidence",
      report: {
        schema_version: 1,
        body_md:
          "## Original report\n\nEvidence remains readable after acknowledgement.",
      },
    },
  });
  expect(response.ok()).toBeTruthy();
  await page.goto("/?q=" + encodeURIComponent(title));
  await expect(
    page.getByRole("heading", { name: "Original report" }),
  ).toBeVisible();
  const note = page.getByRole("textbox", { name: "Your note", exact: true });
  await note.fill("Keep this follow-up");
  await page.getByRole("button", { name: "Acknowledge", exact: true }).click();
  await expect(
    page.getByRole("button", { name: new RegExp(title) }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Original report" }),
  ).toBeVisible();
  await expect(note).toHaveValue("Keep this follow-up");
  await page.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Note saved.");
});

test("complete reports retain a pinned note draft across scrolling, switching, and saving", async ({
  page,
  request,
}) => {
  const marker = crypto.randomUUID();
  const create = async (title: string, body: string) => {
    const response = await request.post("/api/v1/items", {
      data: {
        dedupe_key: title,
        kind: "report",
        title,
        summary: "Review the complete evidence.",
        sources: [
          {
            id: "source",
            label: "Evidence source",
            url: "https://example.com/evidence",
            observed_at: "2026-09-30T01:00:00Z",
          },
        ],
        report: {
          schema_version: 1,
          body_md: body,
          actions: [
            {
              id: "open",
              type: "open_link",
              source_ref: "source",
              label: "Open evidence",
            },
          ],
        },
      },
    });
    expect(response.ok()).toBeTruthy();
    return response.json();
  };
  const first = await create(
    "First " + marker,
    "## Full report\n\n" +
      "Evidence stays visible in the complete report.\n\n".repeat(70) +
      "## Final evidence\n\n| Detail | Result |\n| --- | --- |\n| Coverage | Complete |",
  );
  await create("Second " + marker, "## Second report\n\nA different finding.");
  await page.goto("/library?q=" + marker);
  await page
    .getByRole("button", { name: new RegExp("First " + marker) })
    .click();
  const note = page.getByRole("textbox", { name: "Your note", exact: true });
  await note.fill("Keep this draft while I compare findings.");
  const before = await note.boundingBox();
  expect(before).not.toBeNull();
  await page
    .getByLabel("Report content", { exact: true })
    .evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
  await expect(
    page.getByRole("heading", { name: "Final evidence" }),
  ).toBeVisible();
  await expect(page.getByRole("table")).toBeVisible();
  const after = await note.boundingBox();
  expect(Math.abs(after!.y - before!.y)).toBeLessThan(2);
  await expect(note).toBeInViewport();
  await expect(
    page.getByRole("link", { name: "Open evidence" }),
  ).toHaveAttribute("href", "https://example.com/evidence");
  await page
    .getByRole("button", { name: new RegExp("Second " + marker) })
    .click();
  await expect(note).toHaveValue("");
  await note.fill("Second draft");
  await page
    .getByRole("button", { name: new RegExp("First " + marker) })
    .click();
  await expect(note).toHaveValue("Keep this draft while I compare findings.");
  await page.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Note saved.");
  expect(
    (await (await request.get("/api/v1/items/" + first.id)).json()).user_note,
  ).toBe("Keep this draft while I compare findings.");
  await page.reload();
  await page
    .getByRole("button", { name: new RegExp("First " + marker) })
    .click();
  await expect(note).toHaveValue("Keep this draft while I compare findings.");
});

test("saving an older note does not discard edits typed while it is saving", async ({
  page,
  request,
}) => {
  const title = "Note race " + crypto.randomUUID();
  const response = await request.post("/api/v1/items", {
    data: {
      dedupe_key: title,
      kind: "note",
      title,
      summary: "Compare drafts",
      report: { schema_version: 1, body_md: "Draft race evidence" },
    },
  });
  expect(response.ok()).toBeTruthy();
  const item = await response.json();
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let started!: () => void;
  const saving = new Promise<void>((resolve) => {
    started = resolve;
  });
  await page.route("**/api/v1/items/" + item.id + "/note", async (route) => {
    const saved = await route.fetch();
    started();
    await gate;
    await route.fulfill({ response: saved });
  });
  await page.goto("/library?q=" + encodeURIComponent(title));
  const note = page.getByRole("textbox", { name: "Your note", exact: true });
  await note.fill("Submitted draft");
  await page.getByRole("button", { name: "Save note", exact: true }).click();
  await saving;
  await note.fill("Newer draft typed during save");
  await expect(
    page.getByRole("button", { name: "Saving…", exact: true }),
  ).toBeDisabled();
  release();
  await expect(
    page.getByRole("button", { name: "Save note", exact: true }),
  ).toBeEnabled();
  await expect(note).toHaveValue("Newer draft typed during save");
  await page.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Note saved.");
  expect(
    (await (await request.get("/api/v1/items/" + item.id)).json()).user_note,
  ).toBe("Newer draft typed during save");
});

test("mobile report and pinned note remain usable without horizontal page overflow", async ({
  page,
  request,
}) => {
  const title = "Mobile report " + crypto.randomUUID();
  const response = await request.post("/api/v1/items", {
    data: {
      dedupe_key: title,
      kind: "report",
      title,
      summary: "Costs fell from $100 to $82; target $80.",
      report: {
        schema_version: 1,
        body_md:
          "## Evidence\n\n| Week | Cost |\n| --- | --- |\n| Sep 2 | $100 |\n| Sep 30 | $82 |\n\n" +
          "Full evidence.\n\n".repeat(30),
      },
    },
  });
  expect(response.ok()).toBeTruthy();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/library?q=" + encodeURIComponent(title));
  await page.getByRole("button", { name: new RegExp(title) }).click();
  await expect(page.getByRole("heading", { name: "Evidence" })).toBeVisible();
  const note = page.getByRole("textbox", { name: "Your note", exact: true });
  await expect(note).toBeInViewport();
  await page
    .getByLabel("Report content", { exact: true })
    .evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
  await note.fill("Mobile follow-up");
  await expect(
    page.getByRole("button", { name: "Save note", exact: true }),
  ).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBeTruthy();
  await page
    .getByRole("button", { name: "Close item detail", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "All items", exact: true }),
  ).toBeVisible();
});
