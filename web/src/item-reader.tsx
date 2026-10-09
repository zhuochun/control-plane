import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Alert,
  Button,
  IconButton,
  Menu,
  MenuItem,
  TextField,
} from "@mui/material";
import ArrowBackOutlined from "@mui/icons-material/ArrowBackOutlined";
import DescriptionOutlined from "@mui/icons-material/DescriptionOutlined";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import Check from "@mui/icons-material/Check";
import Schedule from "@mui/icons-material/Schedule";
import VisibilityOutlined from "@mui/icons-material/VisibilityOutlined";
import ExpandMore from "@mui/icons-material/ExpandMore";
import { NavLink } from "react-router";
import { AnswerNotes, ReportContent } from "./report-content";
import { api } from "./api";
import { type Item, parseMetric, ReminderDialog, useItemAction } from "./items";
import { MetricChart } from "./metric-chart";
import { formatDateTime, useSettings } from "./settings";
import { ItemInputs } from "./item-inputs";
import { KeyHint, useKeyboard } from "./keyboard";
import { type NoteEdits, useNoteEdit, useItemPending } from "./note-editor";
import { useItemShortcuts } from "./item-shortcuts";

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
  edits,
  interests,
}: {
  itemId: string;
  close: () => void;
  edits: NoteEdits;
  interests: { id: string; slug: string; title: string }[];
}) {
  const settings = useSettings();
  const query = useQuery({
    queryKey: ["item", itemId],
    queryFn: () => api<Item>("/items/" + itemId),
    refetchInterval: 5000,
  });
  const [reminder, setReminder] = useState(false);
  const [reminderMenu, setReminderMenu] = useState<HTMLElement | null>(null);
  const edit = useNoteEdit(edits, itemId);
  const { enabled: vimEnabled } = useKeyboard();
  const note = edit.draft;
  const { apply, mutation } = useItemAction(itemId, () => setReminder(false));
  const item = query.data;
  const sources = item?.sources ?? [];
  const metric = item ? parseMetric(item) : null;
  const watch = useQuery({
    queryKey: ["watch", item?.watch_id],
    queryFn: () => api<{ slug: string }>("/watches/" + item?.watch_id),
    enabled: !!item?.watch_id,
  });
  const pending = useItemPending(itemId) || edit.pending;
  useItemShortcuts({ item, pending, available: !query.isPending && !query.isError,
    apply: action => item && apply(item, action), remind: () => setReminder(true),
    save: () => item ? edits.save(item) : Promise.resolve(false),
  });
  return (
    <section className="workspace-reader" aria-label="Item details">
      <div className="reader-scroll" data-reader-item={itemId} tabIndex={0} aria-label="Report content">
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
              <div className="reader-meta">
                <div className="reader-context">
                  <span className="reader-kind">{item.kind}</span>
                  {item.interests?.map((reason) => {
                    const interest = interests.find(
                      (entry) => entry.id === reason.id,
                    );
                    return (
                      <NavLink
                        key={reason.id}
                        to={"/interests#interest-" + (interest?.slug ?? "")}
                      >
                        {interest?.title ?? reason.id}
                      </NavLink>
                    );
                  })}
                </div>
                <span>
                  Updated{" "}
                  {formatDateTime(
                    item.content_updated_at,
                    settings.data?.timezone,
                  )}
                </span>
              </div>
              <h2>{item.title}</h2>
              <p className="reader-summary">{item.summary}</p>
            </header>
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
            <article
              className="reader-markdown"
              data-report-schema={item.report.schema_version}
            >
              <ReportContent item={item} />
            </article>
            {item.report.schema_version === 1 &&
              !!item.report.actions?.length && (
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
            <AnswerNotes item={item} />
            {!!item.interests?.length && (
              <section
                className="reader-relevance"
                aria-label="Why this matters"
              >
                <h3>Why this matters</h3>
                <div>
                  {item.interests.map((reason) => (
                    <p key={reason.id}>
                      {reason.reason ||
                        "Historical assignment; original explanation unavailable."}
                    </p>
                  ))}
                </div>
              </section>
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
          </>
        )}
        {item && <ItemInputs item={item} timezone={settings.data?.timezone} />}
      </div>
      {item && (
        <form
          className="reader-note"
          onSubmit={(event) => {
            event.preventDefault();
            if (!pending) void edits.save(item);
          }}
        >
          <div className="reader-note-heading">
            <label htmlFor={"note-" + itemId}>Your note <KeyHint>i</KeyHint></label>
            <Button
              type="submit"
              size="small"
              variant="outlined"
              disabled={note === null || pending}
            >
              {edit.pending ? "Saving…" : "Save note"}
            </Button>
          </div>
          <div className="reader-note-input">
            <TextField
              id={"note-" + itemId}
              multiline
              minRows={2}
              maxRows={4}
              fullWidth
              placeholder="Keep context for the next run…"
              value={note ?? item.user_note}
              slotProps={{ htmlInput: { "data-vim-note": true, readOnly: edit.pending && vimEnabled } }}
              onChange={(event) => {
                edits.change(item, event.target.value);
              }}
            />
            <KeyHint>Esc: save and exit</KeyHint>
          </div>
          <div className="reader-note-footer">
            {item.remind_at ? (
              <div
                className={
                  "reader-reminder" +
                  (new Date(item.remind_at) <= new Date() ? " is-due" : "")
                }
              >
                <Schedule fontSize="small" />
                <span>
                  {new Date(item.remind_at) <= new Date()
                    ? "Reminder due: "
                    : "Reminder: "}
                  {formatDateTime(item.remind_at, settings.data?.timezone)}
                </span>
              </div>
            ) : (
              <span className="reader-followup-state">
                {item.todo_state === "todo"
                  ? "Todo"
                  : item.todo_state === "done"
                    ? "Done"
                    : ""}
              </span>
            )}
            <div className="reader-actions" aria-label="Item follow-up">
              <Button
                type="button"
                variant="contained"
                startIcon={<Check />}
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
                <KeyHint>{item.todo_state === "todo" ? "d" : "t"}</KeyHint>
              </Button>
              <div className="reader-reminder-control">
                <Button
                  type="button"
                  variant="outlined"
                  startIcon={<Schedule />}
                  disabled={pending}
                  onClick={() => setReminder(true)}
                >
                  {item.remind_at ? "Change reminder" : "Remind"}
                  <KeyHint>s</KeyHint>
                </Button>
                {item.remind_at && (
                  <IconButton
                    type="button"
                    size="small"
                    aria-label="Reminder options"
                    aria-haspopup="menu"
                    aria-expanded={!!reminderMenu}
                    disabled={pending}
                    onClick={(event) => setReminderMenu(event.currentTarget)}
                  >
                    <ExpandMore fontSize="small" />
                  </IconButton>
                )}
              </div>
              {item.acknowledged_content_version < item.content_version ? (
                <Button
                  type="button"
                  variant="text"
                  startIcon={<VisibilityOutlined />}
                  disabled={pending}
                  onClick={() =>
                    apply(item, {
                      type: "acknowledge",
                      content_version: item.content_version,
                    })
                  }
                >
                  Mark seen
                  <KeyHint>r</KeyHint>
                </Button>
              ) : (
                <span className="reader-seen">
                  <VisibilityOutlined fontSize="small" /> Seen
                </span>
              )}
            </div>
          </div>
          {mutation.isError && (
            <Alert severity="error">{mutation.error.message}</Alert>
          )}
          {edit.error && (
            <Alert severity="error">
              {edit.error.message} Your draft is kept; retry after the item
              refreshes.
            </Alert>
          )}
          {edit.saved && note === null && (
            <span role="status">Note saved.</span>
          )}
        </form>
      )}
      <Menu
        anchorEl={reminderMenu}
        open={!!reminderMenu}
        onClose={() => setReminderMenu(null)}
      >
        <MenuItem
          onClick={() => {
            setReminderMenu(null);
            setReminder(true);
          }}
        >
          Change reminder
        </MenuItem>
        <MenuItem
          disabled={pending}
          onClick={() => {
            setReminderMenu(null);
            if (item) apply(item, { type: "clear_reminder" });
          }}
        >
          Clear reminder
        </MenuItem>
      </Menu>
      {reminder && item && (
        <ReminderDialog
          contentVersion={item.content_version}
          close={() => setReminder(false)}
          apply={(action) => item && apply(item, action)}
          error={mutation.error}
          pending={mutation.isPending}
        />
      )}
    </section>
  );
}
