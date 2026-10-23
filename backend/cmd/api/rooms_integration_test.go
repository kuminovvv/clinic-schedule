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

func TestOptionalRoomCatalogGenerationAndManualConflicts(t *testing.T) {
	db := isolatedScheduleDatabase(t)
	api := &server{db: db}
	request := func(method, path string, data any, handler func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		res := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if method == http.MethodPut && len(path) > len("/v1/appointments/") && path[:len("/v1/appointments/")] == "/v1/appointments/" {
			req.SetPathValue("id", path[len("/v1/appointments/"):])
		}
		handler(res, req)
		return res
	}
	procs := []procedure{
		{ID: "therapy-a", Name: "Therapy A", DurationMinutes: 30, MinIntervalDays: 1, DefaultWeeklyCount: 7, Room: "Каб. 101"},
		{ID: "therapy-b", Name: "Therapy B", DurationMinutes: 30, MinIntervalDays: 1, DefaultWeeklyCount: 7, Room: "каб. 101"},
		{ID: "therapy-c", Name: "Therapy C", DurationMinutes: 30, MinIntervalDays: 1, DefaultWeeklyCount: 7},
	}
	if res := request(http.MethodPut, "/v1/procedures", procs, api.saveProcedures); res.Code != http.StatusOK {
		t.Fatalf("procedure catalog: %d %s", res.Code, res.Body.String())
	}
	read := httptest.NewRecorder()
	api.listProcedures(read, httptest.NewRequest(http.MethodGet, "/v1/procedures", nil))
	var loaded []procedure
	if read.Code != http.StatusOK || json.Unmarshal(read.Body.Bytes(), &loaded) != nil || len(loaded) != len(procs) {
		t.Fatalf("procedure round trip: %d %s", read.Code, read.Body.String())
	}
	for _, proc := range loaded {
		if proc.ID == "therapy-a" && proc.Room != "Каб. 101" {
			t.Fatalf("room field was not persisted: %+v", proc)
		}
	}
	date := "2099-01-05"
	plan := dayPlan{Date: date, Plans: []patientPlan{
		{PatientNumber: 1, Severity: "medium", StayDays: 1, ProcedureIDs: []string{"therapy-a"}, ProcedureCounts: map[string]int{"therapy-a": 1}},
		{PatientNumber: 2, Severity: "medium", StayDays: 1, ProcedureIDs: []string{"therapy-b"}, ProcedureCounts: map[string]int{"therapy-b": 1}},
		{PatientNumber: 3, Severity: "medium", StayDays: 1, ProcedureIDs: []string{"therapy-c"}, ProcedureCounts: map[string]int{"therapy-c": 1}},
	}}
	if res := request(http.MethodPut, "/v1/day-plans", plan, api.saveDayPlan); res.Code != http.StatusOK {
		t.Fatalf("save plan: %d %s", res.Code, res.Body.String())
	}
	if res := request(http.MethodPost, "/v1/schedules/generate", scheduleRequest{Date: date}, api.generate); res.Code != http.StatusOK {
		t.Fatalf("generate with shared room: %d %s", res.Code, res.Body.String())
	}
	rows, err := db.Query(context.Background(), `SELECT patient_number,to_char(start_at,'HH24:MI'),to_char(end_at,'HH24:MI'),room
		FROM appointments WHERE source_plan_date=$1 ORDER BY patient_number`, date)
	if err != nil {
		t.Fatal(err)
	}
	type slot struct{ start, end, room string }
	slots := map[int]slot{}
	for rows.Next() {
		var patient int
		var item slot
		if err := rows.Scan(&patient, &item.start, &item.end, &item.room); err != nil {
			t.Fatal(err)
		}
		slots[patient] = item
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(slots) != 3 {
		t.Fatalf("expected three persisted appointments, got %v: %v", slots, err)
	}
	if slots[1].end > slots[2].start && slots[2].end > slots[1].start {
		t.Fatalf("same room double booked: %+v", slots)
	}
	if slots[3].start != slots[1].start {
		t.Fatalf("unassigned room should allow concurrent appointments: %+v", slots)
	}

	// Patient-specific manual edits must also enforce the room resource even
	// where distinct patient numbers would otherwise allow overlaps.
	day := time.Date(2099, 1, 6, 0, 0, 0, 0, time.Local)
	start := day.Format("2006-01-02") + "T09:00"
	end := day.Format("2006-01-02") + "T09:30"
	firstPatient, secondPatient := 4, 5
	newAppointment := func(id, room string, patient int) appointment {
		return appointment{ID: id, Title: id, StartAt: start, EndAt: end, Color: "blue", Status: "scheduled", PatientNumber: &patient, Room: room}
	}
	first := newAppointment("manual-room-a", "101", firstPatient)
	if res := request(http.MethodPut, "/v1/appointments/"+first.ID, first, api.saveAppointment); res.Code != http.StatusOK {
		t.Fatalf("manual room insert: %d %s", res.Code, res.Body.String())
	}
	conflicting := newAppointment("manual-room-b", "101", secondPatient)
	if res := request(http.MethodPut, "/v1/appointments/"+conflicting.ID, conflicting, api.saveAppointment); res.Code != http.StatusConflict {
		t.Fatalf("same room must conflict: %d %s", res.Code, res.Body.String())
	}
	conflicting.Room = "102"
	if res := request(http.MethodPut, "/v1/appointments/"+conflicting.ID, conflicting, api.saveAppointment); res.Code != http.StatusOK {
		t.Fatalf("different rooms should allow overlap: %d %s", res.Code, res.Body.String())
	}
	thirdPatient := 6
	conflicting = newAppointment("manual-room-unassigned", "", thirdPatient)
	if res := request(http.MethodPut, "/v1/appointments/"+conflicting.ID, conflicting, api.saveAppointment); res.Code != http.StatusOK {
		t.Fatalf("unassigned room should preserve previous organization-wide behavior: %d %s", res.Code, res.Body.String())
	}
}
