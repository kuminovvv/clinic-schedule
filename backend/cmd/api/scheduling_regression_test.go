package main

import (
	"testing"
	"time"
)

func testShortMonday(t *testing.T, end string) (time.Time, settings) {
	t.Helper()
	day := time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local)
	week := defaultWorkWeek()
	week[0].End = end
	week[0].Breaks = []weeklyBreak{}
	if !validWorkWeek(week) {
		t.Fatal("invalid test work week")
	}
	return day, settings{TimeStepMinutes: 10, WorkWeek: week}
}

func TestScheduleRetainsEarlySessionBeforeUnavoidableConflict(t *testing.T) {
	day, preferences := testShortMonday(t, "10:45")
	procedures := map[string]procedure{
		"p": {ID: "p", Name: "Процедура", DurationMinutes: 30},
	}
	patient := 7
	occupied := []timeSlot{
		{start: day.Add(30 * time.Minute).Add(9 * time.Hour), end: day.Add(10 * time.Hour), patient: &patient},
	}
	result, err := createSchedule(day, []scheduleTask{
		{PatientNumber: patient, Severity: "medium", ProcedureID: "p"},
		{PatientNumber: patient, Severity: "medium", ProcedureID: "p"},
	}, procedures, preferences, occupied)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 2 || result.UnscheduledCount != 0 ||
		result.Appointments[0].StartAt != "2026-10-19T09:00" || result.Appointments[1].StartAt != "2026-10-19T10:00" {
		t.Fatalf("both sessions should fit around an occupied gap: %+v", result)
	}
}

func TestScheduleRechecksClosuresAfterOccupiedSlot(t *testing.T) {
	day, preferences := testShortMonday(t, "12:00")
	preferences.WorkWeek[0].Breaks = []weeklyBreak{
		{ID: "one", Name: "Дезинфекция", Start: "09:30", End: "10:00"},
		{ID: "two", Name: "Кварцевание", Start: "10:00", End: "10:30"},
	}
	procedures := map[string]procedure{"p": {ID: "p", Name: "Процедура", DurationMinutes: 30}}
	patient := 1
	occupied := []timeSlot{{start: day.Add(9*time.Hour + 20*time.Minute), end: day.Add(10*time.Hour + 15*time.Minute), patient: &patient}}
	result, err := createSchedule(day, []scheduleTask{{PatientNumber: patient, Severity: "medium", ProcedureID: "p"}}, procedures, preferences, occupied)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 1 || result.Appointments[0].StartAt != "2026-10-19T10:30" {
		t.Fatalf("occupied time and both breaks must be respected: %+v", result)
	}
}

func TestClosedDayReturnsUnscheduledPatientNumbers(t *testing.T) {
	monday, preferences := testShortMonday(t, "12:00")
	sunday := monday.AddDate(0, 0, 6)
	procedures := map[string]procedure{"p": {ID: "p", Name: "Процедура", DurationMinutes: 30}}
	result, err := createSchedule(sunday, []scheduleTask{
		{PatientNumber: 3, Severity: "medium", ProcedureID: "p"},
		{PatientNumber: 1, Severity: "medium", ProcedureID: "p"},
		{PatientNumber: 3, Severity: "medium", ProcedureID: "p"},
	}, procedures, preferences, nil)
	if err != nil || result.ScheduledCount != 0 || result.UnscheduledCount != 3 ||
		len(result.UnscheduledPatients) != 2 || result.UnscheduledPatients[0] != 1 || result.UnscheduledPatients[1] != 3 {
		t.Fatalf("closed day must report the patients affected: %+v %v", result, err)
	}
}

func TestMidnightExclusiveWorkingHoursBoundary(t *testing.T) {
	monday, preferences := testShortMonday(t, "24:00")
	preferences.WorkWeek[0].Start = "22:00"
	procedures := map[string]procedure{"p": {ID: "p", Name: "Поздняя", DurationMinutes: 30}}
	tasks := []scheduleTask{{PatientNumber: 1, Severity: "medium", ProcedureID: "p"}}
	result, err := createSchedule(monday, tasks, procedures, preferences, []timeSlot{{start: monday.Add(22 * time.Hour), end: monday.Add(23*time.Hour + 30*time.Minute)}})
	if err != nil || result.ScheduledCount != 1 || result.Appointments[0].StartAt != "2026-10-19T23:30" || result.Appointments[0].EndAt != "2026-10-20T00:00" {
		t.Fatalf("24:00 should be represented as the next day's midnight: %+v %v", result, err)
	}
	item := result.Appointments[0]
	if !validAppointment(item) {
		t.Fatalf("generated appointment ending at 24:00 must pass appointment validation: %+v", item)
	}
	start, _ := time.ParseInLocation("2006-01-02T15:04", item.StartAt, time.Local)
	end, _ := time.ParseInLocation("2006-01-02T15:04", item.EndAt, time.Local)
	if !preferences.allowsAppointment(start, end) {
		t.Fatal("next-day midnight must be permitted at 24:00")
	}
	invalidEnd := end.Add(time.Minute)
	if preferences.allowsAppointment(start, invalidEnd) {
		t.Fatal("appointments may not extend past midnight")
	}
	item.EndAt = invalidEnd.Format("2006-01-02T15:04")
	if validAppointment(item) {
		t.Fatal("appointment may not cross into the next working day")
	}
}
