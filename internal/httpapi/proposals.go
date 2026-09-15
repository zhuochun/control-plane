package httpapi

import (
	"net/http"

	"github.com/zhuochun/control-plane/internal/app"
)

func proposalRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/v1/proposals", read(func(r *http.Request) (any, error) {
		items, err := a.Proposals(r.Context(), r.URL.Query().Get("state"))
		if err != nil {
			return nil, err
		}
		return paginate(r, items, func(item app.Proposal) string { return item.ID })
	}))
	mux.HandleFunc("GET /api/v1/proposals/{id}", read(func(r *http.Request) (any, error) { return a.Proposal(r.Context(), r.PathValue("id")) }))
	mux.HandleFunc("POST /api/v1/proposals", command(func(r *http.Request, input app.CreateProposal) (any, error) {
		return a.CreateProposal(r.Context(), input)
	}))
	mux.HandleFunc("POST /api/v1/proposals/{id}/resolve", command(func(r *http.Request, input app.ResolveProposal) (any, error) {
		return a.ResolveProposal(r.Context(), r.PathValue("id"), input)
	}))
}
