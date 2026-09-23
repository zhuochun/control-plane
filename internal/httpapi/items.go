package httpapi

import (
	"net/http"

	"github.com/zhuochun/control-plane/internal/app"
)

func itemRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/v1/items", read(func(r *http.Request) (any, error) {
		items, err := a.Items(r.Context(), app.ItemFilters{View: r.URL.Query().Get("view"), Kind: r.URL.Query().Get("kind"), InterestID: r.URL.Query().Get("interest_id"), WatchID: r.URL.Query().Get("watch_id"), Query: r.URL.Query().Get("q"), DedupeKey: r.URL.Query().Get("dedupe_key")})
		if err != nil {
			return nil, err
		}
		return paginate(r, items, func(item app.Item) string { return item.ID })
	}))
	mux.HandleFunc("GET /api/v1/items/{id}", read(func(r *http.Request) (any, error) { return a.Item(r.Context(), r.PathValue("id")) }))
	mux.HandleFunc("POST /api/v1/items", command(func(r *http.Request, input app.PutItem) (any, error) { return a.PutItem(r.Context(), "", input) }))
	mux.HandleFunc("POST /api/v1/items/interest", command(func(r *http.Request, input app.PutItem) (any, error) { return a.UpsertInterestItem(r.Context(), input) }))
	mux.HandleFunc("PUT /api/v1/items/{id}", command(func(r *http.Request, input app.PutItem) (any, error) {
		return a.PutItem(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /api/v1/items/{id}/actions", command(func(r *http.Request, input app.ApplyItemAction) (any, error) {
		return a.ApplyItemAction(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("PUT /api/v1/items/{id}/note", command(func(r *http.Request, input app.SetUserNote) (any, error) {
		return a.SetUserNote(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /api/v1/items/{id}/history", read(func(r *http.Request) (any, error) { return a.ItemHistory(r.Context(), r.PathValue("id")) }))
	mux.HandleFunc("GET /api/v1/items/{id}/context", read(func(r *http.Request) (any, error) { return a.ItemContext(r.Context(), r.PathValue("id")) }))
}
