// Package app owns commands and transaction boundaries shared by every adapter.
package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata"

	"github.com/google/uuid"
	"github.com/zhuochun/control-plane/internal/store"
)

type App struct {
	Store *store.Store
	Now   func() time.Time
}

func New(s *store.Store) *App { return &App{Store: s, Now: time.Now} }

type Error struct {
	Status    int    `json:"-"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Details   any    `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }
func Invalid(message string) error {
	return &Error{Status: 422, Code: "validation_error", Message: message}
}

// mutate commits the command, its event, and its receipt together. Receipt lookup
// precedes live-state checks, so a successful retry survives later state changes.
func (a *App) mutate(ctx context.Context, requestID, operation string, request any, command func(*sql.Tx) (any, error)) (json.RawMessage, error) {
	if len(requestID) > 200 {
		return nil, Invalid("request_id must contain at most 200 characters")
	}
	if requestID == "" {
		// Local interactive and MCP calls do not need to manufacture an
		// idempotency key. Explicit request IDs remain available when a caller
		// needs replayable retries.
		requestID = uuid.NewString()
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(append([]byte(operation+"\n"), body...))
	hash := hex.EncodeToString(sum[:])
	tx, err := a.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var existingHash string
	var storedResponse string
	var response json.RawMessage
	err = tx.QueryRowContext(ctx, "SELECT request_hash, response FROM command_receipts WHERE request_id = ?", requestID).Scan(&existingHash, &storedResponse)
	if err == nil {
		if hash != existingHash {
			return nil, &Error{Status: 409, Code: "idempotency_conflict", Message: "request_id was already used for a different command"}
		}
		return json.RawMessage(storedResponse), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	result, err := command(tx)
	if err != nil {
		return nil, err
	}
	response, err = json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO command_receipts(request_id, request_hash, response) VALUES (?, ?, ?)", requestID, hash, string(response)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return response, nil
}

func (a *App) event(ctx context.Context, tx *sql.Tx, actor, entityType, entityID, change string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO events(occurred_at, actor, entity_type, entity_id, change_type, payload) VALUES (?, ?, ?, ?, ?, ?)`, a.Now().UTC().UnixMilli(), actor, entityType, entityID, change, string(data))
	return err
}

type Settings struct {
	Timezone string `json:"timezone"`
	AgentsMD string `json:"agents_md"`
	UserMD   string `json:"user_md"`
}
type SetSettings struct {
	RequestID string  `json:"request_id"`
	Timezone  string  `json:"timezone"`
	AgentsMD  *string `json:"agents_md,omitempty"`
	UserMD    *string `json:"user_md,omitempty"`
}

type settingsRow struct {
	timezoneValue string
	timezoneMode  string
	agentsMD      string
	userMD        string
}

func readSettings(ctx context.Context, db querier) (settingsRow, error) {
	var row settingsRow
	err := db.QueryRowContext(ctx, `SELECT
  (SELECT value FROM settings WHERE key='timezone'),
  COALESCE((SELECT value FROM settings WHERE key='timezone_mode'), 'custom'),
  COALESCE((SELECT value FROM settings WHERE key='agents_md'), ''),
  COALESCE((SELECT value FROM settings WHERE key='user_md'), '')`).Scan(&row.timezoneValue, &row.timezoneMode, &row.agentsMD, &row.userMD)
	return row, err
}

func (row settingsRow) public() Settings {
	timezone := row.timezoneValue
	if row.timezoneMode == "auto" {
		timezone = "browser"
	}
	return Settings{Timezone: timezone, AgentsMD: row.agentsMD, UserMD: row.userMD}
}

func (a *App) Settings(ctx context.Context) (Settings, error) {
	row, err := readSettings(ctx, a.Store.DB)
	if err != nil {
		return Settings{}, err
	}
	return row.public(), nil
}

func (a *App) SetSettings(ctx context.Context, input SetSettings) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "PATCH /settings", input, func(tx *sql.Tx) (any, error) {
		current, err := readSettings(ctx, tx)
		if err != nil {
			return nil, err
		}
		if input.Timezone == "" || input.Timezone == "Local" {
			return nil, Invalid("timezone must be an IANA timezone such as Asia/Singapore, or browser")
		}
		timezoneValue := input.Timezone
		timezoneMode := "custom"
		if input.Timezone == "browser" {
			timezoneValue = "UTC"
			timezoneMode = "auto"
		} else if _, err := time.LoadLocation(input.Timezone); err != nil {
			return nil, Invalid(fmt.Sprintf("unknown timezone %q", input.Timezone))
		}
		agentsMD := current.agentsMD
		if input.AgentsMD != nil {
			agentsMD = *input.AgentsMD
		}
		userMD := current.userMD
		if input.UserMD != nil {
			userMD = *input.UserMD
		}
		if len([]byte(agentsMD)) > 64<<10 || len([]byte(userMD)) > 64<<10 {
			return nil, Invalid("context files must each be 64 KiB or smaller")
		}
		if _, err = tx.ExecContext(ctx, "UPDATE settings SET value = ? WHERE key = 'timezone'", timezoneValue); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE settings SET value = ? WHERE key = 'timezone_mode'", timezoneMode); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE settings SET value = ? WHERE key = 'agents_md'", agentsMD); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE settings SET value = ? WHERE key = 'user_md'", userMD); err != nil {
			return nil, err
		}
		settings := Settings{Timezone: input.Timezone, AgentsMD: agentsMD, UserMD: userMD}
		if err := a.event(ctx, tx, "user", "settings", "timezone", "settings.updated", settings); err != nil {
			return nil, err
		}
		return settings, nil
	})
}
