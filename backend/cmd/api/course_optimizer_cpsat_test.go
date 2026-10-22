package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCourseOptimizerRejectsInvalidResults(t *testing.T) {
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	p := procedure{ID: "p", Name: "Test", DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 2, DefaultWeeklyCount: 7}
	plan := testCoursePlan(14, 2, "medium", "p")
	base := optimizedSlot{PatientNumber: 1, ProcedureID: "p", StartAt: "2026-10-22T09:00", EndAt: "2026-10-22T09:50"}
	cases := []struct {
		name  string
		slots []optimizedSlot
	}{
		{"same-day repetition", []optimizedSlot{base, {PatientNumber: 1, ProcedureID: "p", StartAt: "2026-10-22T10:00", EndAt: "2026-10-22T10:50"}}},
		{"overlap", []optimizedSlot{base, {PatientNumber: 1, ProcedureID: "p", StartAt: "2026-10-22T09:30", EndAt: "2026-10-22T10:20"}}},
		{"outside work", []optimizedSlot{{PatientNumber: 1, ProcedureID: "p", StartAt: "2026-10-22T16:00", EndAt: "2026-10-22T16:50"}}},
		{"lunch", []optimizedSlot{{PatientNumber: 1, ProcedureID: "p", StartAt: "2026-10-22T13:00", EndAt: "2026-10-22T13:50"}}},
		{"invalid duration", []optimizedSlot{{PatientNumber: 1, ProcedureID: "p", StartAt: "2026-10-22T09:00", EndAt: "2026-10-22T09:30"}}},
	}
	t.Run("weekly maximum", func(t *testing.T) {
		limited := p
		limited.DefaultWeeklyCount = 1
		slots := []optimizedSlot{base, {PatientNumber: 1, ProcedureID: "p", StartAt: "2026-10-26T09:00", EndAt: "2026-10-26T09:50"}}
		if _, err := validateOptimizedSchedule(start, []patientPlan{plan}, map[string]procedure{"p": limited}, settings{WorkWeek: defaultWorkWeek()}, nil, nil, slots); err == nil {
			t.Fatal("two repetitions in seven days bypassed the weekly limit")
		}
	})
	t.Run("high severity relaxation first", func(t *testing.T) {
		relax := procedure{ID: "r", Name: "Relax", DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1, DefaultWeeklyCount: 7, IsRelaxing: true}
		regular := p
		regular.MinIntervalDays = 1
		plan := patientPlan{PatientNumber: 1, Severity: "high", StayDays: 1, ProcedureIDs: []string{"p", "r"}, ProcedureCounts: map[string]int{"p": 1, "r": 1}}
		slots := []optimizedSlot{base, {PatientNumber: 1, ProcedureID: "r", StartAt: "2026-10-22T10:00", EndAt: "2026-10-22T10:50"}}
		if _, err := validateOptimizedSchedule(start, []patientPlan{plan}, map[string]procedure{"p": regular, "r": relax}, settings{WorkWeek: defaultWorkWeek()}, nil, nil, slots); err == nil {
			t.Fatal("normal session before relaxing session should be rejected")
		}
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateOptimizedSchedule(start, []patientPlan{plan}, map[string]procedure{"p": p}, settings{WorkWeek: defaultWorkWeek()}, nil, nil, tc.slots); err == nil {
				t.Fatalf("unsafe slots passed validation: %+v", tc.slots)
			}
		})
	}
}

func TestLocalCPOptimizerSpreadsDemoCourse(t *testing.T) {
	// This optional integration check runs only where the experiment's Python
	// environment was explicitly installed. Standard CI needs no OR-Tools.
	path := filepath.Join("..", "..", ".venv-cpsat")
	python := filepath.Join(path, "bin", "python")
	if runtime.GOOS == "windows" {
		python = filepath.Join(path, "Scripts", "python.exe")
	}
	if _, err := os.Stat(python); err != nil {
		t.Skip("experimental CP-SAT virtualenv not installed")
	}
	t.Setenv("CLINIC_OPTIMIZER_PYTHON", python)
	t.Setenv("CLINIC_OPTIMIZER_SCRIPT", filepath.Join("..", "..", "optimizer", "cpsat_course.py"))
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	p := map[string]procedure{}
	plan := patientPlan{PatientNumber: 1, Severity: "medium", StayDays: 14, ProcedureCounts: map[string]int{}}
	for index, count := range []int{3, 3, 2, 3, 2, 3} {
		id := fmt.Sprintf("p%d", index)
		p[id] = procedure{ID: id, Name: id, DurationMinutes: 30, PreparationMinutes: 20, MinIntervalDays: 1, DefaultWeeklyCount: 7}
		plan.ProcedureIDs = append(plan.ProcedureIDs, id)
		plan.ProcedureCounts[id] = count
	}
	config := settings{TimeStepMinutes: 10, WorkWeek: defaultWorkWeek()}
	legacy, err := createCourseSchedule(start, []patientPlan{plan}, p, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	optimized, err := createCourseScheduleCP(context.Background(), start, []patientPlan{plan}, p, config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if optimized.UnscheduledCount != 0 || optimized.ScheduledCount != 16 {
		t.Fatalf("expected all 16 sessions, got %+v", optimized)
	}
	byDay := map[string]int{}
	for _, item := range optimized.Appointments {
		byDay[item.StartAt[:10]]++
	}
	legacyDays := map[string]int{}
	for _, item := range legacy.Appointments {
		legacyDays[item.StartAt[:10]]++
	}
	if len(byDay) < 7 || len(byDay) <= len(legacyDays) {
		t.Fatalf("expected improved dispersion, old=%v new=%v", legacyDays, byDay)
	}
	maxPerDay := 0
	for _, count := range byDay {
		if count > maxPerDay {
			maxPerDay = count
		}
	}
	if maxPerDay > 3 {
		t.Fatalf("too many procedures in one day: %v", byDay)
	}
	t.Logf("legacy: %v | CP-SAT: %v", legacyDays, byDay)
}
