package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AnswerValue struct {
	FieldID     string   `json:"field_id"`
	Disposition string   `json:"disposition"`
	SelectedIDs []string `json:"selected_ids,omitempty"`
	Text        string   `json:"text,omitempty"`
}

type SavedAnswerValue struct {
	AnswerValue
	Question string         `json:"question"`
	Type     string         `json:"type"`
	Answer   string         `json:"answer"`
	Options  []ReviewOption `json:"options,omitempty"`
}

type ReviewAnswer struct {
	ID                 string             `json:"id"`
	ReviewID           string             `json:"review_id"`
	Title              string             `json:"title"`
	ContentVersion     int64              `json:"content_version"`
	ReviewMaterialHash string             `json:"review_material_hash"`
	Format             *FormatRef         `json:"format,omitempty"`
	SubmittedAt        time.Time          `json:"submitted_at"`
	Supersedes         string             `json:"supersedes,omitempty"`
	Values             []SavedAnswerValue `json:"values"`
	Note               string             `json:"note"`
	Applicable         bool               `json:"applicable"`
}

type SubmitReviewAnswer struct {
	RequestID              string        `json:"request_id"`
	ReviewID               string        `json:"review_id"`
	ExpectedContentVersion int64         `json:"expected_content_version"`
	ExpectedStateVersion   int64         `json:"expected_state_version"`
	ReviewMaterialHash     string        `json:"review_material_hash"`
	Supersedes             string        `json:"supersedes,omitempty"`
	Values                 []AnswerValue `json:"values"`
}

// The digest includes the full visible decision basis. Handoff context and
// delegations may change without forcing the person to repeat an answer.
func reviewMaterialHash(item Item) string {
	data, _ := json.Marshal(struct {
		Title   string   `json:"title"`
		Summary string   `json:"summary"`
		Report  Report   `json:"report"`
		Sources []Source `json:"sources"`
	}{item.Title, item.Summary, item.Report, item.Sources})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func currentAnswers(ctx context.Context, db querier, item Item) ([]ReviewAnswer, error) {
	ids := []any{item.ID}
	placeholders := []string{}
	for _, block := range item.Report.Blocks {
		if block.Type == "review" {
			ids = append(ids, block.ID)
			placeholders = append(placeholders, "?")
		}
	}
	if len(placeholders) == 0 {
		return []ReviewAnswer{}, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT a.response FROM item_answers a WHERE a.item_id=?
AND a.review_id IN (`+strings.Join(placeholders, ",")+`)
AND NOT EXISTS (SELECT 1 FROM item_answers b WHERE b.supersedes=a.id) ORDER BY a.seq`, ids...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	answers := []ReviewAnswer{}
	for rows.Next() {
		var data string
		var answer ReviewAnswer
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &answer); err != nil {
			return nil, err
		}
		answer.Applicable = answer.ReviewMaterialHash == reviewMaterialHash(item)
		answers = append(answers, answer)
	}
	return answers, rows.Err()
}

func completeItem(ctx context.Context, db querier, item Item) (Item, error) {
	item.ReviewMaterialHash = reviewMaterialHash(item)
	var err error
	item.Answers, err = currentAnswers(ctx, db, item)
	if err != nil {
		return item, err
	}
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM item_answers WHERE item_id=?)", item.ID).Scan(&item.AnswerHistoryAvailable)
	return item, err
}

func snapshotAnswer(field ReportBlock, value AnswerValue) (SavedAnswerValue, error) {
	saved := SavedAnswerValue{AnswerValue: value, Question: field.Question, Type: field.Type}
	if value.Disposition != "answered" && value.Disposition != "skipped" {
		return saved, Invalid("answer disposition must be answered or skipped")
	}
	if value.Disposition == "skipped" {
		if field.Required || len(value.SelectedIDs) > 0 || value.Text != "" {
			return saved, Invalid("only optional fields without values can be skipped")
		}
		saved.Answer = "Skipped"
		return saved, nil
	}
	if field.Type == "text_input" {
		if len(value.SelectedIDs) != 0 || len(value.Text) > 16<<10 || (field.Required && strings.TrimSpace(value.Text) == "") {
			return saved, Invalid("invalid text answer; maximum 16 KiB")
		}
		saved.Answer = value.Text
		return saved, nil
	}
	if value.Text != "" {
		return saved, Invalid("choice answers cannot include text")
	}
	count := len(value.SelectedIDs)
	if (field.Required && count == 0) || count < field.MinSelections || (field.MaxSelections > 0 && count > field.MaxSelections) || (field.Selection == "single" && count != 1) {
		return saved, Invalid("answer does not satisfy selection limits; skip optional single choice explicitly")
	}
	selected := map[string]bool{}
	labels := []string{}
	for _, id := range value.SelectedIDs {
		if selected[id] {
			return saved, Invalid("selected option ids must be unique")
		}
		selected[id] = true
		found := false
		for _, option := range field.Options {
			if option.ID == id {
				found = true
				saved.Options = append(saved.Options, option)
				labels = append(labels, option.Label)
				break
			}
		}
		if !found {
			return saved, Invalid("selected option does not belong to the field")
		}
	}
	saved.Answer = strings.Join(labels, "; ")
	if count == 0 {
		saved.Answer = "Selected none"
	}
	return saved, nil
}

func (a *App) SubmitReviewAnswer(ctx context.Context, itemID string, input SubmitReviewAnswer) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /items/"+itemID+"/answers", input, func(tx *sql.Tx) (any, error) {
		item, err := getItem(ctx, tx, itemID)
		if err != nil {
			return nil, err
		}
		if input.ExpectedStateVersion != item.StateVersion {
			return nil, stateConflict(item)
		}
		if input.ExpectedContentVersion != item.ContentVersion || input.ReviewMaterialHash != reviewMaterialHash(item) {
			return nil, &Error{Status: 409, Code: "content_conflict", Message: "Review content changed; reread the Item before submitting"}
		}
		var review *ReportBlock
		for _, block := range item.Report.Blocks {
			if block.Type == "review" && block.ID == input.ReviewID {
				review = &block
				break
			}
		}
		if review == nil {
			return nil, Invalid("review does not exist on this Item")
		}
		current, err := currentAnswers(ctx, tx, item)
		if err != nil {
			return nil, err
		}
		var currentID string
		for _, answer := range current {
			if answer.ReviewID == review.ID {
				currentID = answer.ID
			}
		}
		if currentID != input.Supersedes {
			return nil, &Error{Status: 409, Code: "answer_conflict", Message: "Read the current answer before replacing it"}
		}
		values := map[string]AnswerValue{}
		for _, value := range input.Values {
			if _, exists := values[value.FieldID]; exists {
				return nil, Invalid("answer fields must be unique")
			}
			values[value.FieldID] = value
		}
		answer := ReviewAnswer{ID: uuid.NewString(), ReviewID: review.ID, Title: review.Title, ContentVersion: item.ContentVersion, ReviewMaterialHash: input.ReviewMaterialHash, Format: review.Format, SubmittedAt: a.Now().UTC(), Supersedes: input.Supersedes, Applicable: true}
		note := []string{fmt.Sprintf("Your answer · %s · %s", review.Title, answer.SubmittedAt.Format(time.RFC3339))}
		for _, field := range review.Blocks {
			if field.Type != "choice" && field.Type != "text_input" {
				continue
			}
			value, exists := values[field.ID]
			if !exists {
				return nil, Invalid("submit a disposition for every input field")
			}
			saved, err := snapshotAnswer(field, value)
			if err != nil {
				return nil, err
			}
			answer.Values = append(answer.Values, saved)
			note = append(note, field.Question+": "+saved.Answer)
			delete(values, field.ID)
		}
		if len(values) != 0 {
			return nil, Invalid("answer contains unknown fields")
		}
		note = append(note, fmt.Sprintf("Answered against Item content version %d.", item.ContentVersion))
		answer.Note = strings.Join(note, "\n")
		data, err := json.Marshal(answer)
		if err != nil {
			return nil, err
		}
		if len(data) > 256<<10 {
			return nil, Invalid("answer snapshot exceeds 256 KiB")
		}
		var supersedes any
		if input.Supersedes != "" {
			supersedes = input.Supersedes
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO item_answers(id,item_id,review_id,supersedes,response) VALUES(?,?,?,?,?)", answer.ID, item.ID, review.ID, supersedes, string(data)); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE items SET state_version=state_version+1,state_updated_at=? WHERE id=?", a.Now().UTC().UnixMilli(), item.ID); err != nil {
			return nil, err
		}
		if err = a.event(ctx, tx, "user", "item", item.ID, "item.answer_submitted", map[string]any{"answer_id": answer.ID, "review_id": review.ID, "state_version": item.StateVersion + 1}); err != nil {
			return nil, err
		}
		return struct {
			Answer       ReviewAnswer `json:"answer"`
			StateVersion int64        `json:"state_version"`
		}{answer, item.StateVersion + 1}, nil
	})
}

type AnswerHistory struct {
	Items     []ReviewAnswer `json:"items"`
	NextAfter int64          `json:"next_after,omitempty"`
}

func (a *App) AnswerHistory(ctx context.Context, itemID string, after int64) (AnswerHistory, error) {
	item, err := getItem(ctx, a.Store.DB, itemID)
	if err != nil {
		return AnswerHistory{}, err
	}
	rows, err := a.Store.DB.QueryContext(ctx, "SELECT seq,response FROM item_answers WHERE item_id=? AND seq>? ORDER BY seq LIMIT 21", item.ID, after)
	if err != nil {
		return AnswerHistory{}, err
	}
	defer func() { _ = rows.Close() }()
	result := AnswerHistory{Items: []ReviewAnswer{}}
	var previous int64
	for rows.Next() {
		var seq int64
		var data string
		if err := rows.Scan(&seq, &data); err != nil {
			return result, err
		}
		if len(result.Items) == 20 {
			result.NextAfter = previous
			break
		}
		var answer ReviewAnswer
		if err := json.Unmarshal([]byte(data), &answer); err != nil {
			return result, err
		}
		// History entries are historical, not assertions of current authority.
		answer.Applicable = false
		result.Items = append(result.Items, answer)
		previous = seq
	}
	return result, rows.Err()
}
