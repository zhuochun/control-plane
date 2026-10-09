import { isolated as test, expect } from "./fixtures";

test("input failure is visible and follow-up notes survive processing", {
  tag: ["@case:input-processing", "@feature:items", "@feature:runs", "@concern:ui", "@concern:functional", "@concern:recovery", "@profile:core"],
}, async ({ page, request }) => {
  await page.goto("/library");
  await page.getByRole("button", { name: "Add to inbox", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox", { name: "Information", exact: true }).fill("Compare capture options");
  await dialog.getByRole("button", { name: "Add to inbox", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  const listed = await (await request.get("/api/v1/items?view=inbox")).json();
  let item = await (await request.get(`/api/v1/items/${listed.items[0].id}`)).json();
  await page.goto("/library?q=Compare%20capture%20options");
  const reader = page.getByRole("region", { name: "Item details" });
  await expect(reader).toContainText("Awaiting agent");
  const started = await (await request.post("/api/v1/runs", { data: { watch_ids: [] } })).json();
  const run = started.run;
  const command = (inputId: string, outcome: string, result: string) => ({ run_id: run.id, input_id: inputId, expected_content_version: item.content_version, expected_state_version: item.state_version, outcome, result_md: result });
  expect((await request.post(`/api/v1/runs/${run.id}/finish`, { data: {} })).status()).toBe(409);
  expect((await request.post(`/api/v1/items/${item.id}/inputs/process`, { data: command(item.pending_inputs[0].id, "failed", "Reference unavailable. Please provide another source.") })).ok()).toBeTruthy();
  await expect(reader).toContainText("Reference unavailable", { timeout: 12000 });
  const note = reader.getByRole("textbox", { name: "Your note", exact: true });
  await note.fill("Use the local reference instead");
  await reader.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(reader.getByRole("status").filter({ hasText: "Note saved." })).toBeVisible();
  item = await (await request.get(`/api/v1/items/${item.id}`)).json();
  const inbox = item.pending_inputs.find((entry: { kind: string }) => entry.kind === "inbox");
  expect((await request.post(`/api/v1/items/${item.id}/inputs/process`, { data: { ...command(inbox.id, "responded", "Local comparison saved"), archive: true } })).ok()).toBeTruthy();
  expect((await request.post(`/api/v1/runs/${run.id}/finish`, { data: {} })).ok()).toBeTruthy();
  const next = await (await request.post("/api/v1/runs", { data: { watch_ids: [] } })).json();
  item = await (await request.get(`/api/v1/items/${item.id}`)).json();
  const guidance = item.pending_inputs.find((entry: { kind: string }) => entry.kind === "note");
  expect((await request.post(`/api/v1/items/${item.id}/inputs/process`, { data: { ...command(guidance.id, "responded", "Guidance incorporated"), run_id: next.run.id } })).ok()).toBeTruthy();
  expect((await request.post(`/api/v1/runs/${next.run.id}/finish`, { data: {} })).ok()).toBeTruthy();
  await expect(note).toHaveValue("", { timeout: 12000 });
  await reader.getByText("Prior notes and inbox submissions", { exact: true }).click();
  await expect(reader.locator(".reader-input-history")).toContainText("Use the local reference instead");
  await expect(reader.locator(".reader-input-history")).toContainText("Compare capture options");
  expect((await (await request.get(`/api/v1/items/${item.id}`)).json()).origin).toBe("user");
});

test("withdrawn note does not claim agent processing", {
  tag: ["@case:input-withdrawal", "@feature:items", "@concern:ui", "@concern:functional", "@profile:core"],
}, async ({ page, request }) => {
  const created = await request.post("/api/v1/items", { data: { dedupe_key: crypto.randomUUID(), kind: "report", title: "Withdraw note", summary: "Ordinary Item", report: { schema_version: 1, body_md: "Report" } } });
  const item = await created.json();
  await page.goto("/library?q=Withdraw%20note");
  const reader = page.getByRole("region", { name: "Item details" });
  const note = reader.getByRole("textbox", { name: "Your note", exact: true });
  await note.fill("Temporary instruction");
  await reader.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(reader.getByRole("status").filter({ hasText: "Note saved." })).toBeVisible();
  await note.fill("");
  await reader.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(reader.locator(".reader-input-history [role=status]")).toHaveText("Note withdrawn", { timeout: 12000 });
  const history = await (await request.get(`/api/v1/items/${item.id}/inputs`)).json();
  expect(history.items[0].status).toBe("withdrawn");
  expect(history.items[0].attempt_count).toBe(0);
});

test("history refreshes on input changes without its own polling loop", {
  tag: ["@case:input-history-refresh", "@feature:items", "@concern:ui", "@concern:functional", "@profile:core"],
}, async ({ page, request }) => {
  const created = await request.post("/api/v1/items", { data: { dedupe_key: crypto.randomUUID(), kind: "report", title: "History refresh", summary: "Polling test", report: { schema_version: 1, body_md: "Report" } } });
  const item = await created.json();
  let detailReads = 0;
  let historyReads = 0;
  page.on("request", request => {
    if (request.method() !== "GET") return;
    const path = new URL(request.url()).pathname;
    if (path === `/api/v1/items/${item.id}`) detailReads++;
    if (path === `/api/v1/items/${item.id}/inputs`) historyReads++;
  });
  await page.goto("/library?q=History%20refresh");
  await expect.poll(() => historyReads).toBe(1);
  await expect.poll(() => detailReads, { timeout: 15000 }).toBeGreaterThan(2);
  expect(historyReads).toBe(1);
  const reader = page.getByRole("region", { name: "Item details" });
  await reader.getByRole("textbox", { name: "Your note", exact: true }).fill("New input");
  await reader.getByRole("button", { name: "Save note", exact: true }).click();
  await expect.poll(() => historyReads).toBe(2);
  const before = detailReads;
  await expect.poll(() => detailReads, { timeout: 12000 }).toBeGreaterThan(before);
  expect(historyReads).toBe(2);
});

test("history catches a note saved and withdrawn between detail polls", {
  tag: ["@case:input-history-between-polls", "@feature:items", "@concern:ui", "@concern:functional", "@profile:core"],
}, async ({ page, request }) => {
  const created = await request.post("/api/v1/items", { data: { dedupe_key: crypto.randomUUID(), kind: "report", title: "Between polls", summary: "History test", report: { schema_version: 1, body_md: "Report" } } });
  const item = await created.json();
  let hold = false;
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  let blocked!: () => void;
  const polling = new Promise<void>(resolve => { blocked = resolve; });
  await page.route(`**/api/v1/items/${item.id}`, async route => {
    if (hold) { blocked(); await gate; }
    await route.continue();
  });
  await page.goto("/library?q=Between%20polls");
  const reader = page.getByRole("region", { name: "Item details" });
  await reader.getByText("Prior notes and inbox submissions", { exact: true }).click();
  await expect(reader).toContainText("No processed, superseded, or withdrawn input yet.");
  hold = true;
  await polling;
  try {
    const saved = await request.put(`/api/v1/items/${item.id}/note`, { data: { expected_state_version: item.state_version, user_note: "Keep this original" } });
    expect(saved.ok()).toBeTruthy();
    const updated = await saved.json();
    const withdrawn = await request.put(`/api/v1/items/${item.id}/note`, { data: { expected_state_version: updated.state_version, user_note: "" } });
    expect(withdrawn.ok()).toBeTruthy();
  } finally { hold = false; release(); }
  await expect(reader.locator(".reader-input-history [role=status]")).toHaveText("Note withdrawn", { timeout: 12000 });
  await expect(reader.locator(".reader-input-history")).toContainText("Keep this original");
});
