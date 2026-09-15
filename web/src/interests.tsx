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
import { api, collection } from "./api";

type Interest = {
  id: string;
  title: string;
  instructions_md: string;
  state: string;
  revision: number;
};
type Watch = {
  id: string;
  interest_id: string;
  source: { kind: string; locator: string };
  instructions_md: string;
  interval_seconds: number;
  lookback_seconds: number;
  state: string;
  revision: number;
};
type Editor =
  | { kind: "interest"; item?: Interest }
  | { kind: "watch"; interestId: string; item?: Watch };

function ConfigurationEditor({
  editor,
  close,
}: {
  editor: Editor;
  close: () => void;
}) {
  const cache = useQueryClient();
  const item = editor.item;
  const watch = editor.kind === "watch" ? editor.item : undefined;
  const [title, setTitle] = useState(
    editor.kind === "interest" ? (editor.item?.title ?? "") : "",
  );
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
      instructions_md: instructions,
      state,
      ...(item ? { expected_revision: item.revision } : {}),
    };
    const body =
      editor.kind === "interest"
        ? { ...common, title }
        : {
            ...common,
            ...(!item ? { interest_id: editor.interestId } : {}),
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
          {editor.kind === "interest" ? "Interest" : "Watch"}
        </DialogTitle>
        <DialogContent>
          <div className="editor-fields">
            <p>
              {editor.kind === "interest"
                ? "Tell your agent what matters and what a useful finding looks like."
                : "Point your agent at a source and describe what to look for. An external agent checks it when due."}
            </p>
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
  const interests = useQuery({
    queryKey: ["interests"],
    queryFn: () => collection<Interest>("/interests"),
  });
  const watches = useQuery({
    queryKey: ["watches"],
    queryFn: () => collection<Watch>("/watches"),
  });
  const [editor, setEditor] = useState<Editor | null>(null);
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
        <p>What matters to you, and where your agents should look.</p>
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
            A project, a question, a corner of the world. Add an Interest, then
            give it a source to watch.
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
          <section className="panel" key={interest.id}>
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
            <div className="watch-list">
              {watches.data
                ?.filter((w) => w.interest_id === interest.id)
                .map((watch) => (
                  <div className="watch-row" key={watch.id}>
                    <div>
                      <span className="source-kind">{watch.source.kind}</span>
                      <strong>{watch.source.locator}</strong>
                      <small>
                        {watch.state} · every {watch.interval_seconds / 60}{" "}
                        minutes
                      </small>
                    </div>
                    <Button
                      size="small"
                      onClick={() =>
                        setEditor({
                          kind: "watch",
                          interestId: interest.id,
                          item: watch,
                        })
                      }
                    >
                      Edit Watch
                    </Button>
                  </div>
                ))}
            </div>
            <Button
              size="small"
              onClick={() =>
                setEditor({ kind: "watch", interestId: interest.id })
              }
            >
              + Add Watch
            </Button>
          </section>
        ))}
      </div>
      {editor && (
        <ConfigurationEditor editor={editor} close={() => setEditor(null)} />
      )}
    </>
  );
}
