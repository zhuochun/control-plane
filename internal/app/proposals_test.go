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

func TestProposalReviewQueueSnoozeMergeAndExpiry(t *testing.T){
	ctx:=context.Background();s,err:=store.Open(ctx,t.TempDir());if err!=nil{t.Fatal(err)};defer s.Close();a:=New(s)
	now:=time.Date(2026,9,27,0,0,0,0,time.UTC);a.Now=func()time.Time{return now}
	confidence:=0.8
	makeProposal:=func(key string,expires *time.Time)Proposal{
		raw,err:=a.CreateProposal(ctx,CreateProposal{ProposalKey:key,TargetType:"interest",Operation:"create",Payload:json.RawMessage(`{"slug":"potential-focus","title":"Potential focus"}`),RationaleMD:"Repeated evidence",EvidenceLinks:[]string{"https://example.com/evidence"},Confidence:&confidence,ExpiresAt:expires})
		if err!=nil{t.Fatal(err)};var item Proposal;if err=json.Unmarshal(raw,&item);err!=nil{t.Fatal(err)};return item
	}
	first:=makeProposal("candidate:first",nil);second:=makeProposal("candidate:second",nil)
	if len(first.EvidenceLinks)!=1||first.Confidence==nil||*first.Confidence!=0.8{t.Fatalf("evidence metadata lost: %+v",first)}
	snooze:=now.Add(24*time.Hour)
	if _,err=a.ResolveProposal(ctx,first.ID,ResolveProposal{Resolution:"snoozed",SnoozeUntil:&snooze});err!=nil{t.Fatal(err)}
	pending,err:=a.Proposals(ctx,"pending");if err!=nil||len(pending)!=1||pending[0].ID!=second.ID{t.Fatalf("snoozed proposal still in queue: %+v %v",pending,err)}
	if _,err=a.ResolveProposal(ctx,first.ID,ResolveProposal{Resolution:"merged",MergeInto:second.ID});err!=nil{t.Fatal(err)}
	merged,err:=a.Proposal(ctx,first.ID);if err!=nil||merged.MergedInto==nil||*merged.MergedInto!=second.ID{t.Fatalf("merge decision lost: %+v %v",merged,err)}
	if _,err=a.ResolveProposal(ctx,second.ID,ResolveProposal{Resolution:"rejected"});err!=nil{t.Fatal(err)}
	expiry:=now.Add(time.Hour);third:=makeProposal("candidate:expiring",&expiry)
	now=now.Add(2*time.Hour)
	pending,err=a.Proposals(ctx,"pending");if err!=nil||len(pending)!=0{t.Fatalf("expired proposal still actionable: %+v %v",pending,err)}
	if _,err=a.ResolveProposal(ctx,third.ID,ResolveProposal{Resolution:"accepted"});err==nil{t.Fatal("accepted expired proposal")}
}

func TestMultiRecordProposalAppliesAtomically(t *testing.T){
	ctx:=context.Background();s,err:=store.Open(ctx,t.TempDir());if err!=nil{t.Fatal(err)};defer s.Close();a:=New(s)
	plan:=json.RawMessage(`{"operations":[{"target_type":"interest","operation":"create","payload":{"slug":"release-focus","title":"Release focus"}},{"target_type":"watch","operation":"create","payload":{"slug":"release-feed","matching_policy":"explicit","interest_ids":["release-focus"],"source":{"kind":"web","locator":"releases"}}}]}`)
	raw,err:=a.CreateProposal(ctx,CreateProposal{ProposalKey:"plan:release",TargetType:"config_plan",Operation:"apply",Payload:plan,RationaleMD:"Cover the release feed"})
	if err!=nil{t.Fatal(err)}
	var proposal Proposal;if err=json.Unmarshal(raw,&proposal);err!=nil{t.Fatal(err)}
	if _,err=a.ResolveProposal(ctx,proposal.ID,ResolveProposal{Resolution:"accepted"});err!=nil{t.Fatal(err)}
	watch,err:=a.Watch(ctx,"release-feed");if err!=nil{t.Fatal(err)}
	interest,err:=a.Interest(ctx,"release-focus");if err!=nil{t.Fatal(err)}
	if len(watch.InterestIDs)!=1||watch.InterestIDs[0]!=interest.ID{t.Fatalf("proposal plan link failed: %+v",watch)}
	bad:=json.RawMessage(`{"operations":[{"target_type":"interest","operation":"create","payload":{"slug":"first-added","title":"First"}},{"target_type":"interest","operation":"create","payload":{"slug":"release-focus","title":"Duplicate"}}]}`)
	raw,err=a.CreateProposal(ctx,CreateProposal{ProposalKey:"plan:bad",TargetType:"config_plan",Operation:"apply",Payload:bad,RationaleMD:"Attempt duplicate"})
	if err!=nil{t.Fatal(err)}
	if err=json.Unmarshal(raw,&proposal);err!=nil{t.Fatal(err)}
	if _,err=a.ResolveProposal(ctx,proposal.ID,ResolveProposal{Resolution:"accepted"});err==nil{t.Fatal("accepted partially invalid plan")}
	if _,err=a.Interest(ctx,"first-added");err==nil{t.Fatal("failed plan partially committed")}
}
