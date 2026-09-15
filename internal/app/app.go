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
	if requestID == "" || len(requestID) > 200 {
		return nil, Invalid("request_id must contain 1 to 200 characters")
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
}
type SetSettings struct {
	RequestID string `json:"request_id"`
	Timezone  string `json:"timezone"`
}

func (a *App) Settings(ctx context.Context) (Settings, error) {
	var settings Settings
	err := a.Store.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'timezone'").Scan(&settings.Timezone)
	return settings, err
}

func (a *App) SetSettings(ctx context.Context, input SetSettings) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "PATCH /settings", input, func(tx *sql.Tx) (any, error) {
		if input.Timezone == "" || input.Timezone == "Local" {
			return nil, Invalid("timezone must be an IANA timezone such as Asia/Singapore")
		}
		if _, err := time.LoadLocation(input.Timezone); err != nil {
			return nil, Invalid(fmt.Sprintf("unknown timezone %q", input.Timezone))
		}
		settings := Settings{Timezone: input.Timezone}
		if _, err := tx.ExecContext(ctx, "UPDATE settings SET value = ? WHERE key = 'timezone'", input.Timezone); err != nil {
			return nil, err
		}
		if err := a.event(ctx, tx, "user", "settings", "timezone", "settings.updated", settings); err != nil {
			return nil, err
		}
		return settings, nil
	})
}
