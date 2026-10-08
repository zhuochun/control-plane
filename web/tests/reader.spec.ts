import { expect, test } from "./fixtures";

test(
  "reader groups follow-up with notes and keeps relevance and source rows visible",
  {
    tag: [
      "@case:reader-12",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const marker = crypto.randomUUID();
    const createdInterest = await request.post("/api/v1/interests", {
      data: { title: `Reader context ${marker}`, instructions_md: "Fixture" },
    });
    expect(createdInterest.ok()).toBeTruthy();
    const interest = await createdInterest.json();
    const title = `Clear reader ${marker}`;
    const response = await request.post("/api/v1/items", {
      data: {
        dedupe_key: marker,
        kind: "report",
        title,
        summary: "A clear opening finding.",
        interests: [
          {
            id: interest.id,
            reason: "An owner is needed before expanding the pilot.",
          },
        ],
        sources: [
          {
            id: "first",
            url: "https://example.com/review",
            label: "Launch review",
            observed_at: "2026-10-08T01:00:00Z",
          },
          {
            id: "second",
            url: "https://example.com/call",
            label: "Customer call",
            observed_at: "2026-10-08T02:00:00Z",
          },
        ],
        report: {
          schema_version: 1,
          body_md:
            "**Recommendation:** Confirm the owner.\n\n## Evidence\n\nBoth sources agree on scope.",
        },
      },
    });
    expect(response.ok()).toBeTruthy();
    const item = await response.json();
    await page.goto("/library?q=" + encodeURIComponent(title));
    const reader = page.getByRole("region", { name: "Item details" });
    await expect(reader.locator(".reader-header button")).toHaveCount(0);
    await expect(
      reader.getByRole("region", { name: "Why this matters" }),
    ).toContainText("An owner is needed");
    await expect(reader.locator(".reader-sources > a")).toHaveCount(2);
    await expect(
      reader.getByRole("link", { name: /Launch review/ }),
    ).toBeVisible();
    await expect(
      reader.getByRole("link", { name: /Customer call/ }),
    ).toBeVisible();
    const composer = reader.locator(".reader-note");
    const note = composer.getByRole("textbox", { name: "Your note" });
    await note.fill("Keep this draft separate from actions");
    await composer
      .getByRole("button", { name: "Add Todo", exact: true })
      .click();
    await expect(
      composer.getByRole("button", { name: "Mark Done", exact: true }),
    ).toBeVisible();
    await expect(note).toHaveValue("Keep this draft separate from actions");
    expect(
      (await (await request.get(`/api/v1/items/${item.id}`)).json()).user_note,
    ).toBe("");
    await composer.getByRole("button", { name: "Remind", exact: true }).click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Set reminder", exact: true })
      .click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await composer.getByRole("button", { name: "Reminder options" }).click();
    await page
      .getByRole("menuitem", { name: "Clear reminder", exact: true })
      .click();
    await expect(composer.locator(".reader-reminder")).toHaveCount(0);
    await expect(note).toHaveValue("Keep this draft separate from actions");
    await composer
      .getByRole("button", { name: "Save note", exact: true })
      .click();
    await expect(composer.getByRole("status")).toHaveText("Note saved.");

    // New evidence arriving behind an open reminder must remain unseen.
    await page.goto("/?q=" + encodeURIComponent(title));
    await page.getByRole("button", { name: `Remind me about ${title}`, exact: true }).click();
    const beforeUpdate = await (await request.get(`/api/v1/items/${item.id}`)).json();
    const refreshedQueue = page.waitForResponse(async (response) => {
      if (!response.url().includes("/api/v1/items?") || !response.ok()) return false;
      const data = await response.json();
      return data.items.some((entry: { id: string; content_version: number }) =>
        entry.id === item.id && entry.content_version > beforeUpdate.content_version);
    });
    const updated = await request.patch(`/api/v1/items/${item.id}/work`, {
      data: {
        expected_content_version: beforeUpdate.content_version,
        report: { schema_version: 1, body_md: "A newly discovered dependency needs review." },
      },
    });
    expect(updated.ok(), await updated.text()).toBeTruthy();
    await refreshedQueue;
    // The response has arrived; let React commit the updated row before submitting.
    await expect(page.getByRole("button", { name: `Mark seen ${title}`, exact: true, includeHidden: true })).toHaveCount(1);
    await page.getByRole("dialog").getByRole("button", { name: "Set reminder", exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    const afterReminder = await (await request.get(`/api/v1/items/${item.id}`)).json();
    expect(afterReminder.remind_at).toBeTruthy();
    expect(afterReminder.content_version).toBeGreaterThan(beforeUpdate.content_version);
    expect(afterReminder.acknowledged_content_version).toBe(beforeUpdate.content_version);
  },
);

test("adding a todo marks the viewed update seen and completing it clears attention", {
  tag:["@case:reader-11", "@feature:items", "@concern:ui", "@concern:functional", "@profile:core"],
}, async ({page, request}) => {
  const title = `Follow through ${crypto.randomUUID()}`;
  const response = await request.post("/api/v1/items", {data:{dedupe_key:title,kind:"note",title,summary:"Follow-through fixture",report:{schema_version:1,body_md:"Read and follow up."}}});
  expect(response.ok()).toBeTruthy();
  const item = await response.json();
  await page.goto("/library?q="+encodeURIComponent(title));
  const reader = page.getByRole("region", {name:"Item details"});
  await expect(reader.getByRole("button", {name:"Mark seen",exact:true})).toBeVisible();
  await reader.getByRole("button", {name:"Add Todo",exact:true}).click();
  await expect(reader.getByRole("button", {name:"Mark Done",exact:true})).toBeVisible();
  await expect(reader.getByRole("button", {name:"Mark seen",exact:true})).toHaveCount(0);
  const todo = await (await request.get(`/api/v1/items/${item.id}`)).json();
  expect(todo.todo_state).toBe("todo");
  expect(todo.acknowledged_content_version).toBe(todo.content_version);
  await page.goto("/?q="+encodeURIComponent(title));
  await expect(page.locator(".queue-row")).toHaveCount(1);
  await reader.getByRole("button", {name:"Mark Done",exact:true}).click();
  await expect(page.locator(".queue-row")).toHaveCount(0);
});

test("each queue view has its own defaults, actions, and proposal scope", {
  tag:["@case:reader-10", "@feature:items", "@feature:setup", "@concern:ui", "@concern:functional", "@profile:core"],
}, async ({page, request}) => {
  const marker = crypto.randomUUID();
  const title = `Finish queue adaptation ${marker}`;
  const created = await request.post("/api/v1/items", {data:{dedupe_key:marker, kind:"task", title, summary:"View-specific task fixture", report:{schema_version:1,body_md:"Complete this local task."}}});
  expect(created.ok()).toBeTruthy();
  const item = await created.json();
  const proposed = await request.post("/api/v1/proposals", {data:{proposal_key:marker, target_type:"interest", operation:"create", rationale_md:"Queue scope fixture", payload:{title:`Queue scope ${marker}`, instructions_md:"Fixture only"}}});
  expect(proposed.ok()).toBeTruthy();
  const proposal = await proposed.json();
  try {
    await page.goto("/");
    const sort = page.getByRole("combobox", {name:"Sort items"});
    await expect(sort).toHaveValue("priority");
    await expect(page.locator(".queue-review-link")).toBeVisible();
    await sort.selectOption("oldest");
    const navigation = page.getByRole("navigation", {name:"Main navigation"});
    await navigation.getByRole("link", {name:/^Todos/}).click();
    await expect(sort).toHaveValue("priority");
    await expect(sort.locator("option:checked")).toHaveText("Due reminders first");
    await expect(page.locator(".queue-review-link")).toHaveCount(0);
    await expect(page.locator(".queue-group > summary").filter({hasText:"To do"})).toBeVisible();
    await page.locator(".queue-row").filter({hasText:title}).click();
    await page.getByRole("button", {name:`Mark done ${title}`,exact:true}).click();
    await expect(page.locator(".queue-row").filter({hasText:title})).toHaveCount(0);
    const done = await (await request.get(`/api/v1/items/${item.id}`)).json();
    expect(done.todo_state).toBe("done");
    expect(done.acknowledged_content_version).toBe(done.content_version);
    await sort.selectOption("title");
    await navigation.getByRole("link", {name:/^All items/}).click();
    await expect(sort).toHaveValue("newest");
    await expect(page.locator(".queue-group")).toHaveCount(0);
    await expect(page.locator(".queue-review-link")).toHaveCount(0);
    await expect(page.locator(".queue-row").filter({hasText:title})).toContainText("Task · Done");
    await sort.selectOption("oldest");
    await navigation.getByRole("link", {name:/^Attention/}).click();
    await expect(sort).toHaveValue("oldest");
    await expect(page.locator(".queue-review-link")).toBeVisible();
    await navigation.getByRole("link", {name:/^Todos/}).click();
    await expect(sort).toHaveValue("title");
    await navigation.getByRole("link", {name:/^All items/}).click();
    await expect(sort).toHaveValue("oldest");
    await expect(page.locator(".queue-review-link")).toHaveCount(0);
  } finally {
    const rejected = await request.post(`/api/v1/proposals/${proposal.id}/resolve`, {data:{resolution:"rejected"}});
    expect(rejected.ok()).toBeTruthy();
  }
});

test("queue sorts the collection and offers acknowledgement and reminder presets", {
  tag: ["@case:reader-09", "@feature:items", "@concern:ui", "@concern:functional", "@profile:core"],
}, async ({ page, request }) => {
  const prefix = `Queue actions ${crypto.randomUUID()}`;
  const ids: string[] = [];
  for (const suffix of ["Zulu", "Alpha", "Mike"]) {
    const response = await request.post("/api/v1/items", {data:{dedupe_key:`${prefix} ${suffix}`, title:`${prefix} ${suffix}`, summary:"Queue fixture", kind:"note", report:{schema_version:1, body_md:"Fixture evidence"}}});
    expect(response.ok(), await response.text()).toBeTruthy();
    ids.push((await response.json()).id);
  }
  await page.goto("/library?q=" + encodeURIComponent(prefix));
  await page.getByRole("combobox", {name:"Sort items"}).selectOption("title");
  await expect(page.locator(".queue-row strong")).toHaveText([`${prefix} Alpha`, `${prefix} Mike`, `${prefix} Zulu`]);
  // The HTTP order is global and continues consistently across pages.
  let cursor: string | undefined;
  const titles: string[] = [];
  do {
    const response = await request.get("/api/v1/items", {params:{q:prefix, sort:"title", limit:"1", ...(cursor ? {cursor} : {})}});
    expect(response.ok()).toBeTruthy();
    const result = await response.json();
    titles.push(...result.items.map((item:{title:string}) => item.title));
    cursor = result.next_cursor;
  } while(cursor);
  expect(titles).toEqual([`${prefix} Alpha`, `${prefix} Mike`, `${prefix} Zulu`]);
  expect((await request.get("/api/v1/items?sort=invalid")).status()).toBe(422);
  const title = `${prefix} Zulu`;
  await page.locator(".queue-row").filter({hasText:title}).click();
  await page.getByRole("button", {name:`Remind me about ${title}`,exact:true}).click();
  const dialog = page.getByRole("dialog");
  const tomorrow = await dialog.getByLabel("Date").inputValue();
  const expectedWeek = new Date(`${tomorrow}T12:00:00Z`);
  expectedWeek.setUTCDate(expectedWeek.getUTCDate() + 6);
  await dialog.getByRole("button", {name:"Next week",exact:true}).click();
  await expect(dialog.getByLabel("Date")).toHaveValue(expectedWeek.toISOString().slice(0,10));
  await expect(dialog.locator('input[type="time"]')).toHaveValue("09:00");
  await dialog.getByRole("button", {name:"Set reminder",exact:true}).click();
  await expect(dialog).toHaveCount(0);
  const reminded = await (await request.get(`/api/v1/items/${ids[0]}`)).json();
  expect(reminded.remind_at).toBeTruthy();
  expect(reminded.acknowledged_content_version).toBe(reminded.content_version);
  await expect(page.getByRole("button", {name:`Mark seen ${title}`,exact:true})).toHaveCount(0);
  await page.getByRole("button", {name:`Actions for ${prefix} Alpha`,exact:true}).click();
  await page.getByRole("menuitem", {name:"Mark seen",exact:true}).click();
  await expect.poll(async () => (await (await request.get(`/api/v1/items/${ids[1]}`)).json()).acknowledged_content_version).toBe(1);
  await page.goto("/?q=" + encodeURIComponent(prefix));
  await expect(page.locator(".queue-group > summary")).toHaveCount(1);
  await expect(page.locator(".queue-group > summary")).toContainText("New evidence");
  await expect(page.locator(".queue-row strong")).toHaveText([`${prefix} Mike`]);
  await expect.poll(() => page.locator(".workspace-list").evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
  await page.locator(".queue-group > summary").click();
  await expect(page.locator(".queue-row")).toBeHidden();
  await page.locator(".queue-group > summary").click();
  await expect(page.locator(".queue-row")).toBeVisible();
});

test(
  "inbox capture accepts one piece of information and offers it to the next run",
  {
    tag: [
      "@case:reader-01",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const content =
      "Inbox context " +
      crypto.randomUUID() +
      "\n\nPlease assess https://example.com/update in the next inspection.";
    await page.goto("/library");
    await expect(
      page.getByRole("navigation", { name: "Queue views" }),
    ).toHaveCount(0);
    await page
      .getByRole("button", { name: "Add to inbox", exact: true })
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("textbox", { name: "Information" }).fill(content);
    const capture = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/items") &&
        response.request().method() === "POST",
    );
    await dialog
      .getByRole("button", { name: "Add to inbox", exact: true })
      .click();
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
      const params = new URLSearchParams({
        after_seq: String(brief.pending_changes.after_seq),
        through_seq: String(brief.pending_changes.through_seq),
      });
      if (cursor) params.set("cursor", cursor);
      const changes = await (
        await request.get(`/api/v1/changes?${params}`)
      ).json();
      found ||= changes.items.some(
        (event: { entity_id: string; change_type: string; actor: string }) =>
          event.entity_id === item.id &&
          event.change_type === "item.created" &&
          event.actor === "user",
      );
      cursor = changes.next_cursor;
    } while (cursor);
    expect(found).toBeTruthy();
  },
);

test(
  "queue and Workspace pages fit phone, tablet, and desktop widths",
  {
    tag: [
      "@case:reader-02",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page }) => {
    for (const width of [390, 780, 1100, 1440]) {
      await page.setViewportSize({ width, height: 900 });
      for (const path of [
        "/library",
        "/interests",
        "/activity",
        "/preferences",
      ]) {
        await page.goto(path);
        await expect(
          page
            .getByRole("banner")
            .getByRole("button", { name: "Add to inbox", exact: true }),
        ).toBeVisible();
        await expect
          .poll(() =>
            page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth,
            ),
          )
          .toBe(true);
        if (path === "/library") {
          await expect
            .poll(() =>
              page.evaluate(
                () => document.documentElement.scrollHeight <= innerHeight + 1,
              ),
            )
            .toBe(true);
        }
      }
    }
  },
);

test(
  "short reports place the note inline and pin it only when the viewport requires scrolling",
  {
    tag: [
      "@case:reader-03",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const title = "Inline note " + crypto.randomUUID();
    const response = await request.post("/api/v1/items", {
      data: {
        dedupe_key: title,
        kind: "note",
        title,
        summary: "A short finding.",
        report: {
          schema_version: 1,
          body_md: "## Evidence\n\nReview this short finding.",
        },
      },
    });
    expect(response.ok()).toBeTruthy();
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto("/?q=" + encodeURIComponent(title));
    const note = page.getByRole("textbox", { name: "Your note", exact: true });
    await expect(note).toBeVisible();
    const layout = () =>
      page.evaluate(() => {
        const content = document.querySelector(".reader-scroll")!;
        const editor = document.querySelector(".reader-note")!;
        return {
          scrolls: content.scrollHeight > content.clientHeight + 1,
          noteTop: editor.getBoundingClientRect().top,
          contentBottom: content.getBoundingClientRect().bottom,
          noteBottom: editor.getBoundingClientRect().bottom,
        };
      });
    await expect.poll(async () => (await layout()).scrolls).toBe(false);
    expect((await layout()).noteBottom).toBeLessThan(900);
    expect(
      Math.abs((await layout()).noteTop - (await layout()).contentBottom),
    ).toBeLessThan(2);
    await note.fill("Keep my draft through resizing");
    await page.setViewportSize({ width: 1440, height: 360 });
    await expect.poll(async () => (await layout()).scrolls).toBe(true);
    await expect.poll(async () => (await layout()).noteBottom).toBe(360);
    await expect(note).toBeInViewport();
    await page.setViewportSize({ width: 1440, height: 1000 });
    await expect.poll(async () => (await layout()).scrolls).toBe(false);
    await expect(note).toHaveValue("Keep my draft through resizing");
    await expect(
      page
        .getByRole("banner")
        .getByRole("button", { name: "Add to inbox", exact: true }),
    ).toBeVisible();
    await expect(page.locator(".workspace-toolbar")).toHaveCount(0);
    await page.getByText("Filters", { exact: true }).click();
    const interest = (await page
      .getByLabel("Filter by Interest", { exact: true })
      .boundingBox())!;
    const kind = (await page
      .getByLabel("Filter by kind", { exact: true })
      .boundingBox())!;
    const panel = (await page.locator(".queue-filters").boundingBox())!;
    expect(kind.x - (interest.x + interest.width)).toBeGreaterThanOrEqual(8);
    expect(kind.x + kind.width).toBeLessThanOrEqual(panel.x + panel.width - 10);
  },
);

test(
  "metric chart uses the explicit target instead of the distance above target",
  {
    tag: [
      "@case:reader-04",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const title = "Metric target " + crypto.randomUUID();
    const response = await request.post("/api/v1/items", {
      data: {
        dedupe_key: title,
        kind: "report",
        title,
        summary: "Cost fell from $100 to $82.",
        report: {
          schema_version: 1,
          body_md:
            "The latest cost remains $2 above target.\n\n| Week | Cost |\n| --- | --- |\n| Sep 2 | $100 |\n| Sep 30 | $82 |\n\nTarget: $80\n\nMetric attention: Automation cost",
        },
      },
    });
    expect(response.ok()).toBeTruthy();
    await page.goto("/?q=" + encodeURIComponent(title));
    await expect(
      page.getByRole("img", {
        name: "Automation cost: Sep 2 $100, Sep 30 $82; target $80",
        exact: true,
      }),
    ).toBeVisible();
    await expect(page.locator(".reader-metrics")).toContainText("$80");
    await expect(page.locator(".reader-chart-heading")).toHaveText(
      "Automation costTarget $80",
    );
    const noTargetTitle = title + " without explicit target";
    const noTarget = await request.post("/api/v1/items", {
      data: {
        dedupe_key: noTargetTitle,
        kind: "report",
        title: noTargetTitle,
        summary: "Cost fell from $100 to $82.",
        report: {
          schema_version: 1,
          body_md:
            "The latest cost remains $2 above target.\n\nMetric attention: Automation cost",
        },
      },
    });
    expect(noTarget.ok()).toBeTruthy();
    await page.goto("/?q=" + encodeURIComponent(noTargetTitle));
    await expect(
      page.getByRole("img", {
        name: "Automation cost: $100, $82",
        exact: true,
      }),
    ).toBeVisible();
    await expect(page.locator(".reader-chart-heading")).toHaveText(
      "Automation cost",
    );
  },
);

test(
  "acknowledging an automatically opened item keeps its report and note draft available",
  {
    tag: [
      "@case:reader-05",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
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
    await page
      .getByRole("button", { name: "Mark seen", exact: true })
      .click();
    await expect(
      page.locator(".queue-row").filter({hasText:title}),
    ).toHaveCount(0);
    await expect(
      page.getByRole("heading", { name: "Original report" }),
    ).toBeVisible();
    await expect(note).toHaveValue("Keep this follow-up");
    await page.getByRole("button", { name: "Save note", exact: true }).click();
    await expect(page.getByRole("status")).toHaveText("Note saved.");
  },
);

test(
  "complete reports retain a pinned note draft across scrolling, switching, and saving",
  {
    tag: [
      "@case:reader-06",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
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
    await create(
      "Second " + marker,
      "## Second report\n\nA different finding.",
    );
    await page.goto("/library?q=" + marker);
    await page
      .locator(".queue-row").filter({hasText:"First " + marker})
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
      .locator(".queue-row").filter({hasText:"Second " + marker})
      .click();
    await expect(note).toHaveValue("");
    await note.fill("Second draft");
    await page
      .locator(".queue-row").filter({hasText:"First " + marker})
      .click();
    await expect(note).toHaveValue("Keep this draft while I compare findings.");
    await page.getByRole("button", { name: "Save note", exact: true }).click();
    await expect(page.getByRole("status")).toHaveText("Note saved.");
    expect(
      (await (await request.get("/api/v1/items/" + first.id)).json()).user_note,
    ).toBe("Keep this draft while I compare findings.");
    await page.reload();
    await page
      .locator(".queue-row").filter({hasText:"First " + marker})
      .click();
    await expect(note).toHaveValue("Keep this draft while I compare findings.");
  },
);

test(
  "saving an older note does not discard edits typed while it is saving",
  {
    tag: [
      "@case:reader-07",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
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
  },
);

test(
  "mobile report and pinned note remain usable without horizontal page overflow",
  {
    tag: [
      "@case:reader-08",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
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
    await page.locator(".queue-row").filter({hasText:title}).click();
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
  },
);
