package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProcedureCatalogRevisionIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	load := func() *httptest.ResponseRecorder {
		t.Helper()
		result := httptest.NewRecorder()
		api.listProcedures(result, httptest.NewRequest(http.MethodGet, "/v1/procedures", nil))
		return result
	}
	save := func(revision, name string) *httptest.ResponseRecorder {
		t.Helper()
		payload := `[{"id":"therapy","name":"` + name + `","durationMinutes":30,"preparationMinutes":20,"minIntervalDays":1,"defaultWeeklyCount":1,"defaultSessionCount":3}]`
		request := httptest.NewRequest(http.MethodPut, "/v1/procedures", bytes.NewBufferString(payload))
		request.Header.Set("If-Match", revision)
		result := httptest.NewRecorder()
		api.saveProcedures(result, request)
		return result
	}
	if result := load(); result.Code != http.StatusOK || result.Header().Get("ETag") != `"0"` {
		t.Fatalf("initial catalog: %d %s", result.Code, result.Header().Get("ETag"))
	}
	if result := save(`"0"`, "Therapy"); result.Code != http.StatusOK || result.Header().Get("ETag") != `"1"` {
		t.Fatalf("first save: %d %s %s", result.Code, result.Header().Get("ETag"), result.Body.String())
	}
	if result := save(`"0"`, "Stale"); result.Code != http.StatusConflict {
		t.Fatalf("stale save accepted: %d %s", result.Code, result.Body.String())
	}
	if result := load(); result.Code != http.StatusOK || result.Header().Get("ETag") != `"1"` || !bytes.Contains(result.Body.Bytes(), []byte(`"Therapy"`)) {
		t.Fatalf("catalog changed after stale save: %d %s", result.Code, result.Body.String())
	}
	if result := save(`"1"`, "Updated"); result.Code != http.StatusOK || result.Header().Get("ETag") != `"2"` {
		t.Fatalf("current save: %d %s %s", result.Code, result.Header().Get("ETag"), result.Body.String())
	}
	if result := save(`"2"`, "Updated"); result.Code != http.StatusOK || result.Header().Get("ETag") != `"2"` {
		t.Fatalf("unchanged save advanced revision: %d %s %s", result.Code, result.Header().Get("ETag"), result.Body.String())
	}
	for _, check := range []struct {
		header string
		want   int
	}{{"", http.StatusPreconditionRequired}, {`"invalid"`, http.StatusBadRequest}} {
		if result := save(check.header, "Unprotected"); result.Code != check.want {
			t.Fatalf("If-Match %q: expected %d, got %d %s", check.header, check.want, result.Code, result.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodDelete, "/v1/procedures/therapy", nil)
	request.SetPathValue("id", "therapy")
	request.Header.Set("If-Match", `"1"`)
	deleted := httptest.NewRecorder()
	api.deleteProcedure(deleted, request)
	if deleted.Code != http.StatusConflict {
		t.Fatalf("stale delete accepted: %d %s", deleted.Code, deleted.Body.String())
	}
	if result := load(); result.Code != http.StatusOK || result.Header().Get("ETag") != `"2"` || !bytes.Contains(result.Body.Bytes(), []byte(`"Updated"`)) {
		t.Fatalf("stale delete changed catalog: %d %s", result.Code, result.Body.String())
	}
	for _, check := range []struct {
		header string
		want   int
	}{{"", http.StatusPreconditionRequired}, {`"invalid"`, http.StatusBadRequest}, {`"2"`, http.StatusNoContent}} {
		request := httptest.NewRequest(http.MethodDelete, "/v1/procedures/therapy", nil)
		request.SetPathValue("id", "therapy")
		request.Header.Set("If-Match", check.header)
		response := httptest.NewRecorder()
		api.deleteProcedure(response, request)
		if response.Code != check.want {
			t.Fatalf("DELETE If-Match %q: %d %s", check.header, response.Code, response.Body.String())
		}
	}
	if result := load(); result.Header().Get("ETag") != `"3"` || result.Body.String() != "[]\n" {
		t.Fatalf("catalog after delete: %s %s", result.Header().Get("ETag"), result.Body.String())
	}
	for range 2 {
		if err := seedProcedures(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	if result := load(); result.Header().Get("ETag") != `"3"` || result.Body.String() != "[]\n" {
		t.Fatalf("seed restored an intentionally emptied catalog: %s %s", result.Header().Get("ETag"), result.Body.String())
	}
}

func TestFreshCatalogSeedIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	for range 2 {
		if err := seedProcedures(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	response := httptest.NewRecorder()
	(&server{db: db}).listProcedures(response, httptest.NewRequest(http.MethodGet, "/v1/procedures", nil))
	if response.Code != http.StatusOK || response.Header().Get("ETag") != `"1"` || !bytes.Contains(response.Body.Bytes(), []byte(`"procedure-20"`)) {
		t.Fatalf("fresh seed must advance revision once: %d %s %s", response.Code, response.Header().Get("ETag"), response.Body.String())
	}
}

func TestConcurrentCatalogWritersIntegration(t *testing.T) {
	api := &server{db: isolatedScheduleDatabase(t)}
	results := make(chan int, 2)
	start := make(chan struct{})
	for _, name := range []string{"First", "Second"} {
		go func(name string) {
			<-start
			payload, _ := json.Marshal([]procedure{{ID: "concurrent", Name: name, DurationMinutes: 30, MinIntervalDays: 1, DefaultWeeklyCount: 1, DefaultSessionCount: 1}})
			req := httptest.NewRequest(http.MethodPut, "/v1/procedures", bytes.NewReader(payload))
			req.Header.Set("If-Match", `"0"`)
			response := httptest.NewRecorder()
			api.saveProcedures(response, req)
			results <- response.Code
		}(name)
	}
	close(start)
	counts := map[int]int{}
	for range 2 {
		counts[<-results]++
	}
	if counts[http.StatusOK] != 1 || counts[http.StatusConflict] != 1 {
		t.Fatalf("concurrent catalog writers: %+v", counts)
	}
}
