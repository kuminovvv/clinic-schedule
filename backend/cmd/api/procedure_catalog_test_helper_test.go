package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func catalogMutationRequest(t *testing.T, api *server, method, path string, body io.Reader) *http.Request {
	t.Helper()
	current := httptest.NewRecorder()
	api.listProcedures(current, httptest.NewRequest(http.MethodGet, "/v1/procedures", nil))
	if current.Code != http.StatusOK {
		t.Fatalf("load catalog revision: %d %s", current.Code, current.Body.String())
	}
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("If-Match", current.Header().Get("ETag"))
	return request
}
