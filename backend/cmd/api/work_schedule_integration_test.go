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

// All writes are restricted to a separate temporary schema in the test database.
func TestWeeklyWorkSchedulePersistenceIntegration(t *testing.T) {
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
	schema := "work_schedule_test_" + strings.ReplaceAll(id, "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("test schema cleanup: %v", err)
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
		statement, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(statement)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	api := &server{db: db}
	get := func() settings {
		recorder := httptest.NewRecorder()
		api.getSettings(recorder, httptest.NewRequest(http.MethodGet, "/v1/calendar-settings", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET returned %d: %s", recorder.Code, recorder.Body.String())
		}
		var value settings
		if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	value := get()
	if !validWorkWeek(value.WorkWeek) || value.WorkWeek[5].Open || value.WorkWeek[6].Open {
		t.Fatalf("unexpected defaults: %+v", value.WorkWeek)
	}
	value.WorkWeek[0].End = "20:00"
	value.WorkWeek[0].Breaks = append(value.WorkWeek[0].Breaks, weeklyBreak{ID: "quartz", Name: "Кварцевание", Start: "15:00", End: "19:00"})
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	api.saveSettings(recorder, httptest.NewRequest(http.MethodPut, "/v1/calendar-settings", bytes.NewReader(encoded)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT returned %d: %s", recorder.Code, recorder.Body.String())
	}
	stored := get()
	if stored.WorkWeek[0].End != "20:00" || len(stored.WorkWeek[0].Breaks) != 2 || stored.WorkWeek[0].Breaks[1].Name != "Кварцевание" {
		t.Fatalf("weekly settings not persisted: %+v", stored.WorkWeek[0])
	}
	// Check that the real HTTP write handler applies persisted closures.
	check := func(id, start, end string, expected int) {
		body, err := json.Marshal(appointment{ID: id, Title: "Test appointment", StartAt: start, EndAt: end, Color: "blue", Status: "scheduled"})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPut, "/v1/appointments/"+id, bytes.NewReader(body))
		req.SetPathValue("id", id)
		result := httptest.NewRecorder()
		api.saveAppointment(result, req)
		if result.Code != expected {
			t.Fatalf("appointment %s: expected %d, got %d: %s", id, expected, result.Code, result.Body.String())
		}
	}
	check("sunday", "2099-01-04T09:00", "2099-01-04T09:50", http.StatusUnprocessableEntity) // Sunday.
	check("quartz", "2099-01-05T15:30", "2099-01-05T16:20", http.StatusUnprocessableEntity) // Monday.
	check("open", "2099-01-05T19:00", "2099-01-05T19:50", http.StatusOK)
}
