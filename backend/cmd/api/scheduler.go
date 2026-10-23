package main

import (
	"crypto/rand"
	"fmt"
	"sort"
	"time"
)

type scheduledTask struct {
	patient   int
	severity  string
	procedure procedure
}

type patientBundle struct {
	patient      int
	severityRank int
	hasRelaxing  bool
	tasks        []scheduledTask
}

type timeSlot struct {
	start          time.Time
	end            time.Time
	patient        *int
	procedureID    string
	sourcePlanDate string
	room           string
}

func createSchedule(day time.Time, input []scheduleTask, procedures map[string]procedure, settings settings, occupied []timeSlot) (scheduleResponse, error) {
	stepMinutes := settings.TimeStepMinutes
	if stepMinutes <= 0 {
		stepMinutes = 10
	}
	bundlesByPatient := map[int]*patientBundle{}
	for _, item := range input {
		p, ok := procedures[item.ProcedureID]
		if !ok || item.PatientNumber < 1 || (item.Severity != "high" && item.Severity != "medium" && item.Severity != "low") {
			return scheduleResponse{}, errInvalidTask
		}
		bundle, exists := bundlesByPatient[item.PatientNumber]
		if !exists {
			bundle = &patientBundle{patient: item.PatientNumber, severityRank: severityRank(item.Severity)}
			bundlesByPatient[item.PatientNumber] = bundle
		} else if severityRank(item.Severity) < bundle.severityRank {
			bundle.severityRank = severityRank(item.Severity)
		}
		task := scheduledTask{patient: item.PatientNumber, severity: item.Severity, procedure: p}
		bundle.tasks = append(bundle.tasks, task)
		bundle.hasRelaxing = bundle.hasRelaxing || p.IsRelaxing
	}

	bundles := make([]patientBundle, 0, len(bundlesByPatient))
	for _, bundle := range bundlesByPatient {
		if bundle.severityRank == severityRank("high") {
			sort.SliceStable(bundle.tasks, func(i, j int) bool {
				return bundle.tasks[i].procedure.IsRelaxing && !bundle.tasks[j].procedure.IsRelaxing
			})
		}
		bundles = append(bundles, *bundle)
	}
	sort.SliceStable(bundles, func(i, j int) bool {
		if bundles[i].severityRank != bundles[j].severityRank {
			return bundles[i].severityRank < bundles[j].severityRank
		}
		return bundles[i].patient < bundles[j].patient
	})

	appointments := make([]appointment, 0, len(input))
	workingDay := settings.dayAt(day)
	if !workingDay.Open {
		unscheduledPatients := make([]int, 0, len(bundles))
		for _, bundle := range bundles {
			unscheduledPatients = append(unscheduledPatients, bundle.patient)
		}
		return scheduleResponse{Appointments: []appointment{}, UnscheduledCount: len(input), UnscheduledPatients: unscheduledPatients}, nil
	}
	workStartMinutes, _ := minutesFromClock(workingDay.Start)
	workEndMinutes, _ := minutesFromClock(workingDay.End)
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local).Add(time.Duration(workStartMinutes) * time.Minute)
	workEnd := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local).Add(time.Duration(workEndMinutes) * time.Minute)
	blocks := make([]timeSlot, 0, len(workingDay.Breaks))
	for _, b := range workingDay.Breaks {
		begin, _ := minutesFromClock(b.Start)
		finish, _ := minutesFromClock(b.End)
		blocks = append(blocks, timeSlot{start: dayStart.Add(time.Duration(begin-workStartMinutes) * time.Minute), end: dayStart.Add(time.Duration(finish-workStartMinutes) * time.Minute)})
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].start.Before(blocks[j].start) })
	unscheduled := make([]int, 0)
	for _, bundle := range bundles {
		slots := findPatientSlots(dayStart, workEnd, blocks, bundle.tasks, occupied, stepMinutes, bundle.patient)
		for index, task := range bundle.tasks {
			if index >= len(slots) {
				unscheduled = append(unscheduled, bundle.patient)
				continue
			}
			start := slots[index].start
			end := slots[index].end
			id, err := randomID()
			if err != nil {
				return scheduleResponse{}, err
			}
			color := "blue"
			if task.procedure.IsRelaxing {
				color = "teal"
			}
			patient := task.patient
			appointments = append(appointments, appointment{ID: id, Title: fmt.Sprintf("Пациент %d · %s", patient, task.procedure.Name), StartAt: start.Format("2006-01-02T15:04"), EndAt: end.Format("2006-01-02T15:04"), Description: fmt.Sprintf("%d мин + %d мин подготовка", task.procedure.DurationMinutes, task.procedure.PreparationMinutes), Color: color, Status: "scheduled", GeneratedByPlanner: true, PatientNumber: &patient, ProcedureID: task.procedure.ID, Room: task.procedure.Room})
			occupied = append(occupied, timeSlot{start: start, end: end, patient: &patient, room: task.procedure.Room})
		}
	}
	return scheduleResponse{Appointments: appointments, ScheduledCount: len(appointments), UnscheduledCount: len(unscheduled), UnscheduledPatients: uniqueInts(unscheduled)}, nil
}

func findPatientSlots(dayStart, workEnd time.Time, blocks []timeSlot, tasks []scheduledTask, occupied []timeSlot, stepMinutes, patient int) []timeSlot {
	var bestSlots []timeSlot
	for candidate := roundUpToStep(dayStart, dayStart, stepMinutes); candidate.Before(workEnd); candidate = candidate.Add(time.Duration(stepMinutes) * time.Minute) {
		cursor := candidate
		shiftedForOccupied := false
		slots := make([]timeSlot, 0, len(tasks))
		for _, task := range tasks {
			duration := time.Duration(task.procedure.DurationMinutes+task.procedure.PreparationMinutes) * time.Minute
			// A conflict later in the patient's course must move just this session,
			// rather than discard sessions that already fit before it. Check all
			// closures again after each move, since an occupied slot can end inside
			// a recurring closure (or another occupied slot).
			for {
				if cursor.Add(duration).After(workEnd) {
					break
				}
				var conflictEnd time.Time
				conflictWasOccupied := false
				for _, block := range blocks {
					if cursor.Before(block.end) && cursor.Add(duration).After(block.start) && (conflictEnd.IsZero() || block.end.Before(conflictEnd)) {
						conflictEnd = block.end
					}
				}
				for _, slot := range occupied {
					if (slot.patient == nil || *slot.patient == patient || (task.procedure.Room != "" && sameRoom(slot.room, task.procedure.Room))) && cursor.Before(slot.end) && cursor.Add(duration).After(slot.start) &&
						(conflictEnd.IsZero() || slot.end.Before(conflictEnd)) {
						conflictEnd = slot.end
						conflictWasOccupied = true
					}
				}
				if conflictEnd.IsZero() {
					break
				}
				shiftedForOccupied = shiftedForOccupied || conflictWasOccupied
				cursor = roundUpToStep(conflictEnd, dayStart, stepMinutes)
			}
			if cursor.Add(duration).After(workEnd) {
				break
			}
			slot := timeSlot{start: cursor, end: cursor.Add(duration)}
			slots = append(slots, slot)
			cursor = slot.end
		}
		if len(slots) > len(bestSlots) {
			bestSlots = slots
		}
		// Keep the patient's procedures together when a candidate exists that
		// avoids waiting for another appointment. Scheduled closures such as
		// lunch are unavoidable, and do not prevent choosing an early start.
		if len(slots) == len(tasks) && !shiftedForOccupied {
			return slots
		}
	}
	return bestSlots
}

// createWeeklySchedule keeps the order of each patient's procedures and carries
// the unplaced suffix into the following day of the same calendar week.
func createWeeklySchedule(weekStart time.Time, input []scheduleTask, procedures map[string]procedure, settings settings, occupied []timeSlot) (scheduleResponse, error) {
	remaining := append([]scheduleTask(nil), input...)
	sort.SliceStable(remaining, func(i, j int) bool {
		if severityRank(remaining[i].Severity) != severityRank(remaining[j].Severity) {
			return severityRank(remaining[i].Severity) < severityRank(remaining[j].Severity)
		}
		if remaining[i].PatientNumber != remaining[j].PatientNumber {
			return remaining[i].PatientNumber < remaining[j].PatientNumber
		}
		return procedures[remaining[i].ProcedureID].IsRelaxing && !procedures[remaining[j].ProcedureID].IsRelaxing
	})
	result := scheduleResponse{Appointments: []appointment{}, UnscheduledPatients: []int{}}
	for offset := 0; offset < 7 && len(remaining) > 0; offset++ {
		day := weekStart.AddDate(0, 0, offset)
		dayOccupied := make([]timeSlot, 0)
		for _, slot := range occupied {
			if slot.start.Year() == day.Year() && slot.start.YearDay() == day.YearDay() {
				dayOccupied = append(dayOccupied, slot)
			}
		}
		daily, err := createSchedule(day, remaining, procedures, settings, dayOccupied)
		if err != nil {
			return scheduleResponse{}, err
		}
		result.Appointments = append(result.Appointments, daily.Appointments...)
		scheduledByPatient := map[int]int{}
		for _, item := range daily.Appointments {
			scheduledByPatient[*item.PatientNumber]++
		}
		pending := make([]scheduleTask, 0)
		for _, task := range remaining {
			if scheduledByPatient[task.PatientNumber] > 0 {
				scheduledByPatient[task.PatientNumber]--
			} else {
				pending = append(pending, task)
			}
		}
		remaining = pending
	}
	result.ScheduledCount = len(result.Appointments)
	result.UnscheduledCount = len(remaining)
	for _, task := range remaining {
		result.UnscheduledPatients = append(result.UnscheduledPatients, task.PatientNumber)
	}
	result.UnscheduledPatients = uniqueInts(result.UnscheduledPatients)
	return result, nil
}

func severityRank(severity string) int {
	switch severity {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func roundUpToStep(value, dayStart time.Time, stepMinutes int) time.Time {
	minutes := int(value.Sub(dayStart).Minutes())
	step := ((minutes + stepMinutes - 1) / stepMinutes) * stepMinutes
	return dayStart.Add(time.Duration(step) * time.Minute)
}

func uniqueInts(values []int) []int {
	seen := map[int]bool{}
	result := make([]int, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Ints(result)
	return result
}

func randomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}
