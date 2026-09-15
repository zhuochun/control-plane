package httpapi

import (
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/zhuochun/control-plane/internal/app"
)

type page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

func paginate[T any](r *http.Request, items []T, id func(T) string) (any, error) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return nil, app.Invalid("limit must be between 1 and 100")
		}
		limit = parsed
	}
	start := 0
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, app.Invalid("Invalid continuation cursor")
		}
		found := false
		for i, item := range items {
			if id(item) == string(decoded) {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return nil, app.Invalid("Continuation cursor is not in this collection")
		}
	}
	end := min(start+limit, len(items))
	result := page[T]{Items: items[start:end]}
	if end < len(items) {
		cursor := base64.RawURLEncoding.EncodeToString([]byte(id(items[end-1])))
		result.NextCursor = &cursor
	}
	return result, nil
}
