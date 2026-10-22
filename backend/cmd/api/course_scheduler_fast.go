package main

import (
	"fmt"
	"sort"
	"time"
)

// createCourseScheduleFast is a deterministic, bounded-time alternative to
// CP-SAT. It chooses calendar days by treatment-load balance and session
// urgency, and uses the existing patient slot packer to check real capacity.
// It does not invent doctor, room or equipment constraints: those resources
// are not represented in the current application data model.
type fastCourseDay struct {
	offset    int
	day       time.Time
	start     time.Time
	end       time.Time
	blocks    []timeSlot
	occupied  []timeSlot
	available bool
}

type fastCourseProcedure struct {
	id        string
	count     int
	procedure procedure
}

func createCourseScheduleFast(start time.Time, plans []patientPlan, procedures map[string]procedure, config settings, occupied []timeSlot, absences map[int]map[string]bool) (scheduleResponse, error) {
	result := scheduleResponse{Appointments: []appointment{}, UnscheduledPatients: []int{}}
	planKey := start.Format("2006-01-02")
	step := config.TimeStepMinutes
	if step <= 0 {
		step = 10
	}
	ordered := append([]patientPlan(nil), plans...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if severityRank(ordered[i].Severity) != severityRank(ordered[j].Severity) {
			return severityRank(ordered[i].Severity) < severityRank(ordered[j].Severity)
		}
		return ordered[i].PatientNumber < ordered[j].PatientNumber
	})

	// Calendar constraints do not depend on patient number. Construct them
	// once, then add the patient-specific absence and occupied intervals below.
	maxDays := 0
	for _, plan := range ordered {
		if plan.StayDays < 1 || plan.StayDays > 365 {
			return result, errInvalidTask
		}
		if plan.StayDays > maxDays {
			maxDays = plan.StayDays
		}
	}
	baseDays := make([]fastCourseDay, maxDays)
	for offset := range baseDays {
		day := start.AddDate(0, 0, offset)
		work := config.dayAt(day)
		item := fastCourseDay{offset: offset, day: day, available: work.Open}
		if work.Open {
			begin, _ := minutesFromClock(work.Start)
			finish, _ := minutesFromClock(work.End)
			midnight := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
			item.start = midnight.Add(time.Duration(begin) * time.Minute)
			item.end = midnight.Add(time.Duration(finish) * time.Minute)
			for _, b := range work.Breaks {
				from, _ := minutesFromClock(b.Start)
				to, _ := minutesFromClock(b.End)
				item.blocks = append(item.blocks, timeSlot{start: midnight.Add(time.Duration(from) * time.Minute), end: midnight.Add(time.Duration(to) * time.Minute)})
			}
		}
		baseDays[offset] = item
	}

	for _, plan := range ordered {
		patient := plan.PatientNumber
		fixed := map[string][]int{}
		days := make([]fastCourseDay, plan.StayDays)
		copy(days, baseDays[:plan.StayDays])
		for offset := range days {
			item := &days[offset]
			if absences[patient][item.day.Format("2006-01-02")] {
				item.available = false
			}
		}
		for _, slot := range occupied {
			// Patient numbers are local to each admission; a different
			// admission's number must not block this patient's calendar.
			if slot.patient != nil && *slot.patient != patient {
				continue
			}
			if slot.patient != nil && slot.sourcePlanDate != "" && slot.sourcePlanDate != planKey {
				continue
			}
			index := int(calendarDayNumber(slot.start) - calendarDayNumber(start))
			if index >= 0 && index < len(days) {
				days[index].occupied = append(days[index].occupied, slot)
			}
			if slot.patient != nil && slot.procedureID != "" {
				fixed[slot.procedureID] = append(fixed[slot.procedureID], index)
			}
		}
		freeDays := make([]int, 0, len(days))
		for offset, day := range days {
			if day.available {
				freeDays = append(freeDays, offset)
			}
		}
		tasks := make(map[int][]scheduleTask)
		used := make(map[int]int)
		placed := make(map[string][]int)
		assigned := make([]fastCourseProcedure, 0, len(plan.ProcedureIDs))
		for _, id := range plan.ProcedureIDs {
			p, ok := procedures[id]
			count := plan.ProcedureCounts[id]
			if !ok || p.MinIntervalDays < 1 || count < 1 || count > 365 || p.DurationMinutes+p.PreparationMinutes < 1 {
				return result, errInvalidTask
			}
			assigned = append(assigned, fastCourseProcedure{id: id, count: count, procedure: p})
		}
		// The least flexible procedures go first, so optional procedures
		// cannot consume the few possible dates of a tightly constrained one.
		sort.SliceStable(assigned, func(i, j int) bool {
			a, b := assigned[i], assigned[j]
			if a.count*a.procedure.MinIntervalDays != b.count*b.procedure.MinIntervalDays {
				return a.count*a.procedure.MinIntervalDays > b.count*b.procedure.MinIntervalDays
			}
			if a.procedure.DefaultWeeklyCount != b.procedure.DefaultWeeklyCount {
				return a.procedure.DefaultWeeklyCount < b.procedure.DefaultWeeklyCount
			}
			return a.count > b.count
		})

		for _, item := range assigned {
			weeklyLimit := item.procedure.DefaultWeeklyCount
			if weeklyLimit <= 0 {
				weeklyLimit = 7
			}
			for repeat := 0; repeat < item.count; repeat++ {
				remaining := item.count - repeat - 1
				last := -item.procedure.MinIntervalDays
				if n := len(placed[item.id]); n > 0 {
					last = placed[item.id][n-1]
				}
				targetIndex := 0
				if len(freeDays) > 0 {
					targetIndex = ((repeat + 1) * (len(freeDays) + 1) / (item.count + 1)) - 1
					if targetIndex < 0 {
						targetIndex = 0
					}
					if targetIndex >= len(freeDays) {
						targetIndex = len(freeDays) - 1
					}
				}
				targetDay := 0
				if len(freeDays) > 0 {
					targetDay = freeDays[targetIndex]
				}
				type candidate struct {
					offset int
					score  int
					future bool
				}
				options := make([]candidate, 0, len(freeDays))
				for _, index := range freeDays {
					if index < last+item.procedure.MinIntervalDays {
						continue
					}
					ok := true
					for _, f := range fixed[item.id] {
						if absInt(index-f) < item.procedure.MinIntervalDays {
							ok = false
							break
						}
					}
					if !ok || !fastWeeklyAllowed(index, append(fixed[item.id], placed[item.id]...), weeklyLimit) {
						continue
					}
					future := fastCanFitRemaining(index, remaining, freeDays, fixed[item.id],
						placed[item.id], item.procedure.MinIntervalDays, weeklyLimit)
					// Strongly prioritize spreading the number of treatments by
					// day; deviations from the ideal repeat date are softer.
					score := used[index]*10000 + absInt(index-targetDay)*200 + index
					options = append(options, candidate{offset: index, score: score, future: future})
				}
				sort.Slice(options, func(i, j int) bool {
					if options[i].future != options[j].future {
						return options[i].future
					}
					if options[i].score != options[j].score {
						return options[i].score < options[j].score
					}
					return options[i].offset < options[j].offset
				})
				for _, option := range options {
					index := option.offset
					candidateTasks := append(append([]scheduleTask(nil), tasks[index]...), scheduleTask{PatientNumber: patient, ProcedureID: item.id, Severity: plan.Severity})
					orderedTasks := fastOrderedTasks(candidateTasks, procedures, plan.Severity)
					candidateSlots := make([]scheduledTask, 0, len(orderedTasks))
					for _, task := range orderedTasks {
						candidateSlots = append(candidateSlots, scheduledTask{patient: patient, severity: plan.Severity, procedure: procedures[task.ProcedureID]})
					}
					day := days[index]
					if len(findPatientSlots(day.start, day.end, day.blocks, candidateSlots, day.occupied, step, patient)) != len(candidateSlots) {
						continue
					}
					tasks[index] = candidateTasks
					placed[item.id] = append(placed[item.id], index)
					used[index]++
					break
				}
				// When a course is impossible, report the unplaced suffix rather
				// than ignoring the limit or forging an appointment.
				if len(placed[item.id]) != repeat+1 {
					break
				}
			}
		}
		for _, offset := range freeDays {
			if len(tasks[offset]) == 0 {
				continue
			}
			daily, err := createSchedule(days[offset].day, tasks[offset], procedures, config, days[offset].occupied)
			if err != nil {
				return result, err
			}
			if daily.UnscheduledCount > 0 {
				return result, fmt.Errorf("fast scheduler could not fit the verified calendar day")
			}
			result.Appointments = append(result.Appointments, daily.Appointments...)
		}
		for _, item := range assigned {
			if missing := item.count - len(placed[item.id]); missing > 0 {
				result.UnscheduledCount += missing
				result.UnscheduledPatients = append(result.UnscheduledPatients, patient)
			}
		}
	}
	result.ScheduledCount = len(result.Appointments)
	result.UnscheduledPatients = uniqueInts(result.UnscheduledPatients)
	sort.Slice(result.Appointments, func(i, j int) bool {
		if result.Appointments[i].StartAt != result.Appointments[j].StartAt {
			return result.Appointments[i].StartAt < result.Appointments[j].StartAt
		}
		return result.Appointments[i].Title < result.Appointments[j].Title
	})
	return result, nil
}

func fastOrderedTasks(tasks []scheduleTask, procedures map[string]procedure, severity string) []scheduleTask {
	copyTasks := append([]scheduleTask(nil), tasks...)
	if severity == "high" {
		sort.SliceStable(copyTasks, func(i, j int) bool {
			return procedures[copyTasks[i].ProcedureID].IsRelaxing && !procedures[copyTasks[j].ProcedureID].IsRelaxing
		})
	}
	return copyTasks
}

// Every rolling seven-day interval that contains the proposed session must
// remain within the catalog limit, including manually preserved sessions.
func fastWeeklyAllowed(day int, previous []int, limit int) bool {
	if limit <= 0 {
		return true
	}
	for start := day - 6; start <= day; start++ {
		count := 1
		for _, existing := range previous {
			if existing >= start && existing < start+7 {
				count++
			}
		}
		if count > limit {
			return false
		}
	}
	return true
}

// Preserve room for the remaining occurrences. Checking only the last possible
// date is insufficient: weekends can leave too few treatment days, which
// strands a course after five placements despite room for seven.
func fastCanFitRemaining(index, remaining int, freeDays, fixed, placed []int, interval, weeklyLimit int) bool {
	if remaining == 0 {
		return true
	}
	used := make([]int, 0, len(fixed)+len(placed)+remaining+1)
	used = append(used, fixed...)
	used = append(used, placed...)
	used = append(used, index)
	last := index
	for _, candidate := range freeDays {
		if candidate < last+interval {
			continue
		}
		valid := true
		for _, existing := range fixed {
			if absInt(candidate-existing) < interval {
				valid = false
				break
			}
		}
		if !valid || !fastWeeklyAllowed(candidate, used, weeklyLimit) {
			continue
		}
		used = append(used, candidate)
		last = candidate
		remaining--
		if remaining == 0 {
			return true
		}
	}
	return false
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
