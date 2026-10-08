import {
  isValidElement,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Dialog,
  DialogContent,
  DialogTitle,
  TextField,
} from "@mui/material";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import DOMPurify from "dompurify";
import { api, APIError } from "./api";
import { type Item, ReminderDialog, useItemAction } from "./items";
import { formatDateTime, useSettings } from "./settings";
import type { AnswerValue, ReportBlock, ReviewAnswer } from "./report-types";

let mermaidModule: Promise<typeof import("mermaid")> | undefined;

async function renderMermaid(source: string) {
  mermaidModule ??= import("mermaid").then((module) => {
    module.default.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      htmlLabels: false,
      maxTextSize: 32768,
      maxEdges: 500,
      secure: [
        "securityLevel",
        "startOnLoad",
        "htmlLabels",
        "maxTextSize",
        "maxEdges",
      ],
    });
    return module;
  });
  const { default: mermaid } = await mermaidModule;
  const result = await mermaid.render("diagram-" + crypto.randomUUID(), source);
  return DOMPurify.sanitize(result.svg, {
    USE_PROFILES: { svg: true, svgFilters: true },
    FORBID_TAGS: ["foreignObject", "script", "image", "a"],
  });
}

function EvidenceImage({
  src,
  description,
  caption,
}: {
  src: string;
  description: string;
  caption?: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <figure className="review-evidence">
      {failed ? (
        <Alert severity="warning">Image unavailable: {description}</Alert>
      ) : (
        <>
          <img src={src} alt={description} onError={() => setFailed(true)} />
          <Button size="small" onClick={() => setExpanded(true)}>
            Enlarge {caption || "image"}
          </Button>
        </>
      )}
      {caption && <figcaption>{caption}</figcaption>}
      <Dialog
        open={expanded}
        onClose={() => setExpanded(false)}
        maxWidth="xl"
        fullWidth
      >
        <DialogTitle>
          {caption || description}
          <Button onClick={() => setExpanded(false)}>Close</Button>
        </DialogTitle>
        <DialogContent>
          <img className="review-expanded-image" src={src} alt={description} />
        </DialogContent>
      </Dialog>
    </figure>
  );
}

export function Diagram({
  language,
  source,
  description,
  caption,
}: {
  language: string;
  source: string;
  description: string;
  caption?: string;
}) {
  const [result, setResult] = useState<{ url?: string; error?: string }>({});
  useEffect(() => {
    let active = true;
    let url: string | undefined;
    const abort = new AbortController();
    setResult({});
    async function render() {
      if (source.length > 32768)
        throw new Error("Diagram source exceeds the rendering limit.");
      let blob: Blob;
      if (language === "mermaid") {
        blob = new Blob([await renderMermaid(source)], {
          type: "image/svg+xml",
        });
      } else {
        const response = await fetch("/api/v1/review-diagrams", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ source }),
          signal: abort.signal,
        });
        if (!response.ok) {
          const error = await response.json();
          throw new Error(error.error?.message || "Diagram rendering failed.");
        }
        blob = await response.blob();
      }
      if (active) {
        url = URL.createObjectURL(blob);
        setResult({ url });
      }
    }
    void render().catch((error: Error) => {
      if (active) setResult({ error: error.message });
    });
    return () => {
      active = false;
      abort.abort();
      if (url) URL.revokeObjectURL(url);
    };
  }, [language, source]);
  return (
    <div className="review-diagram">
      {result.error ? (
        <Alert severity="warning">{result.error}</Alert>
      ) : result.url ? (
        <EvidenceImage
          src={result.url}
          description={description}
          caption={caption}
        />
      ) : (
        <p>Rendering diagram…</p>
      )}
      <details>
        <summary>Diagram source ({language})</summary>
        <pre>{source}</pre>
      </details>
    </div>
  );
}

export function ReportMarkdown({ body }: { body: string }) {
  return (
    <div className="report-prose">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        skipHtml
        components={{
          pre({ children }) {
            if (
              isValidElement<{ className?: string; children?: ReactNode }>(
                children,
              )
            ) {
              const language = children.props.className?.replace(
                "language-",
                "",
              );
              if (language === "mermaid" || language === "plantuml") {
                return (
                  <Diagram
                    language={language}
                    source={String(children.props.children).trimEnd()}
                    description={`${language} diagram`}
                  />
                );
              }
            }
            return <pre>{children}</pre>;
          },
        }}
      >
        {body}
      </ReactMarkdown>
    </div>
  );
}

function DisplayBlock({ block }: { block: ReportBlock }) {
  switch (block.type) {
    case "markdown":
      return <ReportMarkdown body={block.body_md || ""} />;
    case "image":
      return (
        <EvidenceImage
          key={block.artifact_id}
          src={"/api/v1/review-artifacts/" + block.artifact_id}
          description={block.description || "Image"}
          caption={block.caption}
        />
      );
    case "diagram":
      return (
        <Diagram
          language={block.language || "mermaid"}
          source={block.source || ""}
          description={block.description || "Diagram"}
          caption={block.caption}
        />
      );
    default:
      return (
        <Alert severity="warning">
          Unsupported display block: {block.type}
        </Alert>
      );
  }
}

function ReviewForm({ item, review }: { item: Item; review: ReportBlock }) {
  const cache = useQueryClient();
  const formID = useId();
  const fieldRefs = useRef<Record<string, HTMLElement | null>>({});
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [touched, setTouched] = useState<Record<string, boolean>>({});
  const current = item.answers?.find(
    (answer) => answer.review_id === review.id,
  );
  const fields =
    review.blocks?.filter(
      (block) => block.type === "choice" || block.type === "text_input",
    ) || [];
  const [values, setValues] = useState<Record<string, AnswerValue>>(() =>
    Object.fromEntries(
      fields.map((field) => {
        const saved = current?.values.find(
          (value) => value.field_id === field.id,
        );
        const ids = saved?.selected_ids?.filter((id) =>
          field.options?.some((option) => option.id === id),
        );
        return [
          field.id,
          {
            field_id: field.id,
            disposition: saved?.disposition || "skipped",
            text: saved?.text,
            selected_ids: ids,
          },
        ];
      }),
    ),
  );
  const previousFields = useRef(fields);
  const fieldStructure = JSON.stringify(
    fields.map((field) => ({
      id: field.id,
      type: field.type,
      selection: field.selection,
      options: field.options?.map((option) => option.id),
    })),
  );
  useEffect(() => {
    const priorFields = previousFields.current;
    setValues((old) =>
      Object.fromEntries(
        fields.map((field) => {
          const previous = priorFields.find((entry) => entry.id === field.id);
          const value = old[field.id];
          if (
            !value ||
            previous?.type !== field.type ||
            previous.selection !== field.selection
          )
            return [field.id, { field_id: field.id, disposition: "skipped" }];
          if (field.type === "text_input") return [field.id, value];
          const selected = value.selected_ids?.filter((id) =>
            field.options?.some((option) => option.id === id),
          );
          return [
            field.id,
            {
              field_id: field.id,
              disposition:
                field.selection === "single" && !selected?.length
                  ? "skipped"
                  : value.disposition,
              selected_ids: selected,
            },
          ];
        }),
      ),
    );
    previousFields.current = fields;
  }, [fieldStructure]);
  // An uncertain retry reuses the entire original request, even if refetch
  // reveals that the first attempt already advanced state and saved an answer.
  const request = useRef<object | null>(null);
  const [saved, setSaved] = useState(false);
  const mutation = useMutation({
    mutationFn: () => {
      request.current ??= {
        request_id: crypto.randomUUID(),
        review_id: review.id,
        expected_content_version: item.content_version,
        expected_state_version: item.state_version,
        review_material_hash: item.review_material_hash,
        supersedes: current?.id,
        values: fields.map((field) => values[field.id]),
      };
      return api("/items/" + item.id + "/answers", request.current, "POST");
    },
    onSuccess: async () => {
      request.current = null;
      setSaved(true);
      await cache.invalidateQueries({ queryKey: ["item", item.id] });
      await cache.invalidateQueries({ queryKey: ["items"] });
    },
    onError: async (error) => {
      if (
        error instanceof APIError &&
        [
          "content_conflict",
          "state_conflict",
          "answer_conflict",
          "validation_error",
          "portal_required",
        ].includes(error.code || "")
      )
        request.current = null;
      await cache.invalidateQueries({ queryKey: ["item", item.id] });
    },
  });
  const update = (field: string, value: AnswerValue) => {
    request.current = null;
    setValues((old) => ({ ...old, [field]: value }));
    setSaved(false);
    setTouched((old) => ({ ...old, [field]: true }));
    setFieldErrors((old) => ({ ...old, [field]: "" }));
    mutation.reset();
  };
  return (
    <form
      className="review-form"
      aria-label={review.title}
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        if (!request.current) {
          const errors: Record<string, string> = {};
          for (const field of fields) {
            const value = values[field.id];
            if (field.required && value?.disposition !== "answered") {
              errors[field.id] = "Answer this required question.";
            } else if (field.type === "text_input") {
              if (field.required && !value?.text?.trim())
                errors[field.id] = "Enter an answer to this required question.";
            } else if (value?.disposition === "answered") {
              const count = value.selected_ids?.length || 0;
              const minimum = Math.max(
                field.min_selections || 0,
                field.required ? 1 : 0,
              );
              if (field.selection === "single" && count !== 1)
                errors[field.id] = "Choose one option.";
              else if (count < minimum)
                errors[field.id] =
                  `Choose at least ${minimum} option${minimum === 1 ? "" : "s"}.`;
              else if (field.max_selections && count > field.max_selections)
                errors[field.id] =
                  `Choose no more than ${field.max_selections} options.`;
            }
          }
          setFieldErrors(errors);
          const first = fields.find((field) => errors[field.id]);
          if (first) {
            fieldRefs.current[first.id]
              ?.querySelector<HTMLElement>("input, textarea")
              ?.focus();
            return;
          }
        }
        mutation.mutate();
      }}
    >
      <h3>{review.title}</h3>
      {current && (
        <p>
          {current.applicable
            ? "Previously answered. You can submit a correction."
            : "The review material changed. Review your earlier answer before resubmitting."}
        </p>
      )}
      {review.blocks?.map((block, index) => {
        const value = values[block.id];
        const error = fieldErrors[block.id];
        const statusID = `${formID}-status-${index}`;
        const errorID = `${formID}-error-${index}`;
        if (block.type === "text_input")
          return (
            <div
              key={block.id}
              className="review-field"
              ref={(element) => {
                fieldRefs.current[block.id] = element;
              }}
            >
              <TextField
                fullWidth
                multiline
                label={block.question}
                required={block.required}
                error={!!error}
                helperText={
                  error ||
                  (!block.required
                    ? touched[block.id] || current
                      ? value?.disposition === "skipped"
                        ? "Skipped"
                        : value?.text
                          ? "Answer entered"
                          : "Empty answer"
                      : "Optional · Not answered"
                    : "Required")
                }
                value={value?.text || ""}
                disabled={mutation.isPending}
                onChange={(event) =>
                  update(block.id, {
                    field_id: block.id,
                    disposition: "answered",
                    text: event.target.value,
                  })
                }
              />
              {!block.required && (
                <Button
                  size="small"
                  disabled={mutation.isPending}
                  onClick={() =>
                    update(block.id, {
                      field_id: block.id,
                      disposition: "skipped",
                    })
                  }
                >
                  Skip {block.question}
                </Button>
              )}
            </div>
          );
        if (block.type === "choice")
          return (
            <fieldset
              key={block.id}
              disabled={mutation.isPending}
              className="review-field"
              ref={(element) => {
                fieldRefs.current[block.id] = element;
              }}
              aria-describedby={`${statusID}${error ? ` ${errorID}` : ""}`}
            >
              <legend>
                {block.question}
                {block.required ? " (required)" : " (optional)"}
              </legend>
              {block.options?.map((option) => (
                <div key={option.id} className="review-option">
                  <label>
                    <input
                      type={block.selection === "single" ? "radio" : "checkbox"}
                      name={`${item.id}-${review.id}-${block.id}`}
                      aria-invalid={!!error}
                      aria-describedby={`${statusID}${error ? ` ${errorID}` : ""}`}
                      disabled={
                        block.selection === "multiple" &&
                        !!block.max_selections &&
                        (value?.selected_ids?.length || 0) >=
                          block.max_selections &&
                        !value?.selected_ids?.includes(option.id)
                      }
                      checked={
                        value?.selected_ids?.includes(option.id) || false
                      }
                      onChange={(event) => {
                        const selected =
                          block.selection === "single"
                            ? [option.id]
                            : event.target.checked
                              ? [...(value?.selected_ids || []), option.id]
                              : (value?.selected_ids || []).filter(
                                  (id) => id !== option.id,
                                );
                        update(block.id, {
                          field_id: block.id,
                          disposition: "answered",
                          selected_ids: selected,
                        });
                      }}
                    />
                    {option.label}
                  </label>
                  {option.blocks?.map((content) => (
                    <DisplayBlock key={content.id} block={content} />
                  ))}
                </div>
              ))}
              {!block.required && (
                <Button
                  size="small"
                  onClick={() =>
                    update(block.id, {
                      field_id: block.id,
                      disposition: "skipped",
                    })
                  }
                >
                  Skip {block.question}
                </Button>
              )}
              {block.selection === "multiple" &&
                !block.required &&
                !block.min_selections && (
                  <Button
                    size="small"
                    onClick={() =>
                      update(block.id, {
                        field_id: block.id,
                        disposition: "answered",
                        selected_ids: [],
                      })
                    }
                  >
                    Select none
                  </Button>
                )}
              <small
                id={statusID}
                className="review-field-status"
                aria-live="polite"
              >
                {!touched[block.id] && !current
                  ? "Not answered"
                  : value?.disposition === "skipped"
                    ? "Skipped"
                    : `${value?.selected_ids?.length || 0} selected`}
                {block.selection === "single"
                  ? " · Choose one"
                  : `${block.min_selections ? ` · Minimum ${block.min_selections}` : ""}${block.max_selections ? ` · Maximum ${block.max_selections}` : ""}`}
              </small>
              {error && (
                <p id={errorID} className="review-field-error" role="alert">
                  {error}
                </p>
              )}
            </fieldset>
          );
        return <DisplayBlock key={block.id} block={block} />;
      })}
      <Button type="submit" variant="contained" disabled={mutation.isPending}>
        {mutation.isPending ? "Saving…" : review.submit_label || "Save answer"}
      </Button>
      {saved && (
        <Alert severity="success">
          Answer saved for the next agent to use.
        </Alert>
      )}
      {mutation.isError && (
        <Alert severity="error">
          {mutation.error.message} Your input is kept; reread the latest review
          before retrying.
        </Alert>
      )}
    </form>
  );
}

function ReportActions({ item, ids }: { item: Item; ids: string[] }) {
  const [reminder, setReminder] = useState(false);
  const { apply, mutation } = useItemAction(item.id, () => setReminder(false));
  return (
    <>
      {ids.map((id) => {
        const action = item.report.actions?.find((entry) => entry.id === id);
        if (!action) return null;
        if (action.type === "open_link") {
          const source = item.sources?.find(
            (entry) => entry.id === action.source_ref,
          );
          return source?.url ? (
            <Button
              key={id}
              component="a"
              href={source.url}
              target="_blank"
              rel="noopener noreferrer"
            >
              {action.label}
            </Button>
          ) : null;
        }
        return (
          <Button
            key={id}
            disabled={mutation.isPending}
            onClick={() =>
              action.type === "set_reminder"
                ? setReminder(true)
                : apply(
                    item,
                    action.type === "acknowledge"
                      ? {
                          type: action.type,
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
        );
      })}
      {mutation.isError && (
        <Alert severity="error">{mutation.error.message}</Alert>
      )}
      {reminder && (
        <ReminderDialog
          close={() => setReminder(false)}
          apply={(action) => apply(item, action)}
          error={mutation.error}
          pending={mutation.isPending}
        />
      )}
    </>
  );
}

export function ReportContent({ item }: { item: Item }) {
  if (item.report.schema_version === 1)
    return <ReportMarkdown body={item.report.body_md} />;
  if (item.report.schema_version !== 2 || !item.report.blocks?.length)
    return (
      <>
        <Alert severity="warning">
          Structured report unavailable. The explanation and sources remain
          readable.
        </Alert>
        <ReportMarkdown body={item.report.body_md} />
      </>
    );
  return (
    <>
      {item.report.blocks.map((block) =>
        block.type === "review" ? (
          <ReviewForm
            key={`${item.id}-${block.id}`}
            item={item}
            review={block}
          />
        ) : block.type === "actions" ? (
          <div key={block.id} className="reader-report-actions">
            <ReportActions item={item} ids={block.action_ids || []} />
          </div>
        ) : (
          <DisplayBlock key={block.id} block={block} />
        ),
      )}
    </>
  );
}

function SavedAnswer({
  answer,
  timezone,
  label,
}: {
  answer: ReviewAnswer;
  timezone?: string;
  label?: string;
}) {
  return (
    <article className="review-answer">
      <header className="review-answer-heading">
        <strong>{answer.title}</strong>
        <time dateTime={answer.submitted_at}>
          {formatDateTime(answer.submitted_at, timezone)}
        </time>
      </header>
      {label && <small>{label}</small>}
      {answer.values.map((value) => (
        <p key={value.field_id} className="review-answer-value">
          <strong>{value.question}: </strong>
          {value.answer || "Empty answer"}
        </p>
      ))}
      <small>Reviewed content version {answer.content_version}</small>
    </article>
  );
}

export function AnswerNotes({ item }: { item: Item }) {
  const settings = useSettings();
  const historyID = useId();
  const [history, setHistory] = useState(false);
  const [after, setAfter] = useState(0);
  const query = useQuery({
    queryKey: ["answer-history", item.id, after, item.state_version],
    enabled: history,
    queryFn: () =>
      api<{ items: ReviewAnswer[]; next_after?: number }>(
        `/items/${item.id}/answers?after=${after}`,
      ),
  });
  if (!item.answers?.length && !item.answer_history_available && !history)
    return null;
  return (
    <section className="review-notes" aria-label="Your answers">
      <h3>Your answers</h3>
      {item.answers?.map((answer) => (
        <div key={answer.id}>
          {!answer.applicable && (
            <Alert severity="warning">
              Answer is for earlier review material.
            </Alert>
          )}
          <SavedAnswer answer={answer} timezone={settings.data?.timezone} />
        </div>
      ))}
      <Button
        aria-expanded={history}
        aria-controls={history ? historyID : undefined}
        onClick={() => {
          setHistory(!history);
          setAfter(0);
        }}
      >
        {history ? "Hide answer history" : "Answer history"}
      </Button>
      {history && (
        <section id={historyID} aria-label="Answer history">
          {query.isPending && <p role="status">Loading answer history…</p>}
          {query.isError && (
            <Alert severity="error">{query.error.message}</Alert>
          )}
          {query.data?.items.map((answer) => (
            <SavedAnswer
              key={answer.id}
              answer={answer}
              timezone={settings.data?.timezone}
              label={
                item.answers?.some((current) => current.id === answer.id)
                  ? "Latest submission"
                  : "Earlier submission"
              }
            />
          ))}
          {after > 0 && <Button onClick={() => setAfter(0)}>First page</Button>}
          {query.data?.next_after && (
            <Button onClick={() => setAfter(query.data.next_after!)}>
              Next history page
            </Button>
          )}
        </section>
      )}
    </section>
  );
}
