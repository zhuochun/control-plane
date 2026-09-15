package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerServesRootAndKnownClientRoutes(t *testing.T) {
	for _, path := range []string{"/", "/interests", "/items/example"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Errorf("%s returned %d", path, response.Code)
		}
	}
}

func TestHandlerDoesNotHideUnknownOrMutatingRoutes(t *testing.T) {
	for _, test := range []struct{ method, path string }{{"GET", "/unknown"}, {"POST", "/interests"}} {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("%s %s returned %d", test.method, test.path, response.Code)
		}
	}
}
