package app

import (
	"context"
	"encoding/json"
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
	if saved.Timezone != "browser" || saved.AgentsMD != agents || saved.UserMD != user {
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
