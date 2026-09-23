import React from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, NavLink, Route, Routes } from "react-router";
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import { Alert, CssBaseline, ThemeProvider, createTheme } from "@mui/material";
import "./style.css";
import { api, collection } from "./api";
import { Interests } from "./interests";
import { ItemDetail } from "./items";
import { ItemWorkspace } from "./workspace";
import { Preferences } from "./preferences";
import { formatDateTime, useSettings } from "./settings";

type ActivityProposal = {
  id: string;
  target_type: string;
  operation: string;
  rationale_md: string;
  state: string;
};

const theme = createTheme({
  palette: {
    primary: { main: "#246b59" },
    background: { default: "#f6f5f1" },
  },
  typography: {
    fontFamily: 'Inter, "Segoe UI", sans-serif',
    button: { textTransform: "none", fontWeight: 600 },
  },
  shape: { borderRadius: 9 },
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
  const settings = useSettings();
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
  return (
    <>
      <header className="page-header">
        <div className="eyebrow">History that stays close</div>
        <h1>Activity</h1>
        <p>Runs, coverage, and the decisions that shape what happens next.</p>
      </header>
      <div className="activity-summary" aria-label="Activity summary">
        <div className="summary-metric blue">
          <strong>{runs.data?.length ?? "—"}</strong>
          <span>agent runs</span>
        </div>
        <div className="summary-metric gold">
          <strong>{proposals.data?.length ?? "—"}</strong>
          <span>configuration decisions</span>
        </div>
        <div className="summary-metric green">
          <strong>{settings.data ? "Local" : "—"}</strong>
          <span>workspace state</span>
        </div>
      </div>
      <section className="panel activity-panel">
        <div className="section-heading">
          <h2>Agent runs</h2>
          <span className="tag">Local history</span>
        </div>
        {runs.isError && (
          <Alert severity="error">Cannot load run history.</Alert>
        )}
        {runs.data?.length === 0 && (
          <div className="empty-inline">
            <strong>No agent runs yet.</strong>
            <p>Due Watches will be offered through the next agent brief.</p>
          </div>
        )}
        {runs.data?.slice(0, 20).map((run) => (
          <div className="run-row" key={run.id}>
            <div>
              <strong>{run.runner_label}</strong>
              <small>
                {formatDateTime(run.started_at, settings.data?.timezone)} ·{" "}
                {run.selected_watches.length}{" "}
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
    </>
  );
}

function Portal() {
  const status = useQuery({
    queryKey: ["status"],
    queryFn: () => api<{ version: string; database: string }>("/status"),
  });
  const navigationCounts = useQuery({
    queryKey: ["items", "navigation"],
    queryFn: async () => {
      const [all, attention] = await Promise.all([
        collection<{ todo_state: string }>("/items?view=all"),
        collection<unknown>("/items?view=attention"),
      ]);
      return { all: all.length, attention: attention.length, todo: all.filter((item) => item.todo_state === "todo").length };
    },
  });
  const navigation = [
    ["/", "Attention", "◉"],
    ["/todos", "Todos", "✓"],
    ["/interests", "Monitoring", "✳"],
    ["/library", "All items", "▤"],
    ["/activity", "Activity", "↗"],
  ];
  return (
    <div className="workspace">
      <header className="topbar">
        <div className="topbar-inner">
          <NavLink className="brand" to="/">
            <span className="brand-mark">a</span>
            <span>aicp</span>
            <span className="brand-dot">.</span>
          </NavLink>
          <div className="topbar-caption">Your attention, considered.</div>
          <nav className="main-navigation" aria-label="Main navigation">
            {navigation.map(([path, label, icon]) => (
              <NavLink end={path === "/"} key={path} to={path}>
                <span aria-hidden="true">{icon}</span>
                {label}
                {path === "/" && <span className="navigation-count">{navigationCounts.data?.attention ?? ""}</span>}
                {path === "/todos" && <span className="navigation-count">{navigationCounts.data?.todo ?? ""}</span>}
                {path === "/library" && <span className="navigation-count">{navigationCounts.data?.all ?? ""}</span>}
              </NavLink>
            ))}
          </nav>
          <div className="topbar-actions">
            <div className="topbar-status">
              <span
                className={status.isError ? "status-dot offline" : "status-dot"}
              />
              <span className="status-label">
                {status.isError
                  ? "Server unavailable"
                  : status.data
                    ? "Local workspace"
                    : "Connecting…"}
              </span>
            </div>
            <NavLink className="preferences-link" to="/preferences">
              Preferences
            </NavLink>
          </div>
        </div>
      </header>
      <main>
        {status.isError && (
          <Alert severity="warning">
            The local server is unavailable. Start aicp serve to reconnect.
          </Alert>
        )}
        <Routes>
          <Route path="/" element={<ItemWorkspace mode="attention" />} />
          <Route path="/todos" element={<ItemWorkspace mode="todo" />} />
          <Route path="/interests" element={<Interests />} />
          <Route path="/library" element={<ItemWorkspace mode="library" />} />
          <Route path="/activity" element={<Activity />} />
          <Route path="/preferences" element={<Preferences />} />
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
