package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

type patientAbsence struct {
	Date          string `json:"date"`
	PatientNumber int    `json:"patientNumber"`
	From          string `json:"from"`
	To            string `json:"to"`
	Reason        string `json:"reason"`
	Preview       bool   `json:"preview,omitempty"`
}

type absenceResult struct {
	patientAbsence
	AffectedCount    int           `json:"affectedCount"`
	RescheduledCount int           `json:"rescheduledCount"`
	ScheduledCount   int           `json:"scheduledCount"`
	UnscheduledCount int           `json:"unscheduledCount"`
	UnavailableCount int           `json:"unavailableCount"`
	Appointments     []appointment `json:"appointments"`
	Warnings         []string      `json:"warnings"`
}

type storedTreatment struct {
	id          string
	start       time.Time
	end         time.Time
	procedureID string
	manual      bool
	generated   bool
	patient     *int
	planDate    string
	room        string
}

func parseISODate(value string) (time.Time, bool) {
	day, err := time.Parse("2006-01-02", value)
	return day, err == nil && day.Format("2006-01-02") == value
}

type absenceQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// loadAbsenceDays excludes unavailable calendar days from ordinary regeneration.
// Absences belong to a particular admission; temporary patient numbers can be reused.
func loadAbsenceDays(ctx context.Context, db absenceQuerier, planDate string) (map[int]map[string]bool, error) {
	rows, err := db.Query(ctx, "SELECT patient_number,from_date,to_date FROM patient_absences WHERE schedule_date=$1", planDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	absences := map[int]map[string]bool{}
	for rows.Next() {
		var patient int
		var from, to time.Time
		if err := rows.Scan(&patient, &from, &to); err != nil {
			return nil, err
		}
		if absences[patient] == nil {
			absences[patient] = map[string]bool{}
		}
		for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
			absences[patient][day.Format("2006-01-02")] = true
		}
	}
	return absences, rows.Err()
}

func (s *server) listPatientAbsences(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if _, ok := parseISODate(date); !ok {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	patient := 0
	if value := r.URL.Query().Get("patientNumber"); value != "" {
		if _, err := fmt.Sscanf(value, "%d", &patient); err != nil || patient < 1 || fmt.Sprintf("%d", patient) != value {
			writeError(w, http.StatusBadRequest, "invalid patientNumber")
			return
		}
	}
	rows, err := s.db.Query(r.Context(), `SELECT patient_number,to_char(from_date,'YYYY-MM-DD'),to_char(to_date,'YYYY-MM-DD'),reason
		FROM patient_absences WHERE schedule_date=$1 AND ($2::int=0 OR patient_number=$2)
		ORDER BY patient_number,from_date,to_date`, date, patient)
	if err != nil {
		writeError(w, 500, "could not load patient absences")
		return
	}
	defer rows.Close()
	items := []patientAbsence{}
	for rows.Next() {
		item := patientAbsence{Date: date}
		if err := rows.Scan(&item.PatientNumber, &item.From, &item.To, &item.Reason); err != nil {
			writeError(w, 500, "could not load patient absences")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not load patient absences")
		return
	}
	writeJSON(w, 200, items)
}

func (s *server) savePatientAbsence(w http.ResponseWriter, r *http.Request) {
	var input patientAbsence
	if !decode(w, r, &input) {
		return
	}
	admitted, validDate := parseISODate(input.Date)
	from, validFrom := parseISODate(input.From)
	to, validTo := parseISODate(input.To)
	if !validDate || !validFrom || !validTo || input.PatientNumber < 1 || to.Before(from) || to.Sub(from) > 365*24*time.Hour {
		writeError(w, 400, "invalid absence period")
		return
	}
	if input.Reason == "" {
		input.Reason = "illness"
	}
	if input.Reason != "illness" && input.Reason != "other" {
		writeError(w, 400, "reason must be illness or other")
		return
	}
	if to.Format("2006-01-02") < time.Now().In(time.Local).Format("2006-01-02") {
		writeError(w, 422, "absence period is entirely in the past")
		return
	}
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		writeError(w, 500, "could not start absence transaction")
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('clinic-schedule-room-appointments'))"); err != nil {
		writeError(w, 500, "could not lock room occupancy")
		return
	}
	// Use the same plan lock as full regeneration and editing of patient plans.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", input.Date); err != nil {
		writeError(w, 500, "could not lock patient plan")
		return
	}
	var stayDays int
	err = tx.QueryRow(ctx, "SELECT stay_days FROM day_plan_patients WHERE schedule_date=$1 AND patient_number=$2", input.Date, input.PatientNumber).Scan(&stayDays)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "patient is not in this treatment plan")
		return
	}
	if err != nil {
		writeError(w, 500, "could not load treatment period")
		return
	}
	discharge := admitted.AddDate(0, 0, stayDays)
	if calendarDayNumber(from) < calendarDayNumber(admitted) || calendarDayNumber(to) >= calendarDayNumber(discharge) {
		writeError(w, 422, "absence must be within the patient's treatment period")
		return
	}
	result := absenceResult{patientAbsence: input, Appointments: []appointment{}, Warnings: []string{}}
	var existingReason string
	err = tx.QueryRow(ctx, `SELECT reason FROM patient_absences WHERE schedule_date=$1 AND patient_number=$2 AND from_date=$3 AND to_date=$4`, input.Date, input.PatientNumber, from, to).Scan(&existingReason)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "could not check existing absence")
		return
	}
	if err == nil {
		result.Reason = existingReason
		result.Warnings = append(result.Warnings, "Период отсутствия уже сохранён; повторных переносов нет.")
		writeJSON(w, 200, result)
		return
	}
	dayAbsences, err := loadAbsenceDays(ctx, tx, input.Date)
	if err != nil {
		writeError(w, 500, "could not load patient absences")
		return
	}
	if dayAbsences[input.PatientNumber] == nil {
		dayAbsences[input.PatientNumber] = map[string]bool{}
	}
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		dayAbsences[input.PatientNumber][day.Format("2006-01-02")] = true
	}

	// Read all scheduled sessions of this admission before modifying anything.
	// Locking them also makes optimistic manual moves conflict with this transaction.
	rows, err := tx.Query(ctx, `SELECT id,start_at,end_at,COALESCE(source_procedure_id,''),manually_modified,generated_by_planner,room
		FROM appointments WHERE source_plan_date=$1 AND patient_number=$2 AND status='scheduled'
		ORDER BY start_at,id FOR UPDATE`, input.Date, input.PatientNumber)
	if err != nil {
		writeError(w, 500, "could not load patient appointments")
		return
	}
	treatments := []storedTreatment{}
	for rows.Next() {
		var item storedTreatment
		if err := rows.Scan(&item.id, &item.start, &item.end, &item.procedureID, &item.manual, &item.generated, &item.room); err != nil {
			rows.Close()
			writeError(w, 500, "could not load patient appointments")
			return
		}
		treatments = append(treatments, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, 500, "could not load patient appointments")
		return
	}
	rows.Close()

	now := time.Now().In(time.Local)
	dayToday := now.Format("2006-01-02")
	move := []storedTreatment{}
	for _, item := range treatments {
		startDay := item.start.Format("2006-01-02")
		localStart := time.Date(item.start.Year(), item.start.Month(), item.start.Day(), item.start.Hour(), item.start.Minute(), 0, 0, time.Local)
		if !dayAbsences[input.PatientNumber][startDay] || localStart.Before(now) {
			continue
		}
		if item.manual || !item.generated {
			writeError(w, http.StatusConflict, "manually edited session falls within the absence; move or cancel it explicitly first")
			return
		}
		move = append(move, item)
	}
	result.AffectedCount = len(move)

	rows, err = tx.Query(ctx, "SELECT id,name,duration_minutes,preparation_minutes,is_relaxing,min_interval_days,default_weekly_count,room FROM procedures")
	if err != nil {
		writeError(w, 500, "could not load procedures")
		return
	}
	procedureMap := map[string]procedure{}
	for rows.Next() {
		var p procedure
		if err := rows.Scan(&p.ID, &p.Name, &p.DurationMinutes, &p.PreparationMinutes, &p.IsRelaxing, &p.MinIntervalDays, &p.DefaultWeeklyCount, &p.Room); err != nil {
			rows.Close()
			writeError(w, 500, "could not load procedures")
			return
		}
		procedureMap[p.ID] = p
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, 500, "could not load procedures")
		return
	}
	rows.Close()
	workHours, err := loadSettings(ctx, tx)
	if err != nil {
		writeError(w, 500, "could not load organization hours")
		return
	}

	// Extra appointments without a patient number block everybody; manually
	// created appointments for this patient must also remain as they are.
	occupied := []timeSlot{}
	rows, err = tx.Query(ctx, `SELECT start_at,end_at,patient_number,COALESCE(source_procedure_id,''),COALESCE(to_char(source_plan_date,'YYYY-MM-DD'),''),room
		FROM appointments WHERE status='scheduled' AND start_at >= $1 AND start_at < $2
		AND (patient_number IS NULL OR (patient_number=$3 AND (source_plan_date=$4 OR source_plan_date IS NULL)) OR room<>'')`, admitted, discharge, input.PatientNumber, input.Date)
	if err != nil {
		writeError(w, 500, "could not load occupied slots")
		return
	}
	for rows.Next() {
		var slot timeSlot
		if err := rows.Scan(&slot.start, &slot.end, &slot.patient, &slot.procedureID, &slot.sourcePlanDate, &slot.room); err != nil {
			rows.Close()
			writeError(w, 500, "could not read occupied slots")
			return
		}
		slot.start = time.Date(slot.start.Year(), slot.start.Month(), slot.start.Day(), slot.start.Hour(), slot.start.Minute(), 0, 0, time.Local)
		slot.end = time.Date(slot.end.Year(), slot.end.Month(), slot.end.Day(), slot.end.Hour(), slot.end.Minute(), 0, 0, time.Local)
		if slot.patient != nil && *slot.patient == input.PatientNumber && slot.sourcePlanDate != "" && slot.sourcePlanDate != input.Date {
			// Another admission can use the same temporary patient number;
			// keep only its room reservation, not a patient-level block.
			other := 0
			slot.patient = &other
		}
		occupied = append(occupied, slot)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, 500, "could not read occupied slots")
		return
	}
	rows.Close()
	// Remove only the sessions we're relocating from the in-memory occupancy.
	for _, removed := range move {
		for i := 0; i < len(occupied); i++ {
			if occupied[i].start.Equal(removed.start) && occupied[i].end.Equal(removed.end) && occupied[i].patient != nil && *occupied[i].patient == input.PatientNumber && occupied[i].sourcePlanDate == input.Date && occupied[i].procedureID == removed.procedureID {
				occupied = append(occupied[:i], occupied[i+1:]...)
				break
			}
		}
	}

	// The assigned procedures are ordered by their original session date.
	// Prioritize a relaxing treatment when multiple sessions must move together.
	sort.SliceStable(move, func(i, j int) bool {
		if !move[i].start.Equal(move[j].start) {
			return move[i].start.Before(move[j].start)
		}
		return procedureMap[move[i].procedureID].IsRelaxing && !procedureMap[move[j].procedureID].IsRelaxing
	})
	for _, previous := range move {
		p, ok := procedureMap[previous.procedureID]
		if !ok {
			result.UnscheduledCount++
			result.Warnings = append(result.Warnings, fmt.Sprintf("Сеанс %s не связан с существующей процедурой.", previous.id))
			continue
		}
		// A previously booked room is a snapshot, even if the catalog has
		// since been changed. Keep that resource on rescheduled sessions.
		p.Room = previous.room
		procedureForMove := map[string]procedure{p.ID: p}
		first := previous.start
		if previous.start.Format("2006-01-02") < dayToday {
			first = now
		}
		placed := false
		for day := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, time.Local); calendarDayNumber(day) < calendarDayNumber(discharge); day = day.AddDate(0, 0, 1) {
			if dayAbsences[input.PatientNumber][day.Format("2006-01-02")] {
				continue
			}
			validInterval := true
			for _, slot := range occupied {
				if slot.patient != nil && *slot.patient == input.PatientNumber && slot.sourcePlanDate == input.Date && slot.procedureID == previous.procedureID {
					delta := calendarDayNumber(day) - calendarDayNumber(slot.start)
					if delta < int64(p.MinIntervalDays) && delta > -int64(p.MinIntervalDays) {
						validInterval = false
						break
					}
				}
			}
			if !validInterval {
				continue
			}
			dayOccupied := []timeSlot{}
			if day.Format("2006-01-02") == dayToday {
				// Prevent a future session being moved into already elapsed hours.
				dayOccupied = append(dayOccupied, timeSlot{start: day, end: now})
			}
			for _, slot := range occupied {
				if slot.start.Format("2006-01-02") == day.Format("2006-01-02") {
					dayOccupied = append(dayOccupied, slot)
				}
			}
			daily, err := createSchedule(day, []scheduleTask{{PatientNumber: input.PatientNumber, Severity: "high", ProcedureID: p.ID}}, procedureForMove, workHours, dayOccupied)
			if err != nil {
				writeError(w, 500, "could not calculate rescheduling")
				return
			}
			if len(daily.Appointments) == 0 {
				continue
			}
			item := daily.Appointments[0]
			// Retain the original appointment identity so calendar clients can update it.
			item.ID = previous.id
			item.SourcePlanDate = input.Date
			start, _ := time.ParseInLocation("2006-01-02T15:04", item.StartAt, time.Local)
			end, _ := time.ParseInLocation("2006-01-02T15:04", item.EndAt, time.Local)
			patient := input.PatientNumber
			occupied = append(occupied, timeSlot{start: start, end: end, patient: &patient, procedureID: p.ID, sourcePlanDate: input.Date, room: p.Room})
			result.Appointments = append(result.Appointments, item)
			placed = true
			break
		}
		if !placed {
			result.UnscheduledCount++
			result.Warnings = append(result.Warnings, fmt.Sprintf("Не удалось перенести процедуру «%s» до окончания курса.", p.Name))
		}
	}
	result.RescheduledCount = len(result.Appointments)
	result.ScheduledCount = result.RescheduledCount
	result.UnavailableCount = result.UnscheduledCount
	if result.UnscheduledCount > 0 && !input.Preview {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":            "some procedures cannot be rescheduled safely; nothing was changed",
			"affectedCount":    result.AffectedCount,
			"rescheduledCount": result.RescheduledCount,
			"scheduledCount":   result.ScheduledCount,
			"unscheduledCount": result.UnscheduledCount,
			"unavailableCount": result.UnavailableCount,
			"appointments":     result.Appointments,
			"warnings":         result.Warnings,
		})
		return
	}
	if input.Preview {
		writeJSON(w, 200, result)
		return
	}
	for _, item := range result.Appointments {
		command, err := tx.Exec(ctx, `UPDATE appointments SET start_at=$1,end_at=$2,title=$3,description=$4,color=$5,updated_at=now()
			WHERE id=$6 AND source_plan_date=$7 AND patient_number=$8 AND generated_by_planner AND NOT manually_modified AND status='scheduled'`,
			item.StartAt, item.EndAt, item.Title, item.Description, item.Color, item.ID, input.Date, input.PatientNumber)
		if err != nil || command.RowsAffected() != 1 {
			writeError(w, 409, "session changed concurrently; retry rescheduling")
			return
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO patient_absences(schedule_date,patient_number,from_date,to_date,reason)
		VALUES ($1,$2,$3,$4,$5)`, input.Date, input.PatientNumber, from, to, input.Reason); err != nil {
		writeError(w, 409, "absence changed concurrently; retry rescheduling")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, 409, "schedule changed concurrently; retry rescheduling")
		return
	}
	writeJSON(w, 200, result)
}
