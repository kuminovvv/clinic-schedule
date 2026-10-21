package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCreateScheduleOverlapsPatientsAndIncludesAllPatients(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{
		"p1": {ID: "p1", Name: "Процедура", DurationMinutes: 50},
	}
	tasks := []scheduleTask{
		{PatientNumber: 1, Severity: "medium", ProcedureID: "p1"},
		{PatientNumber: 2, Severity: "medium", ProcedureID: "p1"},
		{PatientNumber: 3, Severity: "medium", ProcedureID: "p1"},
		{PatientNumber: 4, Severity: "medium", ProcedureID: "p1"},
		{PatientNumber: 5, Severity: "medium", ProcedureID: "p1"},
		{PatientNumber: 6, Severity: "medium", ProcedureID: "p1"},
		{PatientNumber: 7, Severity: "medium", ProcedureID: "p1"},
	}
	result, err := createSchedule(day, tasks, procedures, settings{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 7 || result.UnscheduledCount != 0 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	if len(result.Appointments) != 7 || len(result.UnscheduledPatients) != 0 {
		t.Fatalf("unexpected patient allocation: %+v", result)
	}
	for _, item := range result.Appointments {
		if item.StartAt[11:16] != "09:00" {
			t.Fatalf("expected patients to be scheduled concurrently, got %+v", result.Appointments)
		}
	}
	for _, item := range result.Appointments {
		if !item.GeneratedByPlanner || item.PatientNumber == nil {
			t.Fatalf("missing planner metadata: %+v", item)
		}
	}
}

func TestCreateScheduleKeepsEachPatientsProceduresTogether(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{
		"relax":   {ID: "relax", Name: "Расслабляющая", DurationMinutes: 30, IsRelaxing: true},
		"regular": {ID: "regular", Name: "Обычная", DurationMinutes: 30},
	}
	tasks := []scheduleTask{
		{PatientNumber: 1, Severity: "high", ProcedureID: "regular"},
		{PatientNumber: 1, Severity: "high", ProcedureID: "relax"},
		{PatientNumber: 1, Severity: "high", ProcedureID: "regular"},
		{PatientNumber: 2, Severity: "medium", ProcedureID: "regular"},
	}
	result, err := createSchedule(day, tasks, procedures, settings{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Appointments[0].Title, "Расслабляющая") {
		t.Fatalf("patient's relaxing procedure was not scheduled first: %+v", result.Appointments)
	}
	for index := range 3 {
		if result.Appointments[index].PatientNumber == nil || *result.Appointments[index].PatientNumber != 1 {
			t.Fatalf("patient 1 procedures were split by another patient: %+v", result.Appointments)
		}
		if index > 0 && result.Appointments[index-1].EndAt != result.Appointments[index].StartAt {
			t.Fatalf("patient 1 procedures are not back-to-back: %+v", result.Appointments)
		}
	}
	if result.Appointments[3].PatientNumber == nil || *result.Appointments[3].PatientNumber != 2 || result.Appointments[3].StartAt != result.Appointments[0].StartAt {
		t.Fatalf("patient 2 should overlap patient 1 in a separate lane: %+v", result.Appointments)
	}
}

func TestCreateScheduleMovesWholePatientBlockAroundOccupiedTimeAndLunch(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{
		"first":  {ID: "first", Name: "Первая", DurationMinutes: 60},
		"second": {ID: "second", Name: "Вторая", DurationMinutes: 60},
	}
	occupied := []timeSlot{
		{start: time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local), end: time.Date(2026, 10, 21, 10, 30, 0, 0, time.Local)},
		{start: time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local), end: time.Date(2026, 10, 21, 13, 0, 0, 0, time.Local)},
	}
	result, err := createSchedule(day, []scheduleTask{
		{PatientNumber: 1, Severity: "high", ProcedureID: "first"},
		{PatientNumber: 1, Severity: "high", ProcedureID: "second"},
	}, procedures, settings{}, occupied)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 2 || result.Appointments[0].StartAt[11:16] != "14:00" || result.Appointments[1].StartAt[11:16] != "15:00" {
		t.Fatalf("expected patient procedures to avoid occupied time and lunch, got %+v", result)
	}
}

func TestCreateScheduleKeepsNonStepDurationProceduresContiguous(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{
		"first":  {ID: "first", Name: "Первая", DurationMinutes: 55},
		"second": {ID: "second", Name: "Вторая", DurationMinutes: 30},
	}
	result, err := createSchedule(day, []scheduleTask{
		{PatientNumber: 1, Severity: "medium", ProcedureID: "first"},
		{PatientNumber: 1, Severity: "medium", ProcedureID: "second"},
	}, procedures, settings{TimeStepMinutes: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Appointments) != 2 || result.Appointments[0].StartAt[11:16] != "09:00" || result.Appointments[0].EndAt[11:16] != "09:55" || result.Appointments[1].StartAt != result.Appointments[0].EndAt {
		t.Fatalf("expected consecutive procedures from a ten-minute-aligned block start: %+v", result.Appointments)
	}
}

func TestCreateScheduleMovesPatientChainAroundOccupiedTime(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{
		"first":  {ID: "first", Name: "Первая", DurationMinutes: 120},
		"second": {ID: "second", Name: "Вторая", DurationMinutes: 120},
	}
	occupied := []timeSlot{{start: time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local), end: time.Date(2026, 10, 21, 10, 0, 0, 0, time.Local)}}
	result, err := createSchedule(day, []scheduleTask{
		{PatientNumber: 1, Severity: "high", ProcedureID: "first"},
		{PatientNumber: 1, Severity: "high", ProcedureID: "second"},
	}, procedures, settings{}, occupied)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 2 || result.UnscheduledCount != 0 || result.Appointments[0].StartAt[11:16] != "10:00" || result.Appointments[1].StartAt[11:16] != "14:00" {
		t.Fatalf("expected patient chain to continue after occupied time and lunch: %+v", result)
	}
}

func TestCreateScheduleDoesNotRunProcedureAcrossLunch(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{
		"first":  {ID: "first", Name: "Первая", DurationMinutes: 150},
		"second": {ID: "second", Name: "Вторая", DurationMinutes: 150},
	}
	result, err := createSchedule(day, []scheduleTask{
		{PatientNumber: 1, Severity: "high", ProcedureID: "first"},
		{PatientNumber: 1, Severity: "high", ProcedureID: "second"},
	}, procedures, settings{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 1 || result.UnscheduledCount != 1 || len(result.UnscheduledPatients) != 1 || result.UnscheduledPatients[0] != 1 {
		t.Fatalf("expected one procedure to fit before lunch and one to remain unscheduled: %+v", result)
	}
	if result.Appointments[0].StartAt[11:16] < "09:00" || result.Appointments[0].EndAt[11:16] > "13:00" {
		t.Fatalf("the placed procedure must not overlap lunch: %+v", result.Appointments)
	}
}

func TestCreateSchedulePartiallySchedulesOverloadedDay(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := make(map[string]procedure)
	tasks := make([]scheduleTask, 0, 24)
	for _, patient := range []struct{ number, count int }{{1, 12}, {2, 5}, {3, 7}} {
		for index := range patient.count {
			id := fmt.Sprintf("patient-%d-procedure-%d", patient.number, index)
			procedures[id] = procedure{ID: id, Name: id, DurationMinutes: 50}
			tasks = append(tasks, scheduleTask{PatientNumber: patient.number, Severity: "medium", ProcedureID: id})
		}
	}
	result, err := createSchedule(day, tasks, procedures, settings{TimeStepMinutes: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 17 || result.UnscheduledCount != 7 || !reflect.DeepEqual(result.UnscheduledPatients, []int{1, 3}) {
		t.Fatalf("expected 17 scheduled procedures and 7 unplaced: %+v", result)
	}
}

func TestCreateScheduleMovesPastOccupiedAppointmentsAndLunch(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{"p": {ID: "p", Name: "П", DurationMinutes: 60}}
	occupied := []timeSlot{{start: time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local), end: time.Date(2026, 10, 21, 10, 0, 0, 0, time.Local)}}
	result, err := createSchedule(day, []scheduleTask{{PatientNumber: 1, Severity: "low", ProcedureID: "p"}}, procedures, settings{}, occupied)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Appointments[0].StartAt[11:16]; got != "10:00" {
		t.Fatalf("expected appointment after occupied slot, got %s", got)
	}
	occupied = nil
	result, err = createSchedule(day, []scheduleTask{{PatientNumber: 1, Severity: "low", ProcedureID: "p"}, {PatientNumber: 2, Severity: "low", ProcedureID: "p"}, {PatientNumber: 3, Severity: "low", ProcedureID: "p"}, {PatientNumber: 4, Severity: "low", ProcedureID: "p"}, {PatientNumber: 5, Severity: "low", ProcedureID: "p"}}, procedures, settings{}, occupied)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Appointments[4].StartAt[11:16]; got != "09:00" {
		t.Fatalf("expected independent patients to overlap, got %s", got)
	}
}

func TestCreateScheduleBackfillsAvailableGapForShorterTask(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{
		"long":  {ID: "long", Name: "Длинная", DurationMinutes: 60},
		"short": {ID: "short", Name: "Короткая", DurationMinutes: 30},
	}
	occupied := []timeSlot{{
		start: time.Date(2026, 10, 21, 9, 30, 0, 0, time.Local),
		end:   time.Date(2026, 10, 21, 10, 30, 0, 0, time.Local),
	}}
	result, err := createSchedule(day, []scheduleTask{
		{PatientNumber: 1, Severity: "medium", ProcedureID: "long"},
		{PatientNumber: 2, Severity: "medium", ProcedureID: "short"},
	}, procedures, settings{}, occupied)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Appointments[0].StartAt[11:16]; got != "10:30" {
		t.Fatalf("expected long procedure after existing appointment, got %s", got)
	}
	if got := result.Appointments[1].StartAt[11:16]; got != "09:00" {
		t.Fatalf("expected short procedure to backfill the early gap, got %s", got)
	}
}

func TestCreateScheduleRoundsEachPatientStartToCalendarStep(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{"p": {ID: "p", Name: "П", DurationMinutes: 55}}
	result, err := createSchedule(day, []scheduleTask{
		{PatientNumber: 1, Severity: "medium", ProcedureID: "p"},
		{PatientNumber: 2, Severity: "medium", ProcedureID: "p"},
	}, procedures, settings{TimeStepMinutes: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Appointments[0].StartAt[11:16]; got != "09:00" || result.Appointments[1].StartAt[11:16] != "09:00" {
		t.Fatalf("expected overlapping patient appointments to start on the ten-minute step, got %+v", result.Appointments)
	}
}

func TestCreateScheduleSchedulesThreePatientsWithOverlappingChains(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{"p": {ID: "p", Name: "П", DurationMinutes: 50}}
	tasks := make([]scheduleTask, 0, 14)
	for patient, count := range map[int]int{1: 4, 2: 5, 3: 5} {
		for range count {
			tasks = append(tasks, scheduleTask{PatientNumber: patient, Severity: "medium", ProcedureID: "p"})
		}
	}
	result, err := createSchedule(day, tasks, procedures, settings{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 14 || result.UnscheduledCount != 0 {
		t.Fatalf("expected all 14 procedures to be scheduled: %+v", result)
	}
	byPatient := map[int][]appointment{}
	for _, item := range result.Appointments {
		byPatient[*item.PatientNumber] = append(byPatient[*item.PatientNumber], item)
	}
	for patient, count := range map[int]int{1: 4, 2: 5, 3: 5} {
		appointments := byPatient[patient]
		if len(appointments) != count {
			t.Fatalf("patient %d has %d procedures, want %d", patient, len(appointments), count)
		}
		for index := 1; index < len(appointments); index++ {
			previousEnd := appointments[index-1].EndAt[11:16]
			nextStart := appointments[index].StartAt[11:16]
			if previousEnd != nextStart && !(previousEnd <= "13:00" && nextStart == "14:00") {
				t.Fatalf("patient %d has an unexpected gap: %+v", patient, appointments)
			}
		}
	}
}

func TestCreateSchedulePrefersNineOClockOverAvoidingLunchIdleTime(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{"p": {ID: "p", Name: "П", DurationMinutes: 50}}
	tasks := make([]scheduleTask, 6)
	for index := range tasks {
		tasks[index] = scheduleTask{PatientNumber: 1, Severity: "medium", ProcedureID: "p"}
	}
	result, err := createSchedule(day, tasks, procedures, settings{TimeStepMinutes: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 6 || result.Appointments[0].StartAt[11:16] != "09:00" {
		t.Fatalf("schedule should begin at 09:00 whenever feasible: %+v", result)
	}
}

func TestCreateScheduleAllowsOverlappingDifferentPatients(t *testing.T) {
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	patientTwo := 2
	occupied := []timeSlot{{
		start:   time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local),
		end:     time.Date(2026, 10, 21, 10, 0, 0, 0, time.Local),
		patient: &patientTwo,
	}}
	procedures := map[string]procedure{"p": {ID: "p", Name: "П", DurationMinutes: 50}}
	result, err := createSchedule(day, []scheduleTask{{PatientNumber: 1, Severity: "medium", ProcedureID: "p"}}, procedures, settings{}, occupied)
	if err != nil {
		t.Fatal(err)
	}
	if result.Appointments[0].StartAt[11:16] != "09:00" {
		t.Fatalf("other patient's appointment should not block 09:00: %+v", result)
	}
	result, err = createSchedule(day, []scheduleTask{{PatientNumber: 2, Severity: "medium", ProcedureID: "p"}}, procedures, settings{}, occupied)
	if err != nil {
		t.Fatal(err)
	}
	if result.Appointments[0].StartAt[11:16] != "10:00" {
		t.Fatalf("same patient's appointment must block 09:00: %+v", result)
	}
}

func TestCreateWeeklyScheduleDistributesProceduresAcrossDays(t *testing.T) {
	day := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{"p": {ID: "p", Name: "П", DurationMinutes: 50}}
	tasks := make([]scheduleTask, 0, 24)
	for _, patient := range []struct{ number, count int }{{1, 12}, {2, 5}, {3, 7}} {
		for range patient.count {
			tasks = append(tasks, scheduleTask{PatientNumber: patient.number, Severity: "medium", ProcedureID: "p"})
		}
	}
	result, err := createWeeklySchedule(day, tasks, procedures, settings{TimeStepMinutes: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 24 || result.UnscheduledCount != 0 {
		t.Fatalf("all 24 procedures should fit into one week: %+v", result)
	}
	counts := map[string]int{}
	for _, item := range result.Appointments {
		counts[item.StartAt[:10]]++
	}
	if counts["2026-10-19"] != 17 || counts["2026-10-20"] != 7 {
		t.Fatalf("expected first-day capacity to continue on Tuesday, got %+v", counts)
	}
	for _, item := range result.Appointments {
		if item.StartAt[11:16] < "09:00" || item.EndAt[11:16] > "16:00" || (item.StartAt[11:16] < "14:00" && item.EndAt[11:16] > "13:00") {
			t.Fatalf("appointment is outside working hours: %+v", item)
		}
	}
}

func TestCreateWeeklyScheduleReportsWeekOverflow(t *testing.T) {
	weekStart := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	procedures := map[string]procedure{}
	tasks := []scheduleTask{}
	for index := range 50 {
		id := fmt.Sprintf("p-%d", index)
		procedures[id] = procedure{ID: id, Name: id, DurationMinutes: 50}
		tasks = append(tasks, scheduleTask{PatientNumber: 1, Severity: "medium", ProcedureID: id})
	}
	result, err := createWeeklySchedule(weekStart, tasks, procedures, settings{TimeStepMinutes: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledCount != 42 || result.UnscheduledCount != 8 || !reflect.DeepEqual(result.UnscheduledPatients, []int{1}) {
		t.Fatalf("expected explicit overflow when a patient needs more than seven workdays: %+v", result)
	}
}
