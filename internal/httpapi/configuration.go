package httpapi

import (
	"net/http"

	"github.com/zhuochun/control-plane/internal/app"
)

// Each adapter delegates commands to app; validation and transactions live there.
func command[T any](run func(*http.Request, T) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input T
		if err := decode(w, r, &input); err != nil {
			fail(w, err)
			return
		}
		result, err := run(r, input)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, result)
	}
}

func read(run func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := run(r)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, result)
	}
}

func configurationRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/v1/interests", read(func(r *http.Request) (any, error) {
		items, err := a.Interests(r.Context())
		if err != nil {
			return nil, err
		}
		return paginate(r, items, func(item app.Interest) string { return item.ID })
	}))
	mux.HandleFunc("GET /api/v1/interests/{id}", read(func(r *http.Request) (any, error) { return a.Interest(r.Context(), r.PathValue("id")) }))
	mux.HandleFunc("POST /api/v1/interests", command(func(r *http.Request, input app.CreateInterest) (any, error) {
		return a.CreateInterest(r.Context(), input)
	}))
	mux.HandleFunc("PATCH /api/v1/interests/{id}", command(func(r *http.Request, input app.UpdateInterest) (any, error) {
		return a.UpdateInterest(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /api/v1/watches", read(func(r *http.Request) (any, error) {
		items, err := a.Watches(r.Context(), r.URL.Query().Get("interest_id"))
		if err != nil {
			return nil, err
		}
		return paginate(r, items, func(item app.Watch) string { return item.ID })
	}))
	mux.HandleFunc("GET /api/v1/watches/{id}", read(func(r *http.Request) (any, error) { return a.Watch(r.Context(), r.PathValue("id")) }))
	mux.HandleFunc("POST /api/v1/watches", command(func(r *http.Request, input app.CreateWatch) (any, error) { return a.CreateWatch(r.Context(), input) }))
	mux.HandleFunc("PATCH /api/v1/watches/{id}", command(func(r *http.Request, input app.UpdateWatch) (any, error) {
		return a.UpdateWatch(r.Context(), r.PathValue("id"), input)
	}))
}
