package main

import (
	"testing"
	"time"
)

func TestWorkScheduleValidation(t *testing.T) {
	if !validWorkWeek(defaultWorkWeek()) {
		t.Fatal("default work week must be valid")
	}
	for _, test := range []struct {
		name   string
		mutate func([]workingDay) []workingDay
	}{
		{"duplicate weekday", func(days []workingDay) []workingDay { days[1].Weekday = 1; return days }},
		{"missing weekday", func(days []workingDay) []workingDay { return days[:6] }},
		{"bad clock", func(days []workingDay) []workingDay { days[0].Start = "09:99"; return days }},
		{"end before start", func(days []workingDay) []workingDay { days[0].End = "08:00"; return days }},
		{"break outside hours", func(days []workingDay) []workingDay { days[0].Breaks[0].End = "19:00"; return days }},
		{"overlapping breaks", func(days []workingDay) []workingDay {
			days[0].Breaks = append(days[0].Breaks, weeklyBreak{ID: "quartz", Name: "Кварцевание", Start: "13:30", End: "14:30"})
			days[0].End = "19:00"
			return days
		}},
		{"empty break name", func(days []workingDay) []workingDay { days[0].Breaks[0].Name = "  "; return days }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if validWorkWeek(test.mutate(defaultWorkWeek())) {
				t.Fatal("invalid week accepted")
			}
		})
	}
}

func TestWorkingHoursAndQuartz(t *testing.T) {
	week := defaultWorkWeek()
	week[0].End = "20:00"
	week[0].Breaks = append(week[0].Breaks, weeklyBreak{ID: "quartz", Name: "Кварцевание", Start: "15:00", End: "19:00"})
	s := settings{TimeStepMinutes: 10, WorkWeek: week}
	if !validWorkWeek(week) {
		t.Fatal("custom work week invalid")
	}
	monday := time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local)
	for _, test := range []struct {
		start, end string
		valid      bool
	}{
		{"09:00", "09:50", true}, {"12:50", "13:10", false},
		{"14:30", "15:20", false}, {"15:10", "16:00", false},
		{"19:00", "19:50", true}, {"19:30", "20:10", false},
	} {
		start, _ := time.Parse("15:04", test.start)
		end, _ := time.Parse("15:04", test.end)
		makeAt := func(clock time.Time) time.Time {
			return monday.Add(time.Duration(clock.Hour()*60+clock.Minute()) * time.Minute)
		}
		if got := s.allowsAppointment(makeAt(start), makeAt(end)); got != test.valid {
			t.Fatalf("slot %s-%s: allowed=%t, expected %t", test.start, test.end, got, test.valid)
		}
	}
	sunday := monday.AddDate(0, 0, 6)
	if s.allowsAppointment(sunday.Add(9*time.Hour), sunday.Add(10*time.Hour)) {
		t.Fatal("weekend accepted")
	}
	procedures := map[string]procedure{"test": {ID: "test", Name: "Курс", DurationMinutes: 50}}
	result, err := createSchedule(monday, []scheduleTask{{PatientNumber: 1, Severity: "medium", ProcedureID: "test"}}, procedures, s, []timeSlot{
		{start: monday.Add(9 * time.Hour), end: monday.Add(15 * time.Hour)},
	})
	if err != nil || len(result.Appointments) != 1 || result.Appointments[0].StartAt != "2026-10-19T19:00" {
		t.Fatalf("quartz should push appointment to 19:00: result=%+v err=%v", result, err)
	}
	result, err = createSchedule(sunday, []scheduleTask{{PatientNumber: 1, Severity: "medium", ProcedureID: "test"}}, procedures, s, nil)
	if err != nil || len(result.Appointments) != 0 {
		t.Fatalf("weekend must be skipped: %+v %v", result, err)
	}
}
