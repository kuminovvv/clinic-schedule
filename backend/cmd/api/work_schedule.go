package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Weekday follows ISO numbering: Monday=1, Sunday=7.
type weeklyBreak struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type workingDay struct {
	Weekday int           `json:"weekday"`
	Open    bool          `json:"open"`
	Start   string        `json:"start"`
	End     string        `json:"end"`
	Breaks  []weeklyBreak `json:"breaks"`
}

func defaultWorkWeek() []workingDay {
	days := make([]workingDay, 7)
	for i := range days {
		days[i] = workingDay{Weekday: i + 1, Open: i < 5, Start: "09:00", End: "16:00", Breaks: []weeklyBreak{}}
		if i < 5 {
			days[i].Breaks = []weeklyBreak{{ID: "lunch", Name: "Обед", Start: "13:00", End: "14:00"}}
		}
	}
	return days
}

func minutesFromClock(value string) (int, bool) {
	if len(value) != 5 || value[2] != ':' {
		return 0, false
	}
	for _, i := range []int{0, 1, 3, 4} {
		if value[i] < '0' || value[i] > '9' {
			return 0, false
		}
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[3]-'0')*10 + int(value[4]-'0')
	if hour > 24 || minute > 59 || (hour == 24 && minute != 0) {
		return 0, false
	}
	return hour*60 + minute, true
}

func validWorkWeek(week []workingDay) bool {
	if len(week) != 7 {
		return false
	}
	seenDays := [8]bool{}
	for _, day := range week {
		if day.Weekday < 1 || day.Weekday > 7 || seenDays[day.Weekday] || len(day.Breaks) > 12 {
			return false
		}
		seenDays[day.Weekday] = true
		start, ok1 := minutesFromClock(day.Start)
		end, ok2 := minutesFromClock(day.End)
		if !ok1 || !ok2 || start >= end || start == 1440 {
			return false
		}
		ids := map[string]bool{}
		intervals := [][2]int{}
		for _, b := range day.Breaks {
			bStart, validStart := minutesFromClock(b.Start)
			bEnd, validEnd := minutesFromClock(b.End)
			if !validStart || !validEnd || bStart < start || bEnd > end || bStart >= bEnd || len(b.ID) > 80 || strings.TrimSpace(b.ID) == "" || ids[b.ID] || strings.TrimSpace(b.Name) == "" || len([]rune(b.Name)) > 80 {
				return false
			}
			ids[b.ID] = true
			intervals = append(intervals, [2]int{bStart, bEnd})
		}
		sort.Slice(intervals, func(i, j int) bool { return intervals[i][0] < intervals[j][0] })
		for i := 1; i < len(intervals); i++ {
			if intervals[i][0] < intervals[i-1][1] {
				return false
			}
		}
	}
	return true
}

func (s settings) week() []workingDay {
	if len(s.WorkWeek) == 7 {
		return s.WorkWeek
	}
	// Legacy callers/tests without a configured week keep the old seven-day schedule.
	// Actual persisted settings always contain all seven explicitly configured days.
	week := defaultWorkWeek()
	for i := range week {
		week[i].Open = true
		if i >= 5 {
			week[i].Breaks = []weeklyBreak{{ID: "lunch", Name: "Обед", Start: "13:00", End: "14:00"}}
		}
	}
	return week
}

func (s settings) dayAt(day time.Time) workingDay {
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	for _, d := range s.week() {
		if d.Weekday == weekday {
			return d
		}
	}
	return workingDay{Weekday: weekday}
}

func (s settings) allowsAppointment(start, end time.Time) bool {
	if !start.Before(end) {
		return false
	}
	// "24:00" denotes the exclusive midnight boundary of the current working
	// day. Stored appointment timestamps represent it as 00:00 the next day.
	midnightEnd := end.Equal(time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location()).AddDate(0, 0, 1))
	if !sameCalendarDate(start, end) && !midnightEnd {
		return false
	}
	day := s.dayAt(start)
	if !day.Open {
		return false
	}
	startMinutes := start.Hour()*60 + start.Minute()
	endMinutes := end.Hour()*60 + end.Minute()
	if midnightEnd {
		endMinutes = 1440
	}
	workStart, okStart := minutesFromClock(day.Start)
	workEnd, okEnd := minutesFromClock(day.End)
	if !okStart || !okEnd || startMinutes < workStart || endMinutes > workEnd {
		return false
	}
	for _, b := range day.Breaks {
		bs, _ := minutesFromClock(b.Start)
		be, _ := minutesFromClock(b.End)
		if startMinutes < be && endMinutes > bs {
			return false
		}
	}
	return true
}

func sameCalendarDate(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func loadSettings(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (settings, error) {
	var value settings
	var raw []byte
	err := q.QueryRow(ctx, "SELECT time_step_minutes,display_start_hour,display_end_hour,work_week FROM calendar_settings WHERE id=1").Scan(&value.TimeStepMinutes, &value.DisplayStartHour, &value.DisplayEndHour, &raw)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value.WorkWeek); err != nil {
		return value, fmt.Errorf("invalid saved work week: %w", err)
	}
	if !validWorkWeek(value.WorkWeek) {
		return value, errors.New("invalid saved work week")
	}
	return value, nil
}
