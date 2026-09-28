package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestSettingsDefaultToBrowserAndTravelWithBrief(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)

	settings, err := a.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Timezone != "browser" {
		t.Fatalf("default timezone = %q, want browser", settings.Timezone)
	}
	if settings.AgentsMD == "" || settings.UserMD == "" {
		t.Fatalf("default contexts are empty: %+v", settings)
	}
	if settings.DefaultAgentsMD != settings.AgentsMD || settings.DefaultUserMD != settings.UserMD {
		t.Fatalf("fresh contexts differ from reset defaults: %+v", settings)
	}

	brief, err := a.Brief(ctx)
	if err != nil {
		t.Fatal(err)
	}
	contexts, ok := brief["contexts"].(map[string]string)
	if !ok || contexts["AGENTS.md"] != settings.AgentsMD || contexts["USER.md"] != settings.UserMD {
		t.Fatalf("brief contexts do not match settings: %#v", brief["contexts"])
	}

	agents := "# Agent rules\n\nKeep it short."
	user := "# Owner\n\nPrefer evidence before action."
	raw, err := a.SetSettings(ctx, SetSettings{
		RequestID: "contexts",
		Timezone:  "browser",
		AgentsMD:  &agents,
		UserMD:    &user,
	})
	if err != nil {
		t.Fatal(err)
	}
	var saved Settings
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Timezone != "browser" || saved.AgentsMD != agents || saved.UserMD != user ||
		saved.DefaultAgentsMD != settings.DefaultAgentsMD || saved.DefaultUserMD != settings.DefaultUserMD {
		t.Fatalf("saved settings mismatch: %+v", saved)
	}

	brief, err = a.Brief(ctx)
	if err != nil {
		t.Fatal(err)
	}
	contexts, ok = brief["contexts"].(map[string]string)
	if !ok || contexts["AGENTS.md"] != agents || contexts["USER.md"] != user {
		t.Fatalf("updated contexts missing from brief: %#v", brief["contexts"])
	}
}

func TestContextLimitsApplyToChangedUTF8FieldsWithoutDiscardingLegacyText(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	settings, err := a.Settings(ctx)
	if err != nil || settings.AgentsMDMaxBytes != AgentsMDMaxBytes || settings.UserMDMaxBytes != UserMDMaxBytes {
		t.Fatalf("context limits missing from settings: %+v %v", settings, err)
	}
	agents := strings.Repeat("a", AgentsMDMaxBytes)
	user := strings.Repeat("é", UserMDMaxBytes/2)
	if _, err = a.SetSettings(ctx, SetSettings{Timezone: "browser", AgentsMD: &agents, UserMD: &user}); err != nil {
		t.Fatalf("rejected valid UTF-8 byte boundaries: %v", err)
	}
	tooLongAgents := agents + "a"
	if _, err = a.SetSettings(ctx, SetSettings{Timezone: "browser", AgentsMD: &tooLongAgents}); err == nil {
		t.Fatal("accepted oversized AGENTS.md")
	}
	tooLongUser := user + "é"
	if _, err = a.SetUserContext(ctx, SetUserContext{UserMD: &tooLongUser}); err == nil {
		t.Fatal("accepted oversized USER.md from direct owner-context path")
	}
	if _, err = a.SetSettings(ctx, SetSettings{Timezone: "browser", UserMD: &tooLongUser}); err == nil {
		t.Fatal("accepted oversized USER.md from preferences path")
	}
	legacyAgents := strings.Repeat("x", AgentsMDMaxBytes+1)
	if _, err = s.DB.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='agents_md'", legacyAgents); err != nil {
		t.Fatal(err)
	}
	if _, err = a.SetSettings(ctx, SetSettings{Timezone: "UTC"}); err != nil {
		t.Fatalf("timezone edit should retain grandfathered context: %v", err)
	}
	if _, err = a.SetSettings(ctx, SetSettings{Timezone: "UTC", AgentsMD: &legacyAgents}); err != nil {
		t.Fatalf("unchanged grandfathered context should remain valid: %v", err)
	}
	settings, err = a.Settings(ctx)
	if err != nil || settings.AgentsMD != legacyAgents || settings.UserMD != user || settings.Timezone != "UTC" {
		t.Fatalf("grandfathered context changed: %+v %v", settings, err)
	}
}
