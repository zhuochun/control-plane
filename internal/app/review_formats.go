package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ReportBlock is a discriminated document node. Validation rejects fields that
// do not belong to its type and limits nesting to reviews and display options.
type ReportBlock struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	BodyMD        string         `json:"body_md,omitempty"`
	Title         string         `json:"title,omitempty"`
	Blocks        []ReportBlock  `json:"blocks,omitempty"`
	Format        *FormatRef     `json:"format,omitempty"`
	SubmitLabel   string         `json:"submit_label,omitempty"`
	Question      string         `json:"question,omitempty"`
	Required      bool           `json:"required,omitempty"`
	Selection     string         `json:"selection,omitempty"`
	MinSelections int            `json:"min_selections,omitempty"`
	MaxSelections int            `json:"max_selections,omitempty"`
	Options       []ReviewOption `json:"options,omitempty"`
	Language      string         `json:"language,omitempty"`
	Source        string         `json:"source,omitempty"`
	ArtifactID    string         `json:"artifact_id,omitempty"`
	Caption       string         `json:"caption,omitempty"`
	Description   string         `json:"description,omitempty"`
	ActionIDs     []string       `json:"action_ids,omitempty"`
}

type ReviewOption struct {
	ID     string        `json:"id"`
	Label  string        `json:"label"`
	Blocks []ReportBlock `json:"blocks,omitempty"`
}

type FormatRef struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// FormatField fixes structure and constraints; instance wording and evidence
// remain in the Item. There is no substitution or expression language.
type FormatField struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Required      bool   `json:"required,omitempty"`
	Selection     string `json:"selection,omitempty"`
	MinSelections int    `json:"min_selections,omitempty"`
	MaxSelections int    `json:"max_selections,omitempty"`
}

type ReviewFormat struct {
	ID      string        `json:"id"`
	Version int           `json:"version"`
	Title   string        `json:"title"`
	Fields  []FormatField `json:"fields"`
}

type RegisterReviewFormat struct {
	RequestID string       `json:"request_id,omitempty"`
	Format    ReviewFormat `json:"format"`
}

var blockFields = map[string][]string{
	"markdown":   {"body_md"},
	"image":      {"artifact_id", "caption", "description"},
	"diagram":    {"language", "source", "caption", "description"},
	"review":     {"title", "blocks", "format", "submit_label"},
	"choice":     {"question", "required", "selection", "min_selections", "max_selections", "options"},
	"text_input": {"question", "required"},
	"actions":    {"action_ids"},
}

func validReviewID(id string) bool {
	return strings.TrimSpace(id) != "" && len(id) <= 100
}

func validateBlockShape(block ReportBlock) error {
	allowed, exists := blockFields[block.Type]
	if !exists || !validReviewID(block.ID) {
		return Invalid("blocks need supported types and nonempty ids of at most 100 bytes")
	}
	data, err := json.Marshal(block)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for field := range fields {
		if field != "id" && field != "type" && !slices.Contains(allowed, field) {
			return Invalid(fmt.Sprintf("%s is not allowed on %s blocks", field, block.Type))
		}
	}
	return nil
}

func validateSelection(selection string, required bool, minimum, maximum int) error {
	if selection != "single" && selection != "multiple" {
		return Invalid("choice selection must be single or multiple")
	}
	if minimum < 0 || maximum < 0 || (maximum > 0 && minimum > maximum) {
		return Invalid("invalid choice selection limits")
	}
	if selection == "single" && (minimum > 1 || maximum > 1) {
		return Invalid("single choice allows at most one selection")
	}
	if !required && minimum > 0 {
		return Invalid("minimum selections require a required field")
	}
	return nil
}

func validateReport(report Report) error {
	if report.SchemaVersion == 1 {
		if len(report.Blocks) > 0 {
			return Invalid("report version 1 cannot contain blocks")
		}
		return nil
	}
	if report.SchemaVersion != 2 || strings.TrimSpace(report.BodyMD) == "" || len(report.Blocks) == 0 {
		return Invalid("report version 2 requires fallback body_md and blocks")
	}
	actions := map[string]bool{}
	for _, action := range report.Actions {
		actions[action.ID] = true
	}
	if err := validateBlocks(report.Blocks, "report", actions); err != nil {
		return err
	}
	count, inputs := 0, 0
	return walkBlocks(report.Blocks, func(block ReportBlock) error {
		count++
		if block.Type == "choice" || block.Type == "text_input" {
			inputs++
		}
		if count > 300 || inputs > 100 {
			return Invalid("report allows at most 300 total blocks and 100 input fields")
		}
		return nil
	})
}

func validateBlocks(blocks []ReportBlock, scope string, actions map[string]bool) error {
	if len(blocks) > 100 {
		return Invalid("a block collection allows at most 100 entries")
	}
	seen := map[string]bool{}
	for _, block := range blocks {
		if err := validateBlockShape(block); err != nil {
			return err
		}
		if seen[block.ID] {
			return Invalid("block ids must be unique within their enclosing scope")
		}
		seen[block.ID] = true
		switch block.Type {
		case "markdown":
		case "image":
			if block.ArtifactID == "" || strings.TrimSpace(block.Description) == "" {
				return Invalid("images need an artifact_id and description")
			}
		case "diagram":
			if (block.Language != "mermaid" && block.Language != "plantuml") || strings.TrimSpace(block.Source) == "" || len(block.Source) > 32<<10 || strings.TrimSpace(block.Description) == "" {
				return Invalid("diagrams need a supported language, source up to 32 KiB, and description")
			}
		case "review":
			if scope != "report" || block.Title == "" || len(block.Blocks) == 0 {
				return Invalid("reviews need a title and blocks and cannot be nested")
			}
			if block.Format != nil && (!validReviewID(block.Format.ID) || block.Format.Version < 1) {
				return Invalid("review format needs an id and positive version")
			}
			if err := validateBlocks(block.Blocks, "review", actions); err != nil {
				return err
			}
			if !slices.ContainsFunc(block.Blocks, func(b ReportBlock) bool { return b.Type == "choice" || b.Type == "text_input" }) {
				return Invalid("review needs at least one input field")
			}
		case "choice":
			if scope != "review" || strings.TrimSpace(block.Question) == "" || len(block.Options) == 0 || len(block.Options) > 100 {
				return Invalid("choice fields belong in a review and need a question and 1–100 options")
			}
			if err := validateSelection(block.Selection, block.Required, block.MinSelections, block.MaxSelections); err != nil {
				return err
			}
			if block.MinSelections > len(block.Options) {
				return Invalid("minimum selections exceed available options")
			}
			options := map[string]bool{}
			for _, option := range block.Options {
				if !validReviewID(option.ID) || strings.TrimSpace(option.Label) == "" || options[option.ID] {
					return Invalid("options need unique ids and labels")
				}
				options[option.ID] = true
				if err := validateBlocks(option.Blocks, "option", actions); err != nil {
					return err
				}
			}
		case "text_input":
			if scope != "review" || strings.TrimSpace(block.Question) == "" {
				return Invalid("text inputs belong in a review and need a question")
			}
		case "actions":
			if scope != "report" || len(block.ActionIDs) == 0 {
				return Invalid("action blocks belong in the report and need action_ids")
			}
			ids := map[string]bool{}
			for _, id := range block.ActionIDs {
				if !actions[id] || ids[id] {
					return Invalid("action blocks need unique existing action_ids")
				}
				ids[id] = true
			}
		}
	}
	return nil
}

func (a *App) RegisterReviewFormat(ctx context.Context, input RegisterReviewFormat) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /review-formats", input, func(tx *sql.Tx) (any, error) {
		format := input.Format
		if !validReviewID(format.ID) || format.Version < 1 || strings.TrimSpace(format.Title) == "" || len(format.Fields) == 0 || len(format.Fields) > 100 {
			return nil, Invalid("format needs id, positive version, title and 1–100 fields")
		}
		seen := map[string]bool{}
		for _, field := range format.Fields {
			if !validReviewID(field.ID) || seen[field.ID] || field.Type == "review" || field.Type == "actions" || blockFields[field.Type] == nil {
				return nil, Invalid("format fields need unique ids and supported review block types")
			}
			seen[field.ID] = true
			if field.Type == "choice" {
				if err := validateSelection(field.Selection, field.Required, field.MinSelections, field.MaxSelections); err != nil {
					return nil, err
				}
			} else if field.Selection != "" || field.MinSelections != 0 || field.MaxSelections != 0 || (field.Type != "text_input" && field.Required) {
				return nil, Invalid("format constraints must belong to their field type")
			}
		}
		if !slices.ContainsFunc(format.Fields, func(f FormatField) bool { return f.Type == "choice" || f.Type == "text_input" }) {
			return nil, Invalid("format needs an input field")
		}
		definition, err := json.Marshal(format)
		if err != nil {
			return nil, err
		}
		if len(definition) > 64<<10 {
			return nil, Invalid("format exceeds 64 KiB")
		}
		var previous string
		err = tx.QueryRowContext(ctx, "SELECT definition FROM review_formats WHERE id=? AND version=?", format.ID, format.Version).Scan(&previous)
		if err == nil {
			if previous != string(definition) {
				return nil, &Error{Status: 409, Code: "format_conflict", Message: "Format versions are immutable; register a new version"}
			}
			return format, nil
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO review_formats(id,version,definition) VALUES(?,?,?)", format.ID, format.Version, string(definition))
		return format, err
	})
}

func (a *App) ReviewFormats(ctx context.Context) ([]ReviewFormat, error) {
	rows, err := a.Store.DB.QueryContext(ctx, "SELECT definition FROM review_formats ORDER BY id,version LIMIT 101")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	formats := []ReviewFormat{}
	for rows.Next() {
		var data string
		var format ReviewFormat
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &format); err != nil {
			return nil, err
		}
		formats = append(formats, format)
	}
	if len(formats) > 100 {
		return nil, Invalid("more than 100 formats; use the exact format lookup")
	}
	return formats, rows.Err()
}

func reviewFormat(ctx context.Context, db querier, ref FormatRef) (ReviewFormat, error) {
	var data string
	var format ReviewFormat
	if err := db.QueryRowContext(ctx, "SELECT definition FROM review_formats WHERE id=? AND version=?", ref.ID, ref.Version).Scan(&data); err != nil {
		return format, missing(err)
	}
	err := json.Unmarshal([]byte(data), &format)
	return format, err
}

func (a *App) ReviewFormat(ctx context.Context, ref FormatRef) (ReviewFormat, error) {
	return reviewFormat(ctx, a.Store.DB, ref)
}

func walkBlocks(blocks []ReportBlock, visit func(ReportBlock) error) error {
	for _, block := range blocks {
		if err := visit(block); err != nil {
			return err
		}
		if err := walkBlocks(block.Blocks, visit); err != nil {
			return err
		}
		for _, option := range block.Options {
			if err := walkBlocks(option.Blocks, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateReviewReferences(ctx context.Context, db querier, report Report) error {
	return walkBlocks(report.Blocks, func(block ReportBlock) error {
		if block.Type == "image" {
			var found string
			if err := db.QueryRowContext(ctx, "SELECT id FROM review_artifacts WHERE id=?", block.ArtifactID).Scan(&found); err != nil {
				return Invalid("image artifact does not exist")
			}
		}
		if block.Format == nil {
			return nil
		}
		format, err := reviewFormat(ctx, db, *block.Format)
		if err != nil {
			return err
		}
		if len(format.Fields) != len(block.Blocks) {
			return Invalid("review blocks do not match the registered format")
		}
		for i, field := range format.Fields {
			instance := block.Blocks[i]
			if field.ID != instance.ID || field.Type != instance.Type || field.Required != instance.Required || field.Selection != instance.Selection || field.MinSelections != instance.MinSelections || field.MaxSelections != instance.MaxSelections {
				return Invalid("review structure or constraints differ from the registered format")
			}
		}
		return nil
	})
}
