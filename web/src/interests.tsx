import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  FormGroup,
  MenuItem,
  TextField,
} from "@mui/material";
import { api, collection } from "./api";
import { NavLink } from "react-router";
import type { ServiceStatus } from "./health";
import { formatDateTime, useSettings } from "./settings";

type Interest = {
  id: string;
  slug: string;
  title: string;
  instructions_md: string;
  state: string;
  revision: number;
};
type Watch = {
  id: string;
  slug: string;
  interest_ids: string[];
  matching_policy: "broad" | "explicit";
  valid_until?: string;
  source: { kind: string; locator: string };
  instructions_md: string;
  interval_seconds: number;
  lookback_seconds: number;
  state: string;
  revision: number;
  cursor?: unknown;
  next_due_at:string;
};
type Editor =
  | { kind: "interest"; item?: Interest }
  | { kind: "watch"; item?: Watch };

type PlanPreview = {
  preview_token:string;
  due_before:number;
  due_after:number;
  changes:{target_type:string;operation:string;target:string;affected_items:number;affected_watchers?:string[];cursor_reset:boolean;consequence?:string;overlapping_watchers?:string[]}[];
};

function ConfigurationPlanDialog({close}:{close:()=>void}){
  const cache=useQueryClient();
  const [draft,setDraft]=useState('{"operations": []}');
  const [parseError,setParseError]=useState("");
  const [reviewed,setReviewed]=useState<PlanPreview|null>(null);
  const requestID=useRef(crypto.randomUUID());
  const preview=useMutation({mutationFn:(plan:object)=>api<PlanPreview>("/config/plans/preview",plan,"POST"),onSuccess:(value)=>setReviewed(value)});
  const apply=useMutation({mutationFn:(plan:object)=>api("/config/plans/apply",{plan,preview_token:reviewed?.preview_token,request_id:requestID.current},"POST"),onSuccess:async()=>{
    await Promise.all([cache.invalidateQueries({queryKey:["interests"]}),cache.invalidateQueries({queryKey:["watches"]}),cache.invalidateQueries({queryKey:["status"]})]);close();
  }});
  function parsed():object|null{try{setParseError("");return JSON.parse(draft)}catch{setParseError("Enter a valid JSON change set.");return null}}
  return <Dialog open onClose={close} fullWidth maxWidth="md"><DialogTitle>Review configuration change set</DialogTitle><DialogContent>
    <p>Describe ordered Interest and Watcher creates, updates, and deprecations. A preview changes no records.</p>
    <TextField fullWidth multiline minRows={10} label="Change set JSON" value={draft} onChange={(event)=>{setDraft(event.target.value);setReviewed(null);requestID.current=crypto.randomUUID()}} />
    {parseError&&<Alert severity="error">{parseError}</Alert>}
    {preview.isError&&<Alert severity="error">{preview.error.message}</Alert>}
    {apply.isError&&<Alert severity="error">{apply.error.message} Preview again if configuration changed.</Alert>}
    {reviewed&&<div className="panel">
      <p>Due Watchers: {reviewed.due_before} → {reviewed.due_after}. Interest assessment changes apply only to future Runs.</p>
      {reviewed.changes.map((change,index)=><p key={index}>{change.operation} {change.target_type} <strong>{change.target}</strong>
        {change.affected_items>0&&` · ${change.affected_items} existing Items`}
        {!!change.affected_watchers?.length&&` · affects ${change.affected_watchers.join(", ")}`}
        {change.cursor_reset&&" · source cursor resets"}
        {!!change.overlapping_watchers?.length&&` · overlaps ${change.overlapping_watchers.join(", ")}`}
        {change.consequence&&<small>{change.consequence}</small>}
      </p>)}
    </div>}
  </DialogContent><DialogActions><Button onClick={close}>Cancel</Button><Button onClick={()=>{const plan=parsed();if(plan)preview.mutate(plan)}} disabled={preview.isPending}>Preview</Button><Button variant="contained" disabled={!reviewed||apply.isPending} onClick={()=>{const plan=parsed();if(plan)apply.mutate(plan)}}>Apply reviewed changes</Button></DialogActions></Dialog>;
}

function ConfigurationEditor({
  editor,
  interests,
  close,
}: {
  editor: Editor;
  interests: Interest[];
  close: () => void;
}) {
  const cache = useQueryClient();
  const item = editor.item;
  const watch = editor.kind === "watch" ? editor.item : undefined;
  const [title, setTitle] = useState(
    editor.kind === "interest" ? (editor.item?.title ?? "") : "",
  );
  const [slug,setSlug]=useState(item?.slug ?? "");
  const [policy,setPolicy]=useState<"broad"|"explicit">(watch?.matching_policy ?? "broad");
  const [interestIDs,setInterestIDs]=useState<string[]>(watch?.interest_ids ?? []);
  const [validUntil,setValidUntil]=useState(watch?.valid_until?.slice(0,16) ?? "");
  const [instructions, setInstructions] = useState(item?.instructions_md ?? "");
  const [state, setState] = useState(item?.state ?? "active");
  const [kind, setKind] = useState(watch?.source.kind ?? "");
  const [locator, setLocator] = useState(watch?.source.locator ?? "");
  const [interval, setInterval] = useState(
    String((watch?.interval_seconds ?? 7200) / 60),
  );
  const [lookback, setLookback] = useState(
    String((watch?.lookback_seconds ?? 604800) / 86400),
  );
  const lastAttempt = useRef<{ payload: string; requestId: string } | null>(
    null,
  );
  const save = useMutation({
    mutationFn: (body: object) =>
      api(
        `/${editor.kind === "interest" ? "interests" : "watches"}${item ? `/${item.id}` : ""}`,
        body,
        item ? "PATCH" : "POST",
      ),
    onSuccess: async () => {
      await Promise.all([
        cache.invalidateQueries({ queryKey: ["interests"] }),
        cache.invalidateQueries({ queryKey: ["watches"] }),
      ]);
      close();
    },
  });
  function submit() {
    const common = {
      slug,
      instructions_md: instructions,
      state,
      ...(item ? { expected_revision: item.revision } : {}),
    };
    const body =
      editor.kind === "interest"
        ? { ...common, title }
        : {
            ...common,
            matching_policy: policy,
            interest_ids: policy==="explicit" ? interestIDs : [],
            ...(validUntil ? {valid_until:new Date(validUntil).toISOString()} : watch?.valid_until ? {clear_valid_until:true} : {}),
            source: { kind, locator },
            interval_seconds: Math.round(Number(interval) * 60),
            lookback_seconds: Math.round(Number(lookback) * 86400),
          };
    const payload = JSON.stringify(body);
    if (lastAttempt.current?.payload !== payload)
      lastAttempt.current = { payload, requestId: crypto.randomUUID() };
    save.mutate({ ...body, request_id: lastAttempt.current.requestId });
  }
  return (
    <Dialog
      open
      onClose={() => {
        if (!save.isPending) close();
      }}
      fullWidth
      maxWidth="sm"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <DialogTitle>
          {item ? "Edit" : "New"}{" "}
          {editor.kind === "interest" ? "Interest" : "Watcher"}
        </DialogTitle>
        <DialogContent>
          <div className="editor-fields">
            <p>
              {editor.kind === "interest"
                ? "Tell your agent what matters and what a useful finding looks like."
                : "Define one bounded input source. Choose whether future runs assess all active Interests or only selected ones."}
            </p>
            <TextField required label="Slug" helperText="Stable link name; lowercase letters, digits, and hyphens." value={slug} onChange={(e)=>setSlug(e.target.value)} />
            {editor.kind === "interest" ? (
              <TextField
                autoFocus
                required
                label="Title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
              />
            ) : (
              <>
                <TextField
                  autoFocus
                  required
                  label="Source kind"
                  placeholder="github, slack, web…"
                  value={kind}
                  onChange={(e) => setKind(e.target.value)}
                />
                <TextField
                  required
                  label="Source location"
                  placeholder="URL, repository, or channel identifier"
                  value={locator}
                  onChange={(e) => setLocator(e.target.value)}
                />
              </>
            )}
            <TextField
              multiline
              minRows={4}
              label="Instructions"
              helperText="Plain text or Markdown. Be as specific as you need."
              value={instructions}
              onChange={(e) => setInstructions(e.target.value)}
            />
            {editor.kind === "watch" && (
              <>
              <TextField select label="Interest matching" value={policy} onChange={(e)=>setPolicy(e.target.value as "broad"|"explicit")}>
                <MenuItem value="broad">Broad: assess all active Interests</MenuItem>
                <MenuItem value="explicit">Explicit: assess only selected Interests</MenuItem>
              </TextField>
              {policy==="explicit" && <FormGroup>
                {interests.filter((interest)=>interest.state==="active"||interestIDs.includes(interest.id)).map((interest)=><FormControlLabel key={interest.id} label={interest.title} control={<Checkbox checked={interestIDs.includes(interest.id)} onChange={(e)=>setInterestIDs((current)=>e.target.checked?[...current,interest.id]:current.filter((id)=>id!==interest.id))}/>} />)}
                {interestIDs.length===0 && <small>No active Interest selected. This Watcher will not be scheduled.</small>}
              </FormGroup>}
              <TextField label="Valid until (optional)" type="datetime-local" value={validUntil} onChange={(e)=>setValidUntil(e.target.value)} slotProps={{inputLabel:{shrink:true}}} />
              <div className="field-pair">
                <TextField
                  required
                  type="number"
                  label="Check every (minutes)"
                  value={interval}
                  onChange={(e) => setInterval(e.target.value)}
                  slotProps={{
                    htmlInput: { min: 1 / 60, max: 525600, step: "any" },
                  }}
                />
                <TextField
                  required
                  type="number"
                  label="First lookback (days)"
                  value={lookback}
                  onChange={(e) => setLookback(e.target.value)}
                  slotProps={{ htmlInput: { min: 1 / 86400, step: "any" } }}
                />
              </div>
              </>
            )}
            <TextField
              select
              label="State"
              value={state}
              onChange={(e) => setState(e.target.value)}
            >
              {["active", "paused", "deprecated"].map((value) => (
                <MenuItem key={value} value={value}>
                  {value[0].toUpperCase() + value.slice(1)}
                </MenuItem>
              ))}
            </TextField>
            {save.isError && (
              <Alert severity="error">
                {save.error.message} Your draft is still here.
              </Alert>
            )}
          </div>
        </DialogContent>
        <DialogActions>
          <Button onClick={close} disabled={save.isPending}>
            Cancel
          </Button>
          <Button type="submit" variant="contained" disabled={save.isPending}>
            {save.isPending ? "Saving…" : "Save"}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

export function Interests() {
	const settings = useSettings();
	const status = useQuery({ queryKey: ["status"], queryFn: () => api<ServiceStatus>("/status") });
  const interests = useQuery({
    queryKey: ["interests"],
    queryFn: () => collection<Interest>("/interests"),
  });
  const watches = useQuery({
    queryKey: ["watches"],
    queryFn: () => collection<Watch>("/watches"),
  });
  const items = useQuery({queryKey:["items","monitoring"],queryFn:()=>collection<{id:string;title:string;watch_id?:string;interests:{id:string;reason:string}[]}>("/items?view=all")});
  const [editor, setEditor] = useState<Editor | null>(null);
  const [planOpen,setPlanOpen]=useState(false);
  return (
    <>
      <header className="page-header">
        <div className="eyebrow">Give your attention a direction</div>
        <div className="section-heading">
          <h1>Interests</h1>
          <Button
            variant="contained"
            onClick={() => setEditor({ kind: "interest" })}
          >
            New Interest
          </Button>
        </div>
        <p>Interests explain relevance. Watchers define the sources to inspect.</p>
      </header>
      {(interests.isError || watches.isError) && (
        <Alert severity="error">
          Could not load your Interests and Watches. Check the local server.
        </Alert>
      )}
      {interests.isPending && <p role="status">Loading Interests…</p>}
      {interests.data?.length === 0 && (
        <section className="panel empty">
          <div className="empty-mark" aria-hidden="true">
            ✳
          </div>
          <h2>Start with something you care about</h2>
          <p>
            Add an Interest to tell the agent what matters. Configure Watchers independently below.
          </p>
          <Button
            variant="outlined"
            onClick={() => setEditor({ kind: "interest" })}
          >
            Create your first Interest
          </Button>
        </section>
      )}
      <div className="interest-list">
        {interests.data?.map((interest) => (
          <section className="panel" key={interest.id} id={`interest-${interest.slug}`}>
            <div className="section-heading">
              <div className="title-with-state">
                <h2>{interest.title}</h2>
                <span className="tag">{interest.state}</span>
              </div>
              <Button
                size="small"
                onClick={() => setEditor({ kind: "interest", item: interest })}
              >
                Edit Interest
              </Button>
            </div>
            <p className="instructions-preview">
              {interest.instructions_md || "No instructions yet."}
            </p>
            <small>#{interest.slug} · {watches.data?.filter((watch)=>watch.matching_policy==="broad"||watch.interest_ids.includes(interest.id)).length ?? 0} applicable Watchers</small>
            {!watches.isPending && !watches.isError && !watches.data?.some((watch)=>watch.state==="active" && (!watch.valid_until || new Date(watch.valid_until)>new Date()) && (watch.matching_policy==="broad"||watch.interest_ids.includes(interest.id))) && <small>Coverage gap: no active Watcher currently assesses this Interest.</small>}
            <small>Changes to this Interest apply from the next Run; earlier source input is not reassessed.</small>
            <div>{watches.data?.filter((watch)=>watch.matching_policy==="broad"||watch.interest_ids.includes(interest.id)).map((watch)=><NavLink key={watch.id} to={`#watcher-${watch.slug}`}>{watch.slug} </NavLink>)}</div>
            <div>{items.data?.filter((item)=>item.interests?.some((reason)=>reason.id===interest.id)).slice(0,10).map((item)=><NavLink key={item.id} to={`/items/${item.id}`}>{item.title} </NavLink>)}</div>
          </section>
        ))}
      </div>
      <section className="panel">
        <div className="section-heading">
          <h2>Watchers</h2>
          <Button variant="contained" onClick={()=>setEditor({kind:"watch"})}>New Watcher</Button>
          <Button onClick={()=>setPlanOpen(true)}>Change set</Button>
        </div>
        <p>Each Watcher has its own source scope and checkpoint. Changing Interest links keeps both.</p>
        <div className="watch-list">
          {watches.data?.map((watch)=>{
            const health=status.data?.health.watches.find((entry)=>entry.watch_id===watch.id);
            const names=watch.matching_policy==="broad"?"All active Interests":watch.interest_ids.map((id)=>interests.data?.find((interest)=>interest.id===id)?.title ?? id).join(", ") || "No Interests; inspection paused";
            return <div className="watch-row" key={watch.id} id={`watcher-${watch.slug}`}>
              <div>
                <span className="source-kind">{watch.source.kind}</span>
                <strong>{watch.source.locator}</strong>
                <small>#{watch.slug} · {watch.state} · every {watch.interval_seconds/60} minutes</small>
                <small>{names}</small>
                {watch.valid_until && <small>Valid until {formatDateTime(watch.valid_until,settings.data?.timezone)}</small>}
                <small>{status.isPending?"Loading inspection status…":status.isError?"Inspection status unavailable":health?.last_status?`Last inspection: ${health.last_status}`:"Never inspected"}
                  {health?.due_reason && ` · ${health.due_reason.replaceAll("_"," ")}`}
                  {health?.last_success_at && ` · last success ${formatDateTime(health.last_success_at,settings.data?.timezone)}`}
                  {health?.next_due_at && ` · ${new Date(health.next_due_at)<=new Date()?"Due now":`next due ${formatDateTime(health.next_due_at,settings.data?.timezone)}`}`}
                </small>
                {health?.last_error && <small>Latest error: {health.last_error}</small>}
                <details><summary>Coverage and Items</summary>
                  <small>Cursor: {watch.cursor ? JSON.stringify(watch.cursor) : "none"} · next due {formatDateTime(watch.next_due_at,settings.data?.timezone)}</small>
                  <div>{items.data?.filter((item)=>item.watch_id===watch.id).slice(0,20).map((item)=><NavLink key={item.id} to={`/items/${item.id}`}>{item.title} </NavLink>)}</div>
                </details>
              </div>
              <Button size="small" onClick={()=>setEditor({kind:"watch",item:watch})}>Edit Watcher</Button>
            </div>
          })}
        </div>
      </section>
      {editor && (
        <ConfigurationEditor editor={editor} interests={interests.data ?? []} close={() => setEditor(null)} />
      )}
      {planOpen&&<ConfigurationPlanDialog close={()=>setPlanOpen(false)}/>}
    </>
  );
}
