package httpapi

import (
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/zhuochun/control-plane/internal/app"
)

func itemRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/v1/items", read(func(r *http.Request) (any, error) {
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 100 {
				return nil, app.Invalid("limit must be between 1 and 100")
			}
			limit = parsed
		}
		var afterID string
		if cursor := r.URL.Query().Get("cursor"); cursor != "" {
			decoded, err := base64.RawURLEncoding.DecodeString(cursor)
			if err != nil {
				return nil, app.Invalid("Invalid continuation cursor")
			}
			afterID = string(decoded)
		}
		items, more, err := a.ItemsPage(r.Context(), app.ItemFilters{Sort: r.URL.Query().Get("sort"), View: r.URL.Query().Get("view"), Kind: r.URL.Query().Get("kind"), InterestID: r.URL.Query().Get("interest_id"), WatchID: r.URL.Query().Get("watch_id"), Query: r.URL.Query().Get("q"), DedupeKey: r.URL.Query().Get("dedupe_key"), DelegationStatus: r.URL.Query().Get("delegation_status"), Executor: r.URL.Query().Get("executor"), ExternalRef: r.URL.Query().Get("external_ref")}, afterID, limit)
		if err != nil {
			return nil, err
		}
		result := page[app.Item]{Items: items}
		if more {
			cursor := base64.RawURLEncoding.EncodeToString([]byte(items[len(items)-1].ID))
			result.NextCursor = &cursor
		}
		return result, nil
	}))
	mux.HandleFunc("GET /api/v1/items/{id}", read(func(r *http.Request) (any, error) { return a.Item(r.Context(), r.PathValue("id")) }))
	mux.HandleFunc("POST /api/v1/items", command(func(r *http.Request, input app.PutItem) (any, error) { return a.PutItem(r.Context(), "", input) }))
	mux.HandleFunc("POST /api/v1/items/interest", command(func(r *http.Request, input app.PutItem) (any, error) { return a.UpsertInterestItem(r.Context(), input) }))
	mux.HandleFunc("PUT /api/v1/items/{id}", command(func(r *http.Request, input app.PutItem) (any, error) {
		return a.PutItem(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("PATCH /api/v1/items/{id}/work", command(func(r *http.Request, input app.UpdateItemWork) (any, error) {
		return a.UpdateItemWork(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /api/v1/items/{id}/actions", command(func(r *http.Request, input app.ApplyItemAction) (any, error) {
		return a.ApplyItemAction(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("PUT /api/v1/items/{id}/note", command(func(r *http.Request, input app.SetUserNote) (any, error) {
		return a.SetUserNote(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /api/v1/items/{id}/history", read(func(r *http.Request) (any, error) { return a.ItemHistory(r.Context(), r.PathValue("id")) }))
	mux.HandleFunc("POST /api/v1/items/{id}/inputs/process", command(func(r *http.Request, input app.ProcessItemInput) (any, error) {
		return a.ProcessItemInput(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /api/v1/items/{id}/inputs", read(func(r *http.Request) (any, error) {
		offset := 0
		if value := r.URL.Query().Get("offset"); value != "" {
			var err error
			offset, err = strconv.Atoi(value)
			if err != nil || offset < 0 {
				return nil, app.Invalid("offset must be nonnegative")
			}
		}
		return a.InputHistory(r.Context(), r.PathValue("id"), offset)
	}))
	mux.HandleFunc("GET /api/v1/items/{id}/context", read(func(r *http.Request) (any, error) { return a.ItemContext(r.Context(), r.PathValue("id")) }))
	mux.HandleFunc("GET /api/v1/items/{id}/inputs/{input_id}/attempts", read(func(r *http.Request) (any, error) {
		offset := 0
		if value := r.URL.Query().Get("offset"); value != "" {
			var err error
			offset, err = strconv.Atoi(value)
			if err != nil || offset < 0 {
				return nil, app.Invalid("offset must be nonnegative")
			}
		}
		return a.InputAttempts(r.Context(), r.PathValue("id"), r.PathValue("input_id"), offset)
	}))
}
