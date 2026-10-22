package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func clearSchedulesRequest(api *server, contentType, confirmation string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/v1/schedules", bytes.NewBufferString(`{"confirmation":"`+confirmation+`"}`))
	req.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	api.clearSchedules(response, req)
	return response
}

func TestClearSchedulesRequiresExplicitConfirmation(t *testing.T) {
	api := &server{}
	for _, test := range []struct {
		name, contentType, confirmation string
		status                          int
	}{
		{"reject plain text", "text/plain", "CLEAR_ALL_SCHEDULES", http.StatusUnsupportedMediaType},
		{"reject missing confirmation", "application/json", "", http.StatusBadRequest},
		{"reject mismatched confirmation", "application/json", "CLEAR_TODAY", http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := clearSchedulesRequest(api, test.contentType, test.confirmation)
			if response.Code != test.status {
				t.Fatalf("unexpected status: %d, body: %s", response.Code, response.Body.String())
			}
		})
	}
}

// Test only touches an isolated, disposable schema, never the application's tables.
func TestClearSchedulesIntegration(t *testing.T) {
	dsn := os.Getenv("CLINIC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CLINIC_TEST_DATABASE_URL to run the isolated PostgreSQL integration test")
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
	schema := "clear_schedules_test_" + strings.ReplaceAll(id, "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("could not remove isolated test schema: %v", err)
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
	defer db.Close()
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("migrations not available: %v", err)
	}
	for _, path := range files {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("migration %s: %v", path, err)
		}
	}
	for _, statement := range []string{
		"INSERT INTO procedures(id,name,duration_minutes) VALUES('fixture','Fixture',50)",
		"UPDATE calendar_settings SET time_step_minutes=15 WHERE id=1",
		"INSERT INTO appointments(id,title,start_at,end_at,color,status) VALUES('scheduled','Session','2099-01-01 09:00','2099-01-01 09:50','blue','scheduled'),('cancelled','Cancelled','2099-01-02 09:00','2099-01-02 09:50','blue','cancelled')",
		"INSERT INTO day_plans(schedule_date) VALUES('2099-01-01')",
		"INSERT INTO day_plan_patients(schedule_date,patient_number,severity) VALUES('2099-01-01',1,'medium')",
		"INSERT INTO day_plan_items(schedule_date,patient_number,procedure_id,position) VALUES('2099-01-01',1,'fixture',0)",
		"INSERT INTO schedule_reports(schedule_date) VALUES('2099-01-01')",
	} {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatalf("could not prepare isolated fixture: %v", err)
		}
	}
	api := &server{db: db}
	for attempt := 0; attempt < 2; attempt++ {
		response := clearSchedulesRequest(api, "application/json", "CLEAR_ALL_SCHEDULES")
		if response.Code != http.StatusNoContent {
			t.Fatalf("clear attempt %d: HTTP %d %s", attempt, response.Code, response.Body.String())
		}
		for _, table := range []string{"appointments", "schedule_reports", "day_plans", "day_plan_patients", "day_plan_items"} {
			var count int
			if err := db.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
				t.Fatalf("%s not empty after clear: count=%d error=%v", table, count, err)
			}
		}
		var procedureCount, timeStep int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM procedures").Scan(&procedureCount); err != nil || procedureCount != 1 {
			t.Fatalf("procedure catalog changed: count=%d error=%v", procedureCount, err)
		}
		if err := db.QueryRow(ctx, "SELECT time_step_minutes FROM calendar_settings WHERE id=1").Scan(&timeStep); err != nil || timeStep != 15 {
			t.Fatalf("calendar preferences changed: step=%d error=%v", timeStep, err)
		}
	}
}
