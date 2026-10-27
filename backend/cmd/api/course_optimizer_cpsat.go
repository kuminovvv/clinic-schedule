package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type optimizerBusySlot struct {
	StartAt        string `json:"startAt"`
	EndAt          string `json:"endAt"`
	PatientNumber  *int   `json:"patientNumber"`
	ProcedureID    string `json:"procedureId"`
	SourcePlanDate string `json:"sourcePlanDate"`
	Room           string `json:"room"`
}

type optimizerPayload struct {
	StartDate  string                  `json:"startDate"`
	CourseKey  string                  `json:"courseKey"`
	Plans      []patientPlan           `json:"plans"`
	Procedures map[string]procedure    `json:"procedures"`
	Settings   settings                `json:"settings"`
	Occupied   []optimizerBusySlot     `json:"occupied"`
	Absences   map[int]map[string]bool `json:"absences"`
}

type optimizedSlot struct {
	PatientNumber int    `json:"patientNumber"`
	ProcedureID   string `json:"procedureId"`
	StartAt       string `json:"startAt"`
	EndAt         string `json:"endAt"`
}

type optimizerResult struct {
	Appointments []optimizedSlot `json:"appointments"`
}

// Local opt-in only. The normal scheduler remains unchanged, including on VPS.
// Missing Python/OR-Tools or solver errors fail explicitly, never silently
// changing the schedule or falling back to a different optimizer.
func createCourseScheduleCP(ctx context.Context, day time.Time, plans []patientPlan, procedures map[string]procedure, configuration settings, occupied []timeSlot, absences map[int]map[string]bool) (scheduleResponse, error) {
	for _, plan := range plans {
		if err := validateCoursePrerequisites(plan, procedures); err != nil {
			return scheduleResponse{}, err
		}
	}
	courseKey := day.Format("2006-01-02")
	if len(plans) > 0 && plans[0].CourseKey != "" {
		courseKey = plans[0].CourseKey
	}
	input := optimizerPayload{StartDate: day.Format("2006-01-02"), CourseKey: courseKey, Plans: plans, Procedures: procedures, Settings: configuration, Absences: absences}
	for _, slot := range occupied {
		input.Occupied = append(input.Occupied, optimizerBusySlot{
			StartAt: slot.start.Format("2006-01-02T15:04"), EndAt: slot.end.Format("2006-01-02T15:04"),
			PatientNumber: slot.patient, ProcedureID: slot.procedureID, SourcePlanDate: slot.sourcePlanDate, Room: slot.room,
		})
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return scheduleResponse{}, fmt.Errorf("cannot encode optimizer input: %w", err)
	}
	python := os.Getenv("CLINIC_OPTIMIZER_PYTHON")
	if python == "" {
		python = "python3"
		if runtime.GOOS == "windows" {
			python = "python"
			if _, err := os.Stat(filepath.Join(".venv-cpsat", "Scripts", "python.exe")); err == nil {
				python = filepath.Join(".venv-cpsat", "Scripts", "python.exe")
			}
		} else if _, err := os.Stat(filepath.Join(".venv-cpsat", "bin", "python")); err == nil {
			python = filepath.Join(".venv-cpsat", "bin", "python")
		}
	}
	script := os.Getenv("CLINIC_OPTIMIZER_SCRIPT")
	if script == "" {
		script = filepath.Join("optimizer", "cpsat_course.py")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, python, script)
	command.Stdin = bytes.NewReader(payload)
	var output, stderr bytes.Buffer
	command.Stdout, command.Stderr = &output, &stderr
	if err := command.Run(); err != nil {
		return scheduleResponse{}, fmt.Errorf("local CP-SAT solver failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var response optimizerResult
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		return scheduleResponse{}, fmt.Errorf("invalid CP-SAT response: %w", err)
	}
	return validateOptimizedSchedule(day, plans, procedures, configuration, occupied, absences, response.Appointments)
}

// The solver is an independent process. Validate its entire result before
// saving anything, so a malformed response can never bypass clinic rules.
func validateOptimizedSchedule(day time.Time, plans []patientPlan, procedures map[string]procedure, configuration settings, occupied []timeSlot, absences map[int]map[string]bool, slots []optimizedSlot) (scheduleResponse, error) {
	result := scheduleResponse{Appointments: []appointment{}, UnscheduledPatients: []int{}}
	courseKey := day.Format("2006-01-02")
	if len(plans) > 0 && plans[0].CourseKey != "" {
		courseKey = plans[0].CourseKey
	}
	byPatient := map[int]patientPlan{}
	remaining := map[int]map[string]int{}
	for _, plan := range plans {
		byPatient[plan.PatientNumber] = plan
		remaining[plan.PatientNumber] = map[string]int{}
		for _, id := range plan.ProcedureIDs {
			remaining[plan.PatientNumber][id] = plan.ProcedureCounts[id]
		}
	}
	allocated := map[int]map[string][]time.Time{}
	for _, item := range slots {
		plan, ok := byPatient[item.PatientNumber]
		p, exists := procedures[item.ProcedureID]
		if !ok || !exists || remaining[item.PatientNumber][item.ProcedureID] < 1 {
			return scheduleResponse{}, fmt.Errorf("optimizer returned an unrequested session")
		}
		start, err1 := time.ParseInLocation("2006-01-02T15:04", item.StartAt, time.Local)
		end, err2 := time.ParseInLocation("2006-01-02T15:04", item.EndAt, time.Local)
		workDay := configuration.dayAt(start)
		workStartMinutes, _ := minutesFromClock(workDay.Start)
		step := configuration.TimeStepMinutes
		if step <= 0 {
			step = 10
		}
		arrival, validArrival := parseISODate(plan.ArrivalDate)
		departure, validDeparture := parseISODate(plan.DepartureDate)
		if !validArrival {
			arrival = day.AddDate(0, 0, plan.StartOffset)
		}
		if !validDeparture {
			departure = arrival.AddDate(0, 0, plan.StayDays-1)
		}
		if err1 != nil || err2 != nil || end.Sub(start) != time.Duration(p.DurationMinutes+p.PreparationMinutes)*time.Minute ||
			calendarDayNumber(start) < calendarDayNumber(arrival) || calendarDayNumber(start) > calendarDayNumber(departure) ||
			!configuration.allowsAppointment(start, end) || (start.Hour()*60+start.Minute()-workStartMinutes)%step != 0 ||
			absences[item.PatientNumber][start.Format("2006-01-02")] {
			return scheduleResponse{}, fmt.Errorf("optimizer returned a session outside working hours or stay")
		}
		for _, previous := range occupied {
			if (previous.patient == nil || (*previous.patient == item.PatientNumber && (previous.sourcePlanDate == "" || previous.sourcePlanDate == courseKey)) || sameRoom(previous.room, p.Room)) && start.Before(previous.end) && end.After(previous.start) {
				return scheduleResponse{}, fmt.Errorf("optimizer overlaps an existing appointment")
			}
			if previous.patient != nil && *previous.patient == item.PatientNumber && previous.procedureID == item.ProcedureID &&
				(previous.sourcePlanDate == "" || previous.sourcePlanDate == courseKey) &&
				abs64(calendarDayNumber(previous.start)-calendarDayNumber(start)) < int64(p.MinIntervalDays) {
				return scheduleResponse{}, fmt.Errorf("optimizer violates spacing with an existing session")
			}
		}
		for _, prior := range result.Appointments {
			if (prior.PatientNumber != nil && *prior.PatientNumber == item.PatientNumber || sameRoom(prior.Room, p.Room)) && prior.StartAt < item.EndAt && item.StartAt < prior.EndAt {
				return scheduleResponse{}, fmt.Errorf("optimizer overlaps patient sessions")
			}
		}
		if allocated[item.PatientNumber] == nil {
			allocated[item.PatientNumber] = map[string][]time.Time{}
		}
		for _, prev := range allocated[item.PatientNumber][item.ProcedureID] {
			if abs64(calendarDayNumber(start)-calendarDayNumber(prev)) < int64(p.MinIntervalDays) {
				return scheduleResponse{}, fmt.Errorf("optimizer violates interval between sessions")
			}
		}
		allocated[item.PatientNumber][item.ProcedureID] = append(allocated[item.PatientNumber][item.ProcedureID], start)
		remaining[item.PatientNumber][item.ProcedureID]--
		id, err := randomID()
		if err != nil {
			return scheduleResponse{}, err
		}
		color := "blue"
		if p.IsRelaxing {
			color = "teal"
		}
		patient := item.PatientNumber
		result.Appointments = append(result.Appointments, appointment{
			ID: id, Title: fmt.Sprintf("Пациент %d · %s", patient, p.Name),
			StartAt: item.StartAt, EndAt: item.EndAt, Description: fmt.Sprintf("%d мин + %d мин подготовка", p.DurationMinutes, p.PreparationMinutes),
			Color: color, Status: "scheduled", GeneratedByPlanner: true, PatientNumber: &patient, ProcedureID: p.ID, Room: p.Room,
		})
	}
	result.ScheduledCount = len(result.Appointments)
	// Recheck weekly catalog limits and the high-severity ordering constraints.
	// A fixed manually edited session counts toward weekly capacity, but even
	// previously exceeded limits should not prevent an unrelated partial course.
	for _, plan := range plans {
		if plan.Severity == "high" {
			usedPrerequisites := map[int]bool{}
			for targetID, prerequisiteID := range plan.ProcedurePrerequisites {
				for _, target := range result.Appointments {
					if target.PatientNumber == nil || *target.PatientNumber != plan.PatientNumber || target.ProcedureID != targetID {
						continue
					}
					found := false
					for index, prerequisite := range result.Appointments {
						if !usedPrerequisites[index] && prerequisite.PatientNumber != nil && *prerequisite.PatientNumber == plan.PatientNumber && prerequisite.ProcedureID == prerequisiteID &&
							prerequisite.StartAt[:10] == target.StartAt[:10] && prerequisite.EndAt <= target.StartAt {
							found = true
							usedPrerequisites[index] = true
							break
						}
					}
					if !found {
						return scheduleResponse{}, fmt.Errorf("optimizer omitted a required relaxing session before %q", procedures[targetID].Name)
					}
				}
			}
		}
		for _, id := range plan.ProcedureIDs {
			limit := procedures[id].DefaultWeeklyCount
			if limit <= 0 {
				limit = 7
			}
			for offset := 0; offset < plan.StayDays; offset++ {
				windowOffset := plan.StartOffset + offset
				fixed, generated := 0, 0
				for _, old := range occupied {
					if old.patient == nil || *old.patient != plan.PatientNumber || old.procedureID != id ||
						(old.sourcePlanDate != "" && old.sourcePlanDate != courseKey) {
						continue
					}
					delta := calendarDayNumber(old.start) - calendarDayNumber(day)
					if int64(windowOffset) <= delta && delta < int64(windowOffset+7) {
						fixed++
					}
				}
				for _, scheduled := range allocated[plan.PatientNumber][id] {
					delta := calendarDayNumber(scheduled) - calendarDayNumber(day)
					if int64(windowOffset) <= delta && delta < int64(windowOffset+7) {
						generated++
					}
				}
				capacity := limit - fixed
				if capacity < 0 {
					capacity = 0
				}
				if generated > capacity {
					return scheduleResponse{}, fmt.Errorf("optimizer exceeds the weekly procedure limit")
				}
			}
		}
		if plan.Severity == "high" {
			for _, relax := range result.Appointments {
				if relax.PatientNumber == nil || *relax.PatientNumber != plan.PatientNumber || !procedures[relax.ProcedureID].IsRelaxing {
					continue
				}
				for _, other := range result.Appointments {
					if other.PatientNumber != nil && *other.PatientNumber == plan.PatientNumber &&
						other.StartAt[:10] == relax.StartAt[:10] && !procedures[other.ProcedureID].IsRelaxing &&
						relax.EndAt > other.StartAt {
						return scheduleResponse{}, fmt.Errorf("optimizer violates the relaxing-procedure priority")
					}
				}
			}
		}
	}
	for _, plan := range plans {
		for _, id := range plan.ProcedureIDs {
			if missing := remaining[plan.PatientNumber][id]; missing > 0 {
				result.UnscheduledCount += missing
				result.UnscheduledPatients = append(result.UnscheduledPatients, plan.PatientNumber)
			}
		}
	}
	result.UnscheduledPatients = uniqueInts(result.UnscheduledPatients)
	return result, nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
