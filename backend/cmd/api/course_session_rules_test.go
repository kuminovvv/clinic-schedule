package main

import (
	"strings"
	"testing"
	"time"
)

func TestCourseSessionRulesRequireSeparateEarlierSameDayRelaxation(t *testing.T) {
	day := time.Date(2099, 1, 5, 0, 0, 0, 0, time.Local)
	session := func(id, procedureID string, offset, hour, minute int) courseSession {
		start := day.AddDate(0, 0, offset).Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
		return courseSession{id: id, procedureID: procedureID, start: start, end: start.Add(40 * time.Minute)}
	}
	rules := map[string]courseProcedureRule{
		"relax": {name: "Relax", minDays: 1, weeklyLimit: 7},
		"hard":  {name: "Hard", minDays: 1, weeklyLimit: 7, prerequisite: "relax"},
		"hard2": {name: "Hard2", minDays: 1, weeklyLimit: 7, prerequisite: "relax"},
	}
	valid := []courseSession{session("r1", "relax", 0, 9, 0), session("h1", "hard", 0, 9, 40)}
	if err := validateCourseSessions(5, rules, valid); err != nil {
		t.Fatalf("a proper pair should be accepted: %v", err)
	}
	for _, tc := range []struct {
		name     string
		sessions []courseSession
	}{
		{"cancel required relaxation", valid[1:]},
		{"move required relaxation to next date", []courseSession{session("r1", "relax", 1, 9, 0), valid[1]}},
		{"move target before relaxation", []courseSession{valid[0], session("h1", "hard", 0, 8, 0)}},
		{"two complex procedures sharing one relaxing session", append(append([]courseSession(nil), valid...), session("h2", "hard2", 0, 10, 20))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateCourseSessions(5, rules, tc.sessions); err == nil || !strings.Contains(err.Error(), "requires a separate earlier") {
				t.Fatalf("invalid prerequisite pairing passed: %v", err)
			}
		})
	}
	// Two targets sharing a prerequisite are supported when each has its own
	// relaxing session on a different day.
	valid = append(valid, session("r2", "relax", 1, 9, 0), session("h2", "hard2", 1, 9, 40))
	if err := validateCourseSessions(5, rules, valid); err != nil {
		t.Fatalf("two complete prerequisite pairs should be valid: %v", err)
	}
}

func TestCourseSessionRulesLimitOnePerDayAndRollingWeek(t *testing.T) {
	day := time.Date(2099, 1, 5, 9, 0, 0, 0, time.Local)
	rules := map[string]courseProcedureRule{"relax": {name: "Relax", minDays: 1, weeklyLimit: 2}}
	makeSession := func(id string, offset int) courseSession {
		start := day.AddDate(0, 0, offset)
		return courseSession{id: id, procedureID: "relax", start: start, end: start.Add(time.Hour)}
	}
	if err := validateCourseSessions(1, rules, []courseSession{makeSession("1", 0), makeSession("2", 0)}); err == nil {
		t.Fatal("two sessions of the same relaxing procedure on one date must be rejected")
	}
	if err := validateCourseSessions(1, rules, []courseSession{makeSession("1", 0), makeSession("2", 1), makeSession("3", 6)}); err == nil || !strings.Contains(err.Error(), "seven days") {
		t.Fatalf("three sessions inside a rolling week must fail: %v", err)
	}
	if err := validateCourseSessions(1, rules, []courseSession{makeSession("1", 0), makeSession("2", 1), makeSession("3", 7)}); err != nil {
		t.Fatalf("two sessions in every rolling week are permitted: %v", err)
	}
}

func TestDefaultSchedulerHonorsRelaxingFrequencyAndSharedPrerequisites(t *testing.T) {
	start := time.Date(2099, 1, 5, 0, 0, 0, 0, time.Local)
	week := defaultWorkWeek()
	for index := range week {
		week[index].Open = true
	}
	config := settings{TimeStepMinutes: 10, WorkWeek: week}
	procedures := map[string]procedure{
		"r": {ID: "r", Name: "Relax", DurationMinutes: 40, MinIntervalDays: 1, DefaultWeeklyCount: 7, IsRelaxing: true},
		"a": {ID: "a", Name: "Hard A", DurationMinutes: 40, MinIntervalDays: 1, DefaultWeeklyCount: 7},
		"b": {ID: "b", Name: "Hard B", DurationMinutes: 40, MinIntervalDays: 1, DefaultWeeklyCount: 7},
	}
	for _, tc := range []struct {
		name       string
		stay       int
		relaxCount int
		weekly     int
		want       int
	}{
		{"daily relaxing allowed with zero full-day break", 7, 7, 7, 7},
		{"weekly frequency still limits relaxing", 7, 7, 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := procedures["r"]
			p.DefaultWeeklyCount = tc.weekly
			result, err := createCourseSchedule(start, []patientPlan{{PatientNumber: 1, Severity: "high", StayDays: tc.stay, ProcedureIDs: []string{"r"}, ProcedureCounts: map[string]int{"r": tc.relaxCount}}}, map[string]procedure{"r": p}, config, nil)
			if err != nil || result.ScheduledCount != tc.want || result.UnscheduledCount != tc.relaxCount-tc.want {
				t.Fatalf("unexpected relaxing session frequency: %+v err=%v", result, err)
			}
		})
	}
	plan := patientPlan{
		PatientNumber: 1, Severity: "high", StayDays: 2,
		ProcedureIDs: []string{"r", "a", "b"}, ProcedureCounts: map[string]int{"r": 2, "a": 1, "b": 1},
		ProcedurePrerequisites: map[string]string{"a": "r", "b": "r"},
	}
	result, err := createCourseSchedule(start, []patientPlan{plan}, procedures, config, nil)
	if err != nil || result.ScheduledCount != 4 || result.UnscheduledCount != 0 {
		t.Fatalf("shared prerequisite must create two complete pairs: %+v err=%v", result, err)
	}
	sessions := make([]courseSession, 0, len(result.Appointments))
	for _, item := range result.Appointments {
		begin, _ := time.ParseInLocation("2006-01-02T15:04", item.StartAt, time.Local)
		end, _ := time.ParseInLocation("2006-01-02T15:04", item.EndAt, time.Local)
		sessions = append(sessions, courseSession{id: item.ID, procedureID: item.ProcedureID, start: begin, end: end})
	}
	rules := map[string]courseProcedureRule{
		"r": {name: "Relax", minDays: 1, weeklyLimit: 7},
		"a": {name: "Hard A", minDays: 1, weeklyLimit: 7, prerequisite: "r"},
		"b": {name: "Hard B", minDays: 1, weeklyLimit: 7, prerequisite: "r"},
	}
	if err := validateCourseSessions(1, rules, sessions); err != nil {
		t.Fatalf("default scheduler output violates mandatory pairing: %v, %+v", err, result.Appointments)
	}
}

func TestUnscheduledReasonReportsFrequencyLimits(t *testing.T) {
	week := defaultWorkWeek()
	for index := range week {
		week[index].Open = true
	}
	plan := patientPlan{PatientNumber: 1, Severity: "medium", StayDays: 7, ArrivalDate: "2099-01-05"}
	code, reason, recommendation := explainUnscheduledProcedure(plan, procedure{DurationMinutes: 40, MinIntervalDays: 1, DefaultWeeklyCount: 2}, 5, settings{WorkWeek: week}, nil)
	if code != "weekly_limit" || reason == "" || recommendation == "" {
		t.Fatalf("missing actionable reason for weekly limit: %q, %q, %q", code, reason, recommendation)
	}
	code, _, _ = explainUnscheduledProcedure(plan, procedure{DurationMinutes: 40, MinIntervalDays: 1, DefaultWeeklyCount: 7}, 5, settings{WorkWeek: week}, map[string]bool{
		"2099-01-05": true, "2099-01-06": true, "2099-01-07": true, "2099-01-08": true,
	})
	if code != "absence" {
		t.Fatalf("absences should be reported as the limiting cause: %s", code)
	}
}
