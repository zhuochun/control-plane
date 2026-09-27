import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, Button, Dialog, DialogActions, DialogContent, DialogTitle, MenuItem, TextField, useMediaQuery } from "@mui/material";
import { NavLink } from "react-router";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { api, collection } from "./api";
import { type Item, parseMetric, ProposalCard, ReminderDialog, useItemAction } from "./items";
import { formatDateTime, useSettings } from "./settings";
import type { ServiceStatus } from "./health";

type Interest = { id: string; title: string };
type Proposal = { id: string; proposal_key:string; target_type: string; operation: string; rationale_md: string; state: string; evidence_links?:string[];confidence?:number;payload?:object };

function stateOf(item: Item) {
  if (item.remind_at && new Date(item.remind_at) <= new Date()) return "Due";
  if (item.acknowledged_content_version < item.content_version) return "New";
  return item.todo_state === "todo" ? "Todo" : "Seen";
}

function findingsExcerpt(item: Item) {
  const body = item.report.body_md;
  const interpretation = body.match(/###? Interpretation\s+([\s\S]*?)(?=\n#{2,3} |$)/i)?.[1];
  if (interpretation) return interpretation.trim();
  const withoutTablesAndCode = body
    .replace(/```[\s\S]*?```/g, "")
    .split("\n")
    .filter((line) => line.trim() && !line.startsWith("#") && !line.startsWith("|"))
    .join("\n");
  return withoutTablesAndCode.trim().slice(0, 650) || item.summary;
}

function NewItemDialog({close}:{close:()=>void}){
  const cache=useQueryClient();
  const identity=useRef(crypto.randomUUID());
  const attempt=useRef<{signature:string;requestId:string}|null>(null);
  const [kind,setKind]=useState("task");
  const [title,setTitle]=useState("");
  const [summary,setSummary]=useState("");
  const [body,setBody]=useState("");
  const [sourceDate,setSourceDate]=useState("");
  const create=useMutation({mutationFn:()=>{
    const signature=JSON.stringify({kind,title,summary,body,sourceDate});
    if(attempt.current?.signature!==signature)attempt.current={signature,requestId:crypto.randomUUID()};
    return api<Item>("/items",{
    request_id:attempt.current.requestId,dedupe_key:`user:${identity.current}`,kind,title,summary,
    sources:sourceDate?[{id:"user-date",label:"Source date",source_date:sourceDate}]:[],
    report:{schema_version:1,body_md:body},
  },"POST")},onSuccess:async()=>{await Promise.all([cache.invalidateQueries({queryKey:["items"]}),cache.invalidateQueries({queryKey:["status"]})]);close()}});
  return <Dialog open onClose={close} fullWidth maxWidth="sm"><form onSubmit={(event)=>{event.preventDefault();create.mutate()}}>
    <DialogTitle>New Item</DialogTitle><DialogContent><div className="editor-fields">
      <p>Capture your own task or note. A source link is optional.</p>
      <TextField select label="Kind" value={kind} onChange={(event)=>setKind(event.target.value)}>{["task","note","report","outcome"].map((value)=><MenuItem key={value} value={value}>{value}</MenuItem>)}</TextField>
      <TextField required label="Title" value={title} onChange={(event)=>setTitle(event.target.value)}/>
      <TextField required label="Summary" value={summary} onChange={(event)=>setSummary(event.target.value)}/>
      <TextField multiline minRows={3} label="Details" value={body} onChange={(event)=>setBody(event.target.value)}/>
      <TextField label="Source date (optional)" type="date" value={sourceDate} onChange={(event)=>setSourceDate(event.target.value)} slotProps={{inputLabel:{shrink:true}}}/>
      {create.isError&&<Alert severity="error">{create.error.message}</Alert>}
    </div></DialogContent><DialogActions><Button onClick={close}>Cancel</Button><Button type="submit" variant="contained" disabled={create.isPending}>Create</Button></DialogActions>
  </form></Dialog>
}

function Trend({ item }: { item: Item }) {
  const metric = parseMetric(item);
  if (!metric) return <span className="ledger-empty">—</span>;
  return (
    <span className="ledger-trend" aria-label={`${metric.current.display}; ${Math.round(metric.changePercent)} percent versus previous`}>
      {metric.series.slice(-7).map((point, index) => {
        const values = metric.series.slice(-7).map((part) => part.value);
        const min = Math.min(...values);
        const max = Math.max(...values);
        const height = 8 + (28 * (point.value - min)) / (max - min || 1);
        return <i key={index} style={{ height }} />;
      })}
    </span>
  );
}

function Inspector({ itemId, close }: { itemId: string; close: () => void }) {
  const cache = useQueryClient();
  const settings = useSettings();
  const query = useQuery({ queryKey: ["item", itemId], queryFn: () => api<Item>(`/items/${itemId}`) });
  const [reminder, setReminder] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const noteRequest = useRef<{ signature: string; id: string } | null>(null);
  const { apply, mutation } = useItemAction(itemId, () => setReminder(false));
  const saveNote = useMutation({
    mutationFn: (input: { item: Item; value: string }) => {
      const signature = JSON.stringify({ version: input.item.state_version, note: input.value });
      if (noteRequest.current?.signature !== signature) noteRequest.current = { signature, id: crypto.randomUUID() };
      return api<Item>(`/items/${itemId}/note`, {
        request_id: noteRequest.current.id,
        expected_state_version: input.item.state_version,
        user_note: input.value,
      }, "PUT");
    },
    onSuccess: (updated) => {
      cache.setQueryData(["item", updated.id], updated);
      cache.invalidateQueries({ queryKey: ["items"] });
      setNote(null);
    },
  });
  useEffect(() => setNote(null), [itemId]);
  const item = query.data;
  const metric = item ? parseMetric(item) : null;
  return (
    <aside className="workspace-inspector" aria-label="Item details">
      <div className="inspector-top"><span>Item detail</span><button onClick={close} aria-label="Close item detail">×</button></div>
      {query.isPending && <p role="status">Opening item…</p>}
      {query.isError && <Alert severity="error">Could not open this item.</Alert>}
      {item && <>
        <div className="inspector-head">
          <span className={`tag kind kind-${item.kind}`}>{item.kind}</span>
          <span className={`tag ${stateOf(item).toLowerCase()}`}>{stateOf(item)}</span>
          <NavLink to={`/items/${item.id}`}>Full detail ↗</NavLink>
        </div>
        <h2>{item.title}</h2>
        <p className="inspector-summary">{item.summary}</p>
        {metric && <div className="inspector-metric">
          <div><span>Latest</span><strong>{metric.current.display}</strong></div>
          {metric.target && <div><span>Target</span><strong>{metric.target.display}</strong></div>}
          <div><span>vs previous</span><strong>{metric.changePercent > 0 ? "+" : ""}{Math.round(metric.changePercent)}%</strong></div>
          <div className="inspector-metric-trend"><Trend item={item} /></div>
        </div>}
        <div className="inspector-block"><h3>Findings</h3>
          {item.report.schema_version === 1 ? <div className="inspector-markdown"><ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>{findingsExcerpt(item)}</ReactMarkdown></div> : <p>Report format unavailable. The summary and sources are still shown.</p>}
        </div>
        <div className="inspector-block"><h3>Sources</h3>
          {item.sources.length ? item.sources.map((source) => source.url ? <a key={source.id} href={source.url} target="_blank" rel="noopener noreferrer" className="inspector-source">{source.label || source.url} ↗{source.observed_at && <small>Observed {formatDateTime(source.observed_at, settings.data?.timezone)}</small>}</a> : <span key={source.id} className="inspector-source">{source.label || "Source date"} · {source.source_date}</span>) : <p>No source attached.</p>}
        </div>
        <div className="inspector-block"><h3>Actions</h3>
          <div className="inspector-actions">
            {item.acknowledged_content_version < item.content_version && <Button variant="contained" disabled={mutation.isPending} onClick={() => apply(item, { type: "acknowledge", content_version: item.content_version })}>Acknowledge</Button>}
            <Button variant="outlined" disabled={mutation.isPending} onClick={() => apply(item, { type: "set_todo", state: item.todo_state === "todo" ? "done" : "todo" })}>{item.todo_state === "todo" ? "Mark done" : item.todo_state === "done" ? "Reopen Todo" : "Add Todo"}</Button>
            <Button variant="outlined" disabled={mutation.isPending} onClick={() => setReminder(true)}>{item.remind_at ? "Change reminder" : "Remind"}</Button>
          </div>
          {item.remind_at && <p className="inspector-reminder">Reminder: {formatDateTime(item.remind_at, settings.data?.timezone)}</p>}
          {mutation.isError && <Alert severity="error">{mutation.error.message}</Alert>}
        </div>
        <div className="inspector-block"><h3>Your note</h3>
          <TextField multiline minRows={3} fullWidth placeholder="Keep context for the next run…" value={note ?? item.user_note} onChange={(event) => setNote(event.target.value)} />
          <Button size="small" disabled={note === null || saveNote.isPending} onClick={() => note !== null && saveNote.mutate({ item, value: note })}>Save note</Button>
          {saveNote.isError && <Alert severity="error">{saveNote.error.message}</Alert>}
        </div>
        <div className="inspector-updated">Updated {formatDateTime(item.content_updated_at, settings.data?.timezone)}</div>
        {reminder && <ReminderDialog close={() => setReminder(false)} apply={(action) => apply(item, action)} error={mutation.error} />}
      </>}
    </aside>
  );
}

export function ItemWorkspace({ mode }: { mode: "attention" | "library" | "todo" }) {
  const settings = useSettings();
  const compact = useMediaQuery("(max-width:720px)");
  const [search, setSearch] = useState("");
  const [kind, setKind] = useState("");
  const [interest, setInterest] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [newItem,setNewItem]=useState(false);
  const view = mode === "library" ? "all" : mode;
  const items = useQuery({ queryKey: ["items", "workspace", view], queryFn: () => collection<Item>(`/items?view=${view}`) });
  const interests = useQuery({ queryKey: ["interests"], queryFn: () => collection<Interest>("/interests") });
  const status = useQuery({ queryKey: ["status"], queryFn: () => api<ServiceStatus>("/status"), enabled: mode === "attention" });
  const proposals = useQuery({ queryKey: ["proposals", "pending"], queryFn: () => collection<Proposal>("/proposals?state=pending"), enabled: mode === "attention" });
  const interestNames = new Map((interests.data ?? []).map((entry) => [entry.id, entry.title]));
  const filtered = (items.data ?? []).filter((item) =>
    (!kind || item.kind === kind) && (!interest || item.interests?.some((reason)=>reason.id===interest)) &&
    (!search.trim() || `${item.title} ${item.summary} ${item.interests?.map((reason)=>interestNames.get(reason.id) ?? "").join(" ") ?? ""}`.toLowerCase().includes(search.trim().toLowerCase()))
  );
  const due = filtered.filter((item) => stateOf(item) === "Due");
  const rest = filtered.filter((item) => stateOf(item) !== "Due");
  const groups = mode === "attention" ? [["Due", due], [rest.every((item) => stateOf(item) === "New") ? "New" : "New & Todo", rest]] as const : [[mode === "todo" ? "Open Todos" : "All items", filtered]] as const;
  const activeId = selected === "" ? null : selected && filtered.some((item) => item.id === selected) ? selected : compact ? null : filtered[0]?.id;
  return <div className={`item-workspace${activeId ? "" : " inspector-closed"}`}>
    <div className="workspace-list">
      <div className="workspace-toolbar">
        <div className="workspace-title"><h1>{mode === "attention" ? "Attention" : mode === "todo" ? "Todos" : "All items"}</h1><span className="workspace-count">{filtered.length}</span><Button size="small" onClick={()=>setNewItem(true)}>New Item</Button></div>
        <div className="workspace-totals"><span>Due <strong>{due.length}</strong></span><span>New <strong>{filtered.filter((item) => stateOf(item) === "New").length}</strong></span></div>
      </div>
      <div className="workspace-filters">
        <TextField size="small" placeholder="Search items, metrics, or notes…" aria-label="Search items" value={search} onChange={(event) => setSearch(event.target.value)} />
        <select aria-label="Filter by Interest" value={interest} onChange={(event) => setInterest(event.target.value)}><option value="">All interests</option>{interests.data?.map((entry) => <option key={entry.id} value={entry.id}>{entry.title}</option>)}</select>
        <select aria-label="Filter by kind" value={kind} onChange={(event) => setKind(event.target.value)}><option value="">All kinds</option>{["report", "outcome", "note", "task"].map((entry) => <option key={entry} value={entry}>{entry[0].toUpperCase() + entry.slice(1)}</option>)}</select>
      </div>
      {items.isError && <Alert severity="error">Could not load items. Check the local server.</Alert>}
      {items.isPending && <p role="status">Loading items…</p>}
      <div className="ledger-head"><span>Item</span><span>Kind</span><span>Interest</span><span>Value / target</span><span>Trend</span><span>Updated</span><span>State</span></div>
      {groups.map(([label, items]) => items.length > 0 && <section className="ledger-group" key={label} aria-label={`${label} items`}>
        <h2>{label} <span>{items.length}</span></h2>
        {items.map((item) => {
          const metric = parseMetric(item);
          return <button className={`ledger-row${activeId === item.id ? " selected" : ""}`} key={item.id} onClick={() => setSelected(item.id)} aria-pressed={activeId === item.id}>
            <span className="ledger-name"><strong>{item.title}</strong><small>{item.summary}</small></span>
            <span className={`tag kind kind-${item.kind}`}>{item.kind}</span>
            <span className="ledger-interest">{item.interests?.map((reason)=>interestNames.get(reason.id) ?? reason.id).join(", ") || "—"}</span>
            <span className="ledger-value">{metric ? <><strong>{metric.current.display}</strong><small>{metric.target ? `target ${metric.target.display}` : ""}</small></> : "—"}</span>
            <Trend item={item} />
            <span className="ledger-updated">{formatDateTime(item.content_updated_at, settings.data?.timezone)}</span>
            <span className={`tag ${stateOf(item).toLowerCase()}`}>{stateOf(item)}</span>
          </button>;
        })}
      </section>)}
      {mode === "attention" && status.data && !status.data.health.setup.done && <Alert severity="info">
        <strong>Setup needs configuration.</strong> {!status.data.health.setup.user_context_set && <>Add your priorities in <NavLink to="/preferences">Preferences</NavLink>. </>}
        {!status.data.health.setup.has_interest && <>Add an active Interest in <NavLink to="/interests">Configure Monitoring</NavLink>. </>}
        {!status.data.health.setup.has_watcher && <>Add an active Watcher that applies to an active Interest in <NavLink to="/interests">Set up a Watcher</NavLink>. </>}
        Configuration does not start an inspection.
      </Alert>}
      {mode === "attention" && status.data?.health.setup.done && !status.data.health.watches.some((watch) => watch.last_success_at) && <Alert severity="info">Setup done; inspection not yet verified. Connect an external agent and inspect a Watcher to see coverage.</Alert>}
      {!items.isPending && filtered.length === 0 && <div className="ledger-empty-state">{mode === "attention" ? status.isPending ? "Checking setup and inspection…" : !status.data ? "Could not check setup and inspection status." : !status.data.health.setup.done ? "No Items yet. Setup still needs the pieces shown above." : !status.data.health.last_run ? "No inspection results yet." : status.data.health.last_run.status === "success" ? "No Items need attention from reported results." : "No Items are listed. Check Activity and Monitoring for inspection coverage." : "No matching items."}</div>}
      {mode === "attention" && (proposals.data?.length ?? 0) > 0 && <section className="ledger-proposals"><h2>Proposals to review <span>{proposals.data?.length}</span></h2>{proposals.data?.map((proposal) => <ProposalCard key={proposal.id} proposal={proposal} candidates={proposals.data} />)}</section>}
    </div>
    {activeId && <Inspector key={activeId} itemId={activeId} close={() => setSelected("")} />}
    {newItem&&<NewItemDialog close={()=>setNewItem(false)}/>}
  </div>;
}
