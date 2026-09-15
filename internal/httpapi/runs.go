package httpapi

import (
	"net/http"
	"strconv"

	"github.com/zhuochun/control-plane/internal/app"
)

func runRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/v1/brief", read(func(r *http.Request) (any, error) { return a.Brief(r.Context()) }))
	mux.HandleFunc("GET /api/v1/changes", read(func(r *http.Request) (any, error) {
		after, err := queryInt(r, "after_seq")
		if err != nil {
			return nil, err
		}
		through, err := queryInt(r, "through_seq")
		if err != nil {
			return nil, err
		}
		items, err := a.Changes(r.Context(), after, through)
		if err != nil {
			return nil, err
		}
		return paginate(r, items, func(item app.Event) string { return strconv.FormatInt(item.Seq, 10) })
	}))
	mux.HandleFunc("GET /api/v1/runs", read(func(r *http.Request) (any, error) {
		items, err := a.Runs(r.Context())
		if err != nil {
			return nil, err
		}
		return paginate(r, items, func(item app.Run) string { return item.ID })
	}))
	mux.HandleFunc("GET /api/v1/runs/{id}", read(func(r *http.Request) (any, error) { return a.RunDetail(r.Context(), r.PathValue("id")) }))
	mux.HandleFunc("POST /api/v1/runs", command(func(r *http.Request, input app.StartRun) (any, error) { return a.StartRun(r.Context(), input) }))
	mux.HandleFunc("POST /api/v1/runs/{id}/renew", command(func(r *http.Request, input app.RenewRun) (any, error) {
		return a.RenewRun(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /api/v1/runs/{id}/finish", command(func(r *http.Request, input app.FinishRun) (any, error) {
		return a.FinishRun(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("PUT /api/v1/runs/{id}/watches/{watch}/result", command(func(r *http.Request, input app.PublishWatchResult) (any, error) {
		return a.PublishWatchResult(r.Context(), r.PathValue("id"), r.PathValue("watch"), input)
	}))
}

func queryInt(r *http.Request, name string) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, app.Invalid(name + " is required")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, app.Invalid(name + " must be an integer")
	}
	return value, nil
}
