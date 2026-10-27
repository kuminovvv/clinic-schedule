package main

import (
	"testing"
	"time"
)

func testCoursePlan(stay, count int, severity string, ids ...string) patientPlan {
	counts := map[string]int{}
	for _, id := range ids {
		counts[id] = count
	}
	return patientPlan{PatientNumber: 1, Severity: severity, StayDays: stay, ProcedureIDs: ids, ProcedureCounts: counts}
}

func TestCourseScheduleSpreadsRepeatsAndAvoidsSameDay(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{"p": {ID: "p", Name: "Терапия", DurationMinutes: 50, MinIntervalDays: 2, DefaultWeeklyCount: 3}}
	result, err := createCourseSchedule(day, []patientPlan{testCoursePlan(7, 3, "medium", "p")}, procedures, settings{TimeStepMinutes: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 3 || result.UnscheduledCount != 0 {
		t.Fatalf("incorrect counts: %+v", result)
	}
	want := []string{"2026-10-21", "2026-10-24", "2026-10-27"}
	for i, item := range result.Appointments {
		if item.StartAt[:10] != want[i] || item.StartAt[11:16] != "09:00" {
			t.Fatalf("session %d expected %s at 09:00, got %s", i, want[i], item.StartAt)
		}
	}
}

func TestCourseScheduleRejectsImpossibleCountAndSingleDayRepeat(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{"p": {ID: "p", Name: "Терапия", DurationMinutes: 50, MinIntervalDays: 3, DefaultWeeklyCount: 2}}
	result, err := createCourseSchedule(day, []patientPlan{testCoursePlan(4, 3, "medium", "p")}, procedures, settings{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.UnscheduledCount != 1 || result.ScheduledCount != 2 {
		t.Fatalf("too many sessions should be reported: %+v", result)
	}
	dayPlan := testCoursePlan(1, 2, "medium", "p")
	result, err = createCourseSchedule(day, []patientPlan{dayPlan}, procedures, settings{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 1 || result.UnscheduledCount != 1 {
		t.Fatalf("same procedure twice in one day must not be scheduled: %+v", result)
	}
}

func TestHighSeverityStartsWithRelaxingProcedure(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{
		"normal": {ID: "normal", Name: "Обычная", DurationMinutes: 50, MinIntervalDays: 1},
		"relax":  {ID: "relax", Name: "Расслабляющая", DurationMinutes: 50, MinIntervalDays: 1, IsRelaxing: true},
	}
	for _, tc := range []struct{ severity, first string }{{"high", "relax"}, {"medium", "normal"}, {"low", "normal"}} {
		plan := testCoursePlan(1, 1, tc.severity, "normal", "relax")
		result, err := createCourseSchedule(day, []patientPlan{plan}, procedures, settings{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Appointments) != 2 || result.Appointments[0].ProcedureID != tc.first {
			t.Fatalf("severity %s should start with %s: %+v", tc.severity, tc.first, result.Appointments)
		}
	}
}

func TestCourseWarnsIfIntervalExceedsStay(t *testing.T) {
	procedures := map[string]procedure{"p": {ID: "p", Name: "Терапия", MinIntervalDays: 14}}
	warnings := courseWarnings([]patientPlan{testCoursePlan(7, 1, "high", "p")}, procedures)
	if len(warnings) != 0 {
		t.Fatalf("a single session has no repetition interval constraint: %+v", warnings)
	}
	warnings = courseWarnings([]patientPlan{testCoursePlan(7, 2, "high", "p")}, procedures)
	if len(warnings) != 2 {
		t.Fatalf("expected both interval and impossible count warnings: %+v", warnings)
	}
}

func TestCourseDoesNotConfuseSamePatientNumbersAcrossAdmissions(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	patient := 1
	therapy := procedure{ID: "p", Name: "Терапия", DurationMinutes: 50, MinIntervalDays: 1, DefaultWeeklyCount: 1}
	slots := []timeSlot{{
		start:   time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local),
		end:     time.Date(2026, 10, 21, 9, 50, 0, 0, time.Local),
		patient: &patient, procedureID: "p", sourcePlanDate: "2026-10-20",
	}}
	result, err := createCourseSchedule(day, []patientPlan{testCoursePlan(1, 1, "medium", "p")}, map[string]procedure{"p": therapy}, settings{}, slots)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 1 || result.Appointments[0].StartAt != "2026-10-21T09:00" {
		t.Fatalf("different admission with same temporary number must not block this patient: %+v", result)
	}
	slots[0].sourcePlanDate = "2026-10-21"
	result, err = createCourseSchedule(day, []patientPlan{testCoursePlan(1, 1, "medium", "p")}, map[string]procedure{"p": therapy}, settings{}, slots)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 0 || result.UnscheduledCount != 1 {
		t.Fatalf("same admission should not repeat this procedure on one date: %+v", result)
	}
}
