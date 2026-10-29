package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPatientPrerequisiteOverridesIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	ctx := context.Background()
	call := func(payload any, save func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		result := httptest.NewRecorder()
		save(result, httptest.NewRequest(http.MethodPut, "/v1/day-plans", bytes.NewReader(body)))
		return result
	}
	procedures := []procedure{
		{ID: "normal", Name: "Normal", DurationMinutes: 30, MinIntervalDays: 1, DefaultWeeklyCount: 1, DefaultPrerequisiteID: "relax-a"},
		{ID: "relax-a", Name: "Relax A", DurationMinutes: 30, MinIntervalDays: 1, DefaultWeeklyCount: 1, IsRelaxing: true},
		{ID: "relax-b", Name: "Relax B", DurationMinutes: 30, MinIntervalDays: 1, DefaultWeeklyCount: 1, IsRelaxing: true},
	}
	response := httptest.NewRecorder()
	data, err := json.Marshal(procedures)
	if err != nil {
		t.Fatal(err)
	}
	api.saveProcedures(response, catalogMutationRequest(t, api, http.MethodPut, "/v1/procedures", bytes.NewReader(data)))
	if response.Code != http.StatusOK {
		t.Fatalf("save catalog: %d %s", response.Code, response.Body.String())
	}

	date := "2099-01-05"
	plan := dayPlan{Date: date, Plans: []patientPlan{{
		PatientNumber: 1, Severity: "high", StayDays: 5,
		ProcedureIDs:           []string{"normal", "relax-a", "relax-b"},
		ProcedureCounts:        map[string]int{"normal": 1, "relax-a": 1, "relax-b": 1},
		ProcedurePrerequisites: map[string]string{"normal": "relax-a"},
	}}}
	put := func(want int) {
		t.Helper()
		response := call(plan, api.saveDayPlan)
		if response.Code != want {
			t.Fatalf("save day plan: want %d, got %d %s", want, response.Code, response.Body.String())
		}
	}
	load := func() patientPlan {
		t.Helper()
		fresh, err := loadDayPlan(ctx, db, date)
		if err != nil || len(fresh.Plans) != 1 {
			t.Fatalf("load day plan: %+v (%v)", fresh, err)
		}
		plan = fresh
		return fresh.Plans[0]
	}
	put(http.StatusOK)
	if got := load(); len(got.ProcedurePrerequisiteOverrides) != 0 || got.ProcedurePrerequisites["normal"] != "relax-a" {
		t.Fatalf("default link should have no explicit override: %+v", got)
	}
	if plan.Revision != 1 {
		t.Fatalf("unexpected initial revision %d", plan.Revision)
	}

	other := "relax-b"
	plan.Plans[0].ProcedurePrerequisites["normal"] = other
	plan.Plans[0].ProcedurePrerequisiteOverrides = map[string]*string{"normal": &other}
	put(http.StatusOK)
	if got := load(); got.ProcedurePrerequisiteOverrides["normal"] == nil || *got.ProcedurePrerequisiteOverrides["normal"] != other || got.ProcedurePrerequisites["normal"] != other {
		t.Fatalf("explicit override lost: %+v", got)
	}
	var persisted bool
	var overriddenID *string
	if err := db.QueryRow(ctx, "SELECT prerequisite_override_set,prerequisite_override_id FROM day_plan_items WHERE schedule_date=$1 AND patient_number=1 AND procedure_id='normal'", date).Scan(&persisted, &overriddenID); err != nil || !persisted || overriddenID == nil || *overriddenID != other {
		t.Fatalf("explicit override DB roundtrip: %v %v (%v)", persisted, overriddenID, err)
	}

	delete(plan.Plans[0].ProcedurePrerequisites, "normal")
	plan.Plans[0].ProcedurePrerequisiteOverrides["normal"] = nil
	put(http.StatusOK)
	if got := load(); len(got.ProcedurePrerequisites) != 0 {
		t.Fatalf("explicit null still has effective dependency: %+v", got)
	} else if override, present := got.ProcedurePrerequisiteOverrides["normal"]; !present || override != nil {
		t.Fatalf("explicit null override must survive GET: %+v", got.ProcedurePrerequisiteOverrides)
	}
	if err := db.QueryRow(ctx, "SELECT prerequisite_override_set,prerequisite_override_id FROM day_plan_items WHERE schedule_date=$1 AND patient_number=1 AND procedure_id='normal'", date).Scan(&persisted, &overriddenID); err != nil || !persisted || overriddenID != nil {
		t.Fatalf("explicit null DB roundtrip: %v %v (%v)", persisted, overriddenID, err)
	}
	previousRevision := plan.Revision
	put(http.StatusOK)
	load()
	if plan.Revision != previousRevision {
		t.Fatalf("identical save with explicit null changed revision: %d to %d", previousRevision, plan.Revision)
	}

	for _, test := range []struct {
		name   string
		change func(*patientPlan)
		status int
	}{
		{"override target missing", func(p *patientPlan) { p.ProcedurePrerequisiteOverrides = map[string]*string{"absent": nil} }, http.StatusBadRequest},
		{"override id missing", func(p *patientPlan) {
			id := "absent"
			p.ProcedurePrerequisiteOverrides = map[string]*string{"normal": &id}
		}, http.StatusBadRequest},
		{"override mismatches effective", func(p *patientPlan) {
			id := "relax-b"
			p.ProcedurePrerequisiteOverrides = map[string]*string{"normal": &id}
		}, http.StatusBadRequest},
		{"explicit null mismatches effective", func(p *patientPlan) { p.ProcedurePrerequisites = map[string]string{"normal": "relax-a"} }, http.StatusBadRequest},
		{"override on relaxing target", func(p *patientPlan) { p.ProcedurePrerequisiteOverrides = map[string]*string{"relax-a": nil} }, http.StatusUnprocessableEntity},
		{"override on medium severity", func(p *patientPlan) { p.Severity = "medium" }, http.StatusBadRequest},
	} {
		planCopy := plan
		planCopy.Plans = append([]patientPlan(nil), plan.Plans...)
		planCopy.Plans[0].ProcedurePrerequisites = map[string]string{}
		planCopy.Plans[0].ProcedurePrerequisiteOverrides = map[string]*string{"normal": nil}
		test.change(&planCopy.Plans[0])
		response := call(planCopy, api.saveDayPlan)
		if response.Code != test.status {
			t.Errorf("%s: %d %s (expected %d)", test.name, response.Code, response.Body.String(), test.status)
		}
	}
	if err := db.QueryRow(ctx, "SELECT revision FROM day_plans WHERE schedule_date=$1", date).Scan(&previousRevision); err != nil || previousRevision != plan.Revision {
		t.Fatalf("invalid overrides mutated persisted plan: revision %d (%v)", previousRevision, err)
	}
	if _, err := db.Exec(ctx, "UPDATE day_plan_items SET prerequisite_override_set=FALSE,prerequisite_override_id='relax-a' WHERE schedule_date=$1 AND patient_number=1 AND procedure_id='normal'", date); err == nil {
		t.Fatal("database accepted unset override with a non-NULL value")
	}
}
