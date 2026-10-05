package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"testing"
)

func TestReviewFormatImmutabilityAndInstanceValidation(t *testing.T) {
	ctx := context.Background()
	a, item, _ := reviewFixture(t)
	format, err := a.ReviewFormat(ctx, FormatRef{ID: "architecture", Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RegisterReviewFormat(ctx, RegisterReviewFormat{Format: format}); err != nil {
		t.Fatal(err)
	}
	format.Title = "Changed"
	if _, err := a.RegisterReviewFormat(ctx, RegisterReviewFormat{Format: format}); err == nil {
		t.Fatal("changed an immutable version")
	}
	format.Version++
	if _, err := a.RegisterReviewFormat(ctx, RegisterReviewFormat{Format: format}); err != nil {
		t.Fatal(err)
	}
	old, err := a.Item(ctx, item.ID)
	if err != nil || old.Report.Blocks[1].Format.Version != 1 {
		t.Fatalf("new format changed old Item: %v", err)
	}
	report := item.Report
	report.Blocks[1].Blocks[0].Required = false
	if _, err := a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: item.ContentVersion, Report: &report}); err == nil {
		t.Fatal("accepted format constraint mismatch")
	}
	format.Fields[0].Required = false
	format.Fields[0].Selection = ""
	format.Version++
	if _, err := a.RegisterReviewFormat(ctx, RegisterReviewFormat{Format: format}); err == nil {
		t.Fatal("registered invalid choice structure")
	}
}

func TestImmutableImageEvidenceSnapshot(t *testing.T) {
	ctx := context.Background()
	a, item, _ := reviewFixture(t)
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	input := UploadReviewArtifact{Data: base64.StdEncoding.EncodeToString(imageBytes.Bytes())}
	raw, err := a.UploadReviewArtifact(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var artifact ReviewArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatal(err)
	}
	replay, err := a.UploadReviewArtifact(ctx, input)
	if err != nil || string(replay) != string(raw) {
		t.Fatalf("image identity changed: %v", err)
	}
	media, stored, err := a.ReviewArtifact(ctx, artifact.ID)
	if err != nil || media != "image/png" || !bytes.Equal(stored, imageBytes.Bytes()) {
		t.Fatalf("stored evidence mismatch: %v", err)
	}
	item.Report.Blocks[1].Blocks[0].Options[0].Blocks = []ReportBlock{{ID: "preview", Type: "image", ArtifactID: artifact.ID, Description: "Local option preview"}}
	if _, err := a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: item.ContentVersion, Report: &item.Report}); err != nil {
		t.Fatal(err)
	}
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SubmitReviewAnswer(ctx, item.ID, reviewSubmission(item)); err != nil {
		t.Fatal(err)
	}
	item, err = a.Item(ctx, item.ID)
	if err != nil || item.Answers[0].Values[0].Options[0].Blocks[0].ArtifactID != artifact.ID {
		t.Fatalf("visual answer lost immutable evidence: %v", err)
	}
	if _, err := a.UploadReviewArtifact(ctx, UploadReviewArtifact{Data: base64.StdEncoding.EncodeToString([]byte("<svg><script/></svg>"))}); err == nil {
		t.Fatal("accepted executable image input")
	}
	if _, err := a.UploadReviewArtifact(ctx, UploadReviewArtifact{Data: input.Data[:len(input.Data)-8]}); err == nil {
		t.Fatal("accepted truncated image")
	}
}

func TestReportRejectsNestedInputsAndUnknownActionReferences(t *testing.T) {
	if err := validateReport(Report{SchemaVersion: 1, Blocks: []ReportBlock{{ID: "input", Type: "choice"}}}); err == nil {
		t.Fatal("version 1 accepted rich blocks")
	}
	if err := validateReport(Report{SchemaVersion: 2, BodyMD: "fallback", Blocks: []ReportBlock{{ID: "actions", Type: "actions", ActionIDs: []string{"missing"}}}}); err == nil {
		t.Fatal("accepted missing action reference")
	}
	if err := validateReport(Report{SchemaVersion: 2, BodyMD: "fallback", Blocks: []ReportBlock{{ID: "input", Type: "text_input", Question: "Outside a review"}}}); err == nil {
		t.Fatal("accepted input without submission group")
	}
	if err := validateReport(Report{SchemaVersion: 2, BodyMD: "fallback", Blocks: []ReportBlock{{ID: "prose", Type: "markdown", Question: "Hidden question"}}}); err == nil {
		t.Fatal("accepted control properties in Markdown")
	}
}
