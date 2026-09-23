package client

import "testing"

func TestAmbiguousIDErrorShowsShortCandidates(t *testing.T) {
	err := &Error{
		Code:    "ambiguous_id",
		Message: "ID prefix matches more than one record",
		Details: map[string]any{"candidates": []any{"12345678-aaaa-bbbb-cccc-dddddddddddd", "abcdef12-bbbb-cccc-dddd-eeeeeeeeeeee"}},
	}
	want := "ID prefix matches more than one record: 12345678, abcdef12"
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
