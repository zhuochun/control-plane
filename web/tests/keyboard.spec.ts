import { isolated as test, expect } from "./fixtures";
import type { APIRequestContext, Page } from "@playwright/test";

async function enable(page: Page) {
  await page.goto("/preferences");
  await page.getByRole("switch", { name: "Vim keyboard shortcuts" }).check();
}
async function seed(request: APIRequestContext, titles: string[]) {
  const items = [];
  for (const title of titles) {
    const response = await request.post("/api/v1/items", {
      data: {
        dedupe_key: crypto.randomUUID(),
        kind: "report",
        title,
        summary: "Keyboard fixture",
        report: { schema_version: 1, body_md: `Details for ${title}` },
      },
    });
    expect(response.ok(), await response.text()).toBeTruthy();
    items.push(await response.json());
  }
  return items;
}
const reader = (page: Page) =>
  page.getByRole("region", { name: "Item details" });
const note = (page: Page) =>
  reader(page).getByRole("textbox", { name: "Your note", exact: true });
const row = (page: Page, title: string) =>
  page
    .locator(".queue-row")
    .filter({ has: page.getByText(title, { exact: true }) });
async function library(page: Page) {
  await page.goto("/library");
  await page.getByLabel("Sort items").selectOption("title");
  await expect(
    reader(page).getByRole("heading", { name: "A", exact: true }),
  ).toBeVisible();
}

test(
  "Vim mode is opt-in, browser-local, discoverable and reload-safe",
  {
    tag: [
      "@case:keyboard-preference",
      "@feature:items",
      "@feature:setup",
      "@concern:ui",
      "@concern:accessibility",
      "@profile:core",
    ],
  },
  async ({ page, request }, testInfo) => {
    await seed(request, ["A", "B"]);
    await library(page);
    await page.keyboard.press("j");
    await expect(
      reader(page).getByRole("heading", { name: "A", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Workspace", exact: true }).click();
    await page.getByRole("menuitem", { name: "Keyboard shortcuts" }).click();
    await expect(page.getByRole("dialog")).toContainText(
      "Vim shortcuts are off",
    );
    await page.keyboard.press("Escape");
    await enable(page);
    await page.reload();
    await expect(
      page.getByRole("switch", { name: "Vim keyboard shortcuts" }),
    ).toBeChecked();
    await library(page);
    await page.keyboard.press("j");
    await expect(
      reader(page).getByRole("heading", { name: "B", exact: true }),
    ).toBeVisible();
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    await page.keyboard.press("?");
    await expect(page.getByRole("dialog")).toContainText(
      "Vim shortcuts are on",
    );
    await expect(page.locator(".MuiDialog-container")).toHaveCSS(
      "opacity",
      "1",
    );
    await page.screenshot({
      path: testInfo.outputPath("vim-help.png"),
      animations: "disabled",
    });
    await page.keyboard.press("Escape");
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    await page.goto("/preferences");
    await page
      .getByRole("switch", { name: "Vim keyboard shortcuts" })
      .uncheck();
    await library(page);
    await page.keyboard.press("i");
    await expect(note(page)).not.toBeFocused();
  },
);

test(
  "queue navigation retains removed Items and follows visible groups without mutation",
  {
    tag: [
      "@case:keyboard-queue",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const items = await seed(request, ["A", "B", "C"]);
    await enable(page);
    await page.goto("/");
    await page.getByLabel("Sort items").selectOption("title");
    await row(page, "B").click();
    await expect(
      reader(page).getByRole("heading", { name: "B", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("r");
    await expect(page.locator(".queue-row")).toHaveCount(2);
    await expect(
      reader(page).getByRole("heading", { name: "B", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("j");
    await expect(
      reader(page).getByRole("heading", { name: "C", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("j");
    await expect(
      reader(page).getByRole("heading", { name: "C", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("k");
    await expect(
      reader(page).getByRole("heading", { name: "A", exact: true }),
    ).toBeVisible();
    for (const item of [items[0], items[2]]) {
      const current = await (
        await request.get(`/api/v1/items/${item.id}`)
      ).json();
      expect(current.acknowledged_content_version).toBeLessThan(
        current.content_version,
      );
    }
    await page.getByLabel("Sort items").selectOption("priority");
    await expect(page.locator(".queue-group")).toHaveCount(1);
    await page.locator(".queue-group > summary").click();
    await page.keyboard.press("j");
    await expect(page.locator(".queue-group")).not.toHaveAttribute("open");
    await expect(
      reader(page).getByRole("heading", { name: "A", exact: true }),
    ).toBeVisible();
  },
);

test(
  "note keyboard saves preserve normal input semantics and avoid unchanged submissions",
  {
    tag: [
      "@case:keyboard-note",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@concern:accessibility",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const [item] = await seed(request, ["A"]);
    await enable(page);
    await library(page);
    let saves = 0;
    page.on("request", (event) => {
      if (event.method() === "PUT" && event.url().endsWith("/note")) saves++;
    });
    await page.keyboard.press("i");
    await expect(note(page)).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    expect(saves).toBe(0);
    await page.keyboard.press("i");
    await note(page).fill("Exact owner context\nSecond line");
    await page.keyboard.press("Control+Enter");
    await expect(
      reader(page).getByRole("status").filter({ hasText: "Note saved." }),
    ).toBeVisible();
    await expect(note(page)).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    expect(saves).toBe(1);
    const current = await (
      await request.get(`/api/v1/items/${item.id}`)
    ).json();
    expect(current.user_note).toBe("Exact owner context\nSecond line");
    expect(current.acknowledged_content_version).toBeLessThan(
      current.content_version,
    );
    await page.keyboard.press("i");
    await note(page).fill("");
    await page.keyboard.press("Escape");
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    expect(
      (await (await request.get(`/api/v1/items/${item.id}`)).json()).user_note,
    ).toBe("");
  },
);

test(
  "draft conflict base and failed request identity survive reader remounts",
  {
    tag: [
      "@case:keyboard-note-recovery",
      "@feature:items",
      "@concern:ui",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const [a] = await seed(request, ["A", "B"]);
    await enable(page);
    await library(page);
    await page.keyboard.press("i");
    await note(page).fill("Local draft");
    await row(page, "B").click();
    await expect(
      reader(page).getByRole("heading", { name: "B", exact: true }),
    ).toBeVisible();
    const changed = await request.put(`/api/v1/items/${a.id}/note`, {
      data: {
        expected_state_version: a.state_version,
        user_note: "Remote note",
      },
    });
    expect(changed.ok()).toBeTruthy();
    await row(page, "A").click();
    await expect(note(page)).toHaveValue("Local draft");
    await expect(reader(page)).toContainText("Remote note");
    await page.keyboard.press("i");
    await page.keyboard.press("Escape");
    await expect(reader(page).getByRole("alert")).toContainText(
      "state changed",
    );
    await expect(note(page)).toBeFocused();
    expect(
      (await (await request.get(`/api/v1/items/${a.id}`)).json()).user_note,
    ).toBe("Remote note");
    await page.keyboard.press("Escape"); // Explicit retry after the refresh.
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    await page.keyboard.press("i");
    await note(page).fill("Retry exact text");
    const bodies: object[] = [];
    let fail = true;
    await page.route(`**/items/${a.id}/note`, async (route) => {
      bodies.push(route.request().postDataJSON());
      if (fail) {
        fail = false;
        await route.abort();
      } else await route.continue();
    });
    await page.keyboard.press("Escape");
    await expect(reader(page).getByRole("alert")).toContainText(
      "draft is kept",
    );
    await row(page, "B").click();
    await row(page, "A").click();
    await expect(note(page)).toHaveValue("Retry exact text");
    await page.keyboard.press("i");
    await page.keyboard.press("Escape");
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    expect(bodies).toHaveLength(2);
    expect(bodies[1]).toEqual(bodies[0]);
  },
);

test(
  "pending save cannot duplicate or steal focus after pointer navigation",
  {
    tag: [
      "@case:keyboard-note-pending",
      "@feature:items",
      "@concern:ui",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const [a] = await seed(request, ["A", "B"]);
    await enable(page);
    await library(page);
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    let saves = 0;
    await page.route(`**/items/${a.id}/note`, async (route) => {
      saves++;
      await gate;
      await route.continue();
    });
    await page.keyboard.press("i");
    await note(page).fill("Pending draft");
    await page.keyboard.press("Escape");
    await expect(
      reader(page).getByRole("button", { name: "Saving…" }),
    ).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(note(page)).toBeFocused();
    await row(page, "B").click();
    await expect(
      reader(page).getByRole("heading", { name: "B", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("i");
    await note(page).fill("B remains focused");
    release();
    await expect
      .poll(
        async () =>
          (await (await request.get(`/api/v1/items/${a.id}`)).json()).user_note,
      )
      .toBe("Pending draft");
    await expect(note(page)).toBeFocused();
    await expect(note(page)).toHaveValue("B remains focused");
    expect(saves).toBe(1);
    await row(page, "A").click();
    await expect(note(page)).toHaveValue("Pending draft");
    await expect(row(page, "A")).not.toContainText("Unsaved note");
  },
);

test(
  "Item actions, inputs, overlays and composition have distinct key contexts",
  {
    tag: [
      "@case:keyboard-contexts",
      "@feature:items",
      "@concern:ui",
      "@concern:functional",
      "@concern:accessibility",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const [a] = await seed(request, ["A", "B"]);
    await enable(page);
    await library(page);
    await page.keyboard.press("t");
    await expect(
      reader(page).getByRole("button", { name: "Mark Done", exact: true }),
    ).toBeVisible();
    await expect(
      page
        .getByRole("status")
        .filter({ hasText: "Todo added or reopened: A." }),
    ).toHaveText("Todo added or reopened: A.");
    await page.keyboard.press("t");
    expect(
      (await (await request.get(`/api/v1/items/${a.id}`)).json()).todo_state,
    ).toBe("todo");
    await page.keyboard.press("s");
    await expect(page.getByRole("dialog")).toBeVisible();
    await page.keyboard.press("r");
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await page.keyboard.press("d");
    await expect(
      reader(page).getByRole("button", { name: "Reopen Todo", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("status").filter({ hasText: "Marked Done: A." }),
    ).toHaveText("Marked Done: A.");
    await page.keyboard.press("/");
    const search = page.getByRole("searchbox", { name: "Search items" });
    await expect(search).toBeFocused();
    await search.fill("jrtdsn?");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await page.keyboard.press("Escape");
    await expect(search).not.toBeFocused();
    await expect(search).toHaveValue("jrtdsn?");
    await page.keyboard.press("i");
    await note(page).fill("Composing");
    await note(page).dispatchEvent("compositionstart");
    await page.keyboard.press("Escape");
    await expect(note(page)).toBeFocused();
    expect(
      (await (await request.get(`/api/v1/items/${a.id}`)).json()).user_note,
    ).toBe("");
    await note(page).dispatchEvent("compositionend");
    await page.keyboard.press("Escape");
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    await page.keyboard.press("n");
    await expect(
      page.getByRole("dialog", { name: "Add to inbox" }),
    ).toBeVisible();
    await page.getByRole("textbox", { name: "Information" }).fill("jrtdsn?");
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toHaveCount(0);
  },
);

test(
  "narrow reader Back preserves drafts and standalone detail shares note commands",
  {
    tag: [
      "@case:keyboard-detail",
      "@feature:items",
      "@concern:ui",
      "@concern:accessibility",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const [a] = await seed(request, ["A"]);
    await enable(page);
    await page.setViewportSize({ width: 600, height: 900 });
    await page.goto("/library");
    await expect(page.locator(".queue-row")).toHaveCount(1);
    await page.keyboard.press("j");
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    await page.keyboard.press("i");
    await note(page).fill("Retained draft");
    // Leave editing through ordinary Tab, without saving.
    await page.locator("[data-reader-item]").focus();
    await page.keyboard.press("/");
    const search = page.getByRole("searchbox", { name: "Search items" });
    await expect(search).toBeFocused();
    expect(
      await search.evaluate((element) => {
        const bounds = element.getBoundingClientRect();
        return (
          document.elementFromPoint(
            bounds.x + bounds.width / 2,
            bounds.y + bounds.height / 2,
          ) === element
        );
      }),
    ).toBe(true);
    await search.fill("A");
    await page.keyboard.press("Escape");
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    await expect(note(page)).toHaveValue("Retained draft");
    await page.keyboard.press("Escape");
    await expect(page.locator(".queue-row")).toBeFocused();
    await page.keyboard.press("j");
    await expect(note(page)).toHaveValue("Retained draft");
    expect(
      (await (await request.get(`/api/v1/items/${a.id}`)).json()).user_note,
    ).toBe("");
    await page.goto(`/items/${a.id}`);
    await expect(
      page.getByRole("heading", { name: "A", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("i");
    const standaloneNote = page.getByRole("textbox", {
      name: "Your note",
      exact: true,
    });
    await expect(standaloneNote).toBeFocused();
    await standaloneNote.fill("Standalone context");
    await page.keyboard.press("Escape");
    await expect(page.locator("[data-reader-item]")).toBeFocused();
    await expect(
      page.getByRole("status").filter({ hasText: "Note saved." }),
    ).toHaveText("Note saved.");
  },
);

test(
  "loaded-page boundaries, loading detail and held mutation keys preserve their limits",
  {
    tag: [
      "@case:keyboard-boundaries",
      "@feature:items",
      "@concern:ui",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const [a, b] = await seed(request, ["A", "B", "C"]);
    let cursorRequests = 0;
    await page.route("**/api/v1/items?**", async (route) => {
      if (new URL(route.request().url()).searchParams.has("cursor"))
        cursorRequests++;
      await route.fulfill({
        json: { items: [a, b], next_cursor: "another-page" },
      });
    });
    await enable(page);
    await library(page);
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/api/v1/items/${b.id}`, async (route) => {
      await gate;
      await route.continue();
    });
    await page.keyboard.press("j");
    await expect(reader(page)).toContainText("Opening item…");
    await page.keyboard.press("t");
    expect(
      (await (await request.get(`/api/v1/items/${a.id}`)).json()).todo_state,
    ).not.toBe("todo");
    expect(
      (await (await request.get(`/api/v1/items/${b.id}`)).json()).todo_state,
    ).not.toBe("todo");
    release();
    await expect(
      reader(page).getByRole("heading", { name: "B", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("j");
    await expect(
      page.getByRole("status").filter({ hasText: "More items available" }),
    ).toBeVisible();
    expect(cursorRequests).toBe(0);
    let actions = 0;
    page.on("request", (event) => {
      if (event.method() === "POST" && event.url().endsWith("/actions"))
        actions++;
    });
    await page.keyboard.down("r");
    await page.keyboard.down("r");
    await page.keyboard.up("r");
    await expect(
      page.getByRole("status").filter({ hasText: "Marked seen: B." }),
    ).toHaveText("Marked seen: B.");
    expect(actions).toBe(1);
    const prevented = await page
      .locator("[data-reader-item]")
      .evaluate((element) => {
        const event = new KeyboardEvent("keydown", {
          key: "j",
          ctrlKey: true,
          bubbles: true,
          cancelable: true,
        });
        element.dispatchEvent(event);
        return event.defaultPrevented;
      });
    expect(prevented).toBe(false);
  },
);

test(
  "storage failure retains a session preference without changing server settings",
  {
    tag: [
      "@case:keyboard-storage",
      "@feature:items",
      "@feature:setup",
      "@concern:ui",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    await seed(request, ["A", "B"]);
    const settings = await (await request.get("/api/v1/settings")).json();
    await page.addInitScript(() => {
      for (const method of ["getItem", "setItem"] as const) {
        const original = Storage.prototype[method];
        Storage.prototype[method] = function (key: string, ...args: string[]) {
          if (key === "aicp.vim-shortcuts")
            throw new DOMException("Storage unavailable", "SecurityError");
          return original.apply(this, [key, ...args] as [string, string]);
        };
      }
    });
    await enable(page);
    await page.getByRole("link", { name: /^All items/ }).click();
    await page.getByLabel("Sort items").selectOption("title");
    await expect(
      reader(page).getByRole("heading", { name: "A", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("j");
    await expect(
      reader(page).getByRole("heading", { name: "B", exact: true }),
    ).toBeVisible();
    expect(await (await request.get("/api/v1/settings")).json()).toEqual(
      settings,
    );
    await page.goto("/preferences");
    await expect(
      page.getByRole("switch", { name: "Vim keyboard shortcuts" }),
    ).not.toBeChecked();
  },
);

test(
  "off-mode saves retain newer text on failure as well as success",
  {
    tag: [
      "@case:keyboard-off-mode-draft",
      "@feature:items",
      "@concern:ui",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const [a] = await seed(request, ["A"]);
    await library(page);
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    let fail = true;
    await page.route(`**/items/${a.id}/note`, async (route) => {
      if (fail) {
        await gate;
        fail = false;
        await route.abort();
      } else await route.continue();
    });
    await note(page).fill("Submitted draft");
    await reader(page)
      .getByRole("button", { name: "Save note", exact: true })
      .click();
    await expect(
      reader(page).getByRole("button", { name: "Saving…" }),
    ).toBeVisible();
    await note(page).fill("Newer text");
    release();
    await expect(reader(page).getByRole("alert")).toContainText(
      "draft is kept",
    );
    await expect(note(page)).toHaveValue("Newer text");
    await reader(page)
      .getByRole("button", { name: "Save note", exact: true })
      .click();
    await expect(
      reader(page).getByRole("status").filter({ hasText: "Note saved." }),
    ).toBeVisible();
    expect(
      (await (await request.get(`/api/v1/items/${a.id}`)).json()).user_note,
    ).toBe("Newer text");
  },
);

test(
  "successive actions on Items with the same title both update the live region",
  {
    tag: [
      "@case:keyboard-announcements",
      "@feature:items",
      "@concern:ui",
      "@concern:accessibility",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    await seed(request, ["A", "A"]);
    await enable(page);
    await library(page);
    await page.keyboard.press("r");
    await expect(
      page.getByRole("status").filter({ hasText: "Marked seen: A." }),
    ).toHaveText("Marked seen: A.");
    await page.evaluate(() => {
      const status = document.querySelector('.visually-hidden[role="status"]')!;
      const tracker = window as unknown as { announcementChanges: number };
      tracker.announcementChanges = 0;
      new MutationObserver(() => {
        tracker.announcementChanges++;
      }).observe(status, { childList: true, subtree: true });
    });
    await page.keyboard.press("j");
    await expect(
      reader(page).getByRole("button", { name: "Mark seen", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("r");
    await expect
      .poll(() =>
        page.evaluate(
          () =>
            (window as unknown as { announcementChanges: number })
              .announcementChanges,
        ),
      )
      .toBeGreaterThan(0);
  },
);
