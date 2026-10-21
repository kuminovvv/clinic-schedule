package main

import "testing"

func TestEqualDayPlanContentsIgnoresRevisionAndPatientOrder(t *testing.T) {
	selected := 2
	base := dayPlan{
		Date: "2026-10-21", Revision: 3, SelectedPatientNumber: &selected,
		Plans: []patientPlan{
			{PatientNumber: 1, Severity: "low", StayDays: 7, ProcedureIDs: []string{"a", "b"}, ProcedureCounts: map[string]int{"a": 2, "b": 1}},
			{PatientNumber: 2, Severity: "high", StayDays: 14, ProcedureIDs: []string{"b"}, ProcedureCounts: map[string]int{"b": 3}},
		},
	}
	copy := base
	copy.Revision = 99
	copy.Plans = []patientPlan{base.Plans[1], base.Plans[0]}
	if !equalDayPlanContents(base, copy) {
		t.Fatal("equivalent plans should be idempotent even if revision or patient order differs")
	}
	copy.Plans = []patientPlan{base.Plans[0], base.Plans[1]}
	copy.Plans[0].ProcedureIDs = []string{"b", "a"}
	if equalDayPlanContents(base, copy) {
		t.Fatal("a changed procedure order is a real edit")
	}
	copy.Plans = []patientPlan{base.Plans[0], base.Plans[1]}
	copy.Plans[0].ProcedureCounts = map[string]int{"a": 4, "b": 1}
	if equalDayPlanContents(base, copy) {
		t.Fatal("changed session counts are a real edit")
	}
}
