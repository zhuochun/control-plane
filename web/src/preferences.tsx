import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Alert, Button, TextField } from "@mui/material";
import { api } from "./api";
import {
  browserTimezone,
  effectiveTimezone,
  Settings,
  timezoneLabel,
  useSettings,
} from "./settings";

export function Preferences() {
  const cache = useQueryClient();
  const settings = useSettings();
  const [loaded, setLoaded] = useState(false);
  const [timezone, setTimezone] = useState("browser");
  const [agentsMD, setAgentsMD] = useState("");
  const [userMD, setUserMD] = useState("");

  useEffect(() => {
    if (!settings.data || loaded) return;
    setTimezone(settings.data.timezone || "browser");
    setAgentsMD(settings.data.agents_md);
    setUserMD(settings.data.user_md);
    setLoaded(true);
  }, [loaded, settings.data]);

  const save = useMutation({
    mutationFn: () =>
      api<Settings>("/settings", {
        request_id: crypto.randomUUID(),
        timezone,
        ...(agentsMD !== settings.data?.agents_md
          ? { agents_md: agentsMD }
          : {}),
        ...(userMD !== settings.data?.user_md ? { user_md: userMD } : {}),
      }),
    onSuccess: (result) => {
      cache.setQueryData(["settings"], result);
      setTimezone(result.timezone);
      setAgentsMD(result.agents_md);
      setUserMD(result.user_md);
    },
  });

  const browserZone = browserTimezone();
  const displayZone = effectiveTimezone(timezone);
  const agentsBytes = new TextEncoder().encode(agentsMD).length;
  const userBytes = new TextEncoder().encode(userMD).length;
  const agentsOverLimit =
    agentsBytes > (settings.data?.agents_md_max_bytes ?? 8192);
  const userOverLimit = userBytes > (settings.data?.user_md_max_bytes ?? 16384);
  const changedContextOverLimit =
    (agentsMD !== settings.data?.agents_md && agentsOverLimit) ||
    (userMD !== settings.data?.user_md && userOverLimit);

  return (
    <>
      <header className="page-header">
        <div className="eyebrow">Make the workspace yours</div>
        <h1>Preferences</h1>
        <p>Set how aicp speaks to you and what every agent should know.</p>
      </header>

      {settings.isError && (
        <Alert severity="error">
          Cannot load preferences. Check that aicp is running.
        </Alert>
      )}

      <form
        onSubmit={(event) => {
          event.preventDefault();
          save.mutate();
        }}
      >
        <section className="panel preference-section timezone-section">
          <div className="section-heading">
            <div>
              <div className="eyebrow">Dates and reminders</div>
              <h2>Display timezone</h2>
            </div>
            <span className="tag">{timezoneLabel(timezone)}</span>
          </div>
          <p>
            Dates follow this preference. A reminder keeps the instant you
            chose, even if you change the display later.
          </p>
          <div className="timezone-choice">
            <TextField
              fullWidth
              label="IANA timezone"
              value={timezone === "browser" ? browserZone : timezone}
              onChange={(event) => {
                setTimezone(event.target.value);
                save.reset();
              }}
              helperText={
                timezone === "browser"
                  ? `Using ${browserZone} from this browser.`
                  : "For example, Asia/Singapore."
              }
              disabled={!settings.data}
            />
            <Button
              type="button"
              variant={timezone === "browser" ? "contained" : "outlined"}
              onClick={() => {
                setTimezone("browser");
                save.reset();
              }}
              disabled={!settings.data}
            >
              Use browser default
            </Button>
          </div>
          <div className="preference-note">
            <span>Current display</span>
            <strong>{displayZone}</strong>
          </div>
        </section>

        <div className="preference-context-grid">
          <section className="panel preference-section context-panel">
            <div className="context-heading">
              <div>
                <code>AGENTS.md</code>
                <h2>How to work with aicp</h2>
              </div>
              <span className="tag">Agent-facing</span>
            </div>
            <p>
              {settings.data?.context_guidance?.agents_md ??
                "Stable operating rules for agents working with aicp."}
            </p>
            <TextField
              fullWidth
              multiline
              minRows={9}
              label="Agent instructions"
              value={agentsMD}
              error={agentsOverLimit}
              helperText={`${agentsBytes} / ${settings.data?.agents_md_max_bytes ?? 8192} bytes. Keep reusable guidance concise.`}
              onChange={(event) => {
                setAgentsMD(event.target.value);
                save.reset();
              }}
              disabled={!settings.data}
            />
            <Button
              type="button"
              variant="text"
              onClick={() => {
                setAgentsMD(settings.data!.default_agents_md);
                save.reset();
              }}
              disabled={!settings.data}
            >
              Reset to default
            </Button>
            <small className="field-hint">
              Keep it stable and operational.
            </small>
          </section>

          <section className="panel preference-section context-panel">
            <div className="context-heading">
              <div>
                <code>USER.md</code>
                <h2>About the owner</h2>
              </div>
              <span className="tag">Agent-facing</span>
            </div>
            <p>
              {settings.data?.context_guidance?.user_md ??
                "Durable owner priorities, preferences, and constraints."}
            </p>
            <TextField
              fullWidth
              multiline
              minRows={9}
              label="Owner context"
              value={userMD}
              error={userOverLimit}
              helperText={`${userBytes} / ${settings.data?.user_md_max_bytes ?? 16384} bytes. Include only durable owner context.`}
              onChange={(event) => {
                setUserMD(event.target.value);
                save.reset();
              }}
              disabled={!settings.data}
            />
            <Button
              type="button"
              variant="text"
              onClick={() => {
                setUserMD(settings.data!.default_user_md);
                save.reset();
              }}
              disabled={!settings.data}
            >
              Reset to default
            </Button>
            <small className="field-hint">
              Add only what you want agents to use.
            </small>
          </section>
        </div>

        {settings.data?.context_guidance && (
          <section
            className="panel preference-section"
            aria-label="Instruction editing guidance"
          >
            <h2>When to update these instructions</h2>
            <p>{settings.data.context_guidance.update_policy}</p>
            <p>{settings.data.context_guidance.item_context}</p>
          </section>
        )}

        <div className="preference-actions">
          <span className="preference-save-note">
            These context files are sent as text; aicp never runs them.
          </span>
          <Button
            type="submit"
            variant="contained"
            disabled={
              save.isPending || !settings.data || changedContextOverLimit
            }
          >
            {save.isPending ? "Saving…" : "Save preferences"}
          </Button>
        </div>
        {save.isError && <Alert severity="error">{save.error.message}</Alert>}
        {save.isSuccess && <Alert severity="success">Preferences saved.</Alert>}
      </form>
    </>
  );
}
