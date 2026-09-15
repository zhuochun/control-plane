package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/zhuochun/control-plane/internal/app"
)

func New(a *app.App, assets http.Handler, version string) http.Handler {
	mux := http.NewServeMux()
	configurationRoutes(mux, a)
	itemRoutes(mux, a)
	runRoutes(mux, a)
	proposalRoutes(mux, a)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.DB.PingContext(r.Context()); err != nil {
			fail(w, err)
			return
		}
		health, err := a.OperationalHealth(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, map[string]any{"version": version, "database": "ok", "schema": "ok", "now": a.Now().UTC(), "health": health})
	})
	mux.HandleFunc("GET /api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		result, err := a.Settings(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, result)
	})
	mux.HandleFunc("PATCH /api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		var input app.SetSettings
		if err := decode(w, r, &input); err != nil {
			fail(w, err)
			return
		}
		result, err := a.SetSettings(r.Context(), input)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, result)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, &app.Error{Status: 404, Code: "not_found", Message: "Unknown API endpoint"})
	})
	mux.Handle("/", assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		host := r.Host
		if colon := strings.LastIndex(host, ":"); colon >= 0 {
			host = host[:colon]
		}
		host = strings.Trim(host, "[]")
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			fail(w, &app.Error{Status: 403, Code: "host_rejected", Message: "Use the loopback aicp address"})
			return
		}
		// The portal and API share one loopback origin. Other websites must not
		// be able to submit commands to a user's local server.
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			fail(w, &app.Error{Status: 403, Code: "origin_rejected", Message: "Use the local aicp portal"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func decode(w http.ResponseWriter, r *http.Request, target any) error {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return &app.Error{Status: 400, Code: "invalid_json", Message: "Send Content-Type: application/json"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return decodeError(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err != nil {
			return decodeError(err)
		}
		return &app.Error{Status: 400, Code: "invalid_json", Message: "Expected one JSON object"}
	}
	return nil
}

func decodeError(err error) error {
	var size *http.MaxBytesError
	if errors.As(err, &size) {
		return &app.Error{Status: 413, Code: "payload_too_large", Message: "Request exceeds 4 MiB"}
	}
	return &app.Error{Status: 400, Code: "invalid_json", Message: err.Error()}
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, err error) {
	var problem *app.Error
	if !errors.As(err, &problem) {
		slog.Error("request failed", "error", err)
		problem = &app.Error{Status: 500, Code: "internal_error", Message: "The server could not complete this request"}
	}
	write(w, problem.Status, map[string]any{"error": problem})
}
