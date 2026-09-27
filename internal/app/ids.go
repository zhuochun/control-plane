package app

import (
	"context"
	"database/sql"
	"strings"
)

func validSlug(value string) bool {
	if len(value) < 2 || len(value) > 80 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func defaultSlug(prefix, id string) string {
	return prefix + "-" + strings.ReplaceAll(id, "-", "")[:12]
}

type rowsQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// resolveID keeps canonical identifiers in machine payloads while allowing
// human adapters to use a unique prefix. The table name is always supplied by
// this package, never by a request, so the SQL remains fixed and safe.
func resolveID(ctx context.Context, db rowsQuerier, table, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", Invalid("id is required")
	}
	if table == "interests" || table == "watches" {
		var id string
		rows, queryErr := db.QueryContext(ctx, "SELECT id FROM "+table+" WHERE slug=?", input)
		if queryErr != nil {
			return "", queryErr
		}
		if rows.Next() {
			if scanErr := rows.Scan(&id); scanErr != nil {
				rows.Close()
				return "", scanErr
			}
		}
		if rowErr := rows.Err(); rowErr != nil {
			rows.Close()
			return "", rowErr
		}
		if closeErr := rows.Close(); closeErr != nil {
			return "", closeErr
		}
		if id != "" {
			return id, nil
		}
	}
	for _, character := range input {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') && !(character >= 'A' && character <= 'F') && character != '-' {
			return "", &Error{Status: 404, Code: "not_found", Message: "Record not found"}
		}
	}
	return resolveIDRows(ctx, db, table, input)
}

func resolveIDRows(ctx context.Context, db rowsQuerier, table, input string) (string, error) {
	rows, err := db.QueryContext(ctx, "SELECT id FROM "+table+" WHERE id=? OR id LIKE ? ORDER BY id LIMIT 2", input, input+"%")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	matches := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		matches = append(matches, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", &Error{Status: 404, Code: "not_found", Message: "Record not found"}
	case 1:
		return matches[0], nil
	default:
		return "", &Error{Status: 409, Code: "ambiguous_id", Message: "ID prefix matches more than one record", Details: map[string]any{"prefix": input, "candidates": matches}}
	}
}
