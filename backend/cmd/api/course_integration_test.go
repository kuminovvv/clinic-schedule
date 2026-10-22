package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCoursePersistenceIntegration(t *testing.T) {
	dsn := os.Getenv("CLINIC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CLINIC_TEST_DATABASE_URL to test PostgreSQL-backed courses")
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	date := "2099-01-05"
	relaxID, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	normalID, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer db.Close()
		ctx := context.Background()
		for _, statement := range []string{
			"DELETE FROM appointments WHERE source_plan_date=$1",
			"DELETE FROM schedule_reports WHERE schedule_date=$1",
			"DELETE FROM day_plans WHERE schedule_date=$1",
		} {
			if _, err := db.Exec(ctx, statement, date); err != nil {
				t.Errorf("cleanup failed: %v", err)
			}
		}
		if _, err := db.Exec(ctx, "DELETE FROM procedures WHERE id=ANY($1)", []string{relaxID, normalID}); err != nil {
			t.Errorf("procedure cleanup failed: %v", err)
		}
	})
	api := &server{db: db}
	call := func(method, path string, body any, handle func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRecorder()
		handle(r, httptest.NewRequest(method, path, bytes.NewReader(encoded)))
		return r
	}
	procedures := []procedure{
		{ID: normalID, Name: "Обычная", DurationMinutes: 50, MinIntervalDays: 1, DefaultWeeklyCount: 2},
		{ID: relaxID, Name: "Расслабляющая", DurationMinutes: 50, IsRelaxing: true, MinIntervalDays: 2, DefaultWeeklyCount: 3},
	}
	if r := call(http.MethodPut, "/v1/procedures", procedures, api.saveProcedures); r.Code != http.StatusOK {
		t.Fatalf("procedures: %d %s", r.Code, r.Body.String())
	}
	plan := dayPlan{Date: date, Plans: []patientPlan{{PatientNumber: 1, Severity: "high", StayDays: 5, ProcedureIDs: []string{normalID, relaxID}, ProcedureCounts: map[string]int{normalID: 2, relaxID: 3}}}}
	if r := call(http.MethodPut, "/v1/day-plans", plan, api.saveDayPlan); r.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", r.Code, r.Body.String())
	}
	loaded, err := loadDayPlan(context.Background(), db, date)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Plans) != 1 || loaded.Plans[0].StayDays != 5 || loaded.Plans[0].ProcedureCounts[relaxID] != 3 {
		t.Fatalf("course counts lost after reload: %+v", loaded)
	}
	var preview scheduleResponse
	r := call(http.MethodPost, "/v1/schedules/generate", scheduleRequest{Date: date, Preview: true}, api.generate)
	if r.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", r.Code, r.Body.String())
	}
	if err := json.Unmarshal(r.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.ScheduledCount != 5 || preview.UnscheduledCount != 0 {
		t.Fatalf("preview mismatch: %+v", preview)
	}
	var before int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM appointments WHERE source_plan_date=$1", date).Scan(&before); err != nil || before != 0 {
		t.Fatalf("preview must not persist records: %d %v", before, err)
	}
	if r := call(http.MethodPost, "/v1/schedules/generate", scheduleRequest{Date: date}, api.generate); r.Code != http.StatusOK {
		t.Fatalf("generate: %d %s", r.Code, r.Body.String())
	}
	if r := call(http.MethodPost, "/v1/schedules/generate", scheduleRequest{Date: date}, api.generate); r.Code != http.StatusOK {
		t.Fatalf("repeat generate: %d %s", r.Code, r.Body.String())
	}
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM appointments WHERE source_plan_date=$1", date).Scan(&count); err != nil || count != 5 {
		t.Fatalf("unexpected persisted sessions: %d %v", count, err)
	}
	var first appointment
	err = db.QueryRow(context.Background(), `SELECT id,title,to_char(start_at,'YYYY-MM-DD"T"HH24:MI'),to_char(end_at,'YYYY-MM-DD"T"HH24:MI'),description,color,patient_number
		FROM appointments WHERE source_plan_date=$1 AND source_procedure_id=$2 ORDER BY start_at LIMIT 1`, date, relaxID).Scan(
		&first.ID, &first.Title, &first.StartAt, &first.EndAt, &first.Description, &first.Color, &first.PatientNumber)
	if err != nil {
		t.Fatal(err)
	}
	first.Status = "scheduled"
	first.GeneratedByPlanner = true
	first.StartAt, first.EndAt = "2099-01-06T09:00", "2099-01-06T09:50"
	put := func() *httptest.ResponseRecorder {
		encoded, err := json.Marshal(first)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPut, "/v1/appointments/"+first.ID, bytes.NewReader(encoded))
		req.SetPathValue("id", first.ID)
		response := httptest.NewRecorder()
		api.saveAppointment(response, req)
		return response
	}
	if response := put(); response.Code != http.StatusConflict {
		t.Fatalf("interval violation must reject manual move: %d %s", response.Code, response.Body.String())
	}
	first.StartAt, first.EndAt = "2099-01-10T09:00", "2099-01-10T09:50"
	if response := put(); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cannot move past discharge: %d %s", response.Code, response.Body.String())
	}
	rows, err := db.Query(context.Background(), "SELECT to_char(start_at,'YYYY-MM-DD'),source_procedure_id FROM appointments WHERE source_plan_date=$1 ORDER BY start_at,source_procedure_id", date)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	lastDates := map[string]string{}
	seen := map[string]bool{}
	for rows.Next() {
		var d, id string
		if err := rows.Scan(&d, &id); err != nil {
			t.Fatal(err)
		}
		if seen[d+id] {
			t.Fatalf("same procedure repeated on %s", d)
		}
		seen[d+id] = true
		if id == relaxID && lastDates[id] != "" {
			previous := calendarDayNumber(mustDate(t, lastDates[id]))
			current := calendarDayNumber(mustDate(t, d))
			if current-previous < 2 {
				t.Fatalf("interval violated between %s and %s", lastDates[id], d)
			}
		}
		lastDates[id] = d
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyCourseCannotReportSuccessfulGeneration(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	date := "2099-02-03"
	request := httptest.NewRequest(http.MethodPost, "/v1/schedules/generate", bytes.NewReader([]byte(`{"date":"`+date+`"}`)))
	response := httptest.NewRecorder()
	api.generate(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty schedule must not report success: %d %s", response.Code, response.Body.String())
	}
	var reports int
	if err := db.QueryRow(context.Background(), "SELECT COUNT(*) FROM schedule_reports WHERE schedule_date=$1", date).Scan(&reports); err != nil || reports != 0 {
		t.Fatalf("empty course must not persist a schedule report: reports=%d err=%v", reports, err)
	}
}

func TestOptimizersPersistCalendarCards(t *testing.T) {
	mode := os.Getenv("CLINIC_COURSE_OPTIMIZER")
	if mode != "cp-sat" && mode != "fast" {
		t.Skip("only applicable when running an experimental optimizer")
	}
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
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
	date := "2099-01-05"
	procedures := []procedure{{ID: "test-treatment", Name: "Терапия", DurationMinutes: 30, MinIntervalDays: 1, DefaultWeeklyCount: 3}}
	if response := call(http.MethodPut, "/v1/procedures", procedures, api.saveProcedures); response.Code != http.StatusOK {
		t.Fatalf("could not create procedure: %d %s", response.Code, response.Body.String())
	}
	plan := dayPlan{Date: date, Plans: []patientPlan{{PatientNumber: 1, Severity: "medium", StayDays: 7, ProcedureIDs: []string{"test-treatment"}, ProcedureCounts: map[string]int{"test-treatment": 3}}}}
	if response := call(http.MethodPut, "/v1/day-plans", plan, api.saveDayPlan); response.Code != http.StatusOK {
		t.Fatalf("could not save plan: %d %s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPost, "/v1/schedules/generate", scheduleRequest{Date: date}, api.generate); response.Code != http.StatusOK {
		t.Fatalf("%s generation did not persist: %d %s", mode, response.Code, response.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/appointments?from=2099-01-05&to=2099-01-12", nil)
	response := httptest.NewRecorder()
	api.listAppointments(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("calendar failed to load appointments: %d %s", response.Code, response.Body.String())
	}
	var appointments []appointment
	if err := json.Unmarshal(response.Body.Bytes(), &appointments); err != nil {
		t.Fatal(err)
	}
	if len(appointments) != 3 {
		t.Fatalf("calendar should show three generated cards, got %d", len(appointments))
	}
}

func TestFastOptimizerPersistsThreeThousandAppointments(t *testing.T) {
	if os.Getenv("CLINIC_COURSE_OPTIMIZER") != "fast" {
		t.Skip("only applicable to the fast local Go optimizer")
	}
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	call := func(method, path string, payload any, handle func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		handle(recorder, httptest.NewRequest(method, path, bytes.NewReader(body)))
		return recorder
	}
	date := "2099-01-05"
	procedures := make([]procedure, 0, 5)
	for i := 0; i < 5; i++ {
		procedures = append(procedures, procedure{ID: fmt.Sprintf("test-%d", i), Name: fmt.Sprintf("Therapy %d", i),
			DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1, DefaultWeeklyCount: 7})
	}
	if response := call(http.MethodPut, "/v1/procedures", procedures, api.saveProcedures); response.Code != http.StatusOK {
		t.Fatalf("procedure setup: %d %s", response.Code, response.Body.String())
	}
	plan := dayPlan{Date: date, Plans: []patientPlan{}}
	for patient := 1; patient <= 100; patient++ {
		p := patientPlan{PatientNumber: patient, Severity: "medium", StayDays: 14, ProcedureIDs: []string{}, ProcedureCounts: map[string]int{}}
		for _, proc := range procedures {
			p.ProcedureIDs = append(p.ProcedureIDs, proc.ID)
			p.ProcedureCounts[proc.ID] = 6
		}
		plan.Plans = append(plan.Plans, p)
	}
	if response := call(http.MethodPut, "/v1/day-plans", plan, api.saveDayPlan); response.Code != http.StatusOK {
		t.Fatalf("plan setup: %d %s", response.Code, response.Body.String())
	}
	begin := time.Now()
	if response := call(http.MethodPost, "/v1/schedules/generate", scheduleRequest{Date: date}, api.generate); response.Code != http.StatusOK {
		t.Fatalf("generate: %d %s", response.Code, response.Body.String())
	}
	t.Logf("3000 sessions full API+PostgreSQL generation: %v", time.Since(begin))
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM appointments WHERE source_plan_date=$1", date).Scan(&count); err != nil || count != 3000 {
		t.Fatalf("expected 3000 persisted appointments; got %d: %v", count, err)
	}
}

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatal(err)
	}
	return date
}
