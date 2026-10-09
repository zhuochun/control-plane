package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"strings"
)

// Delegation is Item-local handoff context, not an execution engine.
type Delegation struct {
	ID             string `json:"id"`
	Executor       string `json:"executor"`
	ExternalRef    string `json:"external_ref,omitempty"`
	InstructionsMD string `json:"instructions_md"`
	Status         string `json:"status"`
	ContextMD      string `json:"context_md,omitempty"`
}

// DelegationPatch merges supplied fields by ID. Empty lists never remove work.
type DelegationPatch struct {
	ID             string  `json:"id"`
	Executor       *string `json:"executor,omitempty"`
	ExternalRef    *string `json:"external_ref,omitempty"`
	InstructionsMD *string `json:"instructions_md,omitempty"`
	Status         *string `json:"status,omitempty"`
	ContextMD      *string `json:"context_md,omitempty"`
}

// UpdateItemWork cannot create an Item or change its provenance/user state.
type UpdateItemWork struct {
	RequestID              string            `json:"request_id,omitempty"`
	ExpectedContentVersion int64             `json:"expected_content_version"`
	Title                  *string           `json:"title,omitempty"`
	Summary                *string           `json:"summary,omitempty"`
	Reason                 string            `json:"reason,omitempty"`
	Report                 *Report           `json:"report,omitempty"`
	ContextMD              *string           `json:"context_md,omitempty"`
	Sources                []Source          `json:"sources,omitempty"`
	Delegations            []DelegationPatch `json:"delegations,omitempty"`
}

func mergeDelegations(old []Delegation, patches []DelegationPatch) ([]Delegation, error) {
	result := append([]Delegation(nil), old...)
	positions := map[string]int{}
	for i, entry := range result {
		positions[entry.ID] = i
	}
	seen := map[string]bool{}
	for _, patch := range patches {
		if strings.TrimSpace(patch.ID) == "" || seen[patch.ID] {
			return nil, Invalid("delegations need unique nonempty ids")
		}
		seen[patch.ID] = true
		index, exists := positions[patch.ID]
		entry := Delegation{ID: patch.ID}
		if exists {
			entry = result[index]
		}
		for _, field := range []struct {
			target *string
			value  *string
		}{
			{&entry.Executor, patch.Executor}, {&entry.ExternalRef, patch.ExternalRef},
			{&entry.InstructionsMD, patch.InstructionsMD}, {&entry.Status, patch.Status}, {&entry.ContextMD, patch.ContextMD},
		} {
			if field.value != nil {
				*field.target = *field.value
			}
		}
		if strings.TrimSpace(entry.Executor) == "" || strings.TrimSpace(entry.InstructionsMD) == "" {
			return nil, Invalid("delegations need executor and instructions_md")
		}
		if entry.Status != "pending" && entry.Status != "blocked" && entry.Status != "closed" {
			return nil, Invalid("delegation status must be pending, blocked, or closed")
		}
		if (entry.Status == "blocked" || entry.ExternalRef == "") && strings.TrimSpace(entry.ContextMD) == "" {
			return nil, Invalid("blocked or unreferenced delegations need explanatory context_md")
		}
		if exists {
			result[index] = entry
		} else {
			positions[entry.ID] = len(result)
			result = append(result, entry)
		}
	}
	return result, nil
}

func (a *App) UpdateItemWork(ctx context.Context, id string, input UpdateItemWork) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "PATCH /items/"+id+"/work", input, func(tx *sql.Tx) (any, error) {
		current, err := getItem(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		body := PutItem{ExpectedContentVersion: input.ExpectedContentVersion,
			DedupeKey: current.DedupeKey, Kind: current.Kind, Title: current.Title, Summary: current.Summary,
			WatchID: current.WatchID, ParentID: current.ParentID, Interests: current.Interests,
			Sources: current.Sources, ContextMD: current.ContextMD, Report: current.Report,
			Delegations: input.Delegations, workUpdate: true, editReason: input.Reason}
		if input.Title != nil {
			body.Title = *input.Title
		}
		if input.Summary != nil {
			body.Summary = *input.Summary
		}
		if input.Report != nil {
			body.Report = *input.Report
		}
		if input.ContextMD != nil {
			body.ContextMD = *input.ContextMD
		}
		// Evidence additions cannot alter or remove an existing source reference.
		known := map[string]bool{}
		for _, source := range current.Sources {
			known[source.ID] = true
		}
		for _, source := range input.Sources {
			if known[source.ID] {
				return nil, Invalid("work sources must use new evidence ids")
			}
			parsed, parseErr := url.Parse(source.URL)
			if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return nil, Invalid("work evidence needs an HTTP(S) URL")
			}
			body.Sources = append(body.Sources, source)
			known[source.ID] = true
		}
		return a.putItemTx(ctx, tx, current.ID, body, current.Origin)
	})
}

func delegationFilter(filter ItemFilters) (string, []any, error) {
	if filter.DelegationStatus == "" && filter.Executor == "" && filter.ExternalRef == "" {
		return "", nil, nil
	}
	parts := []string{}
	args := []any{}
	if filter.DelegationStatus != "" {
		statuses := strings.Split(filter.DelegationStatus, ",")
		marks := []string{}
		for _, status := range statuses {
			if status != "pending" && status != "blocked" && status != "closed" {
				return "", nil, Invalid("delegation_status must list pending, blocked, or closed")
			}
			marks = append(marks, "?")
			args = append(args, status)
		}
		parts = append(parts, "json_extract(d.value,'$.status') IN ("+strings.Join(marks, ",")+")")
	}
	for _, field := range []struct{ name, value string }{{"executor", filter.Executor}, {"external_ref", filter.ExternalRef}} {
		if field.value != "" {
			parts = append(parts, "json_extract(d.value,'$."+field.name+"')=?")
			args = append(args, field.value)
		}
	}
	return " AND EXISTS (SELECT 1 FROM json_each(items.content,'$.delegations') d WHERE " + strings.Join(parts, " AND ") + ")", args, nil
}

func markDelegationMatches(item *Item, filter ItemFilters) {
	if filter.DelegationStatus == "" && filter.Executor == "" && filter.ExternalRef == "" {
		return
	}
	for _, entry := range item.Delegations {
		if filter.Executor != "" && entry.Executor != filter.Executor {
			continue
		}
		if filter.ExternalRef != "" && entry.ExternalRef != filter.ExternalRef {
			continue
		}
		if filter.DelegationStatus != "" && !strings.Contains(","+filter.DelegationStatus+",", ","+entry.Status+",") {
			continue
		}
		item.MatchingDelegationIDs = append(item.MatchingDelegationIDs, entry.ID)
	}
}
