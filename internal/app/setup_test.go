package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestSetupIsDerivedFromCurrentConfiguration(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	check := func(want SetupStatus) {
		t.Helper()
		health, err := a.OperationalHealth(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got := health["setup"].(SetupStatus)
		if got != want {
			t.Fatalf("setup = %+v, want %+v", got, want)
		}
	}
	check(SetupStatus{})
	interestRaw, err := a.CreateInterest(ctx, CreateInterest{Title: "Delivery"})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	if err = json.Unmarshal(interestRaw, &interest); err != nil {
		t.Fatal(err)
	}
	check(SetupStatus{HasInterest: true})
	watchRaw, err := a.CreateWatch(ctx, CreateWatch{MatchingPolicy: "explicit", InterestIDs: []string{interest.ID}, Source: WatchSource{Kind: "fixture", Locator: "inbox"}})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	if err = json.Unmarshal(watchRaw, &watch); err != nil {
		t.Fatal(err)
	}
	check(SetupStatus{HasInterest: true, HasWatcher: true})
	settings, err := a.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.SetUserContext(ctx, SetUserContext{UserMD: &settings.DefaultUserMD}); err != nil {
		t.Fatal(err)
	}
	check(SetupStatus{HasInterest: true, HasWatcher: true})
	owner := "# Owner\n\nFocus on delivery."
	if _, err = a.SetUserContext(ctx, SetUserContext{UserMD: &owner}); err != nil {
		t.Fatal(err)
	}
	check(SetupStatus{UserContextSet: true, HasInterest: true, HasWatcher: true, Done: true})
	if _, err = s.DB.ExecContext(ctx, "UPDATE watches SET valid_until=? WHERE id=?", time.Now().Add(-time.Hour).UnixMilli(), watch.ID); err != nil {
		t.Fatal(err)
	}
	check(SetupStatus{UserContextSet: true, HasInterest: true})
	if _, err = s.DB.ExecContext(ctx, "UPDATE interests SET state='paused' WHERE id=?", interest.ID); err != nil {
		t.Fatal(err)
	}
	check(SetupStatus{UserContextSet: true})
}

func TestSetUserContextPreservesOtherSettings(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	agents := "# Custom agent note"
	if _, err = a.SetSettings(ctx, SetSettings{Timezone: "Asia/Singapore", AgentsMD: &agents}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.SetUserContext(ctx, SetUserContext{}); err == nil {
		t.Fatal("missing user_md must not clear context")
	}
	owner := "# Owner"
	if _, err = a.SetUserContext(ctx, SetUserContext{UserMD: &owner}); err != nil {
		t.Fatal(err)
	}
	settings, err := a.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Timezone != "Asia/Singapore" || settings.AgentsMD != agents || settings.UserMD != "# Owner" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}
