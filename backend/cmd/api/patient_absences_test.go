package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProcedureReservesTherapyAndPreparation(t *testing.T) {
	day := time.Date(2099, 1, 5, 0, 0, 0, 0, time.Local)
	procedures := map[string]procedure{"therapy": {
		ID: "therapy", Name: "Therapy", DurationMinutes: 30, PreparationMinutes: 20,
	}}
	result, err := createSchedule(day, []scheduleTask{{PatientNumber: 1, Severity: "medium", ProcedureID: "therapy"}}, procedures, settings{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 1 || result.Appointments[0].StartAt != "2099-01-05T09:00" || result.Appointments[0].EndAt != "2099-01-05T09:50" {
		t.Fatalf("treatment and preparation must reserve 50 minutes: %+v", result)
	}
}

func TestCourseOrdersSeverityAndRelaxationWithReversedPatients(t *testing.T) {
	day := time.Date(2099, 1, 5, 0, 0, 0, 0, time.Local)
	procedures := map[string]procedure{
		"normal": {ID: "normal", Name: "Normal", DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1},
		"relax":  {ID: "relax", Name: "Relax", DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1, IsRelaxing: true},
	}
	plans := []patientPlan{
		{PatientNumber: 2, Severity: "low", StayDays: 1, ProcedureIDs: []string{"normal"}, ProcedureCounts: map[string]int{"normal": 1}},
		{PatientNumber: 3, Severity: "medium", StayDays: 1, ProcedureIDs: []string{"normal"}, ProcedureCounts: map[string]int{"normal": 1}},
		{PatientNumber: 1, Severity: "high", StayDays: 1, ProcedureIDs: []string{"normal", "relax"}, ProcedureCounts: map[string]int{"normal": 1, "relax": 1}},
	}
	result, err := createCourseSchedule(day, plans, procedures, settings{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Appointments) != 4 {
		t.Fatalf("unexpected schedule: %+v", result)
	}
	wantPatients := []int{1, 1, 3, 2}
	for index, patient := range wantPatients {
		if *result.Appointments[index].PatientNumber != patient {
			t.Fatalf("severity precedence incorrect at %d: %+v", index, result.Appointments)
		}
	}
	if result.Appointments[0].ProcedureID != "relax" {
		t.Fatalf("high-severity patient should relax first: %+v", result.Appointments)
	}
}

func TestCourseSkipsRegisteredAbsences(t *testing.T) {
	day := time.Date(2099, 1, 5, 0, 0, 0, 0, time.Local)
	procedures := map[string]procedure{"p": {ID: "p", Name: "Therapy", DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1}}
	plans := []patientPlan{{PatientNumber: 1, Severity: "high", StayDays: 3, ProcedureIDs: []string{"p"}, ProcedureCounts: map[string]int{"p": 2}}}
	absences := map[int]map[string]bool{1: {"2099-01-05": true}}
	result, err := createCourseScheduleWithAbsences(day, plans, procedures, settings{}, nil, absences)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 2 || result.Appointments[0].StartAt[:10] != "2099-01-06" || result.Appointments[1].StartAt[:10] != "2099-01-07" {
		t.Fatalf("absent day must be skipped without losing sessions: %+v", result)
	}
}

func callAbsenceHandler(t *testing.T, api *server, payload patientAbsence) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	api.savePatientAbsence(response, httptest.NewRequest(http.MethodPost, "/v1/patient-absences", bytes.NewReader(encoded)))
	return response
}

func testAbsencePatient(t *testing.T, api *server, date string, stayDays, sessions int) {
	t.Helper()
	tx := api.db
	ctx := context.Background()
	if _, err := tx.Exec(ctx, `INSERT INTO procedures(id,name,duration_minutes,preparation_minutes,min_interval_days,default_weekly_count) VALUES ('absence-therapy','Therapy',30,20,1,2)`); err != nil {
		t.Fatal(err)
	}
	plan := dayPlan{Date: date, Plans: []patientPlan{{PatientNumber: 1, Severity: "high", StayDays: stayDays, ProcedureIDs: []string{"absence-therapy"}, ProcedureCounts: map[string]int{"absence-therapy": sessions}}}}
	encoded, _ := json.Marshal(plan)
	w := httptest.NewRecorder()
	api.saveDayPlan(w, httptest.NewRequest(http.MethodPut, "/v1/day-plans", bytes.NewReader(encoded)))
	if w.Code != 200 {
		t.Fatalf("save plan: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	encoded, _ = json.Marshal(scheduleRequest{Date: date})
	api.generate(w, httptest.NewRequest(http.MethodPost, "/v1/schedules/generate", bytes.NewReader(encoded)))
	if w.Code != 200 {
		t.Fatalf("generate: %d %s", w.Code, w.Body.String())
	}
}

func TestAbsencePreviewAndAtomicTargetedRelocationIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	ctx := context.Background()
	date := "2099-01-05"
	testAbsencePatient(t, api, date, 3, 2)
	var movedID, untouchedID, untouchedTime string
	if err := db.QueryRow(ctx, `SELECT id FROM appointments WHERE source_plan_date=$1 AND start_at::date=$1::date`, date).Scan(&movedID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT id,to_char(start_at,'YYYY-MM-DD"T"HH24:MI') FROM appointments WHERE source_plan_date=$1 AND start_at::date=$1::date+2`, date).Scan(&untouchedID, &untouchedTime); err != nil {
		t.Fatal(err)
	}
	input := patientAbsence{Date: date, PatientNumber: 1, From: date, To: date, Reason: "illness", Preview: true}
	preview := callAbsenceHandler(t, api, input)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", preview.Code, preview.Body.String())
	}
	var result absenceResult
	if err := json.Unmarshal(preview.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.AffectedCount != 1 || result.RescheduledCount != 1 || result.UnscheduledCount != 0 || len(result.Appointments) != 1 || result.Appointments[0].ID != movedID || result.Appointments[0].StartAt != "2099-01-06T09:00" {
		t.Fatalf("targeted preview incorrect: %+v", result)
	}
	var absentCount int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM patient_absences").Scan(&absentCount); err != nil || absentCount != 0 {
		t.Fatalf("preview persisted absence: %d, %v", absentCount, err)
	}
	input.Preview = false
	applied := callAbsenceHandler(t, api, input)
	if applied.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", applied.Code, applied.Body.String())
	}
	var startMoved, startUnchanged string
	if err := db.QueryRow(ctx, `SELECT to_char(start_at,'YYYY-MM-DD"T"HH24:MI') FROM appointments WHERE id=$1`, movedID).Scan(&startMoved); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT to_char(start_at,'YYYY-MM-DD"T"HH24:MI') FROM appointments WHERE id=$1`, untouchedID).Scan(&startUnchanged); err != nil {
		t.Fatal(err)
	}
	if startMoved != "2099-01-06T09:00" || startUnchanged != untouchedTime {
		t.Fatalf("unaffected session was changed: moved %s unaffected %s", startMoved, startUnchanged)
	}
	if repeat := callAbsenceHandler(t, api, input); repeat.Code != http.StatusOK {
		t.Fatalf("idempotent repeat: %d %s", repeat.Code, repeat.Body.String())
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM patient_absences").Scan(&absentCount); err != nil || absentCount != 1 {
		t.Fatalf("absence not saved exactly once: %d %v", absentCount, err)
	}
	// A regular regeneration must still honor the persisted absence.
	encoded, _ := json.Marshal(scheduleRequest{Date: date})
	w := httptest.NewRecorder()
	api.generate(w, httptest.NewRequest(http.MethodPost, "/v1/schedules/generate", bytes.NewReader(encoded)))
	if w.Code != 200 {
		t.Fatalf("regenerate: %d %s", w.Code, w.Body.String())
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM appointments WHERE source_plan_date=$1 AND status='scheduled' AND start_at::date=$1::date`, date).Scan(&absentCount); err != nil || absentCount != 0 {
		t.Fatalf("regeneration restored absent-day treatment: %d %v", absentCount, err)
	}
}

func TestAbsenceCannotLoseSessionsWhenCourseIsFullIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	ctx := context.Background()
	testAbsencePatient(t, api, "2099-01-05", 1, 1)
	input := patientAbsence{Date: "2099-01-05", PatientNumber: 1, From: "2099-01-05", To: "2099-01-05", Reason: "other", Preview: true}
	preview := callAbsenceHandler(t, api, input)
	var proposed absenceResult
	if preview.Code != 200 || json.Unmarshal(preview.Body.Bytes(), &proposed) != nil || proposed.UnscheduledCount != 1 {
		t.Fatalf("preview should report impossible relocation: %d %s", preview.Code, preview.Body.String())
	}
	input.Preview = false
	apply := callAbsenceHandler(t, api, input)
	if apply.Code != 422 {
		t.Fatalf("incomplete relocation must fail atomically: %d %s", apply.Code, apply.Body.String())
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM patient_absences").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed absence was persisted: %d %v", count, err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM appointments WHERE status='scheduled' AND start_at::date='2099-01-05'::date").Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed absence lost original appointment: %d %v", count, err)
	}
}

func TestAbsenceRejectsManuallyModifiedSessionsIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	ctx := context.Background()
	testAbsencePatient(t, api, "2099-01-05", 3, 2)
	if _, err := db.Exec(ctx, `UPDATE appointments SET manually_modified=true WHERE source_plan_date='2099-01-05' AND start_at::date='2099-01-05'::date`); err != nil {
		t.Fatal(err)
	}
	response := callAbsenceHandler(t, api, patientAbsence{Date: "2099-01-05", PatientNumber: 1, From: "2099-01-05", To: "2099-01-05", Reason: "illness"})
	if response.Code != http.StatusConflict {
		t.Fatalf("manually edited appointment must be protected: %d %s", response.Code, response.Body.String())
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM patient_absences").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed absence persisted: %d %v", count, err)
	}
}

func TestAbsenceSurvivesPlanEditsAndDisappearsWhenPatientRemovedIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	ctx := context.Background()
	date := "2099-01-05"
	testAbsencePatient(t, api, date, 3, 2)
	input := patientAbsence{Date: date, PatientNumber: 1, From: date, To: date, Reason: "illness"}
	if r := callAbsenceHandler(t, api, input); r.Code != 200 {
		t.Fatalf("absence: %d %s", r.Code, r.Body.String())
	}
	plan, err := loadDayPlan(ctx, db, date)
	if err != nil {
		t.Fatal(err)
	}
	plan.Plans[0].Severity = "low"
	putPlan := func() *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(plan)
		rec := httptest.NewRecorder()
		api.saveDayPlan(rec, httptest.NewRequest(http.MethodPut, "/v1/day-plans", bytes.NewReader(encoded)))
		return rec
	}
	if r := putPlan(); r.Code != 200 {
		t.Fatalf("update existing patient: %d %s", r.Code, r.Body.String())
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM patient_absences WHERE schedule_date=$1", date).Scan(&count); err != nil || count != 1 {
		t.Fatalf("editing same patient must preserve absence: %d %v", count, err)
	}
	plan, err = loadDayPlan(ctx, db, date)
	if err != nil {
		t.Fatal(err)
	}
	plan.Plans = []patientPlan{}
	plan.SelectedPatientNumber = nil
	if r := putPlan(); r.Code != 200 {
		t.Fatalf("remove patient: %d %s", r.Code, r.Body.String())
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM patient_absences WHERE schedule_date=$1", date).Scan(&count); err != nil || count != 0 {
		t.Fatalf("removed patient's absences must cascade: %d %v", count, err)
	}
}

func TestProcedureDurationPersistsAsTherapyAndPreparationIntegration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	ctx := context.Background()
	api := &server{db: db}
	items := []procedure{{ID: "duration-test", Name: "Therapy", DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1, DefaultWeeklyCount: 1}}
	encoded, _ := json.Marshal(items)
	r := httptest.NewRecorder()
	api.saveProcedures(r, httptest.NewRequest(http.MethodPut, "/v1/procedures", bytes.NewReader(encoded)))
	if r.Code != 200 {
		t.Fatalf("save therapy and preparation: %d %s", r.Code, r.Body.String())
	}
	var therapy, prep int
	if err := db.QueryRow(ctx, "SELECT duration_minutes,preparation_minutes FROM procedures WHERE id='duration-test'").Scan(&therapy, &prep); err != nil || therapy != 30 || prep != 20 {
		t.Fatalf("duration fields lost: therapy %d prep %d error %v", therapy, prep, err)
	}
	items[0].DurationMinutes = 410
	items[0].PreparationMinutes = 20
	encoded, _ = json.Marshal(items)
	r = httptest.NewRecorder()
	api.saveProcedures(r, httptest.NewRequest(http.MethodPut, "/v1/procedures", bytes.NewReader(encoded)))
	if r.Code != 400 {
		t.Fatalf("total reservation longer than 420 minutes must be rejected: %d %s", r.Code, r.Body.String())
	}
}
