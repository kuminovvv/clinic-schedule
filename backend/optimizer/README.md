# Experimental course schedulers (local only)

## Fast Go scheduler (recommended for interactive use)

`CLINIC_COURSE_OPTIMIZER=fast` selects a deterministic Go heuristic:

- No Python/OR-Tools process startup, external worker or fixed time-budget wait.
- Supports the existing API limit of **3,000 requested sessions** and patient
  stays up to **365 days**, instead of the CP-SAT prototype's 80/31 cap.
- Balances the number of sessions between available days, packs the patient's
  sessions into reasonably short visits, respects work windows, closures,
  absences, manual sessions, minimum repetition intervals, the catalog's
  rolling seven-day maximum and severity-specific relaxing-first ordering.
- Reports unscheduled sessions when no valid slot exists rather than violating
  restrictions. Like CP-SAT, it has **no knowledge of doctors, rooms or equipment**
  because those are not modeled by the application.
- It is a **heuristic**: unlike a fully solved CP-SAT problem, the result is not
  a proof of global optimality, and it might miss a feasible allocation on a
  very constrained course. It must not set clinical treatment policy.

The local launcher below now selects this fast algorithm by default. To
compare the CP-SAT prototype, set `CLINIC_COURSE_OPTIMIZER=cp-sat` before
launching the Go backend directly (the launcher intentionally selects fast).
Neither change affects the production Docker images or their environment.

## Optional CP-SAT prototype

This is a **local, opt-in prototype**, not a medical treatment recommender and
not part of the deployment Docker images. Default behavior is unchanged.

## Enable on Windows (PowerShell)

From the repository root:

```powershell
py -m venv backend\.venv-cpsat
backend\.venv-cpsat\Scripts\python.exe -m pip install -r backend\optimizer\requirements.txt
powershell -NoProfile -ExecutionPolicy Bypass -File backend\optimizer\start-local.ps1
```

The local PowerShell launcher loads the already configured ignored
`backend/.env.local` and selects `CLINIC_COURSE_OPTIMIZER=fast`, without
printing the database password. Stop the existing Go API on port 8081 before
running it; PostgreSQL and the Next.js dev server need not restart.

The Go API loads `backend/optimizer/cpsat_course.py` **only when the CP-SAT
flag is chosen** and it is launched with
`backend` as its working directory. Otherwise set `CLINIC_OPTIMIZER_SCRIPT` and
`CLINIC_OPTIMIZER_PYTHON` to the full script and interpreter paths. **Restart
the local Go API** after setting the flag. Leave frontend and PostgreSQL
configured as before. Generating a course in the UI then uses the new solver;
it still writes to the **local** database. Use the preview endpoint / test
data to compare outputs without replacing saved appointments.

To return to the original algorithm, remove `CLINIC_COURSE_OPTIMIZER` from the
backend environment and restart. Nothing here changes production services,
GitHub Actions, or deployed images.

## Constraints and objective

- Each requested procedure occurrence is optional only when the course cannot
  be fully fitted; unscheduled sessions are reported. No additional treatment
  sessions are created.
- Calendar windows, closure periods, patient absences, existing appointments,
  total duration including preparation, minimum calendar-day intervals, and
  rolling seven-day maximum from the procedure catalog are hard constraints.
- The same patient's appointments never overlap. An appointment with no
  patient blocks all patients. Different patients may be scheduled concurrently
  **because specialists/rooms are not represented in the current data model**.
- High-severity patients receive relaxing procedures first on shared days.
- The objective prioritizes fitting sessions, then balances daily treatment
  load, reduces patient waiting within a day, and prefers early appointment
  starts. The solver normally searches for 2 seconds (previously 8 seconds);
  if it cannot find a feasible solution in that interval, it retries for up to
  6 additional seconds. Mathematical optimality is **not guaranteed**.

**Not implemented:** physician/room/equipment assignment, avoidance of doctor
idle time, treatment compatibility/contraindications, or evidence-based daily
maximums. The treating clinician must supply medically valid treatment plans.

The experimental guard limits a course to 31 days and 80 appointments; larger
requests receive a clear error rather than silently using the old algorithm.

## Verify

```powershell
backend\.venv-cpsat\Scripts\python.exe -m unittest discover -s backend\optimizer -v
go -C backend test ./...
go -C backend test -run TestLocalCPOptimizerSpreadsDemoCourse -v ./cmd/api
```

The last test compares the same 16-session, 14-day schedule under both
algorithms. Existing tests continue to run without Python installed in CI.
