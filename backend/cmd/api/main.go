package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type appointment struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	StartAt            string `json:"startAt"`
	EndAt              string `json:"endAt"`
	Description        string `json:"description"`
	Color              string `json:"color"`
	Status             string `json:"status,omitempty"`
	GeneratedByPlanner bool   `json:"generatedByPlanner,omitempty"`
	ManuallyModified   bool   `json:"manuallyModified,omitempty"`
	PatientNumber      *int   `json:"patientNumber,omitempty"`
	ProcedureID        string `json:"procedureId,omitempty"`
	SourcePlanDate     string `json:"sourcePlanDate,omitempty"`
	Room               string `json:"room,omitempty"`
}

type procedure struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	DurationMinutes       int    `json:"durationMinutes"`
	PreparationMinutes    int    `json:"preparationMinutes"`
	IsRelaxing            bool   `json:"isRelaxing"`
	MinIntervalDays       int    `json:"minIntervalDays"`
	DefaultWeeklyCount    int    `json:"defaultWeeklyCount"`
	DefaultSessionCount   int    `json:"defaultSessionCount,omitempty"`
	DefaultPrerequisiteID string `json:"defaultPrerequisiteId"`
	Room                  string `json:"room,omitempty"`
}

type settings struct {
	TimeStepMinutes  int          `json:"timeStepMinutes"`
	DisplayStartHour int          `json:"displayStartHour"`
	DisplayEndHour   int          `json:"displayEndHour"`
	WorkWeek         []workingDay `json:"workWeek"`
}

type scheduleTask struct {
	PatientNumber int    `json:"patientNumber"`
	Severity      string `json:"severity"`
	ProcedureID   string `json:"procedureId"`
}

type patientPlan struct {
	PatientNumber                  int                `json:"patientNumber"`
	Severity                       string             `json:"severity"`
	ProcedureIDs                   []string           `json:"procedureIds"`
	ArrivalDate                    string             `json:"arrivalDate,omitempty"`
	DepartureDate                  string             `json:"departureDate,omitempty"`
	StayDays                       int                `json:"stayDays"`
	StartOffset                    int                `json:"startOffset,omitempty"`
	CourseKey                      string             `json:"-"`
	ProcedureCounts                map[string]int     `json:"procedureCounts"`
	ProcedurePrerequisites         map[string]string  `json:"procedurePrerequisites,omitempty"`
	ProcedurePrerequisiteOverrides map[string]*string `json:"procedurePrerequisiteOverrides,omitempty"`
}

type dayPlan struct {
	Date                  string        `json:"date"`
	Revision              int           `json:"revision"`
	SelectedPatientNumber *int          `json:"selectedPatientNumber"`
	Plans                 []patientPlan `json:"plans"`
}

type scheduleRequest struct {
	Date    string `json:"date"`
	Preview bool   `json:"preview,omitempty"`
}

type scheduleResponse struct {
	Appointments        []appointment          `json:"appointments"`
	ScheduledCount      int                    `json:"scheduledCount"`
	UnscheduledCount    int                    `json:"unscheduledCount"`
	UnscheduledPatients []int                  `json:"unscheduledPatientNumbers"`
	UnscheduledDetails  []unscheduledProcedure `json:"unscheduledDetails"`
	Warnings            []string               `json:"warnings"`
}

type unscheduledProcedure struct {
	PatientNumber  int    `json:"patientNumber"`
	ProcedureID    string `json:"procedureId"`
	ProcedureName  string `json:"procedureName"`
	Requested      int    `json:"requestedCount"`
	Scheduled      int    `json:"scheduledCount"`
	Missing        int    `json:"missingCount"`
	ReasonCode     string `json:"reasonCode"`
	Reason         string `json:"reason"`
	Recommendation string `json:"recommendation"`
}

func normalizePatientTreatmentDates(courseKey string, patient *patientPlan) error {
	courseDay, validCourseKey := parseISODate(courseKey)
	if !validCourseKey {
		return fmt.Errorf("invalid course date")
	}
	if patient.ArrivalDate == "" && patient.DepartureDate == "" {
		if patient.StayDays < 1 || patient.StayDays > 365 {
			return fmt.Errorf("patient %d needs a valid arrival/departure range", patient.PatientNumber)
		}
		patient.ArrivalDate = courseKey
		patient.DepartureDate = courseDay.AddDate(0, 0, patient.StayDays-1).Format("2006-01-02")
		return nil
	}
	if patient.ArrivalDate == "" || patient.DepartureDate == "" {
		return fmt.Errorf("patient %d needs both arrivalDate and departureDate", patient.PatientNumber)
	}
	arrival, validArrival := parseISODate(patient.ArrivalDate)
	departure, validDeparture := parseISODate(patient.DepartureDate)
	if !validArrival || !validDeparture {
		return fmt.Errorf("patient %d has an invalid treatment date", patient.PatientNumber)
	}
	if arrival.Before(courseDay) || !departure.Before(courseDay.AddDate(0, 0, 365)) {
		return fmt.Errorf("patient %d treatment dates must fall within the 365-day horizon of the course", patient.PatientNumber)
	}
	days := calendarDayNumber(departure) - calendarDayNumber(arrival) + 1
	if days < 1 || days > 365 {
		return fmt.Errorf("patient %d treatment range must be between 1 and 365 days", patient.PatientNumber)
	}
	if patient.StayDays > 0 && patient.StayDays != int(days) {
		return fmt.Errorf("patient %d stayDays does not match the date range", patient.PatientNumber)
	}
	patient.StayDays = int(days)
	return nil
}

type scheduleReport struct {
	Date                string `json:"date"`
	ScheduledCount      int    `json:"scheduledCount"`
	UnscheduledCount    int    `json:"unscheduledCount"`
	UnscheduledPatients []int  `json:"unscheduledPatientNumbers"`
}

type server struct{ db *pgxpool.Pool }

func main() {
	clinicTimezone := os.Getenv("CLINIC_TIMEZONE")
	if clinicTimezone == "" {
		clinicTimezone = "Asia/Omsk"
	}
	location, err := time.LoadLocation(clinicTimezone)
	if err != nil {
		log.Fatalf("invalid CLINIC_TIMEZONE: %v", err)
	}
	time.Local = location
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		log.Fatal(err)
	}
	if err := migrate(ctx, db); err != nil {
		log.Fatal(err)
	}
	if err := seedProcedures(ctx, db); err != nil {
		log.Fatal(err)
	}

	api := &server{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("GET /v1/appointments", api.listAppointments)
	mux.HandleFunc("PUT /v1/appointments/{id}", api.saveAppointment)
	mux.HandleFunc("DELETE /v1/appointments/{id}", api.deleteAppointment)
	mux.HandleFunc("GET /v1/procedures", api.listProcedures)
	mux.HandleFunc("PUT /v1/procedures", api.saveProcedures)
	mux.HandleFunc("DELETE /v1/procedures/{id}", api.deleteProcedure)
	mux.HandleFunc("GET /v1/day-plans", api.getDayPlan)
	mux.HandleFunc("PUT /v1/day-plans", api.saveDayPlan)
	mux.HandleFunc("GET /v1/calendar-settings", api.getSettings)
	mux.HandleFunc("PUT /v1/calendar-settings", api.saveSettings)
	mux.HandleFunc("POST /v1/schedules/generate", api.generate)
	mux.HandleFunc("DELETE /v1/schedules", api.clearSchedules)
	mux.HandleFunc("GET /v1/schedule-report", api.getScheduleReport)
	mux.HandleFunc("GET /v1/patient-absences", api.listPatientAbsences)
	mux.HandleFunc("POST /v1/patient-absences", api.savePatientAbsence)
	address := os.Getenv("LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	log.Printf("clinic schedule API listening on %s", address)
	log.Fatal((&http.Server{Addr: address, Handler: withHeaders(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}).ListenAndServe())
}

func withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	connection, err := db.Acquire(ctx)
	if err != nil {
		return err
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, "SELECT pg_advisory_lock(hashtext('clinic-schedule-migrations'))"); err != nil {
		return err
	}
	defer connection.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext('clinic-schedule-migrations'))")
	entries, err := os.ReadDir("migrations")
	if err != nil {
		return err
	}
	if _, err := connection.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		sql, err := os.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		var applied bool
		if err := connection.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name=$1)", entry.Name()).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		tx, err := connection.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", entry.Name(), err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations (name) VALUES ($1)", entry.Name()); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func seedProcedures(ctx context.Context, db *pgxpool.Pool) error {
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM procedures").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	for i := 1; i <= 20; i++ {
		_, err := db.Exec(ctx, "INSERT INTO procedures (id, name, duration_minutes, preparation_minutes) VALUES ($1, $2, 30, 20) ON CONFLICT (id) DO NOTHING", fmt.Sprintf("procedure-%d", i), fmt.Sprintf("Процедура %d", i))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listAppointments(w http.ResponseWriter, r *http.Request) {
	from, err := time.Parse("2006-01-02", r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "from must be YYYY-MM-DD")
		return
	}
	to, err := time.Parse("2006-01-02", r.URL.Query().Get("to"))
	if err != nil || !to.After(from) || to.Sub(from) > 32*24*time.Hour {
		writeError(w, http.StatusBadRequest, "to must be after from, within 32 days")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, title, to_char(start_at, 'YYYY-MM-DD"T"HH24:MI'), to_char(end_at, 'YYYY-MM-DD"T"HH24:MI'), description, color, status, generated_by_planner, manually_modified, patient_number,COALESCE(source_procedure_id,''),COALESCE(to_char(source_plan_date,'YYYY-MM-DD'),''),room FROM appointments WHERE start_at >= $1 AND start_at < $2 ORDER BY start_at`, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load appointments")
		return
	}
	defer rows.Close()
	items := make([]appointment, 0)
	for rows.Next() {
		var item appointment
		if err := rows.Scan(&item.ID, &item.Title, &item.StartAt, &item.EndAt, &item.Description, &item.Color, &item.Status, &item.GeneratedByPlanner, &item.ManuallyModified, &item.PatientNumber, &item.ProcedureID, &item.SourcePlanDate, &item.Room); err != nil {
			writeError(w, http.StatusInternalServerError, "could not decode appointments")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "could not load appointments")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) saveAppointment(w http.ResponseWriter, r *http.Request) {
	var item appointment
	if !decode(w, r, &item) {
		return
	}
	item.ID = r.PathValue("id")
	item.ManuallyModified = item.ManuallyModified || !item.GeneratedByPlanner
	if !validAppointment(item) || !validRoom(item.Room) {
		writeError(w, http.StatusBadRequest, "invalid appointment")
		return
	}
	if item.Status == "" {
		item.Status = "scheduled"
	}
	start, _ := time.Parse("2006-01-02T15:04", item.StartAt)
	end, _ := time.Parse("2006-01-02T15:04", item.EndAt)
	tx, err := s.db.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save appointment")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtext('clinic-schedule-room-appointments'))"); err != nil {
		writeError(w, 500, "could not lock room occupancy")
		return
	}
	if _, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtext($1))", item.StartAt[:10]); err != nil {
		writeError(w, http.StatusInternalServerError, "could not lock appointment day")
		return
	}
	if item.Status != "cancelled" {
		workingHours, loadErr := loadSettings(r.Context(), tx)
		if loadErr != nil {
			writeError(w, 500, "could not load organization hours")
			return
		}
		if !workingHours.allowsAppointment(start, end) {
			writeError(w, http.StatusUnprocessableEntity, "appointment is outside organization hours or overlaps a closure")
			return
		}
	}
	var procedureID string
	var sourcePlanDate string
	var previousPatient *int
	var wasGenerated bool
	err = tx.QueryRow(r.Context(), "SELECT COALESCE(source_procedure_id,''),COALESCE(to_char(source_plan_date,'YYYY-MM-DD'),''),patient_number,generated_by_planner FROM appointments WHERE id=$1 FOR UPDATE", item.ID).Scan(&procedureID, &sourcePlanDate, &previousPatient, &wasGenerated)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "could not load appointment procedure")
		return
	}
	item.ProcedureID = procedureID
	item.SourcePlanDate = sourcePlanDate
	if sourcePlanDate != "" {
		if previousPatient == nil || item.PatientNumber == nil || *previousPatient != *item.PatientNumber {
			writeError(w, http.StatusConflict, "cannot reassign an appointment belonging to a treatment course")
			return
		}
		if _, err := tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtext($1))", sourcePlanDate); err != nil {
			writeError(w, http.StatusInternalServerError, "could not lock treatment course")
			return
		}
		item.GeneratedByPlanner = wasGenerated
	}
	if sourcePlanDate != "" && item.PatientNumber != nil && item.Status != "cancelled" {
		var absent bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM patient_absences WHERE schedule_date=$1 AND patient_number=$2 AND $3::date BETWEEN from_date AND to_date)`, sourcePlanDate, item.PatientNumber, item.StartAt[:10]).Scan(&absent); err != nil {
			writeError(w, 500, "could not check patient absence")
			return
		}
		if absent {
			writeError(w, http.StatusUnprocessableEntity, "patient is absent on this date")
			return
		}
	}
	if sourcePlanDate != "" && item.PatientNumber != nil && item.Status != "cancelled" {
		var arrivalDate, departureDate string
		err := tx.QueryRow(r.Context(), `SELECT COALESCE(to_char(arrival_date,'YYYY-MM-DD'),to_char(schedule_date,'YYYY-MM-DD')),
			COALESCE(to_char(discharge_date,'YYYY-MM-DD'),to_char(schedule_date + stay_days - 1,'YYYY-MM-DD'))
			FROM day_plan_patients WHERE schedule_date=$1 AND patient_number=$2`, sourcePlanDate, item.PatientNumber).Scan(&arrivalDate, &departureDate)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 500, "could not validate patient stay")
			return
		}
		if err == nil {
			if item.StartAt[:10] < arrivalDate || item.StartAt[:10] > departureDate {
				writeError(w, http.StatusUnprocessableEntity, "appointment is outside patient treatment period")
				return
			}
		}
	}
	if procedureID != "" && item.PatientNumber != nil && item.Status != "cancelled" {
		var minInterval int
		var procedureName string
		if err := tx.QueryRow(r.Context(), "SELECT min_interval_days,name FROM procedures WHERE id=$1", procedureID).Scan(&minInterval, &procedureName); err != nil {
			writeError(w, 409, "procedure was removed from catalog")
			return
		}
		var repeatConflict bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM appointments
			WHERE id<>$1 AND patient_number=$2 AND status<>'cancelled' AND source_procedure_id=$3
			AND ($6::text='' OR source_plan_date IS NULL OR source_plan_date=$6::date)
			AND abs(start_at::date-$4::date) < $5)`, item.ID, item.PatientNumber, procedureID, item.StartAt[:10], minInterval, sourcePlanDate).Scan(&repeatConflict); err != nil {
			writeError(w, 500, "could not validate procedure repetition")
			return
		}
		if repeatConflict {
			writeError(w, 409, fmt.Sprintf("procedure %q requires %d full days between starts", procedureName, minInterval-1))
			return
		}
	}
	if sourcePlanDate != "" && item.PatientNumber != nil {
		var candidate *courseSession
		if item.Status != "cancelled" && procedureID != "" {
			candidate = &courseSession{id: item.ID, procedureID: procedureID, start: start, end: end}
		}
		if err := validateProposedCourseSessions(r.Context(), tx, sourcePlanDate, *item.PatientNumber, item.ID, candidate); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	if item.Status != "cancelled" {
		var conflict bool
		err = tx.QueryRow(r.Context(), "SELECT EXISTS (SELECT 1 FROM appointments WHERE id <> $1 AND status <> 'cancelled' AND start_at < $3 AND end_at > $2 AND ($4::integer IS NULL OR patient_number IS NULL OR (patient_number=$4 AND ($5::text='' OR source_plan_date IS NULL OR source_plan_date=$5::date)) OR ($6::text<>'' AND lower(btrim(room))=lower(btrim($6)))))", item.ID, start, end, item.PatientNumber, sourcePlanDate, strings.TrimSpace(item.Room)).Scan(&conflict)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not validate appointment time")
			return
		}
		if conflict {
			writeError(w, http.StatusConflict, "appointment time is already occupied")
			return
		}
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO appointments (id,title,start_at,end_at,description,color,status,generated_by_planner,manually_modified,patient_number,room) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title,start_at=EXCLUDED.start_at,end_at=EXCLUDED.end_at,description=EXCLUDED.description,color=EXCLUDED.color,room=EXCLUDED.room,status=EXCLUDED.status,generated_by_planner=EXCLUDED.generated_by_planner,manually_modified=appointments.manually_modified OR EXCLUDED.manually_modified OR (appointments.generated_by_planner AND (appointments.title<>EXCLUDED.title OR appointments.start_at<>EXCLUDED.start_at OR appointments.end_at<>EXCLUDED.end_at OR appointments.description<>EXCLUDED.description OR appointments.color<>EXCLUDED.color OR appointments.room<>EXCLUDED.room)),patient_number=EXCLUDED.patient_number,updated_at=now()`, item.ID, item.Title, item.StartAt, item.EndAt, item.Description, item.Color, item.Status, item.GeneratedByPlanner, item.ManuallyModified, item.PatientNumber, strings.TrimSpace(item.Room))
	if err != nil {
		writeError(w, http.StatusConflict, "could not save appointment")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusConflict, "appointment changed concurrently")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *server) deleteAppointment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not cancel appointment")
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('clinic-schedule-room-appointments'))"); err != nil {
		writeError(w, http.StatusInternalServerError, "could not lock appointment changes")
		return
	}
	var courseDate string
	var patient *int
	err = tx.QueryRow(ctx, "SELECT COALESCE(to_char(source_plan_date,'YYYY-MM-DD'),''),patient_number FROM appointments WHERE id=$1 FOR UPDATE", r.PathValue("id")).Scan(&courseDate, &patient)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "could not load appointment")
		return
	}
	if err == nil && courseDate != "" && patient != nil {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", courseDate); err != nil {
			writeError(w, http.StatusInternalServerError, "could not lock treatment course")
			return
		}
		if err := validateProposedCourseSessions(ctx, tx, courseDate, *patient, r.PathValue("id"), nil); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE appointments SET status='cancelled', manually_modified=manually_modified OR generated_by_planner, updated_at=now() WHERE id=$1", r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, "could not cancel appointment")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusConflict, "appointment changed concurrently")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// clearSchedules removes all calendar entries, patient plans and generated reports,
// but deliberately preserves the procedure catalog and calendar preferences.
func (s *server) clearSchedules(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "application/json is required")
		return
	}
	var input struct {
		Confirmation string `json:"confirmation"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Confirmation != "CLEAR_ALL_SCHEDULES" {
		writeError(w, http.StatusBadRequest, "confirmation is required")
		return
	}

	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start schedule clearing")
		return
	}
	defer tx.Rollback(ctx)

	// Block concurrent schedule writes until the complete reset is committed.
	if _, err = tx.Exec(ctx, "LOCK TABLE appointments, schedule_reports, day_plans, day_plan_patients, day_plan_items, patient_absences IN ACCESS EXCLUSIVE MODE"); err != nil {
		writeError(w, http.StatusInternalServerError, "could not lock schedules")
		return
	}
	for _, statement := range []string{
		"DELETE FROM appointments",
		"DELETE FROM schedule_reports",
		"DELETE FROM day_plans", // Cascades to day_plan_patients and day_plan_items.
	} {
		if _, err = tx.Exec(ctx, statement); err != nil {
			writeError(w, http.StatusInternalServerError, "could not clear schedules")
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not commit schedule clearing")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listProcedures(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), "SELECT id,name,duration_minutes,preparation_minutes,is_relaxing,min_interval_days,default_weekly_count,default_session_count,room,COALESCE(default_prerequisite_id,'') FROM procedures ORDER BY id")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load procedures")
		return
	}
	defer rows.Close()
	items := make([]procedure, 0)
	for rows.Next() {
		var item procedure
		if err := rows.Scan(&item.ID, &item.Name, &item.DurationMinutes, &item.PreparationMinutes, &item.IsRelaxing, &item.MinIntervalDays, &item.DefaultWeeklyCount, &item.DefaultSessionCount, &item.Room, &item.DefaultPrerequisiteID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not decode procedures")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "could not load procedures")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) saveProcedures(w http.ResponseWriter, r *http.Request) {
	// Distinguish omitted fields from explicit values for older clients. In
	// particular, a missing prerequisite must not erase an existing default.
	var payload []struct {
		procedure
		DefaultSessionCount   json.RawMessage `json:"defaultSessionCount"`
		DefaultPrerequisiteID json.RawMessage `json:"defaultPrerequisiteId"`
	}
	if !decode(w, r, &payload) {
		return
	}
	if len(payload) > 500 {
		writeError(w, http.StatusBadRequest, "too many procedures")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not save procedures")
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize catalog updates and deletions so validation sees the full final
	// state, including references from procedures outside this PUT batch.
	if _, err := tx.Exec(r.Context(), "LOCK TABLE procedures IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		writeError(w, 500, "could not save procedures")
		return
	}
	type prerequisiteRule struct {
		isRelaxing     bool
		prerequisiteID string
	}
	rules := make(map[string]prerequisiteRule)
	rows, err := tx.Query(r.Context(), "SELECT id,is_relaxing,COALESCE(default_prerequisite_id,'') FROM procedures")
	if err != nil {
		writeError(w, 500, "could not load procedures")
		return
	}
	for rows.Next() {
		var id string
		var rule prerequisiteRule
		if err := rows.Scan(&id, &rule.isRelaxing, &rule.prerequisiteID); err != nil {
			rows.Close()
			writeError(w, 500, "could not load procedures")
			return
		}
		rules[id] = rule
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, 500, "could not load procedures")
		return
	}
	rows.Close()
	items := make([]procedure, 0, len(payload))
	seen := make(map[string]bool, len(payload))
	for _, incoming := range payload {
		item := incoming.procedure
		item.DefaultSessionCount = 1
		if incoming.DefaultSessionCount != nil {
			var count *int
			if err := json.Unmarshal(incoming.DefaultSessionCount, &count); err != nil || count == nil {
				writeError(w, 400, "invalid procedure")
				return
			}
			item.DefaultSessionCount = *count
		}
		if incoming.DefaultPrerequisiteID == nil {
			item.DefaultPrerequisiteID = rules[item.ID].prerequisiteID
		} else {
			var prerequisiteID *string
			if err := json.Unmarshal(incoming.DefaultPrerequisiteID, &prerequisiteID); err != nil || prerequisiteID == nil {
				writeError(w, http.StatusBadRequest, "invalid procedure prerequisite")
				return
			}
			item.DefaultPrerequisiteID = *prerequisiteID
		}
		if seen[item.ID] || strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" || len([]rune(item.Name)) > 480 || item.DurationMinutes < 1 || item.DurationMinutes > 420 || item.PreparationMinutes < 0 || item.PreparationMinutes > 419 || item.DurationMinutes+item.PreparationMinutes > 420 || item.MinIntervalDays < 1 || item.MinIntervalDays > 365 || item.DefaultWeeklyCount < 1 || item.DefaultWeeklyCount > 7 || item.DefaultSessionCount < 1 || item.DefaultSessionCount > 365 || !validRoom(item.Room) {
			writeError(w, 400, "invalid procedure")
			return
		}
		seen[item.ID] = true
		rules[item.ID] = prerequisiteRule{isRelaxing: item.IsRelaxing, prerequisiteID: item.DefaultPrerequisiteID}
		items = append(items, item)
	}
	for id, rule := range rules {
		if rule.prerequisiteID == "" {
			continue
		}
		prerequisite, exists := rules[rule.prerequisiteID]
		if rule.isRelaxing || rule.prerequisiteID == id || !exists || !prerequisite.isRelaxing {
			writeError(w, http.StatusBadRequest, "prerequisite must be an existing relaxing procedure assigned to a non-relaxing procedure")
			return
		}
	}
	for _, item := range items {
		_, err = tx.Exec(r.Context(), "INSERT INTO procedures (id,name,duration_minutes,preparation_minutes,is_relaxing,min_interval_days,default_weekly_count,default_session_count,room,default_prerequisite_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')) ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name,duration_minutes=EXCLUDED.duration_minutes,preparation_minutes=EXCLUDED.preparation_minutes,is_relaxing=EXCLUDED.is_relaxing,min_interval_days=EXCLUDED.min_interval_days,default_weekly_count=EXCLUDED.default_weekly_count,default_session_count=EXCLUDED.default_session_count,room=EXCLUDED.room,default_prerequisite_id=EXCLUDED.default_prerequisite_id,updated_at=now()", item.ID, strings.TrimSpace(item.Name), item.DurationMinutes, item.PreparationMinutes, item.IsRelaxing, item.MinIntervalDays, item.DefaultWeeklyCount, item.DefaultSessionCount, strings.TrimSpace(item.Room), item.DefaultPrerequisiteID)
		if err != nil {
			writeError(w, 500, "could not save procedures")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "could not save procedures")
		return
	}
	writeJSON(w, 200, items)
}

func (s *server) deleteProcedure(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete procedure")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), "LOCK TABLE procedures IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check procedure usage")
		return
	}
	var referenced, requiredByCatalog bool
	if err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM day_plan_items WHERE procedure_id=$1), EXISTS(SELECT 1 FROM procedures WHERE default_prerequisite_id=$1)", r.PathValue("id")).Scan(&referenced, &requiredByCatalog); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check procedure usage")
		return
	}
	if requiredByCatalog {
		writeError(w, http.StatusConflict, "procedure is a prerequisite for another catalog procedure")
		return
	}
	if referenced {
		writeError(w, http.StatusConflict, "procedure is used in one or more patient plans")
		return
	}
	if _, err := tx.Exec(r.Context(), "DELETE FROM procedures WHERE id=$1", r.PathValue("id")); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23503" {
			writeError(w, http.StatusConflict, "procedure is used in one or more patient plans")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not delete procedure")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete procedure")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) getDayPlan(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	plan, err := loadDayPlan(r.Context(), s.db, date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load patient plans")
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func loadDayPlan(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, date string) (dayPlan, error) {
	plan := dayPlan{Date: date, Plans: []patientPlan{}}
	err := db.QueryRow(ctx, "SELECT revision, selected_patient_number FROM day_plans WHERE schedule_date=$1", date).Scan(&plan.Revision, &plan.SelectedPatientNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		selectedPatient := 1
		arrival, valid := parseISODate(date)
		if !valid {
			return dayPlan{}, fmt.Errorf("invalid course date")
		}
		return dayPlan{Date: date, SelectedPatientNumber: &selectedPatient, Plans: []patientPlan{{PatientNumber: 1, Severity: "medium", ArrivalDate: date, DepartureDate: arrival.AddDate(0, 0, 6).Format("2006-01-02"), StayDays: 7, CourseKey: date, ProcedureIDs: []string{}, ProcedureCounts: map[string]int{}}}}, nil
	}
	if err != nil {
		return plan, err
	}
	rows, err := db.Query(ctx, `SELECT p.patient_number,p.severity,p.stay_days,
		COALESCE(to_char(p.arrival_date,'YYYY-MM-DD'),to_char(p.schedule_date,'YYYY-MM-DD')),
		COALESCE(to_char(p.discharge_date,'YYYY-MM-DD'),to_char(p.schedule_date + p.stay_days - 1,'YYYY-MM-DD')),
			i.procedure_id,i.session_count,i.prerequisite_procedure_id,i.prerequisite_override_set,i.prerequisite_override_id
		FROM day_plan_patients p LEFT JOIN day_plan_items i ON i.schedule_date=p.schedule_date AND i.patient_number=p.patient_number
		WHERE p.schedule_date=$1 ORDER BY p.patient_number,i.position`, date)
	if err != nil {
		return plan, err
	}
	defer rows.Close()
	byPatient := map[int]int{}
	for rows.Next() {
		var patientNumber int
		var severity string
		var stayDays int
		var arrivalDate, departureDate string
		var procedureID *string
		var sessionCount *int
		var prerequisiteID *string
		var overrideSet *bool
		var overrideID *string
		if err := rows.Scan(&patientNumber, &severity, &stayDays, &arrivalDate, &departureDate, &procedureID, &sessionCount, &prerequisiteID, &overrideSet, &overrideID); err != nil {
			return plan, err
		}
		index, exists := byPatient[patientNumber]
		if !exists {
			index = len(plan.Plans)
			byPatient[patientNumber] = index
			plan.Plans = append(plan.Plans, patientPlan{PatientNumber: patientNumber, Severity: severity, StayDays: stayDays, ArrivalDate: arrivalDate, DepartureDate: departureDate, CourseKey: date, ProcedureIDs: []string{}, ProcedureCounts: map[string]int{}, ProcedurePrerequisites: map[string]string{}, ProcedurePrerequisiteOverrides: map[string]*string{}})
		}
		if procedureID != nil {
			plan.Plans[index].ProcedureIDs = append(plan.Plans[index].ProcedureIDs, *procedureID)
			plan.Plans[index].ProcedureCounts[*procedureID] = *sessionCount
			if prerequisiteID != nil {
				plan.Plans[index].ProcedurePrerequisites[*procedureID] = *prerequisiteID
			}
			if overrideSet != nil && *overrideSet {
				plan.Plans[index].ProcedurePrerequisiteOverrides[*procedureID] = overrideID
			}
		}
	}
	return plan, rows.Err()
}

func (s *server) saveDayPlan(w http.ResponseWriter, r *http.Request) {
	var plan dayPlan
	if !decode(w, r, &plan) {
		return
	}
	day, validDate := parseISODate(plan.Date)
	if !validDate || plan.Revision < 0 || len(plan.Plans) > 100 {
		writeError(w, http.StatusBadRequest, "invalid day plan")
		return
	}
	for index := range plan.Plans {
		if err := normalizePatientTreatmentDates(plan.Date, &plan.Plans[index]); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		plan.Plans[index].CourseKey = plan.Date
	}
	seenPatients := map[int]bool{}
	procedureIDs := map[string]bool{}
	procedurePrerequisites := map[int]map[string]int{}
	for _, patient := range plan.Plans {
		if patient.PatientNumber < 1 || seenPatients[patient.PatientNumber] || (patient.Severity != "high" && patient.Severity != "medium" && patient.Severity != "low") || len(patient.ProcedureIDs) > 500 || patient.StayDays < 1 || patient.StayDays > 365 || len(patient.ProcedureCounts) != len(patient.ProcedureIDs) || len(patient.ProcedurePrerequisites) > len(patient.ProcedureIDs) || len(patient.ProcedurePrerequisiteOverrides) > len(patient.ProcedureIDs) {
			writeError(w, http.StatusBadRequest, "invalid patient plan")
			return
		}
		seenPatients[patient.PatientNumber] = true
		seenProcedures := map[string]bool{}
		for _, procedureID := range patient.ProcedureIDs {
			if procedureID == "" || seenProcedures[procedureID] || patient.ProcedureCounts[procedureID] < 1 || patient.ProcedureCounts[procedureID] > 365 || patient.ProcedureCounts[procedureID] > patient.StayDays {
				writeError(w, http.StatusBadRequest, "procedure assigned more than once")
				return
			}
			seenProcedures[procedureID] = true
			procedureIDs[procedureID] = true
		}
		for targetID, prerequisiteID := range patient.ProcedurePrerequisites {
			if patient.Severity != "high" || !seenProcedures[targetID] || !seenProcedures[prerequisiteID] || targetID == prerequisiteID {
				writeError(w, http.StatusBadRequest, "invalid procedure prerequisite")
				return
			}
			if procedurePrerequisites[patient.PatientNumber] == nil {
				procedurePrerequisites[patient.PatientNumber] = map[string]int{}
			}
			procedurePrerequisites[patient.PatientNumber][prerequisiteID] += patient.ProcedureCounts[targetID]
		}
		for targetID, overrideID := range patient.ProcedurePrerequisiteOverrides {
			if patient.Severity != "high" || !seenProcedures[targetID] {
				writeError(w, http.StatusBadRequest, "invalid procedure prerequisite override")
				return
			}
			effectiveID, hasEffective := patient.ProcedurePrerequisites[targetID]
			if overrideID == nil {
				if hasEffective {
					writeError(w, http.StatusBadRequest, "disabled prerequisite must not have an effective prerequisite")
					return
				}
			} else if *overrideID == "" || !seenProcedures[*overrideID] || *overrideID == targetID || !hasEffective || effectiveID != *overrideID {
				writeError(w, http.StatusBadRequest, "invalid procedure prerequisite override")
				return
			}
		}
		for prerequisiteID, requiredCount := range procedurePrerequisites[patient.PatientNumber] {
			if patient.ProcedureCounts[prerequisiteID] < requiredCount {
				writeError(w, http.StatusUnprocessableEntity, "not enough prerequisite sessions assigned")
				return
			}
		}
	}
	if plan.SelectedPatientNumber != nil && !seenPatients[*plan.SelectedPatientNumber] {
		writeError(w, http.StatusBadRequest, "selected patient is not in the plan")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save patient plans")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtext('clinic-schedule-room-appointments'))"); err != nil {
		writeError(w, http.StatusInternalServerError, "could not lock appointment changes")
		return
	}
	if _, err := tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtext($1))", plan.Date); err != nil {
		writeError(w, http.StatusInternalServerError, "could not lock patient plan")
		return
	}
	var currentRevision int
	err = tx.QueryRow(r.Context(), "SELECT revision FROM day_plans WHERE schedule_date=$1", plan.Date).Scan(&currentRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		currentRevision = 0
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check patient plan revision")
		return
	}
	if currentRevision > 0 {
		existing, loadErr := loadDayPlan(r.Context(), tx, plan.Date)
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "could not compare patient plans")
			return
		}
		if equalDayPlanContents(existing, plan) {
			writeJSON(w, http.StatusOK, existing)
			return
		}
	}
	if currentRevision != plan.Revision {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "patient plan changed; reload before saving"})
		return
	}
	// Existing absences and protected appointments must stay within the patient's range.
	for _, patient := range plan.Plans {
		var incompatible bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM patient_absences
			WHERE schedule_date=$1 AND patient_number=$2 AND (from_date < $3 OR to_date > $4))`, plan.Date, patient.PatientNumber, patient.ArrivalDate, patient.DepartureDate).Scan(&incompatible); err != nil {
			writeError(w, 500, "could not validate existing absences")
			return
		}
		if incompatible {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("patient %d treatment range must include all saved absences; update the conflicting absence first", patient.PatientNumber))
			return
		}
		arrival, _ := parseISODate(patient.ArrivalDate)
		departure, _ := parseISODate(patient.DepartureDate)
		arrivalStart := time.Date(arrival.Year(), arrival.Month(), arrival.Day(), 0, 0, 0, 0, time.Local)
		departureEnd := time.Date(departure.Year(), departure.Month(), departure.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, 1)
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM appointments a
			WHERE a.status='scheduled' AND a.patient_number=$1
			AND (a.source_plan_date=$2 OR (a.source_plan_date IS NULL AND EXISTS (
				SELECT 1 FROM day_plan_patients old WHERE old.schedule_date=$2 AND old.patient_number=$1
				AND a.start_at >= (COALESCE(old.arrival_date,old.schedule_date)::timestamp AT TIME ZONE $5)
				AND a.start_at < ((COALESCE(old.discharge_date,old.schedule_date + old.stay_days - 1) + 1)::timestamp AT TIME ZONE $5))))
			AND (a.start_at < $3 OR a.start_at >= $4) AND (NOT a.generated_by_planner OR a.manually_modified))`, patient.PatientNumber, plan.Date, arrivalStart, departureEnd, time.Local.String()).Scan(&incompatible); err != nil {
			writeError(w, 500, "could not validate protected appointments")
			return
		}
		if incompatible {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("patient %d treatment range must include all protected appointments", patient.PatientNumber))
			return
		}
	}
	procedureIDList := make([]string, 0, len(procedureIDs))
	for procedureID := range procedureIDs {
		procedureIDList = append(procedureIDList, procedureID)
	}
	procedureRelaxing := map[string]bool{}
	procedureRows, err := tx.Query(r.Context(), "SELECT id,is_relaxing FROM procedures WHERE id=ANY($1)", procedureIDList)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not validate patient procedures")
		return
	}
	for procedureRows.Next() {
		var id string
		var isRelaxing bool
		if err := procedureRows.Scan(&id, &isRelaxing); err != nil {
			procedureRows.Close()
			writeError(w, http.StatusInternalServerError, "could not validate patient procedures")
			return
		}
		procedureRelaxing[id] = isRelaxing
	}
	if err := procedureRows.Err(); err != nil {
		procedureRows.Close()
		writeError(w, http.StatusInternalServerError, "could not validate patient procedures")
		return
	}
	procedureRows.Close()
	if len(procedureRelaxing) != len(procedureIDList) {
		writeError(w, http.StatusBadRequest, "unknown procedure in patient plan")
		return
	}
	for _, patient := range plan.Plans {
		for targetID, prerequisiteID := range patient.ProcedurePrerequisites {
			if !procedureRelaxing[prerequisiteID] || procedureRelaxing[targetID] {
				writeError(w, http.StatusUnprocessableEntity, "prerequisite must be a relaxing procedure")
				return
			}
		}
		for targetID, overrideID := range patient.ProcedurePrerequisiteOverrides {
			if procedureRelaxing[targetID] || (overrideID != nil && !procedureRelaxing[*overrideID]) {
				writeError(w, http.StatusUnprocessableEntity, "prerequisite override must target a non-relaxing procedure and reference a relaxing procedure")
				return
			}
		}
	}
	plan.Revision++
	_, err = tx.Exec(r.Context(), "INSERT INTO day_plans(schedule_date,revision,selected_patient_number,updated_at) VALUES($1,$2,$3,now()) ON CONFLICT(schedule_date) DO UPDATE SET revision=EXCLUDED.revision,selected_patient_number=EXCLUDED.selected_patient_number,updated_at=now()", day, plan.Revision, plan.SelectedPatientNumber)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save patient plan")
		return
	}
	for _, patient := range plan.Plans {
		// Update patients in place. Deleting and reinserting the whole plan would
		// cascade-delete absences for patients who remain in treatment.
		if _, err = tx.Exec(r.Context(), `INSERT INTO day_plan_patients(schedule_date,patient_number,severity,stay_days,arrival_date,discharge_date) VALUES($1,$2,$3,$4,$5,$6)
			ON CONFLICT(schedule_date,patient_number) DO UPDATE SET severity=EXCLUDED.severity,stay_days=EXCLUDED.stay_days,arrival_date=EXCLUDED.arrival_date,discharge_date=EXCLUDED.discharge_date`, day, patient.PatientNumber, patient.Severity, patient.StayDays, patient.ArrivalDate, patient.DepartureDate); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save patient")
			return
		}
		if _, err = tx.Exec(r.Context(), "DELETE FROM day_plan_items WHERE schedule_date=$1 AND patient_number=$2", day, patient.PatientNumber); err != nil {
			writeError(w, 500, "could not replace patient procedures")
			return
		}
		for index, procedureID := range patient.ProcedureIDs {
			var prerequisiteID any
			if value := patient.ProcedurePrerequisites[procedureID]; value != "" {
				prerequisiteID = value
			}
			overrideID, overrideSet := patient.ProcedurePrerequisiteOverrides[procedureID]
			if _, err = tx.Exec(r.Context(), "INSERT INTO day_plan_items(schedule_date,patient_number,procedure_id,position,session_count,prerequisite_procedure_id,prerequisite_override_set,prerequisite_override_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", day, patient.PatientNumber, procedureID, index, patient.ProcedureCounts[procedureID], prerequisiteID, overrideSet, overrideID); err != nil {
				writeError(w, http.StatusInternalServerError, "could not save patient procedures")
				return
			}
		}
	}
	patientNumbers := make([]int, 0, len(plan.Plans))
	for _, patient := range plan.Plans {
		patientNumbers = append(patientNumbers, patient.PatientNumber)
	}
	if _, err = tx.Exec(r.Context(), "DELETE FROM day_plan_patients WHERE schedule_date=$1 AND NOT (patient_number=ANY($2::integer[]))", plan.Date, patientNumbers); err != nil {
		writeError(w, 500, "could not reconcile patient plans")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusConflict, "patient plan changed concurrently")
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *server) getSettings(w http.ResponseWriter, r *http.Request) {
	value, err := loadSettings(r.Context(), s.db)
	if err != nil {
		writeError(w, 500, "could not load settings")
		return
	}
	writeJSON(w, 200, value)
}

func (s *server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var value settings
	if !decode(w, r, &value) {
		return
	}
	if (value.TimeStepMinutes != 5 && value.TimeStepMinutes != 10 && value.TimeStepMinutes != 15 && value.TimeStepMinutes != 30) || value.DisplayStartHour < 0 || value.DisplayStartHour > 23 || value.DisplayEndHour <= value.DisplayStartHour || value.DisplayEndHour > 24 || !validWorkWeek(value.WorkWeek) {
		writeError(w, 400, "invalid calendar settings")
		return
	}
	weekJSON, err := json.Marshal(value.WorkWeek)
	if err != nil {
		writeError(w, 400, "invalid work week")
		return
	}
	_, err = s.db.Exec(r.Context(), "UPDATE calendar_settings SET time_step_minutes=$1,display_start_hour=$2,display_end_hour=$3,work_week=$4::jsonb WHERE id=1", value.TimeStepMinutes, value.DisplayStartHour, value.DisplayEndHour, string(weekJSON))
	if err != nil {
		writeError(w, 500, "could not save settings")
		return
	}
	writeJSON(w, 200, value)
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, 400, "invalid request body")
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func validAppointment(item appointment) bool {
	start, err1 := time.Parse("2006-01-02T15:04", item.StartAt)
	end, err2 := time.Parse("2006-01-02T15:04", item.EndAt)
	if err1 != nil || err2 != nil {
		return false
	}
	endsAtMidnight := end.Equal(start.Truncate(24*time.Hour).AddDate(0, 0, 1))
	return item.ID != "" && strings.TrimSpace(item.Title) != "" && len([]rune(item.Title)) <= 512 && start.Before(end) && (sameCalendarDate(start, end) || endsAtMidnight) && validColor(item.Color) && (item.Status == "" || item.Status == "scheduled" || item.Status == "cancelled") && len([]rune(item.Description)) <= 1000
}
func validColor(color string) bool {
	return color == "blue" || color == "teal" || color == "orange" || color == "purple"
}

func unscheduledProcedureDetails(plans []patientPlan, procedures map[string]procedure, appointments []appointment, protected map[int]map[string]int, configuration settings, absences map[int]map[string]bool) []unscheduledProcedure {
	scheduled := map[int]map[string]int{}
	for patientNumber, counts := range protected {
		scheduled[patientNumber] = map[string]int{}
		for procedureID, count := range counts {
			scheduled[patientNumber][procedureID] = count
		}
	}
	for _, item := range appointments {
		if item.PatientNumber == nil {
			continue
		}
		if scheduled[*item.PatientNumber] == nil {
			scheduled[*item.PatientNumber] = map[string]int{}
		}
		scheduled[*item.PatientNumber][item.ProcedureID]++
	}
	details := []unscheduledProcedure{}
	for _, plan := range plans {
		for _, procedureID := range plan.ProcedureIDs {
			requested := plan.ProcedureCounts[procedureID]
			completed := scheduled[plan.PatientNumber][procedureID]
			if missing := requested - completed; missing > 0 {
				p := procedures[procedureID]
				code, reason, recommendation := explainUnscheduledProcedure(plan, p, requested, configuration, absences[plan.PatientNumber])
				if prerequisiteID := plan.ProcedurePrerequisites[procedureID]; plan.Severity == "high" && prerequisiteID != "" {
					requiredRelaxing := 0
					for targetID, linkedRelaxing := range plan.ProcedurePrerequisites {
						if linkedRelaxing == prerequisiteID {
							requiredRelaxing += plan.ProcedureCounts[targetID]
						}
					}
					if scheduled[plan.PatientNumber][prerequisiteID] < requiredRelaxing {
						code = "prerequisite"
						reason = fmt.Sprintf("Недостаточно сеансов обязательной расслабляющей процедуры «%s» перед «%s».", procedures[prerequisiteID].Name, p.Name)
						recommendation = "Освободите время для обязательной расслабляющей процедуры перед сложной процедурой в тот же день."
					}
				}
				details = append(details, unscheduledProcedure{
					PatientNumber:  plan.PatientNumber,
					ProcedureID:    procedureID,
					ProcedureName:  procedures[procedureID].Name,
					Requested:      requested,
					Scheduled:      completed,
					Missing:        missing,
					ReasonCode:     code,
					Reason:         reason,
					Recommendation: recommendation,
				})
			}
		}
	}
	return details
}

// Explanations are conservative: only name a specific limiting rule when it
// alone proves that the requested number cannot fit. Otherwise report capacity.
func explainUnscheduledProcedure(plan patientPlan, p procedure, requested int, configuration settings, absent map[string]bool) (code, reason, recommendation string) {
	arrival, valid := parseISODate(plan.ArrivalDate)
	if !valid {
		return "capacity", "Не удалось разместить все сеансы в доступных временных окнах.", "Освободите время пациента или кабинета либо увеличьте период лечения."
	}
	workingDays := []time.Time{}
	availableDays := []time.Time{}
	for offset := 0; offset < plan.StayDays; offset++ {
		day := arrival.AddDate(0, 0, offset)
		work := configuration.dayAt(day)
		if !work.Open {
			continue
		}
		start, _ := minutesFromClock(work.Start)
		end, _ := minutesFromClock(work.End)
		if end-start < p.DurationMinutes+p.PreparationMinutes {
			continue
		}
		workingDays = append(workingDays, day)
		if !absent[day.Format("2006-01-02")] {
			availableDays = append(availableDays, day)
		}
	}
	if len(workingDays) == 0 {
		return "working_days", "Нет рабочих дней, достаточно длинных для этой процедуры.", "Измените рабочие часы или период лечения пациента."
	}
	if len(availableDays) == 0 {
		return "absence", "Все подходящие рабочие дни попадают на отсутствие пациента.", "Измените даты лечения или период отсутствия пациента."
	}
	maxSessions := func(days []time.Time, interval, weekly int) int {
		selected := []time.Time{}
		for _, day := range days {
			if len(selected) > 0 && calendarDayNumber(day)-calendarDayNumber(selected[len(selected)-1]) < int64(interval) {
				continue
			}
			if courseWeeklyAllowed(day, selected, weekly) {
				selected = append(selected, day)
			}
		}
		return len(selected)
	}
	if requested > maxSessions(workingDays, p.MinIntervalDays, p.DefaultWeeklyCount) {
		if requested <= maxSessions(workingDays, 1, p.DefaultWeeklyCount) {
			return "interval", fmt.Sprintf("Перерыв %d дн. между сеансами не позволяет разместить весь курс.", p.MinIntervalDays-1), "Сократите перерыв между процедурами или увеличьте период лечения."
		}
		if requested <= maxSessions(workingDays, p.MinIntervalDays, 7) {
			return "weekly_limit", fmt.Sprintf("Лимит %d сеансов за 7 дней не позволяет разместить весь курс.", p.DefaultWeeklyCount), "Увеличьте недельную частоту или продолжительность курса."
		}
		return "working_days", "Для указанного количества сеансов недостаточно рабочих дней.", "Продлите период лечения или добавьте рабочие дни."
	}
	if requested > maxSessions(availableDays, p.MinIntervalDays, p.DefaultWeeklyCount) {
		return "absence", "Отсутствие пациента сокращает число доступных дней для курса.", "Измените период лечения или пересмотрите дни отсутствия."
	}
	return "capacity", "Не хватило свободных временных окон с учётом других процедур и занятости кабинетов.", "Освободите время в расписании, измените длительность процедур или продлите период лечения."
}

func (s *server) generate(w http.ResponseWriter, r *http.Request) {
	var input scheduleRequest
	if !decode(w, r, &input) {
		return
	}
	day, validDate := parseISODate(input.Date)
	if !validDate {
		writeError(w, 400, "invalid schedule request")
		return
	}
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		writeError(w, 500, "could not start schedule transaction")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('clinic-schedule-room-appointments'))"); err != nil {
		writeError(w, 500, "could not lock room occupancy")
		return
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", input.Date); err != nil {
		writeError(w, 500, "could not lock schedule")
		return
	}
	plan, err := loadDayPlan(ctx, tx, input.Date)
	if err != nil {
		writeError(w, 500, "could not load patient plans")
		return
	}
	windowStart := day
	windowEndExclusive := day
	for index := range plan.Plans {
		patient := &plan.Plans[index]
		if err := normalizePatientTreatmentDates(input.Date, patient); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		patient.CourseKey = input.Date
		arrival, _ := parseISODate(patient.ArrivalDate)
		departure, _ := parseISODate(patient.DepartureDate)
		if index == 0 || arrival.Before(windowStart) {
			windowStart = arrival
		}
		if index == 0 || !departure.AddDate(0, 0, 1).Before(windowEndExclusive) {
			windowEndExclusive = departure.AddDate(0, 0, 1)
		}
	}
	for index := range plan.Plans {
		arrival, _ := parseISODate(plan.Plans[index].ArrivalDate)
		plan.Plans[index].StartOffset = int(calendarDayNumber(arrival) - calendarDayNumber(windowStart))
	}
	if _, err = tx.Exec(ctx, "DELETE FROM appointments WHERE generated_by_planner=true AND manually_modified=false AND (source_plan_date=$1 OR (source_plan_date IS NULL AND start_at >= $1 AND start_at < $2))", day, day.AddDate(0, 0, 1)); err != nil {
		writeError(w, 500, "could not replace generated schedule")
		return
	}
	occupiedRows, err := tx.Query(ctx, "SELECT start_at,end_at,patient_number,COALESCE(source_procedure_id,''),COALESCE(to_char(source_plan_date,'YYYY-MM-DD'),''),room FROM appointments WHERE status <> 'cancelled' AND start_at >= $1 AND start_at < $2", windowStart.AddDate(0, 0, -365), windowEndExclusive)
	if err != nil {
		writeError(w, 500, "could not load existing appointments")
		return
	}
	occupied := make([]timeSlot, 0)
	for occupiedRows.Next() {
		var slot timeSlot
		if err = occupiedRows.Scan(&slot.start, &slot.end, &slot.patient, &slot.procedureID, &slot.sourcePlanDate, &slot.room); err != nil {
			occupiedRows.Close()
			writeError(w, 500, "could not read existing appointments")
			return
		}
		slot.start = time.Date(slot.start.Year(), slot.start.Month(), slot.start.Day(), slot.start.Hour(), slot.start.Minute(), 0, 0, time.Local)
		slot.end = time.Date(slot.end.Year(), slot.end.Month(), slot.end.Day(), slot.end.Hour(), slot.end.Minute(), 0, 0, time.Local)
		occupied = append(occupied, slot)
	}
	if err = occupiedRows.Err(); err != nil {
		occupiedRows.Close()
		writeError(w, 500, "could not read existing appointments")
		return
	}
	occupiedRows.Close()
	rows, err := tx.Query(ctx, "SELECT id,name,duration_minutes,preparation_minutes,is_relaxing,min_interval_days,default_weekly_count,default_session_count,room,COALESCE(default_prerequisite_id,'') FROM procedures")
	if err != nil {
		writeError(w, 500, "could not load procedures")
		return
	}
	procedureMap := map[string]procedure{}
	for rows.Next() {
		var p procedure
		if err = rows.Scan(&p.ID, &p.Name, &p.DurationMinutes, &p.PreparationMinutes, &p.IsRelaxing, &p.MinIntervalDays, &p.DefaultWeeklyCount, &p.DefaultSessionCount, &p.Room, &p.DefaultPrerequisiteID); err != nil {
			rows.Close()
			writeError(w, 500, "could not load procedures")
			return
		}
		procedureMap[p.ID] = p
	}
	rows.Close()
	totalSessions := 0
	for _, patient := range plan.Plans {
		for _, procedureID := range patient.ProcedureIDs {
			totalSessions += patient.ProcedureCounts[procedureID]
		}
	}
	if totalSessions == 0 {
		writeError(w, http.StatusUnprocessableEntity, "no procedures assigned to patients")
		return
	}
	if totalSessions > 3000 {
		writeError(w, 400, "too many sessions in one treatment plan (maximum 3000)")
		return
	}
	warnings := courseWarnings(plan.Plans, procedureMap)
	absences, err := loadAbsenceDays(ctx, tx, input.Date)
	if err != nil {
		writeError(w, 500, "could not load patient absences")
		return
	}
	protectedRows, err := tx.Query(ctx, `SELECT id,patient_number,title,source_procedure_id,start_at,end_at FROM appointments
		WHERE generated_by_planner AND manually_modified AND status='scheduled'
		AND (source_plan_date=$1 OR (source_plan_date IS NULL AND start_at >= $1 AND start_at < $2))`, day, day.AddDate(0, 0, 1))
	if err != nil {
		writeError(w, 500, "could not load protected appointments")
		return
	}
	type protectedAppointment struct {
		id          string
		patient     *int
		title       string
		procedureID *string
		startAt     time.Time
		endAt       time.Time
	}
	protected := []protectedAppointment{}
	for protectedRows.Next() {
		var item protectedAppointment
		if err = protectedRows.Scan(&item.id, &item.patient, &item.title, &item.procedureID, &item.startAt, &item.endAt); err != nil {
			protectedRows.Close()
			writeError(w, 500, "could not load protected appointments")
			return
		}
		protected = append(protected, item)
	}
	if err = protectedRows.Err(); err != nil {
		protectedRows.Close()
		writeError(w, 500, "could not load protected appointments")
		return
	}
	protectedRows.Close()
	protectedCount := 0
	protectedByProcedure := map[int]map[string]int{}
	remainingPlans := make([]patientPlan, len(plan.Plans))
	for index, patient := range plan.Plans {
		remainingPlans[index] = patient
		remainingPlans[index].ProcedureCounts = make(map[string]int, len(patient.ProcedureCounts))
		remainingPlans[index].ProcedurePrerequisites = make(map[string]string, len(patient.ProcedurePrerequisites))
		for id, count := range patient.ProcedureCounts {
			remainingPlans[index].ProcedureCounts[id] = count
		}
		for targetID, prerequisiteID := range patient.ProcedurePrerequisites {
			remainingPlans[index].ProcedurePrerequisites[targetID] = prerequisiteID
		}
	}
	for protectedIndex := range protected {
		kept := protected[protectedIndex]
		if kept.patient == nil {
			continue
		}
		for index := range remainingPlans {
			patient := &remainingPlans[index]
			if patient.PatientNumber != *kept.patient {
				continue
			}
			for _, id := range patient.ProcedureIDs {
				if patient.ProcedureCounts[id] < 1 {
					continue
				}
				if kept.procedureID != nil && *kept.procedureID != id {
					continue
				}
				if kept.procedureID == nil && kept.title != fmt.Sprintf("Пациент %d · %s", patient.PatientNumber, procedureMap[id].Name) {
					continue
				}
				protectedCount++
				if protectedByProcedure[patient.PatientNumber] == nil {
					protectedByProcedure[patient.PatientNumber] = map[string]int{}
				}
				protectedByProcedure[patient.PatientNumber][id]++
				patient.ProcedureCounts[id]--
				if kept.procedureID == nil {
					procedureID := id
					kept.procedureID = &procedureID
					protected[protectedIndex].procedureID = kept.procedureID
					if _, err = tx.Exec(ctx, "UPDATE appointments SET source_plan_date=$1,source_procedure_id=$2 WHERE id=$3", day, id, kept.id); err != nil {
						writeError(w, 500, "could not associate existing appointment")
						return
					}
				}
				break
			}
		}
	}
	for index := range remainingPlans {
		patient := &remainingPlans[index]
		if patient.Severity != "high" {
			continue
		}
		for targetID, prerequisiteID := range patient.ProcedurePrerequisites {
			var targets, prerequisites []protectedAppointment
			for _, kept := range protected {
				if kept.patient == nil || *kept.patient != patient.PatientNumber || kept.procedureID == nil {
					continue
				}
				if *kept.procedureID == targetID {
					targets = append(targets, kept)
				} else if *kept.procedureID == prerequisiteID {
					prerequisites = append(prerequisites, kept)
				}
			}
			used := make([]bool, len(prerequisites))
			for _, target := range targets {
				matched := false
				for prerequisiteIndex, prerequisite := range prerequisites {
					if !used[prerequisiteIndex] && prerequisite.startAt.Format("2006-01-02") == target.startAt.Format("2006-01-02") && !prerequisite.endAt.After(target.startAt) {
						used[prerequisiteIndex] = true
						matched = true
						break
					}
				}
				if !matched {
					writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("manually protected %s session for patient %d needs a protected earlier %s session on the same day", procedureMap[targetID].Name, patient.PatientNumber, procedureMap[prerequisiteID].Name))
					return
				}
			}
		}
	}
	for index := range remainingPlans {
		patient := &remainingPlans[index]
		if patient.Severity == "high" {
			remainingPrerequisites := map[string]int{}
			for targetID, prerequisiteID := range patient.ProcedurePrerequisites {
				remainingPrerequisites[prerequisiteID] += patient.ProcedureCounts[targetID]
			}
			for prerequisiteID, requiredCount := range remainingPrerequisites {
				if patient.ProcedureCounts[prerequisiteID] < requiredCount {
					targetName := "сложной процедуры"
					for targetID, linkedPrerequisiteID := range patient.ProcedurePrerequisites {
						if linkedPrerequisiteID == prerequisiteID && patient.ProcedureCounts[targetID] > 0 {
							targetName = procedureMap[targetID].Name
							break
						}
					}
					writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("patient %d: осталось %d сеансов «%s», но доступно только %d сеансов расслабляющей процедуры «%s»; сохраните защищённые сеансы парой или пересоздайте расписание пары вместе", patient.PatientNumber, requiredCount, targetName, patient.ProcedureCounts[prerequisiteID], procedureMap[prerequisiteID].Name))
					return
				}
			}
		}
		active := patient.ProcedureIDs[:0:0]
		for _, id := range patient.ProcedureIDs {
			if patient.ProcedureCounts[id] > 0 {
				active = append(active, id)
			}
		}
		patient.ProcedureIDs = active
		activeIDs := map[string]bool{}
		for _, id := range active {
			activeIDs[id] = true
		}
		for targetID := range patient.ProcedurePrerequisites {
			if !activeIDs[targetID] {
				delete(patient.ProcedurePrerequisites, targetID)
			}
		}
		if err := validateCoursePrerequisites(*patient, procedureMap); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	settingsValue, err := loadSettings(ctx, tx)
	if err != nil {
		writeError(w, 500, "could not load calendar settings")
		return
	}
	var result scheduleResponse
	switch os.Getenv("CLINIC_COURSE_OPTIMIZER") {
	case "fast":
		result, err = createCourseScheduleFast(windowStart, remainingPlans, procedureMap, settingsValue, occupied, absences)
	case "cp-sat":
		result, err = createCourseScheduleCP(r.Context(), windowStart, remainingPlans, procedureMap, settingsValue, occupied, absences)
	default:
		result, err = createCourseScheduleWithAbsences(windowStart, remainingPlans, procedureMap, settingsValue, occupied, absences)
	}
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	result.UnscheduledDetails = unscheduledProcedureDetails(plan.Plans, procedureMap, result.Appointments, protectedByProcedure, settingsValue, absences)
	result.ScheduledCount += protectedCount
	result.Warnings = warnings
	if input.Preview {
		writeJSON(w, 200, result)
		return
	}
	if result.UnscheduledCount > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":                     "some procedures do not fit into patients' treatment periods",
			"scheduledCount":            result.ScheduledCount,
			"unscheduledCount":          result.UnscheduledCount,
			"unscheduledPatientNumbers": result.UnscheduledPatients,
			"unscheduledDetails":        result.UnscheduledDetails,
			"warnings":                  result.Warnings,
		})
		return
	}
	if len(result.Appointments) > 0 {
		// Pipeline insert statements in one network round-trip rather than
		// executing 1–3000 independent database round-trips. All statements
		// still run in the same serializable transaction and roll back together.
		batch := &pgx.Batch{}
		for _, item := range result.Appointments {
			batch.Queue("INSERT INTO appointments (id,title,start_at,end_at,description,color,status,generated_by_planner,manually_modified,patient_number,source_plan_date,source_procedure_id,room) VALUES ($1,$2,$3,$4,$5,$6,'scheduled',true,false,$7,$8,$9,$10)",
				item.ID, item.Title, item.StartAt, item.EndAt, item.Description, item.Color, item.PatientNumber, input.Date, item.ProcedureID, item.Room)
		}
		batchResults := tx.SendBatch(ctx, batch)
		for range result.Appointments {
			if _, err = batchResults.Exec(); err != nil {
				_ = batchResults.Close()
				writeError(w, 409, "schedule conflicts with an existing appointment")
				return
			}
		}
		if err = batchResults.Close(); err != nil {
			writeError(w, 409, "schedule conflicts with an existing appointment")
			return
		}
	}
	_, err = tx.Exec(ctx, "INSERT INTO schedule_reports (schedule_date,scheduled_count,unscheduled_count,unscheduled_patient_numbers) VALUES ($1,$2,$3,$4) ON CONFLICT (schedule_date) DO UPDATE SET scheduled_count=EXCLUDED.scheduled_count,unscheduled_count=EXCLUDED.unscheduled_count,unscheduled_patient_numbers=EXCLUDED.unscheduled_patient_numbers,updated_at=now()", input.Date, result.ScheduledCount, result.UnscheduledCount, result.UnscheduledPatients)
	if err != nil {
		writeError(w, 500, "could not save schedule report")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, 409, "schedule changed concurrently; retry generation")
		return
	}
	writeJSON(w, 200, result)
}

func (s *server) getScheduleReport(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	var report scheduleReport
	err = s.db.QueryRow(r.Context(), "SELECT to_char(schedule_date,'YYYY-MM-DD'),scheduled_count,unscheduled_count,unscheduled_patient_numbers FROM schedule_reports WHERE schedule_date=$1", parsed).Scan(&report.Date, &report.ScheduledCount, &report.UnscheduledCount, &report.UnscheduledPatients)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, scheduleReport{Date: date, UnscheduledPatients: []int{}})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load schedule report")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

var errInvalidTask = errors.New("invalid schedule task")
