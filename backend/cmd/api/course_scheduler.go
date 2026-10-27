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

func validateCoursePrerequisites(plan patientPlan, procedures map[string]procedure) error {
	if plan.Severity != "high" || len(plan.ProcedurePrerequisites) == 0 {
		return nil
	}
	selected := map[string]bool{}
	for _, id := range plan.ProcedureIDs {
		selected[id] = true
	}
	required := map[string]int{}
	for targetID, prerequisiteID := range plan.ProcedurePrerequisites {
		prerequisite, exists := procedures[prerequisiteID]
		target, targetExists := procedures[targetID]
		if !selected[targetID] || !selected[prerequisiteID] || !exists || !targetExists || !prerequisite.IsRelaxing || target.IsRelaxing || targetID == prerequisiteID {
			return fmt.Errorf("patient %d has an invalid relaxing prerequisite", plan.PatientNumber)
		}
		required[prerequisiteID] += plan.ProcedureCounts[targetID]
	}
	for prerequisiteID, count := range required {
		if plan.ProcedureCounts[prerequisiteID] < count {
			return fmt.Errorf("patient %d needs at least %d sessions of relaxing procedure %q", plan.PatientNumber, count, procedures[prerequisiteID].Name)
		}
	}
	return nil
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
	if len(plans) > 0 && plans[0].CourseKey != "" {
		planDateKey = plans[0].CourseKey
	}
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
		if err := validateCoursePrerequisites(plan, procedures); err != nil {
			return result, err
		}
		if plan.StartOffset+plan.StayDays > maxDays {
			maxDays = plan.StartOffset + plan.StayDays
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
	requiredPrerequisiteCounts := map[int]map[string]int{}
	for _, plan := range plans {
		if plan.Severity != "high" {
			continue
		}
		for targetID, prerequisiteID := range plan.ProcedurePrerequisites {
			if requiredPrerequisiteCounts[plan.PatientNumber] == nil {
				requiredPrerequisiteCounts[plan.PatientNumber] = map[string]int{}
			}
			requiredPrerequisiteCounts[plan.PatientNumber][prerequisiteID] += plan.ProcedureCounts[targetID]
		}
	}
	for offset := 0; offset < maxDays; offset++ {
		day := start.AddDate(0, 0, offset)
		for _, plan := range plans {
			if offset < plan.StartOffset || offset >= plan.StartOffset+plan.StayDays {
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
				if required := requiredPrerequisiteCounts[plan.PatientNumber][id]; required > state.completed {
					continue
				}
				if state.completed >= state.requested {
					continue
				}
				// Each course procedure may occur only once per calendar day,
				// even if it was already reserved as another task's prerequisite.
				alreadyPending := false
				for _, task := range pending {
					if task.ProcedureID == id {
						alreadyPending = true
						break
					}
				}
				if alreadyPending || !courseWeeklyAllowed(day, state.lastDays, state.procedure.DefaultWeeklyCount) {
					continue
				}
				plannedDay := 0
				if state.requested > 1 {
					plannedDay = plan.StartOffset + (plan.StayDays-1)*state.completed/(state.requested-1)
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
				if prerequisiteID := plan.ProcedurePrerequisites[id]; plan.Severity == "high" && prerequisiteID != "" {
					prerequisiteState := states[plan.PatientNumber][prerequisiteID]
					pendingPrerequisites, pendingTargets := 0, 0
					for _, task := range pending {
						if task.ProcedureID == prerequisiteID {
							pendingPrerequisites++
						}
						if plan.ProcedurePrerequisites[task.ProcedureID] == prerequisiteID {
							pendingTargets++
						}
					}
					// One relaxing session can prepare one complex procedure.
					// Another target sharing it must be assigned on a later day.
					if pendingTargets > 0 {
						continue
					}
					if pendingPrerequisites <= pendingTargets {
						if prerequisiteState.completed+pendingPrerequisites >= prerequisiteState.requested {
							continue
						}
						if !courseWeeklyAllowed(day, prerequisiteState.lastDays, prerequisiteState.procedure.DefaultWeeklyCount) {
							continue
						}
						prerequisiteValid := true
						for _, last := range prerequisiteState.lastDays {
							if delta := calendarDayNumber(day) - calendarDayNumber(last); delta < int64(prerequisiteState.procedure.MinIntervalDays) && delta > -int64(prerequisiteState.procedure.MinIntervalDays) {
								prerequisiteValid = false
								break
							}
						}
						if !prerequisiteValid {
							continue
						}
						pending = append(pending, scheduleTask{PatientNumber: plan.PatientNumber, Severity: plan.Severity, ProcedureID: prerequisiteID})
					}
				}
				pending = append(pending, scheduleTask{PatientNumber: plan.PatientNumber, Severity: plan.Severity, ProcedureID: id})
			}
			if len(pending) == 0 {
				continue
			}
			dailyOccupied := make([]timeSlot, 0)
			for _, slot := range occupied {
				if slot.start.Year() == day.Year() && slot.start.YearDay() == day.YearDay() && (slot.sourcePlanDate == "" || slot.sourcePlanDate == planDateKey || slot.room != "") {
					if slot.sourcePlanDate != "" && slot.sourcePlanDate != planDateKey && slot.patient != nil {
						outsidePatient := 0
						slot.patient = &outsidePatient
					}
					dailyOccupied = append(dailyOccupied, slot)
				}
			}
			daily, err := createSchedule(day, pending, procedures, settings, dailyOccupied)
			if err != nil {
				return result, err
			}
			usedPrerequisiteSessions := map[int]bool{}
			pairedPrerequisiteSessions := map[int]bool{}
			pairedTargetSessions := map[int]bool{}
			pendingTargetsByPrerequisite := map[string]int{}
			pendingPrerequisitesByID := map[string]int{}
			for _, task := range pending {
				if prerequisiteID := plan.ProcedurePrerequisites[task.ProcedureID]; plan.Severity == "high" && prerequisiteID != "" {
					pendingTargetsByPrerequisite[prerequisiteID]++
				}
				if procedures[task.ProcedureID].IsRelaxing {
					pendingPrerequisitesByID[task.ProcedureID]++
				}
			}
			for targetIndex, item := range daily.Appointments {
				prerequisiteID := plan.ProcedurePrerequisites[item.ProcedureID]
				if plan.Severity != "high" || prerequisiteID == "" {
					continue
				}
				for prerequisiteIndex, candidate := range daily.Appointments {
					if !usedPrerequisiteSessions[prerequisiteIndex] && candidate.ProcedureID == prerequisiteID && candidate.StartAt[:10] == item.StartAt[:10] && candidate.EndAt <= item.StartAt {
						usedPrerequisiteSessions[prerequisiteIndex] = true
						pairedPrerequisiteSessions[prerequisiteIndex] = true
						pairedTargetSessions[targetIndex] = true
						break
					}
				}
			}
			unpairedPrerequisiteKept := map[string]int{}
			for index, item := range daily.Appointments {
				if prerequisiteID := plan.ProcedurePrerequisites[item.ProcedureID]; plan.Severity == "high" && prerequisiteID != "" && !pairedTargetSessions[index] {
					continue
				}
				if pendingTargetsByPrerequisite[item.ProcedureID] > 0 && procedures[item.ProcedureID].IsRelaxing && !pairedPrerequisiteSessions[index] {
					surplus := pendingPrerequisitesByID[item.ProcedureID] - pendingTargetsByPrerequisite[item.ProcedureID]
					if unpairedPrerequisiteKept[item.ProcedureID] >= surplus {
						continue
					}
					unpairedPrerequisiteKept[item.ProcedureID]++
				}
				state := states[plan.PatientNumber][item.ProcedureID]
				state.completed++
				state.lastDays = append(state.lastDays, day)
				startTime, _ := time.ParseInLocation("2006-01-02T15:04", item.StartAt, time.Local)
				endTime, _ := time.ParseInLocation("2006-01-02T15:04", item.EndAt, time.Local)
				patient := plan.PatientNumber
				occupied = append(occupied, timeSlot{start: startTime, end: endTime, patient: &patient, procedureID: item.ProcedureID, sourcePlanDate: planDateKey, room: item.Room})
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
			if plan.ProcedureCounts[id] > 1 && p.MinIntervalDays > plan.StayDays {
				warnings = append(warnings, fmt.Sprintf("Пациент %d: перерыв процедуры «%s» (%d дн.) превышает период лечения (%d дн.)", plan.PatientNumber, p.Name, p.MinIntervalDays-1, plan.StayDays))
			}
			if count := plan.ProcedureCounts[id]; count > 0 && (count-1)*p.MinIntervalDays >= plan.StayDays {
				warnings = append(warnings, fmt.Sprintf("Пациент %d: %d повторений процедуры «%s» не помещаются в период лечения (%d дн.) при перерыве %d дн.", plan.PatientNumber, count, p.Name, plan.StayDays, p.MinIntervalDays-1))
			}
		}
	}
	return warnings
}
