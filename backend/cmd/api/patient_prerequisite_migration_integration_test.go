package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExistingPatientPrerequisitesBecomeExplicitOverridesIntegration(t *testing.T) {
	dsn := os.Getenv("CLINIC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CLINIC_TEST_DATABASE_URL not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	id, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "patient_override_migration_" + strings.ReplaceAll(id, "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("temporary schema cleanup: %v", err)
		}
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	migrations, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range migrations {
		if filepath.Base(path) == "014_patient_prerequisite_overrides.sql" {
			break
		}
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("legacy fixture migration %s: %v", path, err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO procedures (id,name,duration_minutes,is_relaxing) VALUES ('legacy-relax','Relax',30,TRUE),('legacy-target','Target',30,FALSE)`,
		`INSERT INTO day_plans (schedule_date,revision) VALUES ('2099-01-05',1)`,
		`INSERT INTO day_plan_patients (schedule_date,patient_number,severity,stay_days,arrival_date,discharge_date)
		 VALUES ('2099-01-05',1,'high',5,'2099-01-05','2099-01-09')`,
		`INSERT INTO day_plan_items(schedule_date,patient_number,procedure_id,position,session_count,prerequisite_procedure_id)
		 VALUES ('2099-01-05',1,'legacy-target',0,1,'legacy-relax'),('2099-01-05',1,'legacy-relax',1,1,NULL)`,
	} {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	migration, err := os.ReadFile("../../migrations/014_patient_prerequisite_overrides.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("backfill migration failed: %v", err)
	}
	plan, err := loadDayPlan(ctx, db, "2099-01-05")
	if err != nil || len(plan.Plans) != 1 {
		t.Fatalf("load migrated plan: %+v (%v)", plan, err)
	}
	patient := plan.Plans[0]
	if patient.ProcedurePrerequisites["legacy-target"] != "legacy-relax" {
		t.Fatalf("legacy effective prerequisite lost: %+v", patient)
	}
	linked, exists := patient.ProcedurePrerequisiteOverrides["legacy-target"]
	if !exists || linked == nil || *linked != "legacy-relax" {
		t.Fatalf("legacy prerequisite was not made explicit: %+v", patient.ProcedurePrerequisiteOverrides)
	}
	if _, exists := patient.ProcedurePrerequisiteOverrides["legacy-relax"]; exists {
		t.Fatal("unlinked relaxing treatment should not get an override")
	}
}
