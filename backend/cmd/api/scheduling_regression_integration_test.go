package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func isolatedScheduleDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("CLINIC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CLINIC_TEST_DATABASE_URL not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	id, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "schedule_regression_" + strings.ReplaceAll(id, "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("temporary schema cleanup: %v", err)
		}
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(files) < 7 {
		t.Fatalf("migrations missing: %v", err)
	}
	for _, path := range files {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("migration %s: %v", path, err)
		}
	}
	return db
}

func TestScheduleRegenerationPreservesManualChangesAndRollsBackOnClosures(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	ctx := context.Background()
	call := func(method, path string, payload any, handler func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		response := httptest.NewRecorder()
		handler(response, req)
		return response
	}
	date := "2099-01-05" // Monday.
	week := defaultWorkWeek()
	week[0].End = "11:00"
	week[0].Breaks = []weeklyBreak{{ID: "quartz", Name: "Кварцевание", Start: "09:30", End: "10:00"}}
	week[1].Open = false
	value := settings{TimeStepMinutes: 10, DisplayStartHour: 8, DisplayEndHour: 18, WorkWeek: week}
	if r := call(http.MethodPut, "/v1/calendar-settings", value, api.saveSettings); r.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", r.Code, r.Body.String())
	}
	procedureID := "regression-treatment"
	procedures := []procedure{{ID: procedureID, Name: "Терапия", DurationMinutes: 30, MinIntervalDays: 1, DefaultWeeklyCount: 3}}
	if r := call(http.MethodPut, "/v1/procedures", procedures, api.saveProcedures); r.Code != http.StatusOK {
		t.Fatalf("procedures: %d %s", r.Code, r.Body.String())
	}
	plan := dayPlan{Date: date, Plans: []patientPlan{{PatientNumber: 1, Severity: "medium", StayDays: 4, ProcedureIDs: []string{procedureID}, ProcedureCounts: map[string]int{procedureID: 3}}}}
	if r := call(http.MethodPut, "/v1/day-plans", plan, api.saveDayPlan); r.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", r.Code, r.Body.String())
	}
	generate := func(want int) {
		t.Helper()
		r := call(http.MethodPost, "/v1/schedules/generate", scheduleRequest{Date: date}, api.generate)
		if r.Code != want {
			t.Fatalf("generate: want %d got %d: %s", want, r.Code, r.Body.String())
		}
	}
	generate(http.StatusOK)
	var first appointment
	err := db.QueryRow(ctx, `SELECT id,title,to_char(start_at,'YYYY-MM-DD"T"HH24:MI'),to_char(end_at,'YYYY-MM-DD"T"HH24:MI'),description,color,patient_number
		FROM appointments WHERE source_plan_date=$1 AND start_at::date=$1::date`, date).Scan(
		&first.ID, &first.Title, &first.StartAt, &first.EndAt, &first.Description, &first.Color, &first.PatientNumber)
	if err != nil || first.StartAt != date+"T09:00" {
		t.Fatalf("Monday procedure must be before the quartz closure: %+v %v", first, err)
	}
	first.StartAt, first.EndAt = date+"T10:00", date+"T10:30"
	first.Status = "scheduled"
	first.GeneratedByPlanner = true
	body, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/v1/appointments/"+first.ID, bytes.NewReader(body))
	req.SetPathValue("id", first.ID)
	recorder := httptest.NewRecorder()
	api.saveAppointment(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("manual movement: %d %s", recorder.Code, recorder.Body.String())
	}
	generate(http.StatusOK)
	var protected, total int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM appointments WHERE id=$1 AND manually_modified AND start_at=$2::timestamp", first.ID, first.StartAt).Scan(&protected); err != nil || protected != 1 {
		t.Fatalf("manual appointment changed during regeneration: %d %v", protected, err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM appointments WHERE source_plan_date=$1 AND status='scheduled'", date).Scan(&total); err != nil || total != 3 {
		t.Fatalf("expected three scheduled procedures: %d %v", total, err)
	}
	// A cancelled generated session no longer occupies time. Regeneration
	// replaces the missed treatment while retaining its cancelled record.
	var cancelledID string
	if err := db.QueryRow(ctx, "SELECT id FROM appointments WHERE source_plan_date=$1 AND start_at::date='2099-01-07'::date AND status='scheduled'", date).Scan(&cancelledID); err != nil {
		t.Fatal(err)
	}
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/appointments/"+cancelledID, nil)
	deleteReq.SetPathValue("id", cancelledID)
	deleteResponse := httptest.NewRecorder()
	api.deleteAppointment(deleteResponse, deleteReq)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("cancel: %d %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	generate(http.StatusOK)
	if err := db.QueryRow(ctx, "SELECT count(*) FROM appointments WHERE source_plan_date=$1 AND status='scheduled'", date).Scan(&total); err != nil || total != 3 {
		t.Fatalf("cancelled treatment not replaced: %d %v", total, err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM appointments WHERE id=$1 AND status='cancelled' AND manually_modified", cancelledID).Scan(&total); err != nil || total != 1 {
		t.Fatalf("cancelled record not preserved: %d %v", total, err)
	}
	// A new closure makes the remaining course impossible. The failed commit
	// must keep all prior sessions, including the manually moved one.
	week[2].Open = false
	value.WorkWeek = week
	if r := call(http.MethodPut, "/v1/calendar-settings", value, api.saveSettings); r.Code != http.StatusOK {
		t.Fatalf("updated settings: %d %s", r.Code, r.Body.String())
	}
	generate(http.StatusUnprocessableEntity)
	if err := db.QueryRow(ctx, "SELECT count(*) FROM appointments WHERE source_plan_date=$1 AND status='scheduled'", date).Scan(&total); err != nil || total != 3 {
		t.Fatalf("failed regeneration erased previous sessions: %d %v", total, err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM appointments WHERE id=$1 AND manually_modified AND start_at=$2::timestamp", first.ID, first.StartAt).Scan(&protected); err != nil || protected != 1 {
		t.Fatalf("failed regeneration moved manual session: %d %v", protected, err)
	}
}
