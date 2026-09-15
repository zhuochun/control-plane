import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Alert, Button, TextField } from "@mui/material";
import { api } from "./api";
import {
  browserTimezone,
  defaultAgentContext,
  defaultUserContext,
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
  const [agentsMD, setAgentsMD] = useState(defaultAgentContext);
  const [userMD, setUserMD] = useState(defaultUserContext);

  useEffect(() => {
    if (!settings.data || loaded) return;
    setTimezone(settings.data.timezone || "browser");
    setAgentsMD(settings.data.agents_md || defaultAgentContext);
    setUserMD(settings.data.user_md || defaultUserContext);
    setLoaded(true);
  }, [loaded, settings.data]);

  const save = useMutation({
    mutationFn: () =>
      api<Settings>("/settings", {
        request_id: crypto.randomUUID(),
        timezone,
        agents_md: agentsMD,
        user_md: userMD,
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
              A short operating note included with every brief and run start.
            </p>
            <TextField
              fullWidth
              multiline
              minRows={9}
              label="Agent instructions"
              value={agentsMD}
              onChange={(event) => {
                setAgentsMD(event.target.value);
                save.reset();
              }}
              disabled={!settings.data}
            />
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
              The human context that helps an agent judge relevance, tone, and
              follow-through.
            </p>
            <TextField
              fullWidth
              multiline
              minRows={9}
              label="Owner context"
              value={userMD}
              onChange={(event) => {
                setUserMD(event.target.value);
                save.reset();
              }}
              disabled={!settings.data}
            />
            <small className="field-hint">
              Add only what you want agents to use.
            </small>
          </section>
        </div>

        <div className="preference-actions">
          <span className="preference-save-note">
            These context files are sent as text; aicp never runs them.
          </span>
          <Button
            type="submit"
            variant="contained"
            disabled={save.isPending || !settings.data}
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
