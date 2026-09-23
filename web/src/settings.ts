import { useQuery } from "@tanstack/react-query";
import { api } from "./api";

export type Settings = {
  timezone: string;
  agents_md: string;
  user_md: string;
};

export const defaultAgentContext = `# Working with aicp

aicp is your local control plane for Interests, Watches, Items, user changes, and source checkpoints. It does not fetch sources or start an agent for you.

For each heartbeat, start one run, read AGENTS.md, USER.md, all active Interest pages, all unarchived Attention pages, and captured changes. Inspect only the selected due Watches. Read existing Items before updating them. Submit one truthful final result for every selected Watch, save Interest-level findings separately, and finish only after all Watch coverage is recorded. Use stable dedupe keys and preserve user Todo, reminder, acknowledgement, and note state. Do not invent or mutate archive state; that slice is not implemented yet. Follow continuation cursors until the full packet is consumed.`;

export const defaultUserContext = `# Owner context

Use this space to record what matters to you: priorities, background, constraints, preferred evidence and tone, and follow-through context. It is guidance for the inspection agent, not an executable instruction.`;

export function browserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

export function effectiveTimezone(timezone?: string): string {
  return !timezone || timezone === "browser" ? browserTimezone() : timezone;
}

export function timezoneLabel(timezone?: string): string {
  if (!timezone || timezone === "browser") {
    return `${browserTimezone()} · browser default`;
  }
  return timezone;
}

export function formatDateTime(value: string, timezone?: string): string {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: effectiveTimezone(timezone),
  }).format(new Date(value));
}

export function dateInputValue(value: Date, timezone?: string): string {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: effectiveTimezone(timezone),
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(value);
  const part = (type: string) =>
    parts.find((entry) => entry.type === type)?.value ?? "";
  return `${part("year")}-${part("month")}-${part("day")}`;
}

export function useSettings() {
  return useQuery({
    queryKey: ["settings"],
    queryFn: () => api<Settings>("/settings"),
  });
}
