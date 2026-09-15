import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  TextField,
} from "@mui/material";
import { NavLink, useParams } from "react-router";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { api, APIError, collection } from "./api";
import {
  dateInputValue,
  effectiveTimezone,
  formatDateTime,
  useSettings,
} from "./settings";

export type Source = {
  id: string;
  url: string;
  label: string;
  observed_at: string;
};
export type ReportAction = {
  id: string;
  type:
    | "open_link"
    | "set_todo"
    | "set_reminder"
    | "clear_reminder"
    | "acknowledge";
  label: string;
  source_ref?: string;
  state?: string;
};
export type Item = {
  id: string;
  dedupe_key: string;
  kind: string;
  interest_id?: string;
  title: string;
  summary: string;
  sources: Source[];
  context_md?: string;
  report: { schema_version: number; body_md: string; actions?: ReportAction[] };
  content_version: number;
  todo_state: "none" | "todo" | "done";
  remind_at?: string;
  reminder_timezone?: string;
  acknowledged_content_version: number;
  user_note: string;
  state_version: number;
  created_at: string;
  content_updated_at: string;
  state_updated_at: string;
};
type Proposal = {
  id: string;
  target_type: string;
  operation: string;
  rationale_md: string;
  state: string;
};

const when = (value: string, timezone?: string) =>
  formatDateTime(value, timezone);

type MetricValue = {
  value: number;
  display: string;
};
type Metric = {
  label: string;
  previous: MetricValue;
  current: MetricValue;
  series: MetricValue[];
  target?: MetricValue;
  direction: "up" | "down" | "flat";
  changePercent: number;
};

const metricValuePattern =
  "[+-]?(?:[$€£¥])?\\d[\\d,]*(?:\\.\\d+)?(?:\\s*[a-zA-Z%]+)?";

function plainMarkdown(value: string): string {
  return value
    .replaceAll("*", "")
    .replaceAll("_", "")
    .replaceAll(String.fromCharCode(96), "");
}

function parseMetricValue(raw: string): MetricValue | null {
  const display = raw
    .replaceAll("*", "")
    .replaceAll("_", "")
    .replaceAll(String.fromCharCode(96), "")
    .trim();
  const match = display.match(/[+-]?[0-9][0-9,]*([.][0-9]+)?/);
  if (!match || match.index === undefined) return null;
  const value = Number(match[0].replaceAll(",", ""));
  return Number.isFinite(value) && display ? { value, display } : null;
}

function parseMetricSeries(body: string): MetricValue[] {
  const newline = String.fromCharCode(10);
  return body
    .split(newline)
    .filter((line) => line.trimStart().startsWith("|"))
    .map((line) =>
      line
        .split("|")
        .slice(1, -1)
        .map((cell) => cell.trim()),
    )
    .filter(
      (cells) =>
        cells.length > 1 &&
        !cells.every(
          (cell) => cell.length > 1 && cell.replace(/[-:]/g, "").trim() === "",
        ),
    )
    .map((cells) =>
      cells
        .slice(1)
        .map(parseMetricValue)
        .find((value) => value !== null),
    )
    .filter(
      (value): value is MetricValue => value !== undefined && value !== null,
    );
}

export function parseMetric(item: Item): Metric | null {
  const summary = plainMarkdown(item.summary);
  const body = plainMarkdown(item.report.body_md);
  const combined = summary + String.fromCharCode(10) + body;
  const valuePattern = "(" + metricValuePattern + ")";
  const fromTo = combined.match(
    new RegExp(
      "from[ ]+" + valuePattern + "[ ]+(?:to|→)[ ]+" + valuePattern,
      "i",
    ),
  );
  const series = parseMetricSeries(body);
  const previous = fromTo
    ? parseMetricValue(fromTo[1])
    : series.length > 1
      ? series[series.length - 2]
      : null;
  const current = fromTo
    ? parseMetricValue(fromTo[2])
    : series.length > 0
      ? series[series.length - 1]
      : null;
  if (!previous || !current || previous.value === 0) return null;

  const targetMatch = combined.match(
    new RegExp(
      "(?:target[ ]*(?:is|of|at|:)?[ ]*" +
        valuePattern +
        "|" +
        valuePattern +
        "[ ]+target)",
      "i",
    ),
  );
  const target = targetMatch
    ? parseMetricValue(targetMatch[1] ?? targetMatch[2])
    : undefined;
  const labelMarker = "metric attention:";
  const labelLine = body
    .split(String.fromCharCode(10))
    .find((line) => line.toLowerCase().includes(labelMarker));
  const label = labelLine
    ? labelLine
        .slice(
          labelLine.toLowerCase().indexOf(labelMarker) + labelMarker.length,
        )
        .trim()
    : item.title;
  const changePercent =
    ((current.value - previous.value) / previous.value) * 100;
  return {
    label,
    previous,
    current,
    series: series.length > 1 ? series : [previous, current],
    target: target ?? undefined,
    direction:
      current.value === previous.value
        ? "flat"
        : current.value > previous.value
          ? "up"
          : "down",
    changePercent,
  };
}

function MetricVisual({
  metric,
  compact = false,
}: {
  metric: Metric;
  compact?: boolean;
}) {
  const values = metric.series.map((value) => value.value);
  const max = Math.max(...values, metric.target?.value ?? 0);
  const min = Math.min(...values, metric.target?.value ?? max);
  const spread = max - min || max || 1;
  const change = Math.round(Math.abs(metric.changePercent));
  const arrow =
    metric.direction === "up" ? "↑" : metric.direction === "down" ? "↓" : "→";
  return (
    <div
      className={`metric-visual${compact ? " metric-visual-compact" : ""}`}
      aria-label={`${metric.label}: ${metric.current.display}, ${arrow} ${change}% versus previous`}
    >
      <div className="metric-visual-heading">
        <span className="metric-label">{metric.label}</span>
        <span className={`metric-delta ${metric.direction}`}>
          {arrow} {change}% vs previous
        </span>
      </div>
      <div className="metric-readout">
        <strong>{metric.current.display}</strong>
        {metric.target && <span>Target {metric.target.display}</span>}
      </div>
      <div className="metric-bars" aria-hidden="true">
        {metric.series.map((value, index) => (
          <span
            className={`metric-bar${index === metric.series.length - 1 ? " latest" : ""}`}
            key={`${value.display}-${index}`}
            style={{
              height: `${Math.max(18, ((value.value - min) / spread) * 82 + 18)}%`,
            }}
          />
        ))}
      </div>
      <div className="metric-axis" aria-hidden="true">
        <span>previous</span>
        <span>latest</span>
      </div>
    </div>
  );
}

function ItemBadges({ item }: { item: Item }) {
  const due = item.remind_at && new Date(item.remind_at) <= new Date();
  return (
    <div className="badges">
      <span className={`tag kind kind-${item.kind}`}>{item.kind}</span>
      {item.todo_state !== "none" && (
        <span className={`tag ${item.todo_state}`}>{item.todo_state}</span>
      )}
      {due && <span className="tag due">Reminder due</span>}
      {item.acknowledged_content_version < item.content_version && (
        <span className="tag new">New</span>
      )}
    </div>
  );
}

function ReminderChip({ item, timezone }: { item: Item; timezone?: string }) {
  if (!item.remind_at) return null;
  const due = new Date(item.remind_at) <= new Date();
  return (
    <span className={`reminder-chip ${due ? "is-due" : "is-upcoming"}`}>
      <span className="reminder-dot" aria-hidden="true" />
      <span>{due ? "Due" : "Reminder"}</span>
      <strong>{when(item.remind_at, timezone)}</strong>
      <small>{item.reminder_timezone}</small>
    </span>
  );
}

function useItemAction(itemId: string, onSuccess?: () => void) {
  const cache = useQueryClient();
  const request = useRef<{ signature: string; id: string } | null>(null);
  const mutation = useMutation({
    mutationFn: (input: { item: Item; action: object }) => {
      const signature = JSON.stringify({
        version: input.item.state_version,
        action: input.action,
      });
      if (request.current?.signature !== signature) {
        request.current = { signature, id: crypto.randomUUID() };
      }
      return api<Item>(
        `/items/${itemId}/actions`,
        {
          request_id: request.current.id,
          expected_state_version: input.item.state_version,
          action: input.action,
        },
        "POST",
      );
    },
    onSuccess: (item) => {
      cache.setQueryData(["item", item.id], item);
      cache.invalidateQueries({ queryKey: ["items"] });
      onSuccess?.();
    },
  });
  return {
    mutation,
    apply: (item: Item, action: object) => mutation.mutate({ item, action }),
  };
}

function todoAction(item: Item) {
  if (item.todo_state === "todo") return { label: "Done", state: "done" };
  if (item.todo_state === "done") return { label: "Reopen", state: "todo" };
  return { label: "Todo", state: "todo" };
}

function QuickItemActions({
  item,
  timezone,
  task = false,
}: {
  item: Item;
  timezone?: string;
  task?: boolean;
}) {
  const [reminder, setReminder] = useState(false);
  const { apply, mutation } = useItemAction(item.id, () => setReminder(false));
  const todo = todoAction(item);
  return (
    <div className={`quick-actions${task ? " task-actions" : ""}`}>
      <Button
        size="small"
        variant={task ? "outlined" : "text"}
        onClick={() => apply(item, { type: "set_todo", state: todo.state })}
        disabled={mutation.isPending}
      >
        {todo.label}
      </Button>
      {!task && (
        <Button
          size="small"
          onClick={() => setReminder(true)}
          disabled={mutation.isPending}
        >
          {item.remind_at ? "Change reminder" : "Remind"}
        </Button>
      )}
      <NavLink to={`/items/${item.id}`}>
        <Button size="small">Open</Button>
      </NavLink>
      {mutation.isError && (
        <span className="action-error" role="alert">
          {mutation.error.message}
        </span>
      )}
      {reminder && (
        <ReminderDialog
          close={() => setReminder(false)}
          apply={(action) => apply(item, action)}
          error={mutation.error}
        />
      )}
      {item.remind_at && !task && (
        <ReminderChip item={item} timezone={timezone} />
      )}
    </div>
  );
}

function TaskRow({ item, timezone }: { item: Item; timezone?: string }) {
  const todo = todoAction(item);
  const { apply, mutation } = useItemAction(item.id);
  return (
    <article className="task-row">
      <button
        className={`task-check ${item.todo_state === "done" ? "checked" : ""}`}
        aria-label={`${todo.label}: ${item.title}`}
        onClick={() => apply(item, { type: "set_todo", state: todo.state })}
        disabled={mutation.isPending}
      >
        {item.todo_state === "done" ? "✓" : ""}
      </button>
      <div className="task-copy">
        <div className="task-row-meta">
          <span className="tag kind kind-task">task</span>
          {item.remind_at && <ReminderChip item={item} timezone={timezone} />}
        </div>
        <NavLink to={`/items/${item.id}`} className="task-title">
          {item.title}
        </NavLink>
        <p>{item.summary}</p>
      </div>
      <QuickItemActions item={item} timezone={timezone} task />
    </article>
  );
}

function DenseItemRow({ item, timezone }: { item: Item; timezone?: string }) {
  return (
    <article className={`item-row kind-${item.kind}`}>
      <div className="item-row-main">
        <div className="item-row-top">
          <ItemBadges item={item} />
          <small>{when(item.content_updated_at, timezone)}</small>
        </div>
        <NavLink to={`/items/${item.id}`} className="item-row-title">
          {item.title}
        </NavLink>
        <p>{item.summary}</p>
      </div>
      <QuickItemActions item={item} timezone={timezone} />
    </article>
  );
}

function OutcomeCard({ item, timezone }: { item: Item; timezone?: string }) {
  const metric = parseMetric(item);
  return (
    <NavLink to={`/items/${item.id}`} className="outcome-card">
      <div className="outcome-card-top">
        <ItemBadges item={item} />
        <span>Open detail ↗</span>
      </div>
      <h3>{item.title}</h3>
      {metric ? (
        <MetricVisual metric={metric} compact />
      ) : (
        <p>{item.summary}</p>
      )}
      {item.remind_at && <ReminderChip item={item} timezone={timezone} />}
    </NavLink>
  );
}

function ProposalCard({ proposal }: { proposal: Proposal }) {
  const cache = useQueryClient();
  const request = useRef<{ resolution: string; id: string } | null>(null);
  const resolve = useMutation({
    mutationFn: (resolution: string) => {
      if (request.current?.resolution !== resolution)
        request.current = { resolution, id: crypto.randomUUID() };
      return api<Proposal>(
        `/proposals/${proposal.id}/resolve`,
        { request_id: request.current.id, resolution },
        "POST",
      );
    },
    onSuccess: () => {
      cache.invalidateQueries({ queryKey: ["proposals"] });
      cache.invalidateQueries({ queryKey: ["interests"] });
      cache.invalidateQueries({ queryKey: ["watches"] });
    },
  });
  return (
    <section className="panel proposal-card">
      <div className="section-heading">
        <div className="badges">
          <span className="tag">Proposal</span>
          <span className="tag kind">
            {proposal.operation} {proposal.target_type}
          </span>
        </div>
      </div>
      <ReactMarkdown skipHtml>{proposal.rationale_md}</ReactMarkdown>
      <div className="proposal-actions">
        <Button
          variant="contained"
          size="small"
          onClick={() => resolve.mutate("accepted")}
        >
          Accept
        </Button>
        <Button size="small" onClick={() => resolve.mutate("rejected")}>
          Reject
        </Button>
      </div>
      {resolve.isError && (
        <Alert severity="error">
          {resolve.error.message} Refresh to review the current configuration.
        </Alert>
      )}
    </section>
  );
}

function SummaryMetric({
  label,
  value,
  tone,
}: {
  label: string;
  value: number | string;
  tone: "warm" | "green" | "blue" | "gold";
}) {
  return (
    <div className={`summary-metric ${tone}`}>
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  );
}

export function Attention() {
  const query = useQuery({
    queryKey: ["items", "attention"],
    queryFn: () => collection<Item>("/items?view=attention"),
  });
  const proposals = useQuery({
    queryKey: ["proposals", "pending"],
    queryFn: () => collection<Proposal>("/proposals?state=pending"),
  });
  const settings = useSettings();
  const items = query.data ?? [];
  const outcomes = items.filter(
    (item) =>
      item.kind === "outcome" ||
      (item.kind !== "task" && parseMetric(item) !== null),
  );
  const tasks = items.filter((item) => item.kind === "task");
  const outcomeIds = new Set(outcomes.map((item) => item.id));
  const findings = items.filter(
    (item) => item.kind !== "task" && !outcomeIds.has(item.id),
  );
  const now = new Date();
  const dueCount = items.filter(
    (item) => item.remind_at && new Date(item.remind_at) <= now,
  ).length;
  const todoCount = items.filter((item) => item.todo_state === "todo").length;
  const freshCount = items.filter(
    (item) => item.acknowledged_content_version < item.content_version,
  ).length;
  return (
    <>
      <header className="page-header">
        <div className="eyebrow">A little space for what matters</div>
        <h1>Attention</h1>
        <p>
          See what changed, decide what matters, and move one thing forward.
        </p>
        <div className="attention-summary">
          <SummaryMetric label="due now" value={dueCount} tone="warm" />
          <SummaryMetric label="open Todos" value={todoCount} tone="green" />
          <SummaryMetric label="new findings" value={freshCount} tone="blue" />
          <SummaryMetric
            label="proposals"
            value={proposals.data?.length ?? "—"}
            tone="gold"
          />
        </div>
      </header>
      {query.isError && (
        <Alert severity="error">
          Could not load Attention. Check the local server.
        </Alert>
      )}
      {query.isPending && <p role="status">Gathering what needs attention…</p>}
      {outcomes.length > 0 && (
        <section className="outcome-rail" aria-labelledby="outcomes-heading">
          <div className="section-heading rail-heading">
            <div>
              <div className="eyebrow">What is moving</div>
              <h2 id="outcomes-heading">Outcomes</h2>
            </div>
            <span className="tag">Metric signals</span>
          </div>
          <div className="outcome-grid">
            {outcomes.map((item) => (
              <OutcomeCard
                item={item}
                timezone={settings.data?.timezone}
                key={item.id}
              />
            ))}
          </div>
        </section>
      )}
      {proposals.data && proposals.data.length > 0 && (
        <section
          className="attention-section"
          aria-labelledby="proposals-heading"
        >
          <div className="section-heading section-heading-tight">
            <h2 id="proposals-heading">Proposals to review</h2>
            <span className="tag">{proposals.data.length}</span>
          </div>
          <div className="proposal-list">
            {proposals.data.map((proposal) => (
              <ProposalCard proposal={proposal} key={proposal.id} />
            ))}
          </div>
        </section>
      )}
      {tasks.length > 0 && (
        <section className="attention-section" aria-labelledby="tasks-heading">
          <div className="section-heading section-heading-tight">
            <h2 id="tasks-heading">Tasks</h2>
            <span className="tag">Act or check out</span>
          </div>
          <div className="task-list">
            {tasks.map((item) => (
              <TaskRow
                item={item}
                timezone={settings.data?.timezone}
                key={item.id}
              />
            ))}
          </div>
        </section>
      )}
      {findings.length > 0 && (
        <section
          className="attention-section"
          aria-labelledby="findings-heading"
        >
          <div className="section-heading section-heading-tight">
            <h2 id="findings-heading">Reports & notes</h2>
            <span className="tag">Act directly</span>
          </div>
          <div className="item-list">
            {findings.map((item) => (
              <DenseItemRow
                item={item}
                timezone={settings.data?.timezone}
                key={item.id}
              />
            ))}
          </div>
        </section>
      )}
      {query.data?.length === 0 && proposals.data?.length === 0 && (
        <section className="panel empty">
          <div className="empty-mark">◉</div>
          <h2>You’re all caught up</h2>
          <p>Due reminders, open Todos, and new reports will appear here.</p>
          <NavLink to="/interests">
            <Button variant="outlined">Review Interests</Button>
          </NavLink>
        </section>
      )}
    </>
  );
}

export function Library() {
  const [view, setView] = useState("all");
  const [kind, setKind] = useState("");
  const [search, setSearch] = useState("");
  const params = new URLSearchParams({
    view,
    ...(kind ? { kind } : {}),
    ...(search.trim() ? { q: search.trim() } : {}),
  });
  const query = useQuery({
    queryKey: ["items", "library", view, kind, search],
    queryFn: () => collection<Item>(`/items?${params}`),
  });
  const settings = useSettings();
  const items = query.data ?? [];
  const outcomes = items.filter(
    (item) =>
      item.kind === "outcome" ||
      (item.kind !== "task" && parseMetric(item) !== null),
  );
  const tasks = items.filter((item) => item.kind === "task");
  const outcomeIds = new Set(outcomes.map((item) => item.id));
  const findings = items.filter(
    (item) => item.kind !== "task" && !outcomeIds.has(item.id),
  );
  return (
    <>
      <header className="page-header">
        <div className="eyebrow">Everything worth keeping</div>
        <h1>Library</h1>
        <p>Keep the useful parts close, with the next action in reach.</p>
      </header>
      <div className="library-summary" aria-label="Library summary">
        <SummaryMetric label="items" value={items.length} tone="blue" />
        <SummaryMetric label="outcomes" value={outcomes.length} tone="green" />
        <SummaryMetric label="tasks" value={tasks.length} tone="gold" />
        <SummaryMetric
          label="reports & notes"
          value={findings.length}
          tone="warm"
        />
      </div>
      <div className="library-tools">
        <TextField
          size="small"
          label="Search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <TextField
          size="small"
          select
          label="Work state"
          value={view}
          onChange={(e) => setView(e.target.value)}
        >
          {[
            ["all", "All"],
            ["todo", "Todo"],
            ["done", "Done"],
          ].map(([v, l]) => (
            <MenuItem key={v} value={v}>
              {l}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          size="small"
          select
          label="Kind"
          value={kind}
          onChange={(e) => setKind(e.target.value)}
        >
          {[
            ["", "Any kind"],
            ["report", "Reports"],
            ["note", "Notes"],
            ["task", "Tasks"],
            ["outcome", "Outcomes"],
          ].map(([v, l]) => (
            <MenuItem key={v} value={v}>
              {l}
            </MenuItem>
          ))}
        </TextField>
      </div>
      {query.isError && (
        <Alert severity="error">Could not load the Library.</Alert>
      )}
      {outcomes.length > 0 && (
        <section
          className="outcome-rail library-outcomes"
          aria-labelledby="library-outcomes-heading"
        >
          <div className="section-heading rail-heading">
            <div>
              <div className="eyebrow">At a glance</div>
              <h2 id="library-outcomes-heading">Outcomes</h2>
            </div>
            <span className="tag">Metric signals</span>
          </div>
          <div className="outcome-grid">
            {outcomes.map((item) => (
              <OutcomeCard
                item={item}
                timezone={settings.data?.timezone}
                key={item.id}
              />
            ))}
          </div>
        </section>
      )}
      {(tasks.length > 0 || findings.length > 0) && (
        <section
          className="library-items"
          aria-labelledby="library-items-heading"
        >
          <div className="section-heading section-heading-tight">
            <h2 id="library-items-heading">All items</h2>
            <span className="tag">{tasks.length + findings.length}</span>
          </div>
          <div className="task-list">
            {tasks.map((item) => (
              <TaskRow
                item={item}
                timezone={settings.data?.timezone}
                key={item.id}
              />
            ))}
          </div>
          <div className="item-list">
            {findings.map((item) => (
              <DenseItemRow
                item={item}
                timezone={settings.data?.timezone}
                key={item.id}
              />
            ))}
          </div>
        </section>
      )}
      {query.data?.length === 0 && (
        <section className="panel empty">
          <h2>No matching items</h2>
          <p>Try another filter or wait for an agent to publish a finding.</p>
        </section>
      )}
    </>
  );
}

function ReminderDialog({
  close,
  apply,
  error,
}: {
  close: () => void;
  apply: (action: object) => void;
  error: Error | null;
}) {
  const settings = useSettings();
  const tomorrow = dateInputValue(
    new Date(Date.now() + 86400000),
    settings.data?.timezone,
  );
  const [date, setDate] = useState(tomorrow);
  const [clock, setClock] = useState("09:00");
  const [timezone, setTimezone] = useState<string | null>(null);
  const [offset, setOffset] = useState("");
  const offsets =
    error instanceof APIError && error.code === "ambiguous_local_time"
      ? (error.details?.utc_offsets as string[] | undefined)
      : undefined;
  return (
    <Dialog open onClose={close} fullWidth maxWidth="xs">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          apply({
            type: "set_reminder",
            date,
            time: clock,
            timezone: effectiveTimezone(timezone ?? settings.data?.timezone),
            ...(offset ? { utc_offset: offset } : {}),
          });
        }}
      >
        <DialogTitle>Remind me</DialogTitle>
        <DialogContent>
          <div className="editor-fields">
            <p>Set one local reminder for this item.</p>
            <TextField
              required
              type="date"
              label="Date"
              value={date}
              onChange={(e) => {
                setDate(e.target.value);
                setOffset("");
              }}
              slotProps={{ inputLabel: { shrink: true } }}
            />
            <TextField
              required
              type="time"
              label="Time"
              value={clock}
              onChange={(e) => {
                setClock(e.target.value);
                setOffset("");
              }}
              slotProps={{ inputLabel: { shrink: true } }}
            />
            <TextField
              required
              label="Timezone"
              value={timezone ?? effectiveTimezone(settings.data?.timezone)}
              onChange={(e) => {
                setTimezone(e.target.value);
                setOffset("");
              }}
              helperText={
                timezone
                  ? "IANA timezone, such as Asia/Singapore"
                  : "Using the browser default"
              }
            />
            {offsets && (
              <TextField
                required
                select
                label="This time occurs twice"
                value={offset}
                onChange={(e) => setOffset(e.target.value)}
                helperText="Choose the UTC offset for the intended occurrence."
              >
                {offsets.map((value) => (
                  <MenuItem key={value} value={value}>
                    {value}
                  </MenuItem>
                ))}
              </TextField>
            )}
            {error && !offsets && (
              <Alert severity="error">{error.message}</Alert>
            )}
          </div>
        </DialogContent>
        <DialogActions>
          <Button onClick={close}>Cancel</Button>
          <Button type="submit" variant="contained">
            Set reminder
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

export function ItemDetail() {
  const { id = "" } = useParams();
  const cache = useQueryClient();
  const settings = useSettings();
  const query = useQuery({
    queryKey: ["item", id],
    queryFn: () => api<Item>(`/items/${id}`),
  });
  const [reminder, setReminder] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const noteRequest = useRef<{ signature: string; id: string } | null>(null);
  const { apply: applyItemAction, mutation } = useItemAction(id, () =>
    setReminder(false),
  );
  const noteMutation = useMutation({
    mutationFn: (body: object) => api<Item>(`/items/${id}/note`, body, "PUT"),
    onSuccess: (item) => {
      cache.setQueryData(["item", id], item);
      cache.invalidateQueries({ queryKey: ["items"] });
      setNote(null);
    },
  });
  const item = query.data;
  function apply(action: object) {
    if (!item) return;
    applyItemAction(item, action);
  }
  function saveNote() {
    if (!item || note === null) return;
    const signature = JSON.stringify({ version: item.state_version, note });
    if (noteRequest.current?.signature !== signature)
      noteRequest.current = { signature, id: crypto.randomUUID() };
    noteMutation.mutate({
      request_id: noteRequest.current.id,
      expected_state_version: item.state_version,
      user_note: note,
    });
  }
  if (query.isPending) return <p role="status">Opening report…</p>;
  if (query.isError || !item)
    return <Alert severity="error">This item could not be opened.</Alert>;
  const sourceById = Object.fromEntries(
    item.sources.map((source) => [source.id, source]),
  );
  const metric = parseMetric(item);
  return (
    <>
      <NavLink to="/" className="back-link">
        ← Attention
      </NavLink>
      <header className="detail-header">
        <ItemBadges item={item} />
        <h1>{item.title}</h1>
        <p>{item.summary}</p>
        <div className="item-meta">
          Content updated{" "}
          {when(item.content_updated_at, settings.data?.timezone)} · version{" "}
          {item.content_version}
        </div>
      </header>
      {metric && (
        <section className="panel metric-detail">
          <div className="section-heading">
            <div>
              <div className="eyebrow">Outcome signal</div>
              <h2>Metric at a glance</h2>
            </div>
            <span className="tag">Derived from the report</span>
          </div>
          <MetricVisual metric={metric} />
        </section>
      )}
      <div className="detail-layout">
        <article className="panel report-body">
          {item.report.schema_version === 1 ? (
            <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>
              {item.report.body_md}
            </ReactMarkdown>
          ) : (
            <Alert severity="warning">
              This report format is not supported yet. The summary and sources
              remain available.
            </Alert>
          )}
        </article>
        <aside className="detail-side">
          <section className="panel">
            <h2>Follow through</h2>
            <div className="action-stack">
              {item.todo_state === "todo" ? (
                <Button
                  variant="outlined"
                  onClick={() => apply({ type: "set_todo", state: "done" })}
                >
                  Mark Done
                </Button>
              ) : (
                <Button
                  variant="outlined"
                  onClick={() => apply({ type: "set_todo", state: "todo" })}
                >
                  {item.todo_state === "done" ? "Reopen Todo" : "Set Todo"}
                </Button>
              )}
              <Button variant="outlined" onClick={() => setReminder(true)}>
                {item.remind_at ? "Change reminder" : "Remind me"}
              </Button>
              {item.remind_at && (
                <Button onClick={() => apply({ type: "clear_reminder" })}>
                  Clear reminder
                </Button>
              )}
              {item.acknowledged_content_version < item.content_version && (
                <Button
                  onClick={() =>
                    apply({
                      type: "acknowledge",
                      content_version: item.content_version,
                    })
                  }
                >
                  Acknowledge update
                </Button>
              )}
            </div>
            {item.remind_at && (
              <div className="reminder-callout">
                <ReminderChip item={item} timezone={settings.data?.timezone} />
              </div>
            )}
            {mutation.isError && (
              <Alert severity="error">{mutation.error.message}</Alert>
            )}
          </section>
          <section className="panel">
            <h2>Sources</h2>
            <div className="sources">
              {item.sources.map((source) => (
                <a
                  key={source.id}
                  href={source.url}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {source.label}
                  <small>
                    Observed {when(source.observed_at, settings.data?.timezone)}
                  </small>
                </a>
              ))}
            </div>
          </section>
          <section className="panel">
            <h2>Your note</h2>
            <TextField
              multiline
              minRows={3}
              fullWidth
              value={note ?? item.user_note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="Keep context for the next run…"
            />
            <Button
              size="small"
              disabled={note === null || noteMutation.isPending}
              onClick={saveNote}
            >
              Save note
            </Button>
            {noteMutation.isError && (
              <Alert severity="error">{noteMutation.error.message}</Alert>
            )}
          </section>
        </aside>
      </div>
      {(item.report.actions?.length ?? 0) > 0 && (
        <section className="generated-actions">
          <span>Suggested</span>
          {item.report.actions?.map((action) =>
            action.type === "open_link" ? (
              <a
                key={action.id}
                href={sourceById[action.source_ref ?? ""]?.url}
                target="_blank"
                rel="noopener noreferrer"
              >
                <Button size="small">{action.label}</Button>
              </a>
            ) : (
              <Button
                size="small"
                key={action.id}
                onClick={() =>
                  action.type === "set_reminder"
                    ? setReminder(true)
                    : apply(
                        action.type === "acknowledge"
                          ? {
                              type: "acknowledge",
                              content_version: item.content_version,
                            }
                          : {
                              type: action.type,
                              ...(action.state ? { state: action.state } : {}),
                            },
                      )
                }
              >
                {action.label}
              </Button>
            ),
          )}
        </section>
      )}
      {reminder && (
        <ReminderDialog
          close={() => setReminder(false)}
          apply={apply}
          error={mutation.error}
        />
      )}
    </>
  );
}
