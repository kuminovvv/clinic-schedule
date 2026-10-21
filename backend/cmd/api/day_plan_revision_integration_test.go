package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDayPlanDuplicateSavesDoNotConflictIntegration(t *testing.T) {
	dsn := os.Getenv("CLINIC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CLINIC_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	number, err := strconv.ParseUint(id[:8], 16, 32)
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(number%36000)).Format("2006-01-02")
	var exists bool
	if err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM day_plans WHERE schedule_date=$1)", date).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Skip("reserved test date already contains a plan")
	}
	t.Cleanup(func() {
		defer db.Close()
		if _, err := db.Exec(ctx, "DELETE FROM day_plans WHERE schedule_date=$1", date); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	})
	api := &server{db: db}
	selected := 1
	plan := dayPlan{
		Date: date, Revision: 0, SelectedPatientNumber: &selected,
		Plans: []patientPlan{{PatientNumber: 1, Severity: "medium", StayDays: 7, ProcedureIDs: []string{}, ProcedureCounts: map[string]int{}}},
	}
	call := func(input dayPlan) (int, dayPlan) {
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPut, "/v1/day-plans", bytes.NewReader(body))
		response := httptest.NewRecorder()
		api.saveDayPlan(response, req)
		var saved dayPlan
		if response.Code == http.StatusOK {
			if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
				t.Error(err)
			}
		}
		return response.Code, saved
	}
	const parallelSaves = 5
	var group sync.WaitGroup
	statuses := make([]int, parallelSaves)
	values := make([]dayPlan, parallelSaves)
	for i := 0; i < parallelSaves; i++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			statuses[index], values[index] = call(plan)
		}(i)
	}
	group.Wait()
	for index, status := range statuses {
		if status != http.StatusOK || values[index].Revision != 1 {
			t.Fatalf("identical concurrent save %d: status=%d revision=%d", index, status, values[index].Revision)
		}
	}
	code, unchanged := call(plan)
	if code != http.StatusOK || unchanged.Revision != 1 {
		t.Fatalf("unchanged stale request should be idempotent: %d %+v", code, unchanged)
	}
	modified := plan
	modified.Plans = []patientPlan{{PatientNumber: 1, Severity: "high", StayDays: 7, ProcedureIDs: []string{}, ProcedureCounts: map[string]int{}}}
	if code, _ := call(modified); code != http.StatusConflict {
		t.Fatalf("a real stale edit must remain a conflict: %d", code)
	}
	modified.Revision = 1
	if code, changed := call(modified); code != http.StatusOK || changed.Revision != 2 {
		t.Fatalf("a current edit must save: %d %+v", code, changed)
	}
}
