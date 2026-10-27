package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

// Course constraints are scoped to an admission (source_plan_date), because
// patient numbers can be reused on a different admission.
type courseProcedureRule struct {
	name         string
	minDays      int
	weeklyLimit  int
	prerequisite string
}

type courseSession struct {
	id          string
	procedureID string
	start       time.Time
	end         time.Time
}

type courseConstraintQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadCourseRules(ctx context.Context, db courseConstraintQuerier, courseDate string, patient int) (map[string]courseProcedureRule, error) {
	rows, err := db.Query(ctx, `SELECT i.procedure_id,p.name,p.min_interval_days,p.default_weekly_count,
		CASE WHEN d.severity='high' THEN COALESCE(i.prerequisite_procedure_id,'') ELSE '' END
		FROM day_plan_items i
		JOIN day_plan_patients d ON d.schedule_date=i.schedule_date AND d.patient_number=i.patient_number
		JOIN procedures p ON p.id=i.procedure_id
		WHERE i.schedule_date=$1 AND i.patient_number=$2`, courseDate, patient)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := map[string]courseProcedureRule{}
	for rows.Next() {
		var id string
		var rule courseProcedureRule
		if err := rows.Scan(&id, &rule.name, &rule.minDays, &rule.weeklyLimit, &rule.prerequisite); err != nil {
			return nil, err
		}
		rules[id] = rule
	}
	return rules, rows.Err()
}

// The candidate replaces one existing appointment, or is nil when cancelling.
// All rules are checked against the complete post-edit admission schedule.
func validateProposedCourseSessions(ctx context.Context, tx courseConstraintQuerier, courseDate string, patient int, excludedID string, candidate *courseSession) error {
	rules, err := loadCourseRules(ctx, tx, courseDate, patient)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id,COALESCE(source_procedure_id,''),start_at,end_at
		FROM appointments WHERE source_plan_date=$1 AND patient_number=$2
		AND status='scheduled' AND id<>$3`, courseDate, patient, excludedID)
	if err != nil {
		return err
	}
	sessions := []courseSession{}
	for rows.Next() {
		var session courseSession
		if err := rows.Scan(&session.id, &session.procedureID, &session.start, &session.end); err != nil {
			rows.Close()
			return err
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if candidate != nil {
		sessions = append(sessions, *candidate)
	}
	return validateCourseSessions(patient, rules, sessions)
}

// Validate the whole course, including protected and manually moved sessions.
// One relaxing session can satisfy exactly one prerequisite target.
func validateCourseSessions(patient int, rules map[string]courseProcedureRule, sessions []courseSession) error {
	byProcedure := map[string][]courseSession{}
	for _, session := range sessions {
		if _, exists := rules[session.procedureID]; exists {
			byProcedure[session.procedureID] = append(byProcedure[session.procedureID], session)
		}
	}
	for id, occurrences := range byProcedure {
		rule := rules[id]
		sort.Slice(occurrences, func(i, j int) bool {
			if occurrences[i].start.Equal(occurrences[j].start) {
				return occurrences[i].id < occurrences[j].id
			}
			return occurrences[i].start.Before(occurrences[j].start)
		})
		limit := rule.weeklyLimit
		if limit <= 0 {
			limit = 7
		}
		for i, session := range occurrences {
			if i > 0 && calendarDayNumber(session.start)-calendarDayNumber(occurrences[i-1].start) < int64(rule.minDays) {
				return fmt.Errorf("patient %d: procedure %q requires %d full days between starts", patient, rule.name, rule.minDays-1)
			}
			count := 0
			for j := i; j >= 0 && calendarDayNumber(session.start)-calendarDayNumber(occurrences[j].start) < 7; j-- {
				count++
			}
			if count > limit {
				return fmt.Errorf("patient %d: procedure %q exceeds %d sessions in seven days", patient, rule.name, limit)
			}
		}
		byProcedure[id] = occurrences
	}
	usedPrerequisites := map[string]bool{}
	// Sorting paired targets ensures stable matching when multiple targets share
	// the same relaxing procedure.
	targets := []courseSession{}
	for id, rule := range rules {
		if rule.prerequisite != "" {
			targets = append(targets, byProcedure[id]...)
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].start.Before(targets[j].start) })
	for _, target := range targets {
		prerequisiteID := rules[target.procedureID].prerequisite
		matched := false
		for _, relaxing := range byProcedure[prerequisiteID] {
			if !usedPrerequisites[relaxing.id] &&
				calendarDayNumber(relaxing.start) == calendarDayNumber(target.start) &&
				!relaxing.end.After(target.start) {
				usedPrerequisites[relaxing.id] = true
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("patient %d: %q requires a separate earlier %q session on the same day", patient, rules[target.procedureID].name, rules[prerequisiteID].name)
		}
	}
	return nil
}

// A proposed day must fit every seven-day window containing it.
func courseWeeklyAllowed(day time.Time, previous []time.Time, limit int) bool {
	if limit <= 0 {
		limit = 7
	}
	dayNumber := calendarDayNumber(day)
	for windowStart := dayNumber - 6; windowStart <= dayNumber; windowStart++ {
		count := 1
		for _, existing := range previous {
			if existingDay := calendarDayNumber(existing); existingDay >= windowStart && existingDay < windowStart+7 {
				count++
			}
		}
		if count > limit {
			return false
		}
	}
	return true
}
