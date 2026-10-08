import { useEffect, useRef, useState } from "react";
import { useInfiniteQuery, useMutation, useQuery } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Menu,
  MenuItem,
  TextField,
  useMediaQuery,
} from "@mui/material";
import MoreVert from "@mui/icons-material/MoreVert";
import Check from "@mui/icons-material/Check";
import VisibilityOutlined from "@mui/icons-material/VisibilityOutlined";
import Schedule from "@mui/icons-material/Schedule";
import FilterList from "@mui/icons-material/FilterList";
import DescriptionOutlined from "@mui/icons-material/DescriptionOutlined";
import ExpandLess from "@mui/icons-material/ExpandLess";
import { NavLink, useSearchParams } from "react-router";
import { api, collection } from "./api";
import { type Item, ProposalCard, ReminderDialog, useItemAction } from "./items";
import { ItemReader, attentionReasons } from "./item-reader";
import { dateInputValue, effectiveTimezone, formatDateTime, useSettings } from "./settings";
import type { ServiceStatus } from "./health";

type Interest = { id: string; slug: string; title: string };
type WorkspaceMode = "attention" | "library" | "todo";
type Proposal = {
  id: string;
  proposal_key: string;
  target_type: string;
  operation: string;
  rationale_md: string;
  state: string;
  evidence_links?: string[];
  confidence?: number;
  payload?: object;
};

export function InboxCaptureDialog({ close }: { close: () => void }) {
  const cache = useQueryClient();
  const identity = useRef(crypto.randomUUID());
  const attempt = useRef<{ signature: string; requestId: string } | null>(null);
  const [title, setTitle] = useState("");
  const [information, setInformation] = useState("");
  const [sourceDate, setSourceDate] = useState("");
  const create = useMutation({
    mutationFn: () => {
      const signature = JSON.stringify({
        title,
        information,
        sourceDate,
      });
      if (attempt.current?.signature !== signature)
        attempt.current = { signature, requestId: crypto.randomUUID() };
      return api<Item>(
        "/items",
        {
          request_id: attempt.current.requestId,
          dedupe_key: `user:${identity.current}`,
          kind: "note",
          title: title.trim() || information.trim().split(/\r?\n/)[0].slice(0, 120),
          summary: information.trim().slice(0, 280),
          sources: sourceDate
            ? [
                {
                  id: "user-date",
                  label: "Source date",
                  source_date: sourceDate,
                },
              ]
            : [],
          report: { schema_version: 1, body_md: information.trim() },
        },
        "POST",
      );
    },
    onSuccess: async () => {
      await Promise.all([
        cache.invalidateQueries({ queryKey: ["items"] }),
        cache.invalidateQueries({ queryKey: ["status"] }),
      ]);
      close();
    },
  });
  return (
    <Dialog open onClose={() => { if (!create.isPending) close(); }} fullWidth maxWidth="sm">
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (information.trim() && !create.isPending) create.mutate();
        }}
      >
        <DialogTitle>Add to inbox</DialogTitle>
        <DialogContent>
          <div className="editor-fields">
            <p>Drop a note, link, or instruction here. The agent will review it in the next run.</p>
            <TextField
              autoFocus
              required
              multiline
              minRows={5}
              label="Information"
              value={information}
              onChange={(event) => setInformation(event.target.value)}
            />
            <details className="capture-context">
              <summary>Optional context</summary>
              <div className="editor-fields">
                <TextField label="Title (optional)" value={title} onChange={(event) => setTitle(event.target.value)} />
            <TextField
              label="Source date (optional)"
              type="date"
              value={sourceDate}
              onChange={(event) => setSourceDate(event.target.value)}
              slotProps={{ inputLabel: { shrink: true } }}
            />
              </div>
            </details>
            {create.isError && (
              <Alert severity="error">{create.error.message}</Alert>
            )}
          </div>
        </DialogContent>
        <DialogActions>
          <Button onClick={close} disabled={create.isPending}>Cancel</Button>
          <Button type="submit" variant="contained" disabled={!information.trim() || create.isPending}>
            {create.isPending ? "Adding…" : "Add to inbox"}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

export function ItemWorkspace({
  mode,
}: {
  mode: WorkspaceMode;
}) {
  const settings = useSettings();
  const compact = useMediaQuery("(max-width:720px)");
  const [searchParams] = useSearchParams();
  const search = searchParams.get("q") ?? "";
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [kind, setKind] = useState("");
  const [interest, setInterest] = useState("");
  const [sorts, setSorts] = useState<Record<WorkspaceMode, string>>({attention:"priority", todo:"priority", library:"newest"});
  const sort = sorts[mode];
  const setSort = (value:string) => setSorts(current => ({...current, [mode]:value}));
  const [selected, setSelected] = useState<string | null>(null);
  const [proposalReview, setProposalReview] = useState(false);
  useEffect(() => setSelected(null), [search, mode]);
  useEffect(() => setProposalReview(false), [mode]);
  const view = mode === "library" ? "all" : mode;
  const items = useInfiniteQuery({
    queryKey: ["items", "workspace", view, kind, interest, search.trim(), sort],
    initialPageParam: "",
    queryFn: ({ pageParam }) => {
      const query = new URLSearchParams({ view, limit: "100" });
      query.set("sort", sort);
      if (kind) query.set("kind", kind);
      if (interest) query.set("interest_id", interest);
      if (search.trim()) query.set("q", search.trim());
      if (pageParam) query.set("cursor", pageParam);
      return api<{ items: Item[]; next_cursor: string | null }>(
        `/items?${query}`,
      );
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    refetchInterval: mode === "attention" ? 5000 : false,
  });
  const interests = useQuery({
    queryKey: ["interests"],
    queryFn: () => collection<Interest>("/interests?state=all"),
  });
  const status = useQuery({
    queryKey: ["status"],
    queryFn: () => api<ServiceStatus>("/status"),
    enabled: mode === "attention",
  });
  const proposals = useQuery({
    queryKey: ["proposals", "pending"],
    queryFn: () => collection<Proposal>("/proposals?state=pending"),
    enabled: mode === "attention",
  });
  const filtered = items.data?.pages.flatMap((page) => page.items) ?? [];
  const grouped = mode !== "library" && sort === "priority";
  const groups = (mode === "todo" ? ["Reminder due", "To do"] : ["Reminder due", "Todos", "New evidence"]).map(label => ({
    label,
    items: filtered.filter(item => queueGroup(item, mode) === label),
  })).filter(group => group.items.length);
  const firstId = filtered[0]?.id;
  useEffect(() => {
    if (selected === null && !compact && firstId) setSelected(firstId);
  }, [selected, compact, firstId]);
  // Keep the open report available after acknowledgement removes it from Attention.
  const activeId =
    selected === "" ? null : (selected ?? (compact ? null : filtered[0]?.id));
  const title =
    mode === "attention"
      ? "Attention"
      : mode === "todo"
        ? "Todos"
        : "All items";
  return (
    <div className={"item-workspace" + (activeId ? " has-reader" : "")}>
      <div className="workspace-list" aria-label="Item queue">
        <h1 className="visually-hidden">{title}</h1>
        <div className="queue-tools">
          <label className="queue-sort">Sort
            <select aria-label="Sort items" value={sort} onChange={event => setSort(event.target.value)}>
              <option value="priority">{mode === "todo" ? "Due reminders first" : mode === "library" ? "Needs attention first" : "Attention first"}</option>
              <option value="newest">Newest updates</option>
              <option value="oldest">Oldest updates</option>
              <option value="title">Title A–Z</option>
            </select>
          </label>
          <details className="queue-filters">
            <summary><FilterList fontSize="small" /> Filters{(kind || interest) && <span className="filter-active">{Number(!!kind) + Number(!!interest)}</span>}</summary>
            <div className="workspace-filters">
              <select
                aria-label="Filter by Interest"
                value={interest}
                onChange={(event) => {
                  setInterest(event.target.value);
                  setSelected(null);
                }}
              >
                <option value="">All interests</option>
                {interests.data?.map((entry) => (
                  <option key={entry.id} value={entry.id}>
                    {entry.title}
                  </option>
                ))}
              </select>
              <select
                aria-label="Filter by kind"
                value={kind}
                onChange={(event) => {
                  setKind(event.target.value);
                  setSelected(null);
                }}
              >
                <option value="">All kinds</option>
                {["report", "outcome", "note", "task"].map((entry) => (
                  <option key={entry} value={entry}>
                    {entry[0].toUpperCase() + entry.slice(1)}
                  </option>
                ))}
              </select>
            </div>
          </details>
        </div>
        {mode === "attention" && !!proposals.data?.length && <a className="queue-review-link" href="#pending-proposals" onClick={event => { event.preventDefault(); setProposalReview(true); }} aria-label={`${proposals.data.length} proposal${proposals.data.length === 1 ? "" : "s"} to review`}>
          <DescriptionOutlined fontSize="small" />
          <span><strong>{proposals.data.length} proposal{proposals.data.length === 1 ? "" : "s"} to review</strong><small>Monitoring changes</small></span>
          <span className="queue-review-button">Review</span>
        </a>}
        {search && <p className="queue-search-label">Results for “{search}”</p>}
        {items.isError && (
          <Alert severity="error">
            Could not load items. Check the local server.
          </Alert>
        )}
        {items.isPending && <p role="status">Loading items…</p>}
        <div className="queue-items">
          {grouped ? groups.map(group => <details className="queue-group" key={group.label} open>
            <summary><span>{group.label}</span><span className="queue-group-count" aria-label={`${group.items.length} loaded items`}>{group.items.length}</span><ExpandLess fontSize="small" /></summary>
            {group.items.map(item => <QueueRow key={item.id} item={item} mode={mode} selected={activeId === item.id} select={() => setSelected(item.id)} grouped timezone={settings.data?.timezone} draft={drafts[item.id] !== undefined} />)}
          </details>) : filtered.map(item => <QueueRow key={item.id} item={item} mode={mode} selected={activeId === item.id} select={() => setSelected(item.id)} timezone={settings.data?.timezone} draft={drafts[item.id] !== undefined} />)}
        </div>
        {items.hasNextPage && (
          <Button
            disabled={items.isFetchingNextPage}
            onClick={() => items.fetchNextPage()}
          >
            {items.isFetchingNextPage ? "Loading…" : "Load more items"}
          </Button>
        )}
        {mode === "attention" &&
          status.data &&
          !status.data.health.setup.done && (
            <Alert severity="info">
              <strong>Setup needs configuration.</strong>{" "}
              {!status.data.health.setup.user_context_set && (
                <>
                  Add your priorities in{" "}
                  <NavLink to="/preferences">Preferences</NavLink>.{" "}
                </>
              )}
              {!status.data.health.setup.has_interest && (
                <>
                  Add an active Interest in{" "}
                  <NavLink to="/interests">Configure Monitoring</NavLink>.{" "}
                </>
              )}
              {!status.data.health.setup.has_watcher && (
                <>
                  Add an active Watcher that applies to an active Interest in{" "}
                  <NavLink to="/interests">Set up a Watcher</NavLink>.{" "}
                </>
              )}
              Configuration does not start an inspection.
            </Alert>
          )}
        {mode === "attention" &&
          status.data?.health.setup.done &&
          !status.data.health.watches.some(
            (watch) => watch.last_success_at,
          ) && (
            <Alert severity="info">
              Setup done; inspection not yet verified. Connect an external agent
              and inspect a Watcher to see coverage.
            </Alert>
          )}
        {!items.isPending && filtered.length === 0 && (
          <div className="ledger-empty-state">
            {mode === "attention"
              ? status.isPending
                ? "Checking setup and inspection…"
                : !status.data
                  ? "Could not check setup and inspection status."
                  : !status.data.health.setup.done
                    ? "No Items yet. Setup still needs the pieces shown above."
                    : !status.data.health.last_run
                      ? "No inspection results yet."
                      : status.data.health.last_run.status === "success"
                        ? "No Items need attention from reported results."
                        : "No Items are listed. Check Activity and Monitoring for inspection coverage."
              : "No matching items."}
          </div>
        )}
      </div>
      <Dialog open={mode === "attention" && proposalReview} onClose={() => setProposalReview(false)} fullWidth maxWidth="sm" aria-labelledby="proposal-queue-title">
        <DialogTitle id="proposal-queue-title">Proposals to review</DialogTitle>
        <DialogContent className="proposal-review-content">
          {proposals.data?.length ? proposals.data.map(proposal => <ProposalCard key={proposal.id} proposal={proposal} candidates={proposals.data} />) : <p>No proposals need review.</p>}
        </DialogContent>
        <DialogActions><Button onClick={() => setProposalReview(false)}>Close</Button></DialogActions>
      </Dialog>
      {activeId ? (
        <ItemReader
          key={activeId}
          itemId={activeId}
          close={() => setSelected("")}
          interests={interests.data ?? []}
          note={drafts[activeId] ?? null}
          setNote={(value) =>
            setDrafts((current) => ({ ...current, [activeId]: value }))
          }
          savedNote={(value) =>
            setDrafts((current) => {
              if (current[activeId] !== value) return current;
              const next = { ...current };
              delete next[activeId];
              return next;
            })
          }
        />
      ) : (
        <div className="reader-empty">
          <h2>
            {filtered.length ? "Select an item to read" : "Your reading space"}
          </h2>
          <p>Findings, evidence, and follow-up stay together here.</p>
        </div>
      )}
    </div>
  );
}

function queueGroup(item: Item, mode:WorkspaceMode = "attention") {
  if (item.remind_at && new Date(item.remind_at) <= new Date()) return "Reminder due";
  if (item.todo_state === "todo") return mode === "todo" ? "To do" : "Todos";
  return "New evidence";
}

function QueueRow({item, mode, selected, select, timezone, draft, grouped = false}: {item:Item; mode:WorkspaceMode; selected:boolean; select:()=>void; timezone?:string; draft:boolean; grouped?:boolean}) {
  const due = queueGroup(item) === "Reminder due";
  const rowStatus = [item.todo_state === "done" ? "Done" : null, ...attentionReasons(item)].filter(Boolean).join(" · ") || "Seen";
  const rowDate = new Date(due ? item.remind_at! : item.content_updated_at);
  const day = due && dateInputValue(rowDate, timezone) === dateInputValue(new Date(), timezone) ? "Today" : new Intl.DateTimeFormat(undefined, {month:"short",day:"numeric",timeZone:effectiveTimezone(timezone)}).format(rowDate);
  return <div className={"queue-entry" + (selected ? " selected" : "") + (due ? " is-due" : "")}>
    <button className={"queue-row" + (selected ? " selected" : "")} onClick={select} aria-pressed={selected}>
      <span className="queue-item-top"><strong>{due && <Schedule fontSize="small" />}{item.title}</strong></span>
      <span className="queue-item-date" title={formatDateTime(rowDate.toISOString(), timezone)}>{day}{due && <><br />{new Intl.DateTimeFormat(undefined, {timeStyle:"short",timeZone:effectiveTimezone(timezone)}).format(rowDate)}</>}</span>
      <span className="queue-item-summary">{item.summary}</span>
      {draft && <span className="queue-item-meta">Unsaved note</span>}
      {!grouped && <span className="queue-item-meta">{mode === "library" && `${item.kind[0].toUpperCase()}${item.kind.slice(1)} · `}{rowStatus}</span>}
      {item.remind_at && !due && <span className="queue-item-meta" title={formatDateTime(item.remind_at, timezone)}>Reminder {new Intl.DateTimeFormat(undefined, {month:"short",day:"numeric",timeZone:effectiveTimezone(timezone)}).format(new Date(item.remind_at))}</span>}
    </button>
    <QueueActions item={item} mode={mode} selected={selected} />
  </div>;
}

function QueueActions({ item, mode, selected }: { item: Item; mode:WorkspaceMode; selected:boolean }) {
  const [reminder, setReminder] = useState(false);
  const [menu, setMenu] = useState<HTMLElement | null>(null);
  const { mutation, apply } = useItemAction(item.id, () => setReminder(false));
  const acknowledge = () => { setMenu(null); apply(item, {type:"acknowledge", content_version:item.content_version}); };
  const remind = () => { setMenu(null); mutation.reset(); setReminder(true); };
  const complete = () => { setMenu(null); apply(item, {type:"set_todo", state:"done"}); };
  return <div className="queue-actions">
    <IconButton className="queue-more" size="small" aria-label={`Actions for ${item.title}`} aria-haspopup="menu" aria-expanded={!!menu} disabled={mutation.isPending} onClick={event => setMenu(event.currentTarget)}><MoreVert fontSize="small" /></IconButton>
    {selected && <>
      {mode === "todo" ? <Button size="small" variant="contained" startIcon={<Check fontSize="small" />} aria-label={`Mark done ${item.title}`} disabled={mutation.isPending} onClick={complete}>Mark Done</Button> : item.acknowledged_content_version < item.content_version && <Button size="small" variant="contained" startIcon={<VisibilityOutlined fontSize="small" />} aria-label={`Mark seen ${item.title}`} disabled={mutation.isPending} onClick={acknowledge}>Mark seen</Button>}
      <Button size="small" variant="outlined" startIcon={<Schedule fontSize="small" />} aria-label={`Remind me about ${item.title}`} disabled={mutation.isPending} onClick={remind}>Remind</Button>
    </>}
    <Menu anchorEl={menu} open={!!menu} onClose={() => setMenu(null)} anchorOrigin={{vertical:"bottom",horizontal:"right"}} transformOrigin={{vertical:"top",horizontal:"right"}} slotProps={{paper:{className:"queue-action-menu"}}}>
      {mode === "todo" && <MenuItem onClick={complete}><Check fontSize="small" /> Mark Done</MenuItem>}
      {item.acknowledged_content_version < item.content_version && <MenuItem onClick={acknowledge}><VisibilityOutlined fontSize="small" /> Mark seen</MenuItem>}
      <MenuItem onClick={remind}><Schedule fontSize="small" /> {item.remind_at ? "Change reminder" : "Remind later"}</MenuItem>
    </Menu>
    {mutation.isError && !reminder && <Alert severity="error">{mutation.error.message}</Alert>}
    {reminder && <ReminderDialog contentVersion={item.content_version} close={() => setReminder(false)} apply={action => apply(item, action)} error={mutation.error} pending={mutation.isPending} />}
  </div>;
}
