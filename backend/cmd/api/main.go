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
}

type procedure struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	DurationMinutes    int    `json:"durationMinutes"`
	IsRelaxing         bool   `json:"isRelaxing"`
	MinIntervalDays    int    `json:"minIntervalDays"`
	DefaultWeeklyCount int    `json:"defaultWeeklyCount"`
}

type settings struct {
	TimeStepMinutes  int `json:"timeStepMinutes"`
	DisplayStartHour int `json:"displayStartHour"`
	DisplayEndHour   int `json:"displayEndHour"`
}

type scheduleTask struct {
	PatientNumber int    `json:"patientNumber"`
	Severity      string `json:"severity"`
	ProcedureID   string `json:"procedureId"`
}

type patientPlan struct {
	PatientNumber   int            `json:"patientNumber"`
	Severity        string         `json:"severity"`
	ProcedureIDs    []string       `json:"procedureIds"`
	StayDays        int            `json:"stayDays"`
	ProcedureCounts map[string]int `json:"procedureCounts"`
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
	Appointments        []appointment `json:"appointments"`
	ScheduledCount      int           `json:"scheduledCount"`
	UnscheduledCount    int           `json:"unscheduledCount"`
	UnscheduledPatients []int         `json:"unscheduledPatientNumbers"`
	Warnings            []string      `json:"warnings"`
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
	mux.HandleFunc("GET /v1/schedule-report", api.getScheduleReport)
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
		_, err := db.Exec(ctx, "INSERT INTO procedures (id, name, duration_minutes) VALUES ($1, $2, 50) ON CONFLICT (id) DO NOTHING", fmt.Sprintf("procedure-%d", i), fmt.Sprintf("Процедура %d", i))
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
	rows, err := s.db.Query(r.Context(), `SELECT id, title, to_char(start_at, 'YYYY-MM-DD"T"HH24:MI'), to_char(end_at, 'YYYY-MM-DD"T"HH24:MI'), description, color, status, generated_by_planner, manually_modified, patient_number,COALESCE(source_procedure_id,''),COALESCE(to_char(source_plan_date,'YYYY-MM-DD'),'') FROM appointments WHERE start_at >= $1 AND start_at < $2 ORDER BY start_at`, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load appointments")
		return
	}
	defer rows.Close()
	items := make([]appointment, 0)
	for rows.Next() {
		var item appointment
		if err := rows.Scan(&item.ID, &item.Title, &item.StartAt, &item.EndAt, &item.Description, &item.Color, &item.Status, &item.GeneratedByPlanner, &item.ManuallyModified, &item.PatientNumber, &item.ProcedureID, &item.SourcePlanDate); err != nil {
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
	if !validAppointment(item) {
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
	if _, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtext($1))", item.StartAt[:10]); err != nil {
		writeError(w, http.StatusInternalServerError, "could not lock appointment day")
		return
	}
	var procedureID string
	var sourcePlanDate string
	err = tx.QueryRow(r.Context(), "SELECT COALESCE(source_procedure_id,''),COALESCE(to_char(source_plan_date,'YYYY-MM-DD'),'') FROM appointments WHERE id=$1 FOR UPDATE", item.ID).Scan(&procedureID, &sourcePlanDate)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "could not load appointment procedure")
		return
	}
	item.ProcedureID = procedureID
	item.SourcePlanDate = sourcePlanDate
	if sourcePlanDate != "" && item.PatientNumber != nil && item.Status != "cancelled" {
		var stayDays int
		err := tx.QueryRow(r.Context(), "SELECT stay_days FROM day_plan_patients WHERE schedule_date=$1 AND patient_number=$2", sourcePlanDate, item.PatientNumber).Scan(&stayDays)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 500, "could not validate patient stay")
			return
		}
		if err == nil {
			admitted, _ := time.Parse("2006-01-02", sourcePlanDate)
			appointmentDay, _ := time.Parse("2006-01-02", item.StartAt[:10])
			if appointmentDay.Before(admitted) || !appointmentDay.Before(admitted.AddDate(0, 0, stayDays)) {
				writeError(w, http.StatusUnprocessableEntity, "appointment is outside patient treatment period")
				return
			}
		}
	}
	if procedureID != "" && item.PatientNumber != nil && item.Status != "cancelled" {
		var minInterval int
		if err := tx.QueryRow(r.Context(), "SELECT min_interval_days FROM procedures WHERE id=$1", procedureID).Scan(&minInterval); err != nil {
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
			writeError(w, 409, "procedure cannot repeat before its minimum interval")
			return
		}
	}
	var conflict bool
	err = tx.QueryRow(r.Context(), "SELECT EXISTS (SELECT 1 FROM appointments WHERE id <> $1 AND status <> 'cancelled' AND start_at < $3 AND end_at > $2 AND ($4::integer IS NULL OR patient_number IS NULL OR (patient_number=$4 AND ($5::text='' OR source_plan_date IS NULL OR source_plan_date=$5::date))))", item.ID, start, end, item.PatientNumber, sourcePlanDate).Scan(&conflict)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not validate appointment time")
		return
	}
	if conflict {
		writeError(w, http.StatusConflict, "appointment time is already occupied")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO appointments (id,title,start_at,end_at,description,color,status,generated_by_planner,manually_modified,patient_number) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title,start_at=EXCLUDED.start_at,end_at=EXCLUDED.end_at,description=EXCLUDED.description,color=EXCLUDED.color,status=EXCLUDED.status,generated_by_planner=EXCLUDED.generated_by_planner,manually_modified=appointments.manually_modified OR EXCLUDED.manually_modified OR (appointments.generated_by_planner AND (appointments.title<>EXCLUDED.title OR appointments.start_at<>EXCLUDED.start_at OR appointments.end_at<>EXCLUDED.end_at OR appointments.description<>EXCLUDED.description OR appointments.color<>EXCLUDED.color)),patient_number=EXCLUDED.patient_number,updated_at=now()`, item.ID, item.Title, item.StartAt, item.EndAt, item.Description, item.Color, item.Status, item.GeneratedByPlanner, item.ManuallyModified, item.PatientNumber)
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
	_, err := s.db.Exec(r.Context(), "UPDATE appointments SET status='cancelled', manually_modified=manually_modified OR generated_by_planner, updated_at=now() WHERE id=$1", r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not cancel appointment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listProcedures(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), "SELECT id,name,duration_minutes,is_relaxing,min_interval_days,default_weekly_count FROM procedures ORDER BY id")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load procedures")
		return
	}
	defer rows.Close()
	items := make([]procedure, 0)
	for rows.Next() {
		var item procedure
		if err := rows.Scan(&item.ID, &item.Name, &item.DurationMinutes, &item.IsRelaxing, &item.MinIntervalDays, &item.DefaultWeeklyCount); err != nil {
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
	var items []procedure
	if !decode(w, r, &items) {
		return
	}
	if len(items) > 500 {
		writeError(w, http.StatusBadRequest, "too many procedures")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not save procedures")
		return
	}
	defer tx.Rollback(r.Context())
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" || len([]rune(item.Name)) > 480 || item.DurationMinutes < 1 || item.DurationMinutes > 420 || item.MinIntervalDays < 1 || item.MinIntervalDays > 365 || item.DefaultWeeklyCount < 1 || item.DefaultWeeklyCount > 7 {
			writeError(w, 400, "invalid procedure")
			return
		}
		_, err = tx.Exec(r.Context(), "INSERT INTO procedures (id,name,duration_minutes,is_relaxing,min_interval_days,default_weekly_count) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name,duration_minutes=EXCLUDED.duration_minutes,is_relaxing=EXCLUDED.is_relaxing,min_interval_days=EXCLUDED.min_interval_days,default_weekly_count=EXCLUDED.default_weekly_count,updated_at=now()", item.ID, strings.TrimSpace(item.Name), item.DurationMinutes, item.IsRelaxing, item.MinIntervalDays, item.DefaultWeeklyCount)
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
	var referenced bool
	if err := s.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM day_plan_items WHERE procedure_id=$1)", r.PathValue("id")).Scan(&referenced); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check procedure usage")
		return
	}
	if referenced {
		writeError(w, http.StatusConflict, "procedure is used in one or more patient plans")
		return
	}
	if _, err := s.db.Exec(r.Context(), "DELETE FROM procedures WHERE id=$1", r.PathValue("id")); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23503" {
			writeError(w, http.StatusConflict, "procedure is used in one or more patient plans")
			return
		}
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
		return dayPlan{Date: date, SelectedPatientNumber: &selectedPatient, Plans: []patientPlan{{PatientNumber: 1, Severity: "medium", StayDays: 7, ProcedureIDs: []string{}, ProcedureCounts: map[string]int{}}}}, nil
	}
	if err != nil {
		return plan, err
	}
	rows, err := db.Query(ctx, "SELECT p.patient_number,p.severity,p.stay_days,i.procedure_id,i.session_count FROM day_plan_patients p LEFT JOIN day_plan_items i ON i.schedule_date=p.schedule_date AND i.patient_number=p.patient_number WHERE p.schedule_date=$1 ORDER BY p.patient_number,i.position", date)
	if err != nil {
		return plan, err
	}
	defer rows.Close()
	byPatient := map[int]int{}
	for rows.Next() {
		var patientNumber int
		var severity string
		var stayDays int
		var procedureID *string
		var sessionCount *int
		if err := rows.Scan(&patientNumber, &severity, &stayDays, &procedureID, &sessionCount); err != nil {
			return plan, err
		}
		index, exists := byPatient[patientNumber]
		if !exists {
			index = len(plan.Plans)
			byPatient[patientNumber] = index
			plan.Plans = append(plan.Plans, patientPlan{PatientNumber: patientNumber, Severity: severity, StayDays: stayDays, ProcedureIDs: []string{}, ProcedureCounts: map[string]int{}})
		}
		if procedureID != nil {
			plan.Plans[index].ProcedureIDs = append(plan.Plans[index].ProcedureIDs, *procedureID)
			plan.Plans[index].ProcedureCounts[*procedureID] = *sessionCount
		}
	}
	return plan, rows.Err()
}

func (s *server) saveDayPlan(w http.ResponseWriter, r *http.Request) {
	var plan dayPlan
	if !decode(w, r, &plan) {
		return
	}
	day, err := time.Parse("2006-01-02", plan.Date)
	if err != nil || plan.Revision < 0 || len(plan.Plans) > 100 {
		writeError(w, http.StatusBadRequest, "invalid day plan")
		return
	}
	seenPatients := map[int]bool{}
	procedureIDs := map[string]bool{}
	for _, patient := range plan.Plans {
		if patient.PatientNumber < 1 || seenPatients[patient.PatientNumber] || (patient.Severity != "high" && patient.Severity != "medium" && patient.Severity != "low") || len(patient.ProcedureIDs) > 500 || patient.StayDays < 1 || patient.StayDays > 365 || len(patient.ProcedureCounts) != len(patient.ProcedureIDs) {
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
	procedureIDList := make([]string, 0, len(procedureIDs))
	for procedureID := range procedureIDs {
		procedureIDList = append(procedureIDList, procedureID)
	}
	var existingProcedureCount int
	if err := tx.QueryRow(r.Context(), "SELECT count(*) FROM procedures WHERE id=ANY($1)", procedureIDList).Scan(&existingProcedureCount); err != nil || existingProcedureCount != len(procedureIDList) {
		writeError(w, http.StatusBadRequest, "unknown procedure in patient plan")
		return
	}
	plan.Revision++
	_, err = tx.Exec(r.Context(), "INSERT INTO day_plans(schedule_date,revision,selected_patient_number,updated_at) VALUES($1,$2,$3,now()) ON CONFLICT(schedule_date) DO UPDATE SET revision=EXCLUDED.revision,selected_patient_number=EXCLUDED.selected_patient_number,updated_at=now()", day, plan.Revision, plan.SelectedPatientNumber)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save patient plan")
		return
	}
	if _, err = tx.Exec(r.Context(), "DELETE FROM day_plan_patients WHERE schedule_date=$1", day); err != nil {
		writeError(w, http.StatusInternalServerError, "could not replace patient plans")
		return
	}
	for _, patient := range plan.Plans {
		if _, err = tx.Exec(r.Context(), "INSERT INTO day_plan_patients(schedule_date,patient_number,severity,stay_days) VALUES($1,$2,$3,$4)", day, patient.PatientNumber, patient.Severity, patient.StayDays); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save patient")
			return
		}
		for index, procedureID := range patient.ProcedureIDs {
			if _, err = tx.Exec(r.Context(), "INSERT INTO day_plan_items(schedule_date,patient_number,procedure_id,position,session_count) VALUES($1,$2,$3,$4,$5)", day, patient.PatientNumber, procedureID, index, patient.ProcedureCounts[procedureID]); err != nil {
				writeError(w, http.StatusInternalServerError, "could not save patient procedures")
				return
			}
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusConflict, "patient plan changed concurrently")
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *server) getSettings(w http.ResponseWriter, r *http.Request) {
	var value settings
	err := s.db.QueryRow(r.Context(), "SELECT time_step_minutes,display_start_hour,display_end_hour FROM calendar_settings WHERE id=1").Scan(&value.TimeStepMinutes, &value.DisplayStartHour, &value.DisplayEndHour)
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
	if (value.TimeStepMinutes != 5 && value.TimeStepMinutes != 10 && value.TimeStepMinutes != 15 && value.TimeStepMinutes != 30) || value.DisplayStartHour < 0 || value.DisplayStartHour > 23 || value.DisplayEndHour <= value.DisplayStartHour || value.DisplayEndHour > 24 {
		writeError(w, 400, "invalid calendar settings")
		return
	}
	_, err := s.db.Exec(r.Context(), "UPDATE calendar_settings SET time_step_minutes=$1,display_start_hour=$2,display_end_hour=$3 WHERE id=1", value.TimeStepMinutes, value.DisplayStartHour, value.DisplayEndHour)
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
	startMinutes := start.Hour()*60 + start.Minute()
	endMinutes := end.Hour()*60 + end.Minute()
	return item.ID != "" && strings.TrimSpace(item.Title) != "" && len([]rune(item.Title)) <= 512 && err1 == nil && err2 == nil && start.Before(end) && start.Format("2006-01-02") == end.Format("2006-01-02") && startMinutes >= 9*60 && endMinutes <= 16*60 && !(startMinutes < 14*60 && endMinutes > 13*60) && validColor(item.Color) && (item.Status == "" || item.Status == "scheduled" || item.Status == "cancelled") && len([]rune(item.Description)) <= 1000
}
func validColor(color string) bool {
	return color == "blue" || color == "teal" || color == "orange" || color == "purple"
}

func (s *server) generate(w http.ResponseWriter, r *http.Request) {
	var input scheduleRequest
	if !decode(w, r, &input) {
		return
	}
	day, err := time.Parse("2006-01-02", input.Date)
	if err != nil {
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
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", input.Date); err != nil {
		writeError(w, 500, "could not lock schedule")
		return
	}
	plan, err := loadDayPlan(ctx, tx, input.Date)
	if err != nil {
		writeError(w, 500, "could not load patient plans")
		return
	}
	maxStayDays := 1
	for _, patient := range plan.Plans {
		if patient.StayDays > maxStayDays {
			maxStayDays = patient.StayDays
		}
	}
	endDate := day.AddDate(0, 0, maxStayDays)
	if _, err = tx.Exec(ctx, "DELETE FROM appointments WHERE generated_by_planner=true AND manually_modified=false AND (source_plan_date=$1 OR (source_plan_date IS NULL AND start_at >= $1 AND start_at < $2))", day, day.AddDate(0, 0, 1)); err != nil {
		writeError(w, 500, "could not replace generated schedule")
		return
	}
	occupiedRows, err := tx.Query(ctx, "SELECT start_at,end_at,patient_number,COALESCE(source_procedure_id,''),COALESCE(to_char(source_plan_date,'YYYY-MM-DD'),'') FROM appointments WHERE status <> 'cancelled' AND start_at >= $1 AND start_at < $2", day.AddDate(0, 0, -365), endDate)
	if err != nil {
		writeError(w, 500, "could not load existing appointments")
		return
	}
	occupied := make([]timeSlot, 0)
	for occupiedRows.Next() {
		var slot timeSlot
		if err = occupiedRows.Scan(&slot.start, &slot.end, &slot.patient, &slot.procedureID, &slot.sourcePlanDate); err != nil {
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
	rows, err := tx.Query(ctx, "SELECT id,name,duration_minutes,is_relaxing,min_interval_days,default_weekly_count FROM procedures")
	if err != nil {
		writeError(w, 500, "could not load procedures")
		return
	}
	procedureMap := map[string]procedure{}
	for rows.Next() {
		var p procedure
		if err = rows.Scan(&p.ID, &p.Name, &p.DurationMinutes, &p.IsRelaxing, &p.MinIntervalDays, &p.DefaultWeeklyCount); err != nil {
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
	if totalSessions > 3000 {
		writeError(w, 400, "too many sessions in one treatment plan (maximum 3000)")
		return
	}
	warnings := courseWarnings(plan.Plans, procedureMap)
	protectedRows, err := tx.Query(ctx, `SELECT id,patient_number,title,source_procedure_id FROM appointments
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
	}
	protected := []protectedAppointment{}
	for protectedRows.Next() {
		var item protectedAppointment
		if err = protectedRows.Scan(&item.id, &item.patient, &item.title, &item.procedureID); err != nil {
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
	remainingPlans := make([]patientPlan, len(plan.Plans))
	for index, patient := range plan.Plans {
		remainingPlans[index] = patient
		remainingPlans[index].ProcedureCounts = make(map[string]int, len(patient.ProcedureCounts))
		for id, count := range patient.ProcedureCounts {
			remainingPlans[index].ProcedureCounts[id] = count
		}
	}
	for _, kept := range protected {
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
				patient.ProcedureCounts[id]--
				if kept.procedureID == nil {
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
		active := patient.ProcedureIDs[:0:0]
		for _, id := range patient.ProcedureIDs {
			if patient.ProcedureCounts[id] > 0 {
				active = append(active, id)
			}
		}
		patient.ProcedureIDs = active
	}
	settingsValue := settings{TimeStepMinutes: 10, DisplayStartHour: 9, DisplayEndHour: 16}
	if err := tx.QueryRow(ctx, "SELECT time_step_minutes FROM calendar_settings WHERE id=1").Scan(&settingsValue.TimeStepMinutes); err != nil {
		writeError(w, 500, "could not load calendar settings")
		return
	}
	result, err := createCourseSchedule(day, remainingPlans, procedureMap, settingsValue, occupied)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
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
			"warnings":                  result.Warnings,
		})
		return
	}
	for _, item := range result.Appointments {
		_, err = tx.Exec(ctx, "INSERT INTO appointments (id,title,start_at,end_at,description,color,status,generated_by_planner,manually_modified,patient_number,source_plan_date,source_procedure_id) VALUES ($1,$2,$3,$4,$5,$6,'scheduled',true,false,$7,$8,$9)", item.ID, item.Title, item.StartAt, item.EndAt, item.Description, item.Color, item.PatientNumber, input.Date, item.ProcedureID)
		if err != nil {
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
