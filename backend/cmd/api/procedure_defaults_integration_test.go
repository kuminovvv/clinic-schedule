package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProcedureDefaultSessionCountIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	ctx := context.Background()
	save := func(payload string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		api.saveProcedures(response, catalogMutationRequest(t, api, http.MethodPut, "/v1/procedures", bytes.NewBufferString(payload)))
		return response
	}
	load := func(id string) procedure {
		t.Helper()
		response := httptest.NewRecorder()
		api.listProcedures(response, httptest.NewRequest(http.MethodGet, "/v1/procedures", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET procedures: %d %s", response.Code, response.Body.String())
		}
		var catalog []procedure
		if err := json.Unmarshal(response.Body.Bytes(), &catalog); err != nil {
			t.Fatal(err)
		}
		for _, item := range catalog {
			if item.ID == id {
				return item
			}
		}
		t.Fatalf("procedure %q missing in GET: %s", id, response.Body.String())
		return procedure{}
	}

	// Older clients can still PUT procedures without the newly introduced key.
	legacy := `[{"id":"therapy","name":"Therapy","durationMinutes":30,"preparationMinutes":20,"isRelaxing":false,"minIntervalDays":1,"defaultWeeklyCount":3}]`
	if response := save(legacy); response.Code != http.StatusOK {
		t.Fatalf("legacy PUT: %d %s", response.Code, response.Body.String())
	} else {
		var saved []procedure
		if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil || len(saved) != 1 || saved[0].DefaultSessionCount != 1 {
			t.Fatalf("legacy response default: %s (%v)", response.Body.String(), err)
		}
	}
	if actual := load("therapy"); actual.DefaultSessionCount != 1 || actual.DefaultWeeklyCount != 3 {
		t.Fatalf("legacy count/weekly defaults: %+v", actual)
	}

	// SQL inserts outside the API also receive the DB default.
	if _, err := db.Exec(ctx, "INSERT INTO procedures (id,name,duration_minutes) VALUES ('sql-default','Raw insert',30)"); err != nil {
		t.Fatal(err)
	}
	if actual := load("sql-default"); actual.DefaultSessionCount != 1 {
		t.Fatalf("database default not exposed by GET: %+v", actual)
	}

	updated := `[{"id":"therapy","name":"Therapy","durationMinutes":30,"preparationMinutes":20,"minIntervalDays":1,"defaultWeeklyCount":3,"defaultSessionCount":365}]`
	if response := save(updated); response.Code != http.StatusOK {
		t.Fatalf("update count PUT: %d %s", response.Code, response.Body.String())
	}
	if actual := load("therapy"); actual.DefaultSessionCount != 365 || actual.DefaultWeeklyCount != 3 {
		t.Fatalf("count/weekly roundtrip: %+v", actual)
	}
	var stored int
	if err := db.QueryRow(ctx, "SELECT default_session_count FROM procedures WHERE id='therapy'").Scan(&stored); err != nil || stored != 365 {
		t.Fatalf("database roundtrip: %d (%v)", stored, err)
	}

	for _, invalid := range []string{"0", "-1", "366", "null", "1.5", `"2"`} {
		payload := fmt.Sprintf(`[{"id":"therapy","name":"Therapy","durationMinutes":30,"preparationMinutes":20,"minIntervalDays":1,"defaultWeeklyCount":3,"defaultSessionCount":%s}]`, invalid)
		if response := save(payload); response.Code != http.StatusBadRequest {
			t.Errorf("invalid defaultSessionCount %s: %d %s", invalid, response.Code, response.Body.String())
		}
	}
	if actual := load("therapy"); actual.DefaultSessionCount != 365 {
		t.Fatalf("invalid values changed stored count: %+v", actual)
	}
	if _, err := db.Exec(ctx, "UPDATE procedures SET default_session_count=0 WHERE id='therapy'"); err == nil {
		t.Fatal("database constraint allowed default_session_count=0")
	}
}
