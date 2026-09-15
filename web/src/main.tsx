import React, { useState } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, NavLink, Route, Routes } from "react-router";
import {
  QueryClient,
  QueryClientProvider,
  useMutation,
  useQuery,
} from "@tanstack/react-query";
import {
  Alert,
  Button,
  CssBaseline,
  TextField,
  ThemeProvider,
  createTheme,
} from "@mui/material";
import "./style.css";
import { api, collection } from "./api";
import { Interests } from "./interests";
import { Attention, ItemDetail, Library } from "./items";

type ActivityProposal = {
  id: string;
  target_type: string;
  operation: string;
  rationale_md: string;
  state: string;
};

const theme = createTheme({
  palette: { primary: { main: "#246b59" }, background: { default: "#f6f5f1" } },
  typography: {
    fontFamily: 'Inter, "Segoe UI", sans-serif',
    button: { textTransform: "none", fontWeight: 600 },
  },
  shape: { borderRadius: 10 },
  components: { MuiButton: { defaultProps: { disableElevation: true } } },
});
const client = new QueryClient({
  defaultOptions: {
    queries: {
      refetchInterval: 5000,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: true,
    },
    mutations: { retry: false },
  },
});

function Activity() {
  const settings = useQuery({
    queryKey: ["settings"],
    queryFn: () => api<{ timezone: string }>("/settings"),
  });
  const [draft, setDraft] = useState<string | null>(null);
  const runs = useQuery({
    queryKey: ["runs"],
    queryFn: () =>
      collection<{
        id: string;
        runner_label: string;
        status: string;
        started_at: string;
        selected_watches: unknown[];
        summary: string;
      }>("/runs"),
  });
  const proposals = useQuery({
    queryKey: ["proposals", "history"],
    queryFn: () => collection<ActivityProposal>("/proposals"),
  });
  const save = useMutation({
    mutationFn: (timezone: string) =>
      api<{ timezone: string }>("/settings", {
        request_id: crypto.randomUUID(),
        timezone,
      }),
    onSuccess: (result) => {
      client.setQueryData(["settings"], result);
      setDraft(null);
    },
  });
  return (
    <>
      <header className="page-header">
        <div className="eyebrow">Your local workspace</div>
        <h1>Activity</h1>
        <p>A clear view of what has happened, and what comes next.</p>
      </header>
      <section className="panel activity-runs">
        <div className="section-heading">
          <h2>Agent runs</h2>
          <span className="tag">Local history</span>
        </div>
        {runs.isError && (
          <Alert severity="error">Cannot load run history.</Alert>
        )}
        {runs.data?.length === 0 && (
          <p>
            No agent runs yet. Due Watches will be offered through the brief.
          </p>
        )}
        {runs.data?.slice(0, 20).map((run) => (
          <div className="run-row" key={run.id}>
            <div>
              <strong>{run.runner_label}</strong>
              <small>
                {new Intl.DateTimeFormat(undefined, {
                  dateStyle: "medium",
                  timeStyle: "short",
                }).format(new Date(run.started_at))}{" "}
                · {run.selected_watches.length}{" "}
                {run.selected_watches.length === 1 ? "Watch" : "Watches"}
              </small>
              {run.summary && <p>{run.summary}</p>}
            </div>
            <span className={`tag run-${run.status}`}>{run.status}</span>
          </div>
        ))}
      </section>
      <section className="panel activity-runs">
        <div className="section-heading">
          <h2>Proposal history</h2>
          <span className="tag">Human decisions</span>
        </div>
        {proposals.isError && (
          <Alert severity="error">Cannot load proposal history.</Alert>
        )}
        {proposals.data?.length === 0 && (
          <p>No proposals have been reviewed.</p>
        )}
        {proposals.data
          ?.slice()
          .reverse()
          .slice(0, 20)
          .map((proposal) => (
            <div className="run-row" key={proposal.id}>
              <div>
                <strong>
                  {proposal.operation} {proposal.target_type}
                </strong>
                <p>{proposal.rationale_md}</p>
              </div>
              <span className={`tag run-${proposal.state}`}>
                {proposal.state}
              </span>
            </div>
          ))}
      </section>
      <section className="panel">
        <div className="section-heading">
          <h2>Display timezone</h2>
          <span className="tag">Local preference</span>
        </div>
        <p>
          Choose the timezone used for dates and reminders. Saved reminders keep
          their original instant.
        </p>
        {settings.isError && (
          <Alert severity="error">
            Cannot load settings. Check that aicp is running.
          </Alert>
        )}
        <form
          className="settings-form"
          onSubmit={(event) => {
            event.preventDefault();
            save.mutate(draft ?? settings.data?.timezone ?? "UTC");
          }}
        >
          <TextField
            label="Timezone"
            size="small"
            value={draft ?? settings.data?.timezone ?? ""}
            onChange={(event) => {
              setDraft(event.target.value);
              save.reset();
            }}
            helperText="For example, Asia/Singapore"
            disabled={!settings.data}
          />
          <Button
            type="submit"
            variant="contained"
            disabled={save.isPending || !settings.data}
          >
            {save.isPending ? "Saving…" : "Save preference"}
          </Button>
        </form>
        {save.isError && <Alert severity="error">{save.error.message}</Alert>}
        {save.isSuccess && <Alert severity="success">Timezone saved.</Alert>}
      </section>
    </>
  );
}

function Portal() {
  const status = useQuery({
    queryKey: ["status"],
    queryFn: () => api<{ version: string; database: string }>("/status"),
  });
  return (
    <div className="workspace">
      <aside className="sidebar">
        <NavLink className="brand" to="/">
          <span className="brand-mark">a</span>aicp
          <span className="brand-dot">.</span>
        </NavLink>
        <div className="sidebar-caption">Your attention, considered.</div>
        <nav aria-label="Main navigation">
          {[
            ["/", "Attention", "◉"],
            ["/interests", "Interests", "✳"],
            ["/library", "Library", "▤"],
            ["/activity", "Activity", "↗"],
          ].map(([path, label, icon]) => (
            <NavLink end key={path} to={path}>
              <span aria-hidden="true">{icon}</span>
              {label}
            </NavLink>
          ))}
        </nav>
        <div className="connection">
          <span
            className={status.isError ? "status-dot offline" : "status-dot"}
          />
          {status.isError
            ? "Server unavailable"
            : status.data
              ? "Local workspace"
              : "Connecting…"}
          <small>Private by design. Yours to keep.</small>
        </div>
      </aside>
      <main>
        {status.isError && (
          <Alert severity="warning">
            The local server is unavailable. Start aicp serve to reconnect.
          </Alert>
        )}
        <Routes>
          <Route path="/" element={<Attention />} />
          <Route path="/interests" element={<Interests />} />
          <Route path="/library" element={<Library />} />
          <Route path="/activity" element={<Activity />} />
          <Route path="/items/:id" element={<ItemDetail />} />
          <Route
            path="*"
            element={
              <section className="panel">
                <h1>Page not found</h1>
                <NavLink to="/">Back to Attention</NavLink>
              </section>
            }
          />
        </Routes>
        <footer>
          aicp <span>Small signals. Thoughtful follow-through.</span>
        </footer>
      </main>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <QueryClientProvider client={client}>
        <BrowserRouter>
          <Portal />
        </BrowserRouter>
      </QueryClientProvider>
    </ThemeProvider>
  </React.StrictMode>,
);
