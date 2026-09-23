package httpapi

import (
	"net/http"
	"strconv"

	"github.com/zhuochun/control-plane/internal/app"
)

func runRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/v1/brief", read(func(r *http.Request) (any, error) { return a.BriefPage(r.Context(), r.URL.Query().Get("cursor")) }))
	mux.HandleFunc("GET /api/v1/changes", read(func(r *http.Request) (any, error) {
		after, err := queryInt(r, "after_seq")
		if err != nil {
			return nil, err
		}
		through, err := queryInt(r, "through_seq")
		if err != nil {
			return nil, err
		}
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			limit, err = strconv.Atoi(raw)
			if err != nil || limit < 1 || limit > 100 {
				return nil, app.Invalid("limit must be between 1 and 100")
			}
		}
		return a.ChangesPage(r.Context(), after, through, r.URL.Query().Get("cursor"), limit)
	}))
	mux.HandleFunc("GET /api/v1/runs", read(func(r *http.Request) (any, error) {
		items, err := a.Runs(r.Context())
		if err != nil {
			return nil, err
		}
		return paginate(r, items, func(item app.RunSummary) string { return item.ID })
	}))
	mux.HandleFunc("GET /api/v1/runs/{id}", read(func(r *http.Request) (any, error) { return a.RunDetail(r.Context(), r.PathValue("id")) }))
	mux.HandleFunc("POST /api/v1/runs", command(func(r *http.Request, input app.StartRun) (any, error) { return a.StartRun(r.Context(), input) }))
	mux.HandleFunc("POST /api/v1/runs/finish", command(func(r *http.Request, input app.FinishRun) (any, error) {
		return a.FinishActiveRun(r.Context(), input)
	}))
	mux.HandleFunc("POST /api/v1/runs/{id}/finish", command(func(r *http.Request, input app.FinishRun) (any, error) {
		return a.FinishRun(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /api/v1/runs/{id}/abandon", command(func(r *http.Request, input app.AbandonRun) (any, error) {
		return a.AbandonRun(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("PUT /api/v1/runs/watches/{watch}/findings", command(func(r *http.Request, input app.SubmitWatchFindings) (any, error) {
		return a.SubmitActiveWatchFindings(r.Context(), r.PathValue("watch"), input)
	}))
	mux.HandleFunc("PUT /api/v1/runs/{id}/watches/{watch}/findings", command(func(r *http.Request, input app.SubmitWatchFindings) (any, error) {
		return a.SubmitWatchFindings(r.Context(), r.PathValue("id"), r.PathValue("watch"), input)
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
