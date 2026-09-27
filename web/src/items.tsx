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
  url?: string;
  label: string;
  observed_at?: string;
  source_date?: string;
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
  origin: "agent" | "user" | "legacy";
  interests: {id:string;reason:string}[];
  watch_id?: string;
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
  proposal_key:string;
  target_type: string;
  operation: string;
  rationale_md: string;
  state: string;
  evidence_links?:string[];
  confidence?:number;
  payload?:object;
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

export function useItemAction(itemId: string, onSuccess?: () => void) {
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
      cache.invalidateQueries({ queryKey: ["status"] });
      onSuccess?.();
    },
  });
  return {
    mutation,
    apply: (item: Item, action: object) => mutation.mutate({ item, action }),
  };
}

export function ProposalCard({ proposal, candidates }: { proposal: Proposal; candidates: Proposal[] }) {
  const cache = useQueryClient();
  const [mergeOpen,setMergeOpen]=useState(false);
  const [mergeInto,setMergeInto]=useState("");
  const planPreview=useQuery({queryKey:["proposal-plan-preview",proposal.id],queryFn:()=>api<{due_before:number;due_after:number;changes:{target_type:string;operation:string;target:string;cursor_reset:boolean;affected_items:number}[]}>("/config/plans/preview",proposal.payload,"POST"),enabled:proposal.target_type==="config_plan"&&!!proposal.payload});
  const request = useRef<{ signature: string; id: string } | null>(null);
  const resolve = useMutation({
    mutationFn: (decision: {resolution:string;snooze_until?:string;merge_into?:string}) => {
      const signature=JSON.stringify(decision);
      if (request.current?.signature !== signature)
        request.current = { signature, id: crypto.randomUUID() };
      return api<Proposal>(
        `/proposals/${proposal.id}/resolve`,
        { request_id: request.current.id, ...decision },
        "POST",
      );
    },
    onSuccess: () => {
      setMergeOpen(false);
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
      {proposal.confidence!==undefined&&<small>Confidence: {Math.round(proposal.confidence*100)}%</small>}
      {!!proposal.evidence_links?.length&&<div>{proposal.evidence_links.map((link)=><a key={link} href={link} target="_blank" rel="noopener noreferrer">{link}</a>)}</div>}
      {proposal.target_type==="config_plan"&&<div>
        {planPreview.isPending&&<p>Checking the change set…</p>}
        {planPreview.isError&&<Alert severity="error">Change set cannot be previewed: {planPreview.error.message}</Alert>}
        {planPreview.data&&<div><p>Due Watchers: {planPreview.data.due_before} → {planPreview.data.due_after}</p>
          {planPreview.data.changes.map((change,index)=><p key={index}>{change.operation} {change.target_type} {change.target}{change.cursor_reset&&" · resets source cursor"}{change.affected_items>0&&` · ${change.affected_items} existing Items`}</p>)}
        </div>}
      </div>}
      <div className="proposal-actions">
        <Button
          variant="contained"
          size="small"
          disabled={resolve.isPending||(proposal.target_type==="config_plan"&&!planPreview.data)}
          onClick={() => resolve.mutate({resolution:"accepted"})}
        >
          Accept
        </Button>
        <Button size="small" onClick={() => resolve.mutate({resolution:"rejected"})}>
          Reject
        </Button>
        <Button size="small" onClick={()=>resolve.mutate({resolution:"snoozed",snooze_until:new Date(Date.now()+7*86400000).toISOString()})}>Snooze 7 days</Button>
        {candidates.length>1&&<Button size="small" onClick={()=>setMergeOpen(true)}>Merge…</Button>}
      </div>
      <Dialog open={mergeOpen} onClose={()=>setMergeOpen(false)}><DialogTitle>Merge duplicate proposal</DialogTitle><DialogContent>
        <TextField select fullWidth label="Keep proposal" value={mergeInto} onChange={(event)=>setMergeInto(event.target.value)}>
          {candidates.filter((candidate)=>candidate.id!==proposal.id).map((candidate)=><MenuItem key={candidate.id} value={candidate.id}>{candidate.proposal_key}</MenuItem>)}
        </TextField>
      </DialogContent><DialogActions><Button onClick={()=>setMergeOpen(false)}>Cancel</Button><Button disabled={!mergeInto||resolve.isPending} onClick={()=>resolve.mutate({resolution:"merged",merge_into:mergeInto})}>Merge</Button></DialogActions></Dialog>
      {resolve.isError && (
        <Alert severity="error">
          {resolve.error.message} Refresh to review the current configuration.
        </Alert>
      )}
    </section>
  );
}

export function ReminderDialog({
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
  const watch = useQuery({queryKey:["watch",item?.watch_id],queryFn:()=>api<{slug:string;source:{kind:string;locator:string}}>(`/watches/${item?.watch_id}`),enabled:!!item?.watch_id});
  const interests = useQuery({queryKey:["interests"],queryFn:()=>collection<{id:string;slug:string;title:string}>("/interests?state=all")});
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
              {item.sources.length===0 && <p>No source reference was supplied.</p>}
              {item.sources.map((source) => source.url ? (
                <a key={source.id} href={source.url} target="_blank" rel="noopener noreferrer">
                  {source.label}
                  {source.observed_at && <small>Observed {when(source.observed_at, settings.data?.timezone)}</small>}
                </a>
              ) : <div key={source.id}>{source.label || "User-supplied source date"}<small>{source.source_date}</small></div>)}
            </div>
            {item.watch_id && <p>Originating Watcher: <NavLink to={`/interests#watcher-${watch.data?.slug ?? ""}`}>{watch.data?.slug ?? item.watch_id}</NavLink></p>}
            {item.origin==="legacy"&&<p>Historical Item: the original source or relevance explanation may be unavailable.</p>}
            {item.interests?.length>0 && <div>
              <h3>Why this matters</h3>
              {item.interests.map((reason)=>{
                const interest=interests.data?.find((entry)=>entry.id===reason.id);
                return <p key={reason.id}><NavLink to={`/interests#interest-${interest?.slug ?? ""}`}>{interest?.title ?? reason.id}</NavLink>: {reason.reason || "Historical assignment; original explanation unavailable."}</p>
              })}
            </div>}
            <p>In Attention because {[
              item.todo_state==="todo"?"Todo is open":null,
              item.remind_at && new Date(item.remind_at)<=new Date()?"reminder is due":null,
              item.acknowledged_content_version<item.content_version?"content update is unacknowledged":null,
            ].filter(Boolean).join(", ") || "no current trigger"}.</p>
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
