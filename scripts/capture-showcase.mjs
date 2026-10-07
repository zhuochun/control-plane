import { chromium, expect } from "../web/node_modules/@playwright/test/index.mjs";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";

const output = fileURLToPath(new URL("../docs/images/", import.meta.url));
const browser = await chromium.launch();
try {
  const page = await browser.newPage({
    viewport: { width: 1440, height: 1280 },
    deviceScaleFactor: 1,
    locale: "en-GB",
    timezoneId: "Asia/Singapore",
    colorScheme: "light",
    reducedMotion: "reduce",
  });
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const response = await page.request.get("http://127.0.0.1:7331/api/v1/items?limit=100");
  if (!response.ok()) throw new Error(`Item lookup failed: ${response.status()}`);
  const { items } = await response.json();
  if (items.length !== 7 || items.some((item) => !item.dedupe_key.startsWith("showcase:")))
    throw new Error("Capture requires the seven-Item fictional showcase workspace.");
  await page.goto("http://127.0.0.1:7331/", { waitUntil: "networkidle" });
  await expect(page.locator(".queue-row")).toHaveCount(7);
  await page.getByRole("button", { name: /October launch: one decision before rollout/ }).click();
  await expect(page.getByRole("heading", { name: "Launch readiness", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Mark Done", exact: true })).toBeVisible();
  await expect(page.getByRole("table")).toBeVisible();
  await expect(page.getByRole("button", { name: "Enlarge Launch dependencies" })).toBeVisible();
  await expect(page.getByRole("link", { name: /Launch review · engineering & product/ })).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
  if (errors.length) throw new Error(errors.join("\n"));
  mkdirSync(output, { recursive: true });
  await page.screenshot({ path: output + "aicp-attention.png", animations: "disabled" });
  await page.getByRole("heading", { name: "Choose launch scope", exact: true }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: output + "aicp-review.png", animations: "disabled" });
  console.log("Saved docs/images/aicp-attention.png (1440 x 1280).");
} finally {
  await browser.close();
}
