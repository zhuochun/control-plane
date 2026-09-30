import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, Button, IconButton, TextField } from "@mui/material";
import ArrowBackOutlined from "@mui/icons-material/ArrowBackOutlined";
import DescriptionOutlined from "@mui/icons-material/DescriptionOutlined";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import { NavLink } from "react-router";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { api } from "./api";
import { type Item, parseMetric, ReminderDialog, useItemAction } from "./items";
import { MetricChart } from "./metric-chart";
import { formatDateTime, useSettings } from "./settings";

export function attentionReasons(item: Item) {
  return [
    item.remind_at && new Date(item.remind_at) <= new Date()
      ? "Reminder due"
      : null,
    item.acknowledged_content_version < item.content_version
      ? "New evidence"
      : null,
    item.todo_state === "todo" ? "Todo" : null,
  ].filter((reason): reason is string => !!reason);
}

export function ItemReader({
  itemId,
  close,
  note,
  setNote,
  savedNote,
  interests,
}: {
  itemId: string;
  close: () => void;
  note: string | null;
  setNote: (value: string) => void;
  savedNote: (value: string) => void;
  interests: { id: string; slug: string; title: string }[];
}) {
  const cache = useQueryClient();
  const settings = useSettings();
  const query = useQuery({
    queryKey: ["item", itemId],
    queryFn: () => api<Item>("/items/" + itemId),
  });
  const [reminder, setReminder] = useState(false);
  const noteRequest = useRef<{ signature: string; id: string } | null>(null);
  const { apply, mutation } = useItemAction(itemId, () => setReminder(false));
  const saveNote = useMutation({
    mutationFn: (input: { item: Item; value: string }) => {
      const signature = JSON.stringify({
        version: input.item.state_version,
        note: input.value,
      });
      if (noteRequest.current?.signature !== signature)
        noteRequest.current = { signature, id: crypto.randomUUID() };
      return api<Item>(
        "/items/" + itemId + "/note",
        {
          request_id: noteRequest.current.id,
          expected_state_version: input.item.state_version,
          user_note: input.value,
        },
        "PUT",
      );
    },
    onSuccess: (updated, input) => {
      cache.setQueryData(["item", updated.id], updated);
      cache.invalidateQueries({ queryKey: ["items"] });
      savedNote(input.value);
    },
    onError: () => {
      cache.invalidateQueries({ queryKey: ["item", itemId] });
    },
  });
  const item = query.data;
  const sources = item?.sources ?? [];
  const metric = item ? parseMetric(item) : null;
  const watch = useQuery({
    queryKey: ["watch", item?.watch_id],
    queryFn: () => api<{ slug: string }>("/watches/" + item?.watch_id),
    enabled: !!item?.watch_id,
  });
  const pending = mutation.isPending || saveNote.isPending;
  return (
    <section className="workspace-reader" aria-label="Item details">
      <div className="reader-scroll" tabIndex={0} aria-label="Report content">
        <div className="mobile-reader-back">
          <IconButton aria-label="Close item detail" onClick={close}>
            <ArrowBackOutlined />
          </IconButton>
          <span>Back to items</span>
        </div>
        {query.isPending && <p role="status">Opening item…</p>}
        {query.isError && (
          <Alert severity="error">Could not open this item.</Alert>
        )}
        {item && (
          <>
            <header className="reader-header">
              <h2>{item.title}</h2>
              <div className="reader-actions">
                {item.acknowledged_content_version < item.content_version && (
                  <Button
                    variant="contained"
                    disabled={pending}
                    onClick={() =>
                      apply(item, {
                        type: "acknowledge",
                        content_version: item.content_version,
                      })
                    }
                  >
                    Acknowledge
                  </Button>
                )}
                <Button
                  variant="outlined"
                  disabled={pending}
                  onClick={() =>
                    apply(item, {
                      type: "set_todo",
                      state: item.todo_state === "todo" ? "done" : "todo",
                    })
                  }
                >
                  {item.todo_state === "todo"
                    ? "Mark Done"
                    : item.todo_state === "done"
                      ? "Reopen Todo"
                      : "Add Todo"}
                </Button>
                <Button
                  variant="outlined"
                  disabled={pending}
                  onClick={() => setReminder(true)}
                >
                  {item.remind_at ? "Change reminder" : "Remind"}
                </Button>
              </div>
            </header>
            <div className="reader-meta">
              {attentionReasons(item).map((reason) => (
                <span className="reader-reason" key={reason}>
                  {reason}
                </span>
              ))}
              {!!item.interests?.length && (
                <span>
                  Interest:{" "}
                  {item.interests
                    .map(
                      (reason) =>
                        interests.find((entry) => entry.id === reason.id)
                          ?.title ?? reason.id,
                    )
                    .join(", ")}
                </span>
              )}
              <span>
                Updated{" "}
                {formatDateTime(
                  item.content_updated_at,
                  settings.data?.timezone,
                )}
              </span>
            </div>
            <p className="reader-summary">{item.summary}</p>
            {item.remind_at && (
              <div className="reader-reminder">
                Reminder:{" "}
                {formatDateTime(item.remind_at, settings.data?.timezone)}
                <Button
                  disabled={pending}
                  onClick={() => apply(item, { type: "clear_reminder" })}
                >
                  Clear reminder
                </Button>
              </div>
            )}
            {mutation.isError && (
              <Alert severity="error">{mutation.error.message}</Alert>
            )}
            {metric && (
              <>
                <div className="reader-metrics">
                  <div>
                    <span>Latest</span>
                    <strong>{metric.current.display}</strong>
                  </div>
                  <div>
                    <span>vs {metric.previous.display}</span>
                    <strong>
                      {metric.changePercent > 0 ? "+" : ""}
                      {Math.round(metric.changePercent)}%
                    </strong>
                  </div>
                  {metric.target && (
                    <div>
                      <span>Target</span>
                      <strong>{metric.target.display}</strong>
                    </div>
                  )}
                </div>
                <MetricChart item={item} />
              </>
            )}
            <article className="reader-markdown">
              {item.report.schema_version === 1 ? (
                <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>
                  {item.report.body_md}
                </ReactMarkdown>
              ) : (
                <Alert severity="warning">
                  Report format unavailable. The summary and sources are still
                  shown.
                </Alert>
              )}
            </article>
            {!!item.report.actions?.length && (
              <div
                className="reader-report-actions"
                aria-label="Report actions"
              >
                {item.report.actions.map((action) => {
                  const source = sources.find(
                    (entry) => entry.id === action.source_ref,
                  );
                  return action.type === "open_link" ? (
                    source?.url ? (
                      <Button
                        key={action.id}
                        component="a"
                        href={source.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        endIcon={<OpenInNewOutlined />}
                      >
                        {action.label}
                      </Button>
                    ) : null
                  ) : (
                    <Button
                      key={action.id}
                      disabled={pending}
                      variant="outlined"
                      onClick={() =>
                        action.type === "set_reminder"
                          ? setReminder(true)
                          : apply(
                              item,
                              action.type === "acknowledge"
                                ? {
                                    type: "acknowledge",
                                    content_version: item.content_version,
                                  }
                                : {
                                    type: action.type,
                                    ...(action.state
                                      ? { state: action.state }
                                      : {}),
                                  },
                            )
                      }
                    >
                      {action.label}
                    </Button>
                  );
                })}
              </div>
            )}
            <section className="reader-sources">
              <h3>Sources</h3>
              {sources.length ? (
                sources.map((source) =>
                  source.url ? (
                    <a
                      key={source.id}
                      href={source.url}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <DescriptionOutlined fontSize="small" />
                      <span>{source.label || source.url}</span>
                      {source.observed_at && (
                        <small>
                          Observed{" "}
                          {formatDateTime(
                            source.observed_at,
                            settings.data?.timezone,
                          )}
                        </small>
                      )}
                      <OpenInNewOutlined fontSize="small" />
                    </a>
                  ) : (
                    <div key={source.id}>
                      <DescriptionOutlined fontSize="small" />
                      <span>{source.label || "Source date"}</span>
                      <small>{source.source_date}</small>
                    </div>
                  ),
                )
              ) : (
                <p>No source attached.</p>
              )}
              {item.watch_id && (
                <p>
                  Originating Watcher:{" "}
                  <NavLink
                    to={"/interests#watcher-" + (watch.data?.slug ?? "")}
                  >
                    {watch.data?.slug ?? item.watch_id}
                  </NavLink>
                </p>
              )}
              {item.origin === "legacy" && (
                <p>
                  Historical Item: the original source or relevance explanation
                  may be unavailable.
                </p>
              )}
            </section>
            {!!item.interests?.length && (
              <section className="reader-relevance">
                <h3>Why this matters</h3>
                {item.interests.map((reason) => {
                  const interest = interests.find(
                    (entry) => entry.id === reason.id,
                  );
                  return (
                    <p key={reason.id}>
                      <NavLink
                        to={"/interests#interest-" + (interest?.slug ?? "")}
                      >
                        {interest?.title ?? reason.id}
                      </NavLink>
                      :{" "}
                      {reason.reason ||
                        "Historical assignment; original explanation unavailable."}
                    </p>
                  );
                })}
              </section>
            )}
          </>
        )}
      </div>
      {item && (
        <form
          className="reader-note"
          onSubmit={(event) => {
            event.preventDefault();
            if (note !== null) saveNote.mutate({ item, value: note });
          }}
        >
          <label htmlFor={"note-" + itemId}>Your note</label>
          <div className="reader-note-input">
            <TextField
              id={"note-" + itemId}
              multiline
              minRows={2}
              maxRows={4}
              fullWidth
              placeholder="Keep context for the next run…"
              value={note ?? item.user_note}
              onChange={(event) => {
                setNote(event.target.value);
                if (!saveNote.isPending) saveNote.reset();
              }}
            />
            <Button
              type="submit"
              variant="contained"
              disabled={note === null || pending}
            >
              {saveNote.isPending ? "Saving…" : "Save note"}
            </Button>
          </div>
          {saveNote.isError && (
            <Alert severity="error">
              {saveNote.error.message} Your draft is kept; retry after the item
              refreshes.
            </Alert>
          )}
          {saveNote.isSuccess && note === null && (
            <span role="status">Note saved.</span>
          )}
        </form>
      )}
      {reminder && (
        <ReminderDialog
          close={() => setReminder(false)}
          apply={(action) => item && apply(item, action)}
          error={mutation.error}
        />
      )}
    </section>
  );
}
