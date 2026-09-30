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
  TextField,
  useMediaQuery,
} from "@mui/material";
import { NavLink, useSearchParams } from "react-router";
import { api, collection } from "./api";
import { type Item, ProposalCard } from "./items";
import { ItemReader, attentionReasons } from "./item-reader";
import { effectiveTimezone, useSettings } from "./settings";
import type { ServiceStatus } from "./health";

type Interest = { id: string; slug: string; title: string };
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
  mode: "attention" | "library" | "todo";
}) {
  const settings = useSettings();
  const compact = useMediaQuery("(max-width:720px)");
  const [searchParams] = useSearchParams();
  const search = searchParams.get("q") ?? "";
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [kind, setKind] = useState("");
  const [interest, setInterest] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  useEffect(() => setSelected(null), [search, mode]);
  const view = mode === "library" ? "all" : mode;
  const items = useInfiniteQuery({
    queryKey: ["items", "workspace", view, kind, interest, search.trim()],
    initialPageParam: "",
    queryFn: ({ pageParam }) => {
      const query = new URLSearchParams({ view, limit: "100" });
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
          <details className="queue-filters">
            <summary>Filters</summary>
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
        {search && <p className="queue-search-label">Results for “{search}”</p>}
        {items.isError && (
          <Alert severity="error">
            Could not load items. Check the local server.
          </Alert>
        )}
        {items.isPending && <p role="status">Loading items…</p>}
        <div className="queue-items">
          {filtered.map((item) => {
            const reasons = attentionReasons(item);
            const reason =
              reasons[0] ?? (item.todo_state === "done" ? "Done" : "Seen");
            return (
              <button
                className={
                  "queue-row" + (activeId === item.id ? " selected" : "")
                }
                key={item.id}
                onClick={() => setSelected(item.id)}
                aria-pressed={activeId === item.id}
              >
                <span className="queue-item-top">
                  <strong>{item.title}</strong>
                </span>
                <span className="queue-item-summary">{item.summary}</span>
                <span className="queue-item-meta">
                  <span
                    className={
                      "queue-state " +
                      (reason === "Reminder due"
                        ? "due"
                        : reason === "Todo"
                          ? "todo"
                          : "")
                    }
                  >
                    {reason}
                  </span>
                  <span>
                    {new Intl.DateTimeFormat(undefined, {
                      month: "short",
                      day: "numeric",
                      timeZone: effectiveTimezone(settings.data?.timezone),
                    }).format(new Date(item.content_updated_at))}
                  </span>
                  {drafts[item.id] !== undefined && <span>Unsaved note</span>}
                </span>
              </button>
            );
          })}
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
        {mode === "attention" && (proposals.data?.length ?? 0) > 0 && (
          <section className="ledger-proposals">
            <h2>
              Proposals to review <span>{proposals.data?.length}</span>
            </h2>
            {proposals.data?.map((proposal) => (
              <ProposalCard
                key={proposal.id}
                proposal={proposal}
                candidates={proposals.data}
              />
            ))}
          </section>
        )}
      </div>
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
