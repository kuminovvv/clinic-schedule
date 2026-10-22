package main

import (
	"fmt"
	"sort"
	"time"
)

type courseState struct {
	procedure procedure
	requested int
	completed int
	lastDays  []time.Time
}

func calendarDayNumber(date time.Time) int64 {
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).Unix() / 86400
}

// Each selected procedure represents a course of N sessions. Sessions are
// spaced across the patient's stay but never closer than minIntervalDays.
// Patients can run concurrently; appointments of one patient cannot overlap.
func createCourseSchedule(start time.Time, plans []patientPlan, procedures map[string]procedure, settings settings, occupied []timeSlot) (scheduleResponse, error) {
	return createCourseScheduleWithAbsences(start, plans, procedures, settings, occupied, nil)
}

func createCourseScheduleWithAbsences(start time.Time, plans []patientPlan, procedures map[string]procedure, settings settings, occupied []timeSlot, absences map[int]map[string]bool) (scheduleResponse, error) {
	result := scheduleResponse{Appointments: []appointment{}, UnscheduledPatients: []int{}}
	planDateKey := start.Format("2006-01-02")
	plans = append([]patientPlan(nil), plans...)
	sort.SliceStable(plans, func(i, j int) bool {
		if severityRank(plans[i].Severity) != severityRank(plans[j].Severity) {
			return severityRank(plans[i].Severity) < severityRank(plans[j].Severity)
		}
		return plans[i].PatientNumber < plans[j].PatientNumber
	})
	maxDays := 0
	states := map[int]map[string]*courseState{}
	for _, plan := range plans {
		if plan.StayDays < 1 || plan.StayDays > 365 {
			return result, errInvalidTask
		}
		if plan.StayDays > maxDays {
			maxDays = plan.StayDays
		}
		states[plan.PatientNumber] = map[string]*courseState{}
		for _, id := range plan.ProcedureIDs {
			p, ok := procedures[id]
			count := plan.ProcedureCounts[id]
			if !ok || p.MinIntervalDays < 1 || count < 1 || count > 365 {
				return result, errInvalidTask
			}
			states[plan.PatientNumber][id] = &courseState{procedure: p, requested: count}
		}
	}
	for _, slot := range occupied {
		if slot.patient == nil {
			continue
		}
		if slot.sourcePlanDate != "" && slot.sourcePlanDate != planDateKey {
			continue
		}
		if byProcedure, ok := states[*slot.patient]; ok {
			if state, ok := byProcedure[slot.procedureID]; ok {
				state.lastDays = append(state.lastDays, slot.start)
			}
		}
	}
	for offset := 0; offset < maxDays; offset++ {
		day := start.AddDate(0, 0, offset)
		for _, plan := range plans {
			if offset >= plan.StayDays {
				continue
			}
			if absences[plan.PatientNumber][day.Format("2006-01-02")] {
				continue
			}
			pending := make([]scheduleTask, 0, len(plan.ProcedureIDs))
			ids := append([]string(nil), plan.ProcedureIDs...)
			if plan.Severity == "high" {
				sort.SliceStable(ids, func(i, j int) bool { return procedures[ids[i]].IsRelaxing && !procedures[ids[j]].IsRelaxing })
			}
			for _, id := range ids {
				state := states[plan.PatientNumber][id]
				if state.completed >= state.requested {
					continue
				}
				plannedDay := 0
				if state.requested > 1 {
					plannedDay = (plan.StayDays - 1) * state.completed / (state.requested - 1)
				}
				if offset < plannedDay {
					continue
				}
				valid := true
				for _, last := range state.lastDays {
					if delta := calendarDayNumber(day) - calendarDayNumber(last); delta < int64(state.procedure.MinIntervalDays) && delta > -int64(state.procedure.MinIntervalDays) {
						valid = false
						break
					}
				}
				if !valid {
					continue
				}
				pending = append(pending, scheduleTask{PatientNumber: plan.PatientNumber, Severity: plan.Severity, ProcedureID: id})
			}
			if len(pending) == 0 {
				continue
			}
			dailyOccupied := make([]timeSlot, 0)
			for _, slot := range occupied {
				if slot.start.Year() == day.Year() && slot.start.YearDay() == day.YearDay() && (slot.sourcePlanDate == "" || slot.sourcePlanDate == planDateKey) {
					dailyOccupied = append(dailyOccupied, slot)
				}
			}
			daily, err := createSchedule(day, pending, procedures, settings, dailyOccupied)
			if err != nil {
				return result, err
			}
			for _, item := range daily.Appointments {
				state := states[plan.PatientNumber][item.ProcedureID]
				state.completed++
				state.lastDays = append(state.lastDays, day)
				startTime, _ := time.ParseInLocation("2006-01-02T15:04", item.StartAt, time.Local)
				endTime, _ := time.ParseInLocation("2006-01-02T15:04", item.EndAt, time.Local)
				patient := plan.PatientNumber
				occupied = append(occupied, timeSlot{start: startTime, end: endTime, patient: &patient, procedureID: item.ProcedureID, sourcePlanDate: planDateKey})
				result.Appointments = append(result.Appointments, item)
			}
		}
	}
	result.ScheduledCount = len(result.Appointments)
	for _, plan := range plans {
		for _, id := range plan.ProcedureIDs {
			state := states[plan.PatientNumber][id]
			if missing := state.requested - state.completed; missing > 0 {
				result.UnscheduledCount += missing
				result.UnscheduledPatients = append(result.UnscheduledPatients, plan.PatientNumber)
			}
		}
	}
	result.UnscheduledPatients = uniqueInts(result.UnscheduledPatients)
	return result, nil
}

func courseWarnings(plans []patientPlan, procedures map[string]procedure) []string {
	warnings := []string{}
	for _, plan := range plans {
		for _, id := range plan.ProcedureIDs {
			p := procedures[id]
			if p.MinIntervalDays > plan.StayDays {
				warnings = append(warnings, fmt.Sprintf("Пациент %d: интервал процедуры «%s» (%d дн.) превышает срок пребывания (%d дн.)", plan.PatientNumber, p.Name, p.MinIntervalDays, plan.StayDays))
			}
			if count := plan.ProcedureCounts[id]; count > 0 && (count-1)*p.MinIntervalDays >= plan.StayDays {
				warnings = append(warnings, fmt.Sprintf("Пациент %d: %d повторений процедуры «%s» не помещаются в %d дн. при интервале %d дн.", plan.PatientNumber, count, p.Name, plan.StayDays, p.MinIntervalDays))
			}
		}
	}
	return warnings
}
