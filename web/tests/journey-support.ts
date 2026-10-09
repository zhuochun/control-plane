import path from "node:path";
import { readFileSync, writeFileSync } from "node:fs";
import { MCPClient, baseURL } from "../../scripts/testing/owned-server.mjs";
import { run } from "../../scripts/testing/process.mjs";
import { root } from "../../scripts/testing/build.mjs";
import { JourneyRecord } from "../../scripts/testing/observations.mjs";
import type { TestInfo } from "@playwright/test";

export function record(id: string, claim: string, info: TestInfo) {
  const expected = JSON.parse(
    readFileSync(path.join(root, "tests/e2e/contracts", id + ".json"), "utf8"),
  );
  return new JourneyRecord(id, claim, expected, info.outputPath("journey"));
}
export async function cli(server: any, args: string[], input?: object) {
  if (input) {
    const file = path.join(server.data, `request-${crypto.randomUUID()}.json`);
    writeFileSync(file, JSON.stringify(input));
    args = [...args, "--file", file];
  }
  const result = await run(server.binary, ["--server",baseURL,...args, "--json"], {
    env: server.environment(),
    timeout: 10_000,
  });
  return JSON.parse(result.stdout);
}
export async function configure(server: any) {
  await cli(server, ["config", "set", "Asia/Singapore"]);
  const interest = await cli(server, ["interest", "create"], {
    request_id: "journey-interest",
    title: "Delivery economics",
    instructions_md: "Notice comparable material changes.",
  });
  const watch = await cli(server, ["watch", "create"], {
    request_id: "journey-watch",
    matching_policy: "explicit",
    interest_ids: [interest.id],
    source: { kind: "fixture", locator: "fixtures/demo-costs" },
    instructions_md: "Compare the same workload; flag incomplete periods.",
    interval_seconds: 7200,
  });
  return { interest, watch };
}
export async function connect(server: any) {
  return new MCPClient(server).connect();
}
export async function answerEvents(mcp: MCPClient, itemID: string) {
  const brief = await mcp.call("get_brief");
  const range = brief.pending_changes;
  const items: any[] = [];
  let cursor: string | undefined;
  do {
    const page = await mcp.call("get_changes", {
      after_seq: range.after_seq,
      through_seq: range.through_seq,
      ...(cursor ? { cursor } : {}),
    });
    items.push(...page.items);
    cursor = page.next_cursor;
  } while (cursor);
  return items.filter(
    (e) => e.entity_id === itemID && e.change_type === "item.answer_submitted",
  );
}
export const reviewFormat = {
  id: "journey-review",
  version: 1,
  title: "Delivery review",
  fields: [
    { id: "approach", type: "choice", selection: "single", required: true },
    { id: "instructions", type: "text_input", required: true },
  ],
};
export const reviewReport = {
  schema_version: 2,
  body_md: "Choose a delivery approach",
  blocks: [
    {
      id: "intro",
      type: "markdown",
      body_md: "## Delivery decision\nKeep the rollout inspectable.",
    },
    {
      id: "diagram",
      type: "diagram",
      language: "mermaid",
      source: "flowchart LR\nAgent --> Item\nItem --> Human",
      description: "Review flow",
    },
    {
      id: "review",
      type: "review",
      title: "Delivery review",
      format: { id: reviewFormat.id, version: 1 },
      blocks: [
        {
          id: "approach",
          type: "choice",
          selection: "single",
          required: true,
          question: "Delivery approach",
          options: [
            { id: "local", label: "Local" },
            { id: "remote", label: "Remote" },
          ],
        },
        {
          id: "instructions",
          type: "text_input",
          required: true,
          question: "Additional instructions",
        },
      ],
    },
  ],
};
export async function publishReview(mcp: MCPClient) {
  await mcp.call("register_review_format", { format: reviewFormat });
  const interest = await mcp.call("create_interest", {
    payload: {
      request_id: "review-interest",
      title: "Delivery choices",
      instructions_md: "Review delivery decisions.",
    },
  });
  const item = await mcp.call("upsert_item", {
    request_id: "journey-publish",
    dedupe_key: "journey:review",
    expected_content_version: 0,
    kind: "report",
    title: "Delivery review",
    summary: "Choose Local or Remote",
    interests: [{ id: interest.id, reason: "Delivery decision" }],
    sources: [],
    report: reviewReport,
  });
  return mcp.call("get_item", { item_id: item.id });
}
export function values(answer: any) {
  const choice = answer.values.find((v: any) => v.field_id === "approach");
  const text = answer.values.find((v: any) => v.field_id === "instructions");
  return {
    approach: {
      disposition: choice.disposition,
      selected_ids: choice.selected_ids,
      question: choice.question,
      answer: choice.answer,
    },
    instructions: {
      disposition: text.disposition,
      text: text.text,
      question: text.question,
      answer: text.answer,
    },
  };
}
