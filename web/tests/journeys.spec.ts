import { journey as test, expect } from "./fixtures";
import {
  record,
  configure,
  cli,
  connect,
  publishReview,
  reviewFormat,
  answerEvents,
  values,
} from "./journey-support";
import { writeFileSync } from "node:fs";
import path from "node:path";

test(
  "MCP review reaches the person and the next consumer after restart",
  {
    tag: [
      "@case:review-roundtrip",
      "@scenario:review-roundtrip",
      "@feature:reviews",
      "@feature:diagrams",
      "@concern:contract",
      "@concern:ui",
      "@concern:persistence",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, request, ownedServer: server }, info) => {
    const report = record("review-roundtrip", "VER-review", info);
    let mcp = await connect(server);
    try {
      const format = await mcp.call("register_review_format", {
        format: reviewFormat,
      });
      report.check(
        "R1",
        "Agent registers an immutable format through MCP stdio",
        { id: format.id, version: format.version },
      );
      const item = await publishReview(mcp);
      report.check("R2", "Agent publishes and reads the review through MCP", {
        content_version: item.content_version,
        answer_count: (item.answers ?? []).length,
      });
      await page.goto("/library?q=Delivery%20review");
      const form = page.getByRole("form", { name: "Delivery review" });
      await expect(
        page.getByRole("img", { name: "Review flow", exact: true }),
      ).toBeVisible();
      await form.getByRole("radio", { name: "Local", exact: true }).check();
      await form
        .getByLabel("Additional instructions")
        .fill("Keep the rollout small.");
      await form
        .getByRole("button", { name: "Save answer", exact: true })
        .click();
      await expect(
        page.getByRole("region", { name: "Your answers" }),
      ).toContainText("Delivery approach: Local");
      const first = await mcp.call("get_item", { item_id: item.id });
      const original = structuredClone(first.answers[0]);
      report.check(
        "R3",
        "Person chooses Local and saves instructions; agent reads labels and IDs",
        values(original),
      );
      expect(first.todo_state).toBe("none");
      expect(first.acknowledged_content_version).toBe(0);
      expect(first.content_version).toBe(item.content_version);
      expect(Number.isNaN(Date.parse(original.submitted_at))).toBe(false);
      await report.capture(page, "03-answer-saved-desktop.png");
      await page.setViewportSize({ width: 390, height: 844 });
      await expect
        .poll(() =>
          page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        )
        .toBe(true);
      await report.capture(page, "03-answer-saved-mobile.png");
      await report.capture(
        page.getByRole("region", { name: "Your answers" }),
        "03-answer-note-mobile.png",
        {},
      );
      await page.setViewportSize({ width: 1440, height: 1000 });
      await report.capture(
        page.getByRole("region", { name: "Your answers" }),
        "03-answer-note-desktop.png",
        {},
      );
      await mcp.close();
      const previous = server.child.pid;
      await server.restart();
      report.check(
        "R4",
        "Maintainer restarts the owned server with the same database",
        { fresh_pid: previous !== server.child.pid },
      );
      mcp = await connect(server);
      const persisted = await mcp.call("get_item", { item_id: item.id });
      const events = await answerEvents(mcp, item.id);
      expect(events[0].actor).toBe("user");
      report.check(
        "R5",
        "Fresh agent discovers the answer in the brief change range and reads the Item",
        {
          same_item: persisted.id === item.id,
          answer_count: persisted.answers.length,
          answer_event_count: events.length,
          applicable: persisted.answers[0].applicable,
          todo_state: persisted.todo_state,
          acknowledged_content_version: persisted.acknowledged_content_version,
          same_snapshot:
            JSON.stringify(persisted.answers[0]) === JSON.stringify(original),
        },
      );
      await page.reload();
      await expect(
        page.getByRole("region", { name: "Your answers" }),
      ).toContainText("Keep the rollout small.");
      await report.capture(page, "05-after-restart.png");
      await form.getByRole("radio", { name: "Remote", exact: true }).check();
      await form
        .getByRole("button", { name: "Save answer", exact: true })
        .click();
      await expect(
        page.getByRole("region", { name: "Your answers" }),
      ).toContainText("Delivery approach: Remote");
      const current = await mcp.call("get_item", { item_id: item.id });
      const history = await mcp.call("get_answer_history", {
        item_id: item.id,
      });
      expect(
        history.items.every((answer: any) => answer.applicable === false),
      ).toBe(true);
      expect(
        Date.parse(current.answers[0].submitted_at),
      ).toBeGreaterThanOrEqual(Date.parse(original.submitted_at));
      report.check(
        "R6",
        "Person corrects to Remote; agent reads current answer and immutable history",
        {
          selected_ids: current.answers[0].values[0].selected_ids,
          answer_label: current.answers[0].values[0].answer,
          current_answer_count: current.answers.length,
          history_count: history.items.length,
          answer_event_count: (await answerEvents(mcp, item.id)).length,
          supersedes_original: current.answers[0].supersedes === original.id,
          original_snapshot_unchanged:
            JSON.stringify(history.items[0]) ===
            JSON.stringify({ ...original, applicable: false }),
        },
      );
      writeFileSync(
        info.outputPath("journey/raw-mcp.json"),
        JSON.stringify(mcp.raw, null, 2),
      );
      await report.capture(
        page.getByRole("region", { name: "Your answers" }),
        "06-correction-history.png",
        {},
      );
    } finally {
      await mcp.close();
      report.write();
    }
  },
);

test(
  "configuration is visible without creating a Run",
  {
    tag: [
      "@case:setup-no-run",
      "@scenario:setup-no-run",
      "@feature:setup",
      "@concern:contract",
      "@concern:ui",
      "@profile:core",
    ],
  },
  async ({ page, ownedServer: server }, info) => {
    const report = record("setup-no-run", "VER-setup", info);
    const ownerFile = path.join(server.data, "USER.md");
    writeFileSync(ownerFile, "Keep inspections bounded.");
    await cli(server, ["config", "set", "Asia/Singapore"]);
    await cli(server, ["config", "user-context", "set", "--file", ownerFile]);
    const settings = await cli(server, ["config", "get"]);
    report.check("S1", "Owner saves timezone and context through CLI", {
      timezone: settings.timezone,
      owner_context: settings.user_md,
    });
    const { interest, watch } = await configure(server);
    const mcp = await connect(server);
    try {
      const brief = await mcp.call("get_brief");
      const runs = await cli(server, ["run", "list"]);
      report.check(
        "S2",
        "CLI configures Interest and Watcher; MCP reads due work",
        {
          interest_title: interest.title,
          source: watch.source.locator,
          run_count: runs.items.length,
          due_watch_count: brief.due_watches.items.length,
        },
      );
      await page.goto("/interests");
      await expect(
        page.getByText("Delivery economics", { exact: true }).first(),
      ).toBeVisible();
      await expect(
        page.getByText("fixtures/demo-costs", { exact: false }).first(),
      ).toBeVisible();
      report.check(
        "S3",
        "Person inspects configured Interest/Watcher without inspection",
        {
          portal_interest_visible: true,
          portal_source_visible: true,
          run_count: (await cli(server, ["run", "list"])).items.length,
        },
      );
      await report.capture(page, "03-configured.png");
    } finally {
      await mcp.close();
      report.write();
    }
  },
);

test(
  "two CLI inspection cycles retain portal-owned follow-up state",
  {
    tag: [
      "@case:inspection-two-cycles",
      "@scenario:inspection-two-cycles",
      "@feature:items",
      "@feature:runs",
      "@concern:contract",
      "@concern:ui",
      "@concern:persistence",
      "@profile:core",
    ],
  },
  async ({ page, ownedServer: server }, info) => {
    const report = record("inspection-two-cycles", "VER-inspection", info);
    const { interest, watch } = await configure(server);
    await cli(server, ["run", "start", "--runner-label", "offline-fixture"]);
    const itemInput = {
      dedupe_key: "demo:costs:comparable",
      expected_content_version: 0,
      kind: "report",
      title: "Automation cost fell 18%",
      summary: "Comparable workload fell from $100 to $82.",
      interests: [{ id: interest.id, reason: "Material delivery cost change" }],
      sources: [
        {
          id: "costs",
          url: "https://example.com/fixtures/costs",
          label: "Comparable cost fixture",
          observed_at: "2026-09-15T02:00:00Z",
        },
      ],
      context_md: "Recheck the next complete period.",
      report: {
        schema_version: 1,
        body_md:
          "## Comparable result\nCost fell from $100 to $82, an 18% reduction.",
      },
    };
    const result = await cli(server, ["run", "submit", watch.id], {
      request_id: "cycle-1",
      expected_watch_revision: 1,
      status: "success",
      coverage: {
        cursor_before: null,
        cursor_after: { cycle: 1 },
        observed_through: "2026-09-15T02:00:00Z",
        limitations: [],
      },
      items: [
        itemInput,
        {
          ...itemInput,
          dedupe_key: "demo:costs:incomplete",
          kind: "note",
          title: "Incomplete period cannot be compared",
          summary: "Comparison unavailable for partial periods.",
          report: { schema_version: 1, body_md: "Wait for a complete period." },
        },
      ],
    });
    await cli(server, [
      "run",
      "finish",
      "--summary",
      "Saved comparable and incomplete cost findings.",
    ]);
    const item = result.items[0];
    report.check(
      "I1",
      "CLI publishes comparable/incomplete findings and advances successful coverage",
      {
        item_count: result.items.length,
        content_version: item.content_version,
        checkpoint: (await cli(server, ["watch", "get", watch.id])).cursor,
      },
    );
    await page.goto("/?q=Automation%20cost");
    await page.getByRole("button", { name: "Add Todo", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "Mark Done", exact: true }),
    ).toBeVisible();
    await page
      .getByRole("textbox", { name: "Your note", exact: true })
      .fill("Check again next week.");
    await page.getByRole("button", { name: "Save note", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "Save note", exact: true }),
    ).toBeDisabled();
    await page.getByRole("button", { name: "Remind", exact: true }).click();
    await page.getByLabel("Date").fill("2099-09-16");
    await page
      .getByRole("button", { name: "Set reminder", exact: true })
      .click();
    await page.getByRole("button", { name: "Reminder options" }).click();
    await expect(
      page.getByRole("menuitem", { name: "Clear reminder", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("Escape");
    const before = await cli(server, ["item", "get", item.id]);
    report.check(
      "I2",
      "Person saves Todo, note and date-only reminder in portal",
      {
        todo_state: before.todo_state,
        user_note: before.user_note,
        reminder_timezone: before.reminder_timezone,
        reminder_date: new Intl.DateTimeFormat("en-CA", {
          timeZone: before.reminder_timezone,
        }).format(new Date(before.remind_at)),
        state_version: before.state_version,
      },
    );
    await report.capture(page, "02-human-state.png");
    await cli(server, [
      "run",
      "start",
      "--runner-label",
      "offline-fixture",
      "--watch",
      watch.id,
      "--force",
    ]);
    const input2 = {
      ...itemInput,
      expected_content_version: 1,
      summary:
        "The comparable workload still shows an 18% reduction; source review complete.",
      report: {
        schema_version: 1,
        body_md:
          "## Reconciled result\nThe same workload remains 18% lower. Source review complete.",
      },
    };
    const publish2 = {
      request_id: "cycle-2",
      expected_watch_revision: 1,
      status: "success",
      coverage: {
        cursor_before: { cycle: 1 },
        cursor_after: { cycle: 2 },
        observed_through: "2026-09-15T04:00:00Z",
        limitations: [],
      },
      items: [input2],
    };
    await cli(server, ["run", "submit", watch.id], publish2);
    // Replay before finish proves the unchanged terminal submission is idempotent.
    const replay = await cli(server, ["run", "submit", watch.id], publish2);
    await cli(server, [
      "run",
      "finish",
      "--summary",
      "Reconciled completed source review.",
    ]);
    const final = await cli(server, ["item", "get", item.id]);
    report.check(
      "I3",
      "CLI republishes the same matter with substantive content change",
      {
        same_item: final.id === item.id,
        content_version: final.content_version,
        state_version: final.state_version,
        todo_state: final.todo_state,
        user_note: final.user_note,
        same_reminder: final.remind_at === before.remind_at,
        checkpoint: (await cli(server, ["watch", "get", watch.id])).cursor,
      },
    );
    report.check(
      "I4",
      "Unchanged retry preserves Item identity, versions and checkpoint",
      {
        same_item: replay.items[0].id === item.id,
        content_version: final.content_version,
        state_version: final.state_version,
        checkpoint: (await cli(server, ["watch", "get", watch.id])).cursor,
      },
    );
    await page.reload();
    await expect(
      page.getByRole("textbox", { name: "Your note", exact: true }),
    ).toHaveValue("Check again next week.");
    await expect(
      page.getByRole("button", { name: "Mark Done", exact: true }),
    ).toBeVisible();
    report.write();
  },
);

test(
  "lost committed answer response and stale correction remain recoverable",
  {
    tag: [
      "@case:recovery-answer",
      "@concern:contract",
      "@scenario:recovery",
      "@feature:reviews",
      "@concern:ui",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, request, ownedServer: server }, info) => {
    const report = record("recovery-answer", "VER-recovery", info);
    const mcp = await connect(server);
    try {
      const item = await publishReview(mcp);
      const requests: string[] = [];
      await page.route(`**/items/${item.id}/answers`, async (route) => {
        requests.push(route.request().postData()!);
        if (requests.length === 1) {
          const committed = await route.fetch();
          expect(committed.ok()).toBeTruthy();
          await route.abort("failed");
        } else await route.continue();
      });
      await page.goto("/library?q=Delivery%20review");
      const form = page.getByRole("form", { name: "Delivery review" });
      const text = form.getByLabel("Additional instructions");
      await form.getByRole("radio", { name: "Local", exact: true }).check();
      await text.fill("Keep this draft");
      await form
        .getByRole("button", { name: "Save answer", exact: true })
        .click();
      await expect(form.getByRole("alert")).toContainText("Your input is kept");
      report.check(
        "A1",
        "Fault barrier commits the answer, loses its response and retains draft",
        {
          response_lost: requests.length === 1,
          draft: await text.inputValue(),
        },
      );
      await form
        .getByRole("button", { name: "Save answer", exact: true })
        .click();
      await expect(form.getByRole("alert")).toContainText("Answer saved");
      report.check("A2", "Person retries the exact original request", {
        same_request: requests[0] === requests[1],
        answer_count: (
          await mcp.call("get_answer_history", { item_id: item.id })
        ).items.length,
        answer_event_count: (await answerEvents(mcp, item.id)).length,
      });
      await text.fill("Preserve the correction");
      const current = await mcp.call("get_item", { item_id: item.id });
      const changed = structuredClone(current.report);
      changed.body_md = "Updated decision evidence";
      await mcp.call("update_item_work", {
        item_id: item.id,
        update: {
          expected_content_version: current.content_version,
          report: changed,
        },
      });
      const rejected = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/items/${item.id}/answers`) && r.status() === 409,
      );
      await form
        .getByRole("button", { name: "Save answer", exact: true })
        .click();
      await rejected;
      await expect(form.getByRole("alert")).toContainText(
        "Review content changed",
      );
      report.check("A3", "Stale correction is refused; person retains input", {
        stale_write_rejected: true,
        draft: await text.inputValue(),
        history_count: (
          await mcp.call("get_answer_history", { item_id: item.id })
        ).items.length,
      });
      await form
        .getByRole("button", { name: "Save answer", exact: true })
        .click();
      await expect(
        page.getByRole("region", { name: "Your answers" }),
      ).toContainText("Preserve the correction");
      const after = await mcp.call("get_item", { item_id: item.id });
      report.check("A4", "Person saves against refreshed material", {
        current_text: after.answers[0].values[1].text,
        history_count: (
          await mcp.call("get_answer_history", { item_id: item.id })
        ).items.length,
      });
      await report.capture(page, "04-recovered.png");
    } finally {
      await mcp.close();
      report.write();
    }
  },
);

test(
  "unfinished Run survives restart and the owner can release its slot",
  {
    tag: [
      "@case:recovery-run",
      "@concern:contract",
      "@scenario:recovery",
      "@feature:runs",
      "@concern:ui",
      "@concern:persistence",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, ownedServer: server }, info) => {
    const report = record("recovery-run", "VER-recovery", info);
    const { watch } = await configure(server);
    let mcp = await connect(server);
    try {
      const started = await mcp.call("start_run", {
        runner_label: "interrupted-fixture",
      });
      report.check("U1", "Agent claims a source inspection through MCP", {
        status: started.run.status,
        watch_count: (await cli(server, ["run", "get", started.run.id]))
          .selected_watches.length,
      });
      const previous = server.child.pid;
      await mcp.close();
      await server.restart();
      mcp = await connect(server);
      const active = await mcp.call("get_status");
      const resumed = await cli(server, ["run", "get", started.run.id]);
      report.check("U2", "Fresh process/consumer inspect the unfinished Run", {
        same_run: resumed.run.id === started.run.id,
        status: resumed.run.status,
        fresh_pid: previous !== server.child.pid,
        checkpoint: (await cli(server, ["watch", "get", watch.id])).cursor,
      });
      await page.goto("/activity");
      await expect(
        page.getByRole("heading", { name: "Active inspection" }),
      ).toBeVisible();
      await expect(page.getByText("No result", { exact: false })).toBeVisible();
      await page
        .getByRole("button", { name: "Abandon interrupted run" })
        .click();
      await page
        .getByRole("textbox", { name: "Reason" })
        .fill("agent exited before inspection");
      await page
        .getByRole("button", { name: "Abandon run", exact: true })
        .click();
      await expect(
        page.getByRole("heading", { name: "Active inspection" }),
      ).toHaveCount(0);
      const abandoned = await cli(server, ["run", "get", started.run.id]);
      report.check(
        "U3",
        "Owner abandons interrupted Run in portal without advancing checkpoint",
        {
          status: abandoned.run.status,
          reason: abandoned.run.summary,
          checkpoint: (await cli(server, ["watch", "get", watch.id])).cursor,
        },
      );
      const next = await mcp.call("start_run", {
        runner_label: "recovery-fixture",
        force: true,
        watch_ids: [watch.id],
      });
      report.check("U4", "A fresh inspection can claim the released slot", {
        new_run: next.run.id !== started.run.id,
        status: next.run.status,
      });
      await report.capture(page, "03-owner-recovery.png");
    } finally {
      await mcp.close();
      report.write();
    }
  },
);
