import { expect, test } from "./fixtures";

async function openReview(
  page: import("@playwright/test").Page,
  request: import("@playwright/test").APIRequestContext,
) {
  const title = "Review layout " + crypto.randomUUID();
  const created = await request.post("/api/v1/items", {
    data: {
      dedupe_key: title,
      kind: "report",
      title,
      summary: "Review the delivery approach",
      report: {
        schema_version: 2,
        body_md: "Choose a delivery approach",
        blocks: [
          {
            id: "intro",
            type: "markdown",
            body_md:
              "A short explanation stays readable while the form uses the available space.",
          },
          {
            id: "review",
            type: "review",
            title: "Delivery preferences " + "LongTitle".repeat(20),
            blocks: [
              {
                id: "approach",
                type: "choice",
                question: "Delivery approach",
                selection: "single",
                required: true,
                options: [
                  { id: "incremental", label: "Incremental rollout" },
                  { id: "all", label: "One release" },
                ],
              },
              {
                id: "checks",
                type: "choice",
                question: "Delivery checks",
                selection: "multiple",
                required: true,
                min_selections: 2,
                max_selections: 2,
                options: [
                  { id: "tests", label: "Automated tests" },
                  { id: "review", label: "Code review" },
                  { id: "smoke", label: "Smoke test" },
                ],
              },
              {
                id: "notes",
                type: "text_input",
                question: "Additional instructions",
                required: false,
              },
            ],
          },
        ],
      },
    },
  });
  expect(created.ok()).toBeTruthy();
  const item = await created.json();
  await page.setViewportSize({ width: 1600, height: 1000 });
  await page.goto("/?q=" + encodeURIComponent(title));
  const form = page.getByRole("form", { name: "Delivery preferences" });
  await expect(form).toBeVisible();
  return { form, item };
}

test(
  "review uses available width and retains drafts across responsive layouts",
  {
    tag: [
      "@case:review-layout",
      "@feature:reviews",
      "@concern:ui",
      "@concern:visual",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const { form, item } = await openReview(page, request);
    const widths = await page.evaluate(() => {
      const reader = document.querySelector(".reader-scroll")!;
      const style = getComputedStyle(reader);
      const available =
        reader.clientWidth -
        parseFloat(style.paddingLeft) -
        parseFloat(style.paddingRight);
      return {
        available,
        form: document.querySelector(".review-form")!.getBoundingClientRect()
          .width,
        prose: document
          .querySelector(".report-prose > p")!
          .getBoundingClientRect().width,
      };
    });
    expect(widths.form).toBeGreaterThan(1000);
    expect(Math.abs(widths.form - widths.available)).toBeLessThan(2);
    expect(widths.prose).toBeLessThan(widths.form);
    await expect(form).not.toContainText("Skipped");
    await expect(form.getByRole("button", { name: "Select none" })).toHaveCount(
      0,
    );
    await form
      .getByLabel("Additional instructions")
      .fill("Keep the rollout small.");
    for (const width of [768, 390]) {
      await page.setViewportSize({ width, height: 844 });
      const overflow = await page.evaluate(() => ({
        page:
          document.documentElement.scrollWidth -
          document.documentElement.clientWidth,
        report:
          document.querySelector(".reader-scroll")!.scrollWidth -
          document.querySelector(".reader-scroll")!.clientWidth,
      }));
      expect(overflow.page).toBeLessThanOrEqual(1);
      expect(overflow.report).toBeLessThanOrEqual(1);
      await expect(form.getByLabel("Additional instructions")).toHaveValue(
        "Keep the rollout small.",
      );
    }
  },
);

test(
  "review focuses invalid fields and enforces selection limits",
  {
    tag: [
      "@case:review-validation",
      "@feature:reviews",
      "@concern:ui",
      "@concern:accessibility",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const { form, item } = await openReview(page, request);
    let submissions = 0;
    page.on("request", (req) => {
      if (
        req.method() === "POST" &&
        req.url().endsWith(`/items/${item.id}/answers`)
      )
        submissions++;
    });
    await form.getByRole("button", { name: "Save answer" }).click();
    await expect(
      form.getByText("Answer this required question.", { exact: true }).first(),
    ).toBeVisible();
    await expect(
      form.getByRole("radio", { name: "Incremental rollout" }),
    ).toBeFocused();
    expect(submissions).toBe(0);
    await form.getByRole("radio", { name: "Incremental rollout" }).check();
    await form.getByRole("checkbox", { name: "Automated tests" }).check();
    await form.getByRole("button", { name: "Save answer" }).click();
    await expect(
      form.getByText("Choose at least 2 options.", { exact: true }),
    ).toBeVisible();
    expect(submissions).toBe(0);
    await form.getByRole("checkbox", { name: "Code review" }).check();
    await expect(
      form.getByRole("checkbox", { name: "Smoke test" }),
    ).toBeDisabled();
    await form.getByRole("checkbox", { name: "Automated tests" }).uncheck();
    await expect(
      form.getByRole("checkbox", { name: "Smoke test" }),
    ).toBeEnabled();
    await form.getByRole("checkbox", { name: "Automated tests" }).check();
    await form
      .getByLabel("Additional instructions")
      .fill("Keep the rollout small.");
  },
);

test(
  "pending review disables controls and saves readable answer history",
  {
    tag: [
      "@case:review-pending",
      "@feature:reviews",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const { form, item } = await openReview(page, request);
    let submissions = 0;
    page.on("request", (req) => {
      if (
        req.method() === "POST" &&
        req.url().endsWith(`/items/${item.id}/answers`)
      )
        submissions++;
    });
    await form.getByRole("radio", { name: "Incremental rollout" }).check();
    await form.getByRole("checkbox", { name: "Automated tests" }).check();
    await form.getByRole("checkbox", { name: "Code review" }).check();
    await form
      .getByLabel("Additional instructions")
      .fill("Keep the rollout small.");
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/items/${item.id}/answers`, async (route) => {
      await held;
      await route.continue();
    });
    await form.getByRole("button", { name: "Save answer" }).click();
    await expect(
      form.getByRole("button", { name: "Skip Additional instructions" }),
    ).toBeDisabled();
    await expect(form.getByLabel("Additional instructions")).toBeDisabled();
    release();
    await expect(
      form.getByText("Answer saved for the next agent to use."),
    ).toBeVisible();
    expect(submissions).toBe(1);
    await expect(
      page.getByRole("region", { name: "Your answers" }),
    ).toContainText("Keep the rollout small.");
    const notes = page.getByRole("region", { name: "Your answers" });
    const savedItem = await (
      await request.get(`/api/v1/items/${item.id}`)
    ).json();
    await expect(notes.locator("time")).toHaveAttribute(
      "datetime",
      savedItem.answers[0].submitted_at,
    );
    await expect(notes).not.toContainText(savedItem.answers[0].submitted_at);
    await notes
      .getByRole("button", { name: "Answer history", exact: true })
      .click();
    await expect(
      notes.getByRole("region", { name: "Answer history" }),
    ).toContainText("Latest submission");
    await expect(
      notes.getByRole("button", { name: "Hide answer history" }),
    ).toHaveAttribute("aria-expanded", "true");
  },
);

test(
  "uncertain committed answer retries the original request without duplicate notes",
  {
    tag: [
      "@case:review-retry",
      "@feature:reviews",
      "@concern:ui",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const title = "Uncertain answer " + crypto.randomUUID();
    const created = await request.post("/api/v1/items", {
      data: {
        dedupe_key: title,
        kind: "report",
        title,
        summary: "Instructions",
        report: {
          schema_version: 2,
          body_md: "Review",
          blocks: [
            {
              id: "review",
              type: "review",
              title: "Retry review",
              blocks: [
                {
                  id: "text",
                  type: "text_input",
                  question: "Instructions",
                  required: true,
                },
              ],
            },
          ],
        },
      },
    });
    expect(created.ok()).toBeTruthy();
    const item = await created.json();
    const requests: string[] = [];
    await page.route(`**/items/${item.id}/answers`, async (route) => {
      requests.push(route.request().postData()!);
      if (requests.length === 1) {
        const committed = await route.fetch();
        expect(committed.ok()).toBeTruthy();
        await route.abort("failed");
      } else await route.continue();
    });
    await page.goto("/?q=" + encodeURIComponent(title));
    const form = page.getByRole("form", { name: "Retry review" });
    await form
      .getByRole("textbox", { name: "Instructions", exact: true })
      .fill("Keep it local");
    await form.getByRole("button", { name: "Save answer" }).click();
    await expect(form.getByRole("alert")).toContainText("Your input is kept");
    await expect(
      page.getByRole("region", { name: "Your answers" }),
    ).toContainText("Keep it local");
    await form.getByRole("button", { name: "Save answer" }).click();
    await expect(form.getByRole("alert")).toContainText("Answer saved");
    expect(requests).toHaveLength(2);
    expect(requests[1]).toBe(requests[0]);
    const history = await (
      await request.get(`/api/v1/items/${item.id}/answers`)
    ).json();
    expect(history.items).toHaveLength(1);
  },
);

test(
  "refresh reconciles removed choices and added fields without losing text drafts",
  {
    tag: [
      "@case:review-refresh",
      "@feature:reviews",
      "@concern:ui",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const title = "Changing fields " + crypto.randomUUID();
    const created = await request.post("/api/v1/items", {
      data: {
        dedupe_key: title,
        kind: "report",
        title,
        summary: "Options change",
        report: {
          schema_version: 2,
          body_md: "Review",
          blocks: [
            {
              id: "review",
              type: "review",
              title: "Changing review",
              blocks: [
                {
                  id: "features",
                  type: "choice",
                  selection: "multiple",
                  question: "Features",
                  options: [
                    { id: "old", label: "Old option" },
                    { id: "retained", label: "Retained option" },
                  ],
                },
                { id: "text", type: "text_input", question: "Instructions" },
              ],
            },
          ],
        },
      },
    });
    expect(created.ok()).toBeTruthy();
    const item = await created.json();
    await page.goto("/?q=" + encodeURIComponent(title));
    const form = page.getByRole("form", { name: "Changing review" });
    await form.getByRole("checkbox", { name: "Old option" }).check();
    await form
      .getByRole("textbox", { name: "Instructions", exact: true })
      .fill("Retain this draft");
    const report = structuredClone(item.report);
    report.blocks[0].blocks[0].options.shift();
    report.blocks[0].blocks.push({
      id: "new",
      type: "text_input",
      question: "New optional input",
    });
    expect(
      (
        await request.patch(`/api/v1/items/${item.id}/work`, {
          data: { expected_content_version: item.content_version, report },
        })
      ).ok(),
    ).toBeTruthy();
    await form.getByRole("button", { name: "Save answer" }).click();
    await expect(
      form.getByRole("textbox", { name: "New optional input", exact: true }),
    ).toBeVisible();
    await expect(
      form.getByRole("checkbox", { name: "Old option" }),
    ).toHaveCount(0);
    await expect(
      form.getByRole("textbox", { name: "Instructions", exact: true }),
    ).toHaveValue("Retain this draft");
    await form.getByRole("button", { name: "Save answer" }).click();
    await expect(
      page.getByRole("region", { name: "Your answers" }),
    ).toContainText("Retain this draft");
    const saved = await (await request.get(`/api/v1/items/${item.id}`)).json();
    expect(saved.answers[0].values[0].answer).toBe("Selected none");
    expect(saved.answers[0].values[2].disposition).toBe("skipped");
  },
);

test(
  "image choices and configured PlantUML previews remain inspectable",
  {
    tag: [
      "@case:review-images",
      "@feature:reviews",
      "@feature:diagrams",
      "@concern:ui",
      "@concern:visual",
      "@profile:renderer",
    ],
  },
  async ({ page, request }, testInfo) => {
    const capabilities = await (
      await request.get("/api/v1/review-capabilities")
    ).json();
    expect(
      capabilities.plantuml_available,
      "required renderer profile must be provisioned",
    ).toBe(true);
    await page.goto("/library");
    const png = await page.screenshot();
    const uploaded = await request.post("/api/v1/review-artifacts", {
      data: { data_base64: png.toString("base64") },
    });
    expect(uploaded.ok()).toBeTruthy();
    const artifact = await uploaded.json();
    const title = "Visual evidence " + crypto.randomUUID();
    const created = await request.post("/api/v1/items", {
      data: {
        dedupe_key: title,
        kind: "report",
        title,
        summary: "Compare visual evidence",
        report: {
          schema_version: 2,
          body_md: "Inspect diagrams and images",
          blocks: [
            {
              id: "diagram",
              type: "diagram",
              language: "plantuml",
              source: "@startuml\nAlice -> Bob: Review\n@enduml",
              description: "PlantUML review flow",
            },
            {
              id: "review",
              type: "review",
              title: "Visual review",
              blocks: [
                {
                  id: "choice",
                  type: "choice",
                  selection: "single",
                  required: true,
                  question: "Select evidence",
                  options: [
                    {
                      id: "image",
                      label: "Captured portal",
                      blocks: [
                        {
                          id: "preview",
                          type: "image",
                          artifact_id: artifact.id,
                          description: "Portal screenshot",
                        },
                      ],
                    },
                  ],
                },
              ],
            },
          ],
        },
      },
    });
    expect(created.ok()).toBeTruthy();
    const item = await created.json();
    await page.goto("/?q=" + encodeURIComponent(title));
    await expect(
      page.getByRole("img", { name: "PlantUML review flow", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("img", { name: "Portal screenshot", exact: true }),
    ).toBeVisible();
    for (const name of ["PlantUML review flow", "Portal screenshot"]) {
      await expect
        .poll(() =>
          page
            .getByRole("img", { name, exact: true })
            .evaluate((image) => (image as HTMLImageElement).naturalWidth),
        )
        .toBeGreaterThan(0);
    }
    await page.getByRole("radio", { name: "Captured portal" }).check();
    await page.getByRole("button", { name: "Save answer" }).click();
    await expect(
      page.getByRole("region", { name: "Your answers" }),
    ).toContainText("Captured portal");
    const saved = await (await request.get(`/api/v1/items/${item.id}`)).json();
    expect(saved.answers[0].values[0].options[0].blocks[0].artifact_id).toBe(
      artifact.id,
    );
    await page.screenshot({ path: testInfo.outputPath("visual-review.png") });
  },
);

test(
  "interleaved review saves readable human answers and preserves user state",
  {
    tag: [
      "@case:review-interleaving",
      "@feature:reviews",
      "@feature:diagrams",
      "@concern:ui",
      "@concern:functional",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const title = "Rich review " + crypto.randomUUID();
    const layoutWorkers: import("@playwright/test").Worker[] = [];
    const closedWorkers = new Set<import("@playwright/test").Worker>();
    page.on("worker", (worker) => {
      if (worker.url().includes("elk-worker")) {
        layoutWorkers.push(worker);
        worker.on("close", () => closedWorkers.add(worker));
      }
    });
    const format = {
      id: title,
      version: 1,
      title: "Diagram selection",
      fields: [
        { id: "diagram", type: "diagram" },
        { id: "approach", type: "choice", selection: "single", required: true },
        {
          id: "features",
          type: "choice",
          selection: "multiple",
          max_selections: 2,
        },
        { id: "feedback", type: "text_input" },
      ],
    };
    expect(
      (await request.post("/api/v1/review-formats", { data: { format } })).ok(),
    ).toBeTruthy();
    const response = await request.post("/api/v1/items", {
      data: {
        dedupe_key: title,
        kind: "report",
        title,
        summary: "Choose a diagram renderer",
        report: {
          schema_version: 2,
          body_md: "Review the rendering approach",
          blocks: [
            {
              id: "intro",
              type: "markdown",
              body_md: "## Before the decision",
            },
            {
              id: "decision",
              type: "review",
              title: "Rendering review",
              format: { id: title, version: 1 },
              blocks: [
                {
                  id: "diagram",
                  type: "diagram",
                  language: "mermaid",
                  source:
                    'flowchart-elk LR\nAgent --> Item\nItem --> Human\nMath["$$x^2$$"] --> Item',
                  description: "Review flow",
                  caption: "Review flow preview",
                },
                {
                  id: "approach",
                  type: "choice",
                  selection: "single",
                  required: true,
                  question: "Which approach?",
                  options: [
                    { id: "local", label: "Local rendering" },
                    { id: "remote", label: "External service" },
                  ],
                },
                {
                  id: "features",
                  type: "choice",
                  selection: "multiple",
                  max_selections: 2,
                  question: "Which diagrams?",
                  options: [
                    { id: "mermaid", label: "Mermaid" },
                    { id: "plantuml", label: "PlantUML" },
                  ],
                },
                {
                  id: "feedback",
                  type: "text_input",
                  question: "Additional instructions",
                },
              ],
            },
            { id: "after", type: "markdown", body_md: "## After the decision" },
            { id: "actions", type: "actions", action_ids: ["todo"] },
          ],
          actions: [
            {
              id: "todo",
              type: "set_todo",
              label: "Track follow-up",
              state: "todo",
            },
          ],
        },
      },
    });
    expect(response.ok()).toBeTruthy();
    const item = await response.json();
    await page.goto("/?q=" + encodeURIComponent(title));
    const form = page.getByRole("form", { name: "Rendering review" });
    await expect(form).toBeVisible();
    const diagram = form.getByRole("img", { name: "Review flow", exact: true });
    await expect(diagram).toBeVisible();
    await expect
      .poll(() =>
        diagram.evaluate((image) => (image as HTMLImageElement).naturalWidth),
      )
      .toBeGreaterThan(0);
    await expect.poll(() => layoutWorkers.length).toBe(1);
    await expect.poll(() => closedWorkers.size).toBe(1);
    await page.reload();
    await expect(diagram).toBeVisible();
    await expect.poll(() => layoutWorkers.length).toBe(2);
    await expect.poll(() => closedWorkers.size).toBe(2);
    await form
      .getByRole("button", { name: "Enlarge Review flow preview" })
      .click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Close", exact: true })
      .click();
    await form.getByText("Diagram source (mermaid)").click();
    await expect(form.locator("details pre")).toContainText("Agent --> Item");
    await form.getByRole("radio", { name: "Local rendering" }).check();
    await form.getByRole("checkbox", { name: "Mermaid", exact: true }).check();
    await form.getByRole("checkbox", { name: "PlantUML", exact: true }).check();
    await form
      .getByRole("textbox", { name: "Additional instructions" })
      .fill("Avoid external services.");
    await form
      .getByRole("button", { name: "Save answer", exact: true })
      .click();
    await expect(
      page.getByRole("region", { name: "Your answers" }),
    ).toContainText("Which approach?: Local rendering");
    await expect(
      page.getByRole("region", { name: "Your answers" }),
    ).toContainText("Which diagrams?: Mermaid; PlantUML");
    let saved = await (await request.get(`/api/v1/items/${item.id}`)).json();
    expect(saved.answers[0].values[2].answer).toBe("Avoid external services.");
    expect(saved.answers[0].applicable).toBe(true);
    expect(saved.todo_state).toBe("none");
    expect(saved.acknowledged_content_version).toBe(0);
    expect(saved.content_version).toBe(item.content_version);
    await page
      .getByRole("button", { name: "Track follow-up", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "Mark Done", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("region", { name: "Your answers" }),
    ).toContainText("Local rendering");
    await form.getByRole("radio", { name: "External service" }).check();
    await form
      .getByRole("button", { name: "Save answer", exact: true })
      .click();
    await expect(
      page.getByRole("region", { name: "Your answers" }),
    ).toContainText("Which approach?: External service");
    const history = await (
      await request.get(`/api/v1/items/${item.id}/answers`)
    ).json();
    expect(history.items).toHaveLength(2);
    expect(history.items[1].supersedes).toBe(history.items[0].id);
    saved = await (await request.get(`/api/v1/items/${item.id}`)).json();
    expect(saved.answers).toHaveLength(1);
    const changed = structuredClone(saved.report);
    changed.blocks[0].body_md = "## Updated review evidence";
    expect(
      (
        await request.patch(`/api/v1/items/${item.id}/work`, {
          data: {
            expected_content_version: saved.content_version,
            report: changed,
          },
        })
      ).ok(),
    ).toBeTruthy();
    await page.reload();
    await expect(
      page.getByRole("region", { name: "Your answers" }),
    ).toContainText("earlier review material");
    await page.route("**/elk-worker*.js", (route) => route.abort());
    await page.reload();
    await expect(form.getByRole("alert")).toContainText(
      "Diagram layout worker failed.",
    );
    await form.getByText("Diagram source (mermaid)").click();
    await expect(form.locator("details pre")).toContainText("Agent --> Item");
    await expect.poll(() => closedWorkers.size).toBe(layoutWorkers.length);
  },
);

test(
  "stale review keeps typed input and PlantUML errors keep source visible",
  {
    tag: [
      "@case:review-stale",
      "@feature:reviews",
      "@feature:diagrams",
      "@concern:ui",
      "@concern:recovery",
      "@profile:core",
    ],
  },
  async ({ page, request }) => {
    const title = "Stale review " + crypto.randomUUID();
    const created = await request.post("/api/v1/items", {
      data: {
        dedupe_key: title,
        kind: "report",
        title,
        summary: "Review instructions",
        report: {
          schema_version: 2,
          body_md: "Review",
          blocks: [
            {
              id: "diagram",
              type: "diagram",
              language: "plantuml",
              source:
                "@startuml\n!include https://example.com/resource\n@enduml",
              description: "Restricted diagram",
            },
            {
              id: "review",
              type: "review",
              title: "Instructions review",
              blocks: [
                {
                  id: "text",
                  type: "text_input",
                  question: "Your instructions",
                  required: true,
                },
              ],
            },
          ],
        },
      },
    });
    expect(created.ok()).toBeTruthy();
    const item = await created.json();
    await page.goto("/?q=" + encodeURIComponent(title));
    const form = page.getByRole("form", { name: "Instructions review" });
    await form
      .getByRole("textbox", { name: "Your instructions" })
      .fill("Keep this draft");
    await page.getByText("Diagram source (plantuml)").click();
    await expect(page.locator("details pre")).toContainText("!include");
    await expect(
      page.getByRole("region", { name: "Item details" }).getByRole("alert"),
    ).toContainText(/unavailable|unsupported/);
    const report = structuredClone(item.report);
    report.body_md = "Changed explanation";
    expect(
      (
        await request.patch(`/api/v1/items/${item.id}/work`, {
          data: { expected_content_version: item.content_version, report },
        })
      ).ok(),
    ).toBeTruthy();
    await form.getByRole("button", { name: "Save answer" }).click();
    await expect(form.getByRole("alert")).toContainText(
      "Review content changed",
    );
    await expect(
      form.getByRole("textbox", { name: "Your instructions" }),
    ).toHaveValue("Keep this draft");
    await form.getByRole("button", { name: "Save answer" }).click();
    await expect(
      page.getByRole("region", { name: "Your answers" }),
    ).toContainText("Keep this draft");
  },
);
