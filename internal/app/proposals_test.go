package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestProposalAcceptanceRejectionAndStaleTarget(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	a.Now = func() time.Time { return time.Date(2026, 9, 15, 4, 0, 0, 0, time.UTC) }
	create := CreateProposal{RequestID: "propose-create", ProposalKey: "interest:delivery:v1", TargetType: "interest", Operation: "create", Payload: json.RawMessage(`{"title":"Delivery","instructions_md":"Notice consequential delivery changes."}`), RationaleMD: "Repeated delivery decisions need one home."}
	raw, err := a.CreateProposal(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	var proposal Proposal
	_ = json.Unmarshal(raw, &proposal)
	if _, err = a.ResolveProposal(ctx, proposal.ID, ResolveProposal{RequestID: "accept", Resolution: "accepted"}); err != nil {
		t.Fatal(err)
	}
	interests, err := a.Interests(ctx)
	if err != nil || len(interests) != 1 || interests[0].Title != "Delivery" {
		t.Fatalf("accepted creation missing: %+v %v", interests, err)
	}
	if _, err = a.ResolveProposal(ctx, proposal.ID, ResolveProposal{RequestID: "accept-again", Resolution: "accepted"}); err != nil {
		t.Fatal("repeat acceptance should return resolved proposal:", err)
	}
	revision := int64(1)
	target := interests[0].ID
	stale := CreateProposal{RequestID: "propose-stale", ProposalKey: "interest:delivery:deprecate:v1", TargetType: "interest", TargetID: &target, ExpectedRevision: &revision, Operation: "deprecate", RationaleMD: "The source is no longer used."}
	raw, err = a.CreateProposal(ctx, stale)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &proposal)
	otherTarget := "another-interest"
	differentTarget := stale
	differentTarget.RequestID = "same-key-different-target"
	differentTarget.TargetID = &otherTarget
	if _, err = a.CreateProposal(ctx, differentTarget); err == nil {
		t.Fatal("proposal key replayed for a different target")
	}
	if _, err = a.UpdateInterest(ctx, target, UpdateInterest{RequestID: "human-edit", ExpectedRevision: 1, Title: Field[string]{Set: true, Value: "Delivery systems"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ResolveProposal(ctx, proposal.ID, ResolveProposal{RequestID: "accept-stale", Resolution: "accepted"}); err == nil {
		t.Fatal("accepted stale proposal")
	}
	proposal, err = a.Proposal(ctx, proposal.ID)
	if err != nil || proposal.State != "pending" {
		t.Fatalf("stale proposal did not remain pending: %+v %v", proposal, err)
	}
	if _, err = a.ResolveProposal(ctx, proposal.ID, ResolveProposal{RequestID: "reject-stale", Resolution: "rejected"}); err != nil {
		t.Fatal(err)
	}
}
