import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, Button } from "@mui/material";
import { api } from "./api";
import type { Item, UserInput } from "./items";
import { formatDateTime } from "./settings";
import { useEffect, useRef, useState } from "react";

function AttemptHistory({ input }: { input: UserInput }) {
  const [open, setOpen] = useState(false);
  const history = useInfiniteQuery({
    queryKey: ["input-attempts", input.id, input.attempt_count],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => api<{ items: NonNullable<UserInput["attempts"]>; next_offset?: number }>(`/items/${input.item_id}/inputs/${input.id}/attempts?offset=${pageParam}`),
    getNextPageParam: last => last.next_offset,
    refetchInterval: false,
    enabled: open,
  });
  return <details onToggle={event => setOpen(event.currentTarget.open)}>
    <summary>Attempt history ({input.attempt_count})</summary>
    {history.isPending && <p>Loading attempts…</p>}
    {history.isError && <Alert severity="error">Attempt history unavailable: {history.error.message}</Alert>}
    {history.data?.pages.flatMap(page => page.items).map(attempt => <div key={attempt.id} className="reader-input-record"><strong>{attempt.outcome}</strong><div className="reader-input-text">{attempt.result_md}</div></div>)}
    {history.hasNextPage && <Button disabled={history.isFetchingNextPage} onClick={() => history.fetchNextPage()}>Load more attempts</Button>}
  </details>;
}

function InputRecord({ input, timezone }: { input: UserInput; timezone?: string }) {
  const last = input.attempts?.at(-1);
  return (
    <article className="reader-input-record">
      <strong>{input.kind === "inbox" ? "Original submission" : "Your note"}</strong>
      <span> · {formatDateTime(input.submitted_at, timezone)} · {input.status}</span>
      {input.original?.submitted_by === "agent" && <span> · Submitted by agent</span>}
      <div className="reader-input-text">{input.text}</div>
      {last && (
        <div>
          <strong>{last.outcome === "follow_up" ? "Follow-up recorded; work remains open" : last.outcome}</strong>
          <div className="reader-input-text">{last.result_md}</div>
          {!!last.references?.length && <div>References: {last.references.join(", ")}</div>}
        </div>
      )}
      {input.attempt_count > 1 && <AttemptHistory input={input} />}
    </article>
  );
}

export function ItemInputs({ item, timezone }: { item: Item; timezone?: string }) {
  const cache = useQueryClient();
  const pending = item.pending_inputs ?? [];
  const signature = JSON.stringify([item.state_version, item.content_version, item.inbox_archived, pending.map(input => [input.id, input.status, input.attempt_count])]);
  const previous = useRef({ itemId: item.id, signature });
  useEffect(() => {
    if (previous.current.itemId === item.id && previous.current.signature !== signature) {
      void cache.invalidateQueries({ queryKey: ["item-inputs", item.id] });
    }
    previous.current = { itemId: item.id, signature };
  }, [cache, item.id, signature]);
  const history = useInfiniteQuery({
    queryKey: ["item-inputs", item.id],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => api<{ items: UserInput[]; next_offset?: number }>(`/items/${item.id}/inputs?offset=${pageParam}`),
    getNextPageParam: (last) => last.next_offset,
    refetchInterval: false,
  });
  const prior = history.data?.pages.flatMap(page => page.items).filter(input => input.status !== "pending") ?? [];
  return (
    <section className="reader-input-history" aria-label="User input processing">
      <div role="status">
        {pending.length ? `Awaiting agent · ${pending.length} input${pending.length === 1 ? "" : "s"}` : prior.some(input => input.status === "processed") ? "Processed" : prior.some(input => input.status === "withdrawn") ? "Note withdrawn" : prior.length ? "Note superseded" : ""}
        {item.inbox_archived && <span> · Inbox capture archived</span>}
      </div>
      {pending.map(input => (
        <details key={input.id} open={!!input.attempts?.length}>
          <summary>{input.kind === "inbox" ? "Original submission" : "Pending note"}</summary>
          {input.attempts?.at(-1) && <Alert severity="warning">The last attempt needs follow-up. Add a note below to guide the next attempt.</Alert>}
          <InputRecord input={input} timezone={timezone} />
        </details>
      ))}
      <details>
        <summary>Prior notes and inbox submissions</summary>
        {history.isPending && <p>Loading input history…</p>}
        {history.isError && <Alert severity="error">Input history is unavailable: {history.error.message}</Alert>}
        {!history.isPending && !history.isError && !prior.length && <p>No processed, superseded, or withdrawn input yet.</p>}
        {prior.map(input => <InputRecord key={input.id} input={input} timezone={timezone} />)}
        {history.hasNextPage && <Button disabled={history.isFetchingNextPage} onClick={() => history.fetchNextPage()}>Load more input history</Button>}
      </details>
    </section>
  );
}
