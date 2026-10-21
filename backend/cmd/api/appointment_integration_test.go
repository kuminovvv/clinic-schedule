package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run with CLINIC_TEST_DATABASE_URL against a disposable/local clinic database.
func TestSaveAppointmentAllowsConcurrentDifferentPatientsIntegration(t *testing.T) {
	dsn := os.Getenv("CLINIC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CLINIC_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	api := &server{db: db}
	id, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer db.Close()
		if _, err := db.Exec(ctx, "DELETE FROM appointments WHERE title='Integration test appointment' AND patient_number=999 AND start_at >= '2026-10-19' AND start_at < '2026-10-21'"); err != nil {
			t.Errorf("could not remove temporary appointment: %v", err)
		}
	})
	patient := 999
	item := appointment{ID: id, Title: "Integration test appointment", StartAt: "2026-10-19T09:00", EndAt: "2026-10-19T09:50", Color: "blue", Status: "scheduled", PatientNumber: &patient}
	put := func() *httptest.ResponseRecorder {
		body, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPut, "/v1/appointments/"+id, bytes.NewReader(body))
		req.SetPathValue("id", id)
		r := httptest.NewRecorder()
		api.saveAppointment(r, req)
		return r
	}
	if response := put(); response.Code != http.StatusOK {
		t.Fatalf("different patient's 09:00 appointment should be allowed: %d %s", response.Code, response.Body.String())
	}
	item.StartAt = "2026-10-20T09:00"
	item.EndAt = "2026-10-20T09:50"
	if response := put(); response.Code != http.StatusOK {
		t.Fatalf("moving to another day should be allowed: %d %s", response.Code, response.Body.String())
	}
	var start string
	if err := db.QueryRow(ctx, "SELECT to_char(start_at, 'YYYY-MM-DD\"T\"HH24:MI') FROM appointments WHERE id=$1", id).Scan(&start); err != nil {
		t.Fatal(err)
	}
	if start != "2026-10-20T09:00" {
		t.Fatalf("move was not saved: %s", start)
	}
	otherID, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	item.ID = otherID
	body, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/v1/appointments/"+otherID, bytes.NewReader(body))
	req.SetPathValue("id", otherID)
	response := httptest.NewRecorder()
	api.saveAppointment(response, req)
	if response.Code != http.StatusConflict {
		t.Fatalf("same patient's overlapping appointment must be rejected: %d %s", response.Code, response.Body.String())
	}
}
