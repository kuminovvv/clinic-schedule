package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func courseRulesRequest(t *testing.T, handler func(http.ResponseWriter, *http.Request), method, path, id string, value any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if id != "" {
		req.SetPathValue("id", id)
	}
	recorder := httptest.NewRecorder()
	handler(recorder, req)
	return recorder
}

func createPairedPatientForTest(t *testing.T, api *server, day string, stay int) []appointment {
	t.Helper()
	procedures := []procedure{
		{ID: "paired-relax", Name: "Relax", DurationMinutes: 40, IsRelaxing: true, MinIntervalDays: 1, DefaultWeeklyCount: 7},
		{ID: "paired-target", Name: "Complex", DurationMinutes: 40, MinIntervalDays: 1, DefaultWeeklyCount: 7},
	}
	if got := courseRulesRequest(t, api.saveProcedures, "PUT", "/v1/procedures", "", procedures); got.Code != 200 {
		t.Fatalf("save procedures: %d %s", got.Code, got.Body.String())
	}
	plan := dayPlan{Date: day, Plans: []patientPlan{{
		PatientNumber: 1, Severity: "high", StayDays: stay,
		ProcedureIDs:           []string{"paired-relax", "paired-target"},
		ProcedureCounts:        map[string]int{"paired-relax": 1, "paired-target": 1},
		ProcedurePrerequisites: map[string]string{"paired-target": "paired-relax"},
	}}}
	if got := courseRulesRequest(t, api.saveDayPlan, "PUT", "/v1/day-plans", "", plan); got.Code != 200 {
		t.Fatalf("save paired plan: %d %s", got.Code, got.Body.String())
	}
	generated := courseRulesRequest(t, api.generate, "POST", "/v1/schedules/generate", "", scheduleRequest{Date: day})
	if generated.Code != 200 {
		t.Fatalf("generate pair: %d %s", generated.Code, generated.Body.String())
	}
	var output scheduleResponse
	if err := json.Unmarshal(generated.Body.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.ScheduledCount != 2 || output.UnscheduledCount != 0 {
		t.Fatalf("missing prerequisite pair: %+v", output)
	}
	return output.Appointments
}

func TestManualMoveAndCancellationKeepPrerequisitePairIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	day := "2099-01-05"
	items := createPairedPatientForTest(t, api, day, 2)
	var relax, complex appointment
	for _, item := range items {
		if item.ProcedureID == "paired-relax" {
			relax = item
		} else {
			complex = item
		}
	}
	if relax.ID == "" || complex.ID == "" || relax.EndAt > complex.StartAt {
		t.Fatalf("invalid generated prerequisite order: %+v", items)
	}
	relax.StartAt, relax.EndAt = "2099-01-06T09:00", "2099-01-06T09:40"
	if result := courseRulesRequest(t, api.saveAppointment, "PUT", "/v1/appointments/"+relax.ID, relax.ID, relax); result.Code != http.StatusConflict {
		t.Fatalf("moving prerequisite away from target should fail: %d %s", result.Code, result.Body.String())
	}
	complex.StartAt, complex.EndAt = "2099-01-06T09:00", "2099-01-06T09:40"
	if result := courseRulesRequest(t, api.saveAppointment, "PUT", "/v1/appointments/"+complex.ID, complex.ID, complex); result.Code != http.StatusConflict {
		t.Fatalf("moving complex treatment away from prerequisite should fail: %d %s", result.Code, result.Body.String())
	}
	otherPatient := 2
	complex.PatientNumber = &otherPatient
	if result := courseRulesRequest(t, api.saveAppointment, "PUT", "/v1/appointments/"+complex.ID, complex.ID, complex); result.Code != http.StatusConflict {
		t.Fatalf("reassigning generated complex treatment should fail: %d %s", result.Code, result.Body.String())
	}
	if result := courseRulesRequest(t, api.deleteAppointment, "DELETE", "/v1/appointments/"+relax.ID, relax.ID, nil); result.Code != http.StatusConflict {
		t.Fatalf("cancelling required relaxation should fail: %d %s", result.Code, result.Body.String())
	}
	if result := courseRulesRequest(t, api.deleteAppointment, "DELETE", "/v1/appointments/"+complex.ID, complex.ID, nil); result.Code != http.StatusNoContent {
		t.Fatalf("cancelling target should succeed: %d %s", result.Code, result.Body.String())
	}
	if result := courseRulesRequest(t, api.deleteAppointment, "DELETE", "/v1/appointments/"+relax.ID, relax.ID, nil); result.Code != http.StatusNoContent {
		t.Fatalf("cancelling now-unused relaxation should succeed: %d %s", result.Code, result.Body.String())
	}
	var cancelled int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM appointments WHERE source_plan_date=$1 AND status='cancelled'", day).Scan(&cancelled); err != nil || cancelled != 2 {
		t.Fatalf("expected both appointments to be cancelled: %d %v", cancelled, err)
	}
}

func TestManualMoveRespectsRollingWeeklyCourseLimitIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	day := "2099-01-05"
	p := []procedure{{ID: "weekly-relax", Name: "Relax", DurationMinutes: 40, MinIntervalDays: 1, DefaultWeeklyCount: 1, IsRelaxing: true}}
	if r := courseRulesRequest(t, api.saveProcedures, "PUT", "/v1/procedures", "", p); r.Code != 200 {
		t.Fatalf("save: %d %s", r.Code, r.Body.String())
	}
	plan := dayPlan{Date: day, Plans: []patientPlan{{PatientNumber: 1, Severity: "high", StayDays: 9, ProcedureIDs: []string{"weekly-relax"}, ProcedureCounts: map[string]int{"weekly-relax": 2}}}}
	if r := courseRulesRequest(t, api.saveDayPlan, "PUT", "/v1/day-plans", "", plan); r.Code != 200 {
		t.Fatalf("plan: %d %s", r.Code, r.Body.String())
	}
	result := courseRulesRequest(t, api.generate, "POST", "/v1/schedules/generate", "", scheduleRequest{Date: day})
	if result.Code != 200 {
		t.Fatalf("generate: %d %s", result.Code, result.Body.String())
	}
	var output scheduleResponse
	if err := json.Unmarshal(result.Body.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Appointments) != 2 {
		t.Fatalf("expected two weekly sessions: %+v", output)
	}
	latest := output.Appointments[1]
	latest.StartAt, latest.EndAt = "2099-01-06T09:00", "2099-01-06T09:40"
	if r := courseRulesRequest(t, api.saveAppointment, "PUT", "/v1/appointments/"+latest.ID, latest.ID, latest); r.Code != http.StatusConflict {
		t.Fatalf("move must not exceed weekly count: %d %s", r.Code, r.Body.String())
	}
}

func TestAbsenceMovesMandatoryPairTogetherIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	day := "2099-01-05"
	createPairedPatientForTest(t, api, day, 2)
	preview := courseRulesRequest(t, api.savePatientAbsence, "POST", "/v1/patient-absences", "", patientAbsence{
		Date: day, PatientNumber: 1, From: day, To: day, Reason: "illness", Preview: true,
	})
	if preview.Code != 200 {
		t.Fatalf("absence preview: %d %s", preview.Code, preview.Body.String())
	}
	var output absenceResult
	if err := json.Unmarshal(preview.Body.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.UnscheduledCount != 0 || output.RescheduledCount != 2 {
		t.Fatalf("absence should relocate the entire pair: %+v", output)
	}
	if output.Appointments[0].StartAt[:10] != "2099-01-06" || output.Appointments[1].StartAt[:10] != "2099-01-06" {
		t.Fatalf("pair must relocate to the same following date: %+v", output.Appointments)
	}
	apply := courseRulesRequest(t, api.savePatientAbsence, "POST", "/v1/patient-absences", "", patientAbsence{
		Date: day, PatientNumber: 1, From: day, To: day, Reason: "illness",
	})
	if apply.Code != 200 {
		t.Fatalf("apply pair relocation: %d %s", apply.Code, apply.Body.String())
	}
}
