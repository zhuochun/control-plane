import { useQuery } from "@tanstack/react-query";
import { api } from "./api";

export type Settings = {
  timezone: string;
  agents_md: string;
  user_md: string;
};

export const defaultAgentContext = `# Working with aicp

Read the brief and relevant context before inspecting sources. Publish concise, source-backed findings with stable dedupe keys. Respect local Todo, reminder, and note state; do not overwrite human decisions.`;

export const defaultUserContext = `# Owner context

This local control plane belongs to one human. Keep updates concise, source-backed, and useful for follow-through. Treat decisions, reminders, and notes here as the owner's final say.`;

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
