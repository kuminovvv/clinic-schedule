package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProcedureCatalogPrerequisitesIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	ctx := context.Background()
	base := func(id string, relaxing bool, prerequisite string) procedure {
		return procedure{ID: id, Name: id, DurationMinutes: 30, MinIntervalDays: 1, DefaultWeeklyCount: 1, IsRelaxing: relaxing, DefaultPrerequisiteID: prerequisite}
	}
	save := func(payload any) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		api.saveProcedures(response, catalogMutationRequest(t, api, http.MethodPut, "/v1/procedures", bytes.NewReader(data)))
		return response
	}
	saveRaw := func(payload string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		api.saveProcedures(response, catalogMutationRequest(t, api, http.MethodPut, "/v1/procedures", strings.NewReader(payload)))
		return response
	}
	get := func(id string) procedure {
		t.Helper()
		response := httptest.NewRecorder()
		api.listProcedures(response, httptest.NewRequest(http.MethodGet, "/v1/procedures", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET: %d %s", response.Code, response.Body.String())
		}
		var list []procedure
		if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		for _, item := range list {
			if item.ID == id {
				return item
			}
		}
		t.Fatalf("GET missing %s: %s", id, response.Body.String())
		return procedure{}
	}
	remove := func(id string) *httptest.ResponseRecorder {
		t.Helper()
		req := catalogMutationRequest(t, api, http.MethodDelete, "/v1/procedures/"+id, nil)
		req.SetPathValue("id", id)
		response := httptest.NewRecorder()
		api.deleteProcedure(response, req)
		return response
	}
	assertStatus := func(name string, response *httptest.ResponseRecorder, status int) {
		t.Helper()
		if response.Code != status {
			t.Fatalf("%s: expected %d, got %d: %s", name, status, response.Code, response.Body.String())
		}
	}

	// Inserting a dependency before the referenced relaxing procedure works in
	// a single batch, regardless of input ordering.
	assertStatus("initial batch", save([]procedure{base("normal", false, "relax"), base("relax", true, "")}), http.StatusOK)
	if got := get("normal").DefaultPrerequisiteID; got != "relax" {
		t.Fatalf("GET prerequisite = %q, want relax", got)
	}
	if got := get("relax").DefaultPrerequisiteID; got != "" {
		t.Fatalf("GET must expose empty string for no prerequisite, got %q", got)
	}
	var stored *string
	if err := db.QueryRow(ctx, "SELECT default_prerequisite_id FROM procedures WHERE id='normal'").Scan(&stored); err != nil || stored == nil || *stored != "relax" {
		t.Fatalf("DB prerequisite = %v (%v)", stored, err)
	}
	if err := db.QueryRow(ctx, "SELECT default_prerequisite_id FROM procedures WHERE id='relax'").Scan(&stored); err != nil || stored != nil {
		t.Fatalf("DB empty prerequisite should be NULL, got %v (%v)", stored, err)
	}

	// Old clients that omit the new field do not unexpectedly clear existing
	// prerequisites. New procedures from those clients still receive no default.
	assertStatus("legacy update", saveRaw(`[{"id":"normal","name":"renamed","durationMinutes":30,"minIntervalDays":1,"defaultWeeklyCount":1}]`), http.StatusOK)
	if got := get("normal"); got.DefaultPrerequisiteID != "relax" || got.Name != "renamed" {
		t.Fatalf("legacy update lost prerequisite or name: %+v", got)
	}
	assertStatus("legacy insert", saveRaw(`[{"id":"legacy","name":"legacy","durationMinutes":30,"minIntervalDays":1,"defaultWeeklyCount":1}]`), http.StatusOK)
	if got := get("legacy").DefaultPrerequisiteID; got != "" {
		t.Fatalf("legacy insert prerequisite = %q", got)
	}

	invalid := []struct {
		name  string
		items any
	}{
		{"self", []procedure{base("normal", false, "normal")}},
		{"unknown", []procedure{base("normal", false, "missing")}},
		{"non-relaxing prerequisite", []procedure{base("normal", false, "legacy")}},
		{"relaxing target", []procedure{base("relax", true, "relax")}},
		{"demote referenced relaxing", []procedure{base("relax", false, "")}},
		{"duplicate ids", []procedure{base("normal", false, ""), base("normal", false, "")}},
		{"atomic invalid batch", []procedure{base("brand-new", false, ""), base("normal", false, "missing")}},
	}
	for _, test := range invalid {
		assertStatus(test.name, save(test.items), http.StatusBadRequest)
		if got := get("normal").DefaultPrerequisiteID; got != "relax" {
			t.Fatalf("%s modified stored dependency to %q", test.name, got)
		}
	}
	for _, raw := range []string{`null`, `123`, `true`, `{}`} {
		payload := `[{"id":"normal","name":"normal","durationMinutes":30,"minIntervalDays":1,"defaultWeeklyCount":1,"defaultPrerequisiteId":` + raw + `}]`
		assertStatus("invalid JSON prerequisite "+raw, saveRaw(payload), http.StatusBadRequest)
	}
	var exists bool
	if err := db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM procedures WHERE id='brand-new')").Scan(&exists); err != nil || exists {
		t.Fatalf("invalid batch must roll back all inserts: %v (%v)", exists, err)
	}
	assertStatus("delete prerequisite", remove("relax"), http.StatusConflict)
	if _, err := db.Exec(ctx, "DELETE FROM procedures WHERE id='relax'"); err == nil {
		t.Fatal("database foreign key allowed deletion of a referenced procedure")
	}

	// A batch can simultaneously remove dependencies and disable relaxing.
	assertStatus("batch clear and demote", save([]procedure{base("relax", false, ""), base("normal", false, "")}), http.StatusOK)
	if got := get("normal").DefaultPrerequisiteID; got != "" {
		t.Fatalf("clearing prerequisite left %q", got)
	}
	assertStatus("delete former prerequisite", remove("relax"), http.StatusNoContent)
	if err := db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM procedures WHERE id='relax')").Scan(&exists); err != nil || exists {
		t.Fatalf("former prerequisite was not deleted: %v (%v)", exists, err)
	}
}
