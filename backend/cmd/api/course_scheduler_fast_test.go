package main

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

func makeFastPlans(patients, sessions, stay int) ([]patientPlan, map[string]procedure) {
	procedures := map[string]procedure{}
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("p%d", i)
		procedures[id] = procedure{ID: id, Name: id, DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1, DefaultWeeklyCount: 7}
	}
	plans := make([]patientPlan, 0, patients)
	for number := 1; number <= patients; number++ {
		plans = append(plans, patientPlan{PatientNumber: number, Severity: "medium", StayDays: stay,
			ProcedureIDs:    []string{"p0", "p1", "p2", "p3"},
			ProcedureCounts: map[string]int{"p0": sessions, "p1": sessions, "p2": sessions, "p3": sessions}})
	}
	return plans, procedures
}

func TestFastCourseSpreadsSixteenSessions(t *testing.T) {
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	plans, procedures := makeFastPlans(1, 3, 14)
	for i, count := range []int{3, 3, 2, 3, 2, 3} {
		id := fmt.Sprintf("p%d", i)
		if _, exists := procedures[id]; !exists {
			procedures[id] = procedure{ID: id, Name: id, DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1, DefaultWeeklyCount: 7}
			plans[0].ProcedureIDs = append(plans[0].ProcedureIDs, id)
		}
		plans[0].ProcedureCounts[id] = count
	}
	output, err := createCourseScheduleFast(start, plans, procedures, settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if output.ScheduledCount != 16 || output.UnscheduledCount != 0 {
		t.Fatalf("expected 16 sessions: %+v", output)
	}
	assertFastScheduleValid(t, output, start, plans, procedures)
	byDay := map[string]int{}
	for _, appt := range output.Appointments {
		byDay[appt.StartAt[:10]]++
	}
	if len(byDay) < 7 {
		t.Fatalf("sessions are excessively clustered: %v", byDay)
	}
	for _, count := range byDay {
		if count > 3 {
			t.Fatalf("too many sessions per day: %v", byDay)
		}
	}
	t.Logf("16 sessions across %d days: %v", len(byDay), byDay)
}

func TestFastCourseScalesBeyondCP80Limit(t *testing.T) {
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	for _, patients := range []int{4, 12, 100} {
		t.Run(fmt.Sprintf("patients_%d", patients), func(t *testing.T) {
			plans, procedures := makeFastPlans(patients, 7, 14)
			began := time.Now()
			output, err := createCourseScheduleFast(start, plans, procedures, settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if output.ScheduledCount != 28*patients || output.UnscheduledCount != 0 {
				t.Fatalf("failed to schedule %d sessions: scheduled=%d unscheduled=%d", 28*patients, output.ScheduledCount, output.UnscheduledCount)
			}
			assertFastScheduleValid(t, output, start, plans, procedures)
			t.Logf("%d sessions: %v", output.ScheduledCount, time.Since(began))
		})
	}
}

func TestFastCourseAtAPI3000SessionLimit(t *testing.T) {
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	plans, procedures := makeFastPlans(100, 6, 14)
	procedures["p4"] = procedure{ID: "p4", Name: "p4", DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1, DefaultWeeklyCount: 7}
	for i := range plans {
		plans[i].ProcedureIDs = append(plans[i].ProcedureIDs, "p4")
		plans[i].ProcedureCounts["p4"] = 6
	}
	begin := time.Now()
	output, err := createCourseScheduleFast(start, plans, procedures, settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if output.ScheduledCount != 3000 || output.UnscheduledCount != 0 {
		t.Fatalf("expected exactly 3000 appointments, got scheduled=%d missing=%d", output.ScheduledCount, output.UnscheduledCount)
	}
	assertFastScheduleValid(t, output, start, plans, procedures)
	t.Logf("3000 sessions: %v", time.Since(begin))
}

func TestFastCourseAnnualHorizon(t *testing.T) {
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	plans, procedures := makeFastPlans(100, 6, 365)
	procedures["p4"] = procedure{ID: "p4", Name: "p4", DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1, DefaultWeeklyCount: 7}
	for i := range plans {
		plans[i].ProcedureIDs = append(plans[i].ProcedureIDs, "p4")
		plans[i].ProcedureCounts["p4"] = 6
	}
	begin := time.Now()
	result, err := createCourseScheduleFast(start, plans, procedures, settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}, nil, nil)
	if err != nil || result.ScheduledCount != 3000 || result.UnscheduledCount != 0 {
		t.Fatalf("annual course: count %d, missing %d, err %v", result.ScheduledCount, result.UnscheduledCount, err)
	}
	t.Logf("3000 sessions over 365 days: %v", time.Since(begin))
}

func TestFastCourseReportsImpossibleWorkload(t *testing.T) {
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	plans, procedures := makeFastPlans(1, 7, 7)
	for id, p := range procedures {
		p.DefaultWeeklyCount = 1
		procedures[id] = p
	}
	output, err := createCourseScheduleFast(start, plans, procedures, settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if output.ScheduledCount != 4 || output.UnscheduledCount != 24 {
		t.Fatalf("weekly limit must hold: scheduled=%d missing=%d", output.ScheduledCount, output.UnscheduledCount)
	}
}

func TestFastCourseRespectsUnavailableDaysAndFixedSessions(t *testing.T) {
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	plans, procedures := makeFastPlans(1, 2, 14)
	plans[0].ProcedureIDs = []string{"p0"}
	plans[0].ProcedureCounts = map[string]int{"p0": 2}
	procedures["p0"] = procedure{ID: "p0", Name: "p0", DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 4, DefaultWeeklyCount: 2}
	patient := 1
	fixed := []timeSlot{{start: time.Date(2026, 10, 22, 9, 0, 0, 0, time.Local),
		end: time.Date(2026, 10, 22, 9, 50, 0, 0, time.Local), patient: &patient, procedureID: "p0", sourcePlanDate: "2026-10-22"}}
	absences := map[int]map[string]bool{patient: {"2026-10-23": true, "2026-10-27": true}}
	output, err := createCourseScheduleFast(start, plans, procedures, settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}, fixed, absences)
	if err != nil {
		t.Fatal(err)
	}
	if output.UnscheduledCount != 0 {
		t.Fatalf("expected both sessions to fit with the weekly constraint: %+v", output)
	}
	for _, a := range output.Appointments {
		if a.StartAt[:10] == "2026-10-22" || a.StartAt[:10] == "2026-10-23" || a.StartAt[:10] == "2026-10-27" {
			t.Fatalf("absent or too-close date chosen: %+v", a)
		}
	}
	assertFastScheduleValid(t, output, start, plans, procedures)
}

func TestFastCourseHighSeverityRelaxingFirst(t *testing.T) {
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	plans, procs := makeFastPlans(1, 1, 1)
	plans[0].Severity = "high"
	plans[0].ProcedureIDs = []string{"p0", "p1"}
	plans[0].ProcedureCounts = map[string]int{"p0": 1, "p1": 1}
	procs["p1"] = procedure{ID: "p1", Name: "Relax", DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1, DefaultWeeklyCount: 7, IsRelaxing: true}
	output, err := createCourseScheduleFast(start, plans, procs, settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if output.ScheduledCount != 2 || output.Appointments[0].ProcedureID != "p1" {
		t.Fatalf("relaxing procedure must start first: %+v", output)
	}
}

func TestFastCourseOptionalRoomsAreSharedResources(t *testing.T) {
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		name     string
		rooms    [2]string
		sameTime bool
	}{
		{"both organization wide", [2]string{"", ""}, true},
		{"different rooms", [2]string{"101", "102"}, true},
		{"same room", [2]string{"101", " 101 "}, false},
		{"one room one organization wide", [2]string{"101", ""}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plans, procedures := makeFastPlans(2, 1, 1)
			plans[0].ProcedureIDs, plans[1].ProcedureIDs = []string{"p0"}, []string{"p1"}
			plans[0].ProcedureCounts, plans[1].ProcedureCounts = map[string]int{"p0": 1}, map[string]int{"p1": 1}
			for i, id := range []string{"p0", "p1"} {
				p := procedures[id]
				p.Room = tc.rooms[i]
				procedures[id] = p
			}
			result, err := createCourseScheduleFast(start, plans, procedures, settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}, nil, nil)
			if err != nil || result.ScheduledCount != 2 {
				t.Fatalf("failed to schedule rooms: %+v err=%v", result, err)
			}
			if same := result.Appointments[0].StartAt == result.Appointments[1].StartAt; same != tc.sameTime {
				t.Fatalf("unexpected room overlap=%t expected %t: %+v", same, tc.sameTime, result.Appointments)
			}
		})
	}
}

func TestFastCourseExistingRoomReservationBlocksAnotherPatient(t *testing.T) {
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	plans, procedures := makeFastPlans(1, 1, 1)
	plans[0].ProcedureIDs = []string{"p0"}
	plans[0].ProcedureCounts = map[string]int{"p0": 1}
	p := procedures["p0"]
	p.Room = "Кабинет 5"
	procedures["p0"] = p
	other := 99
	occupied := []timeSlot{{start: time.Date(2026, 10, 22, 9, 0, 0, 0, time.Local),
		end: time.Date(2026, 10, 22, 10, 0, 0, 0, time.Local), patient: &other, room: "кабинет 5"}}
	result, err := createCourseScheduleFast(start, plans, procedures, settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}, occupied, nil)
	if err != nil || result.ScheduledCount != 1 || result.Appointments[0].StartAt < "2026-10-22T10:00" {
		t.Fatalf("existing patient in same room not respected: %+v err=%v", result, err)
	}
}

func assertFastScheduleValid(t *testing.T, output scheduleResponse, start time.Time, plans []patientPlan, procedures map[string]procedure) {
	t.Helper()
	byPatient := map[int][]appointment{}
	counts := map[int]map[string]int{}
	for _, plan := range plans {
		counts[plan.PatientNumber] = map[string]int{}
	}
	for _, appt := range output.Appointments {
		if appt.PatientNumber == nil {
			t.Fatal("appointment without patient")
		}
		patient := *appt.PatientNumber
		counts[patient][appt.ProcedureID]++
		byPatient[patient] = append(byPatient[patient], appt)
		begin, err := time.ParseInLocation("2006-01-02T15:04", appt.StartAt, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		finish, err := time.ParseInLocation("2006-01-02T15:04", appt.EndAt, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		if !finish.After(begin) || finish.Sub(begin) != time.Duration(procedures[appt.ProcedureID].DurationMinutes+procedures[appt.ProcedureID].PreparationMinutes)*time.Minute {
			t.Fatalf("invalid appointment duration: %+v", appt)
		}
		if !settingsAllowsTest(begin, finish) {
			t.Fatalf("appointment outside workday or overlapping lunch: %+v", appt)
		}
	}
	for _, plan := range plans {
		for id, count := range plan.ProcedureCounts {
			if counts[plan.PatientNumber][id] > count {
				t.Fatalf("generated extra sessions for procedure %s", id)
			}
		}
	}
	for number, items := range byPatient {
		sort.Slice(items, func(i, j int) bool { return items[i].StartAt < items[j].StartAt })
		for i, current := range items {
			for _, previous := range items[:i] {
				if previous.EndAt > current.StartAt && previous.StartAt < current.EndAt {
					t.Fatalf("patient %d sessions overlap: %+v %+v", number, previous, current)
				}
				if previous.ProcedureID == current.ProcedureID {
					p := procedures[current.ProcedureID]
					prev := mustDate(t, previous.StartAt[:10])
					next := mustDate(t, current.StartAt[:10])
					if calendarDayNumber(next)-calendarDayNumber(prev) < int64(p.MinIntervalDays) {
						t.Fatalf("patient %d breaks repetition gap", number)
					}
				}
			}
		}
	}
	_ = start
}

func settingsAllowsTest(start, end time.Time) bool {
	return (settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}).allowsAppointment(start, end)
}
