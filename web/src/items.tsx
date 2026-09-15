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

const when = (value: string) =>
  new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));

function ItemBadges({ item }: { item: Item }) {
  const due = item.remind_at && new Date(item.remind_at) <= new Date();
  return (
    <div className="badges">
      <span className="tag kind">{item.kind}</span>
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

function ItemCard({ item }: { item: Item }) {
  return (
    <NavLink to={`/items/${item.id}`} className="item-card panel">
      <div className="section-heading">
        <ItemBadges item={item} />
        <small>{when(item.content_updated_at)}</small>
      </div>
      <h2>{item.title}</h2>
      <p>{item.summary}</p>
      {item.remind_at && (
        <div className="item-meta">
          Remind {when(item.remind_at)} · {item.reminder_timezone}
        </div>
      )}
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

export function Attention() {
  const query = useQuery({
    queryKey: ["items", "attention"],
    queryFn: () => collection<Item>("/items?view=attention"),
  });
  const proposals = useQuery({
    queryKey: ["proposals", "pending"],
    queryFn: () => collection<Proposal>("/proposals?state=pending"),
  });
  const now = new Date();
  const dueCount =
    query.data?.filter(
      (item) => item.remind_at && new Date(item.remind_at) <= now,
    ).length ?? 0;
  const todoCount =
    query.data?.filter((item) => item.todo_state === "todo").length ?? 0;
  const freshCount =
    query.data?.filter(
      (item) => item.acknowledged_content_version < item.content_version,
    ).length ?? 0;
  return (
    <>
      <header className="page-header">
        <div className="eyebrow">A little space for what matters</div>
        <h1>Attention</h1>
        <p>Reminders, open Todos, and fresh findings—together, once.</p>
        <div className="attention-counts">
          <span>{dueCount} due</span>
          <span>{todoCount} Todo</span>
          <span>{freshCount} new</span>
          <span>{proposals.data?.length ?? 0} proposals</span>
        </div>
      </header>
      {query.isError && (
        <Alert severity="error">
          Could not load Attention. Check the local server.
        </Alert>
      )}
      {query.isPending && <p role="status">Gathering what needs attention…</p>}
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
      <div className="item-grid">
        {proposals.data?.map((proposal) => (
          <ProposalCard proposal={proposal} key={proposal.id} />
        ))}
        {query.data?.map((item) => (
          <ItemCard item={item} key={item.id} />
        ))}
      </div>
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
  return (
    <>
      <header className="page-header">
        <div className="eyebrow">Everything worth keeping</div>
        <h1>Library</h1>
        <p>Reports, notes, tasks, and outcomes from your agents and you.</p>
      </header>
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
      <div className="item-grid">
        {query.data?.map((item) => (
          <ItemCard key={item.id} item={item} />
        ))}
      </div>
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
  const settings = useQuery({
    queryKey: ["settings"],
    queryFn: () => api<{ timezone: string }>("/settings"),
  });
  const tomorrow = new Date(Date.now() + 86400000).toISOString().slice(0, 10);
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
            timezone: timezone ?? settings.data?.timezone ?? "UTC",
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
              value={timezone ?? settings.data?.timezone ?? ""}
              onChange={(e) => {
                setTimezone(e.target.value);
                setOffset("");
              }}
              helperText="IANA timezone, such as Asia/Singapore"
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
  const query = useQuery({
    queryKey: ["item", id],
    queryFn: () => api<Item>(`/items/${id}`),
  });
  const [reminder, setReminder] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const request = useRef<{ signature: string; id: string } | null>(null);
  const noteRequest = useRef<{ signature: string; id: string } | null>(null);
  const mutation = useMutation({
    mutationFn: (body: object) =>
      api<Item>(`/items/${id}/actions`, body, "POST"),
    onSuccess: (item) => {
      cache.setQueryData(["item", id], item);
      cache.invalidateQueries({ queryKey: ["items"] });
      setReminder(false);
    },
  });
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
    const signature = JSON.stringify({ version: item.state_version, action });
    if (request.current?.signature !== signature)
      request.current = { signature, id: crypto.randomUUID() };
    mutation.mutate({
      request_id: request.current.id,
      expected_state_version: item.state_version,
      action,
    });
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
          Content updated {when(item.content_updated_at)} · version{" "}
          {item.content_version}
        </div>
      </header>
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
              <p className="item-meta">
                {when(item.remind_at)}
                <br />
                {item.reminder_timezone}
              </p>
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
                  <small>Observed {when(source.observed_at)}</small>
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
