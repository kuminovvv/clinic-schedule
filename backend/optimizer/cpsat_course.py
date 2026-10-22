"""Experimental CP-SAT treatment-course scheduler, communicating over JSON/stdin.

No patient names or other identifying details are required. This is a scheduling
optimizer, NOT a source of medical treatment recommendations. The clinic must
define medically valid counts, intervals, and combinations before using it.
"""

from __future__ import annotations

import json
import sys
from collections import defaultdict
from datetime import date, datetime, timedelta

from ortools.sat.python import cp_model

DATE = "%Y-%m-%d"
STAMP = "%Y-%m-%dT%H:%M"
MAX_SESSIONS = 80
MAX_STAY_DAYS = 31


def clock(text: str) -> int:
    hour, minute = map(int, text.split(":"))
    return hour * 60 + minute


def overlaps(start: int, end: int, block: tuple[int, int]) -> bool:
    return start < block[1] and end > block[0]


def workdays(data: dict) -> dict[int, dict]:
    configured = data.get("settings", {}).get("workWeek", [])
    if len(configured) == 7:
        return {d["weekday"]: d for d in configured}
    # Mirrors settings.week() for pre-existing callers without persisted settings.
    return {
        wd: {"open": True, "start": "09:00", "end": "16:00",
             "breaks": [{"start": "13:00", "end": "14:00"}]}
        for wd in range(1, 8)
    }


def solve(data: dict) -> dict:
    first_date = date.fromisoformat(data["startDate"])
    plans = data["plans"]
    procedures = data["procedures"]
    absences = data.get("absences") or {}
    existing = data.get("occupied") or []
    step = data.get("settings", {}).get("timeStepMinutes", 10) or 10
    if not 1 <= step <= 120:
        raise ValueError("Invalid time step")
    if not plans:
        return {"appointments": [], "scheduledCount": 0, "unscheduledCount": 0,
                "unscheduledPatientNumbers": []}
    if max((p["stayDays"] for p in plans), default=0) > MAX_STAY_DAYS:
        raise ValueError(f"Experimental solver supports at most {MAX_STAY_DAYS} days")
    count = sum(int(p["procedureCounts"][pid]) for p in plans for pid in p["procedureIds"])
    if count > MAX_SESSIONS:
        raise ValueError(f"Experimental solver supports at most {MAX_SESSIONS} sessions")

    model = cp_model.CpModel()
    week = workdays(data)
    max_days = max(p["stayDays"] for p in plans)
    days = [first_date + timedelta(days=d) for d in range(max_days)]

    # An occupied slot without a patient ID blocks all patients; otherwise the
    # existing appointment only blocks that patient (same semantics as Go).
    occupied_by_day: dict[str, list[dict]] = defaultdict(list)
    for item in existing:
        occupied_by_day[item["startAt"][:10]].append(item)

    # CP-SAT uses an optional time interval for each possible appointment day.
    # NoOverlap prevents a patient's procedures from colliding.
    intervals: dict[tuple[int, int], list] = defaultdict(list)
    per_patient_day: dict[tuple[int, int], list] = defaultdict(list)
    per_procedure: dict[tuple[int, str], list] = defaultdict(list)
    chosen: list[dict] = []
    active: list = []
    late_vars = []
    for plan in sorted(plans, key=lambda p: ({"high": 0, "medium": 1, "low": 2}.get(p["severity"], 3), p["patientNumber"])):
        patient = plan["patientNumber"]
        for proc_id in plan["procedureIds"]:
            proc = procedures[proc_id]
            duration = proc["durationMinutes"] + proc.get("preparationMinutes", 0)
            interval_days = proc["minIntervalDays"]
            requested = plan["procedureCounts"][proc_id]
            fixed_days = [date.fromisoformat(slot["startAt"][:10])
                          for slot in existing
                          if slot.get("patientNumber") == patient
                          and slot.get("procedureId") == proc_id
                          and slot.get("sourcePlanDate", "") in ("", data["startDate"])]
            previous = None
            for repeat in range(requested):
                key = f"p{patient}_{proc_id}_{repeat}"
                present = model.new_bool_var(f"present_{key}")
                day_var = model.new_int_var(0, plan["stayDays"] - 1, f"date_{key}")
                choices = []
                for d in range(plan["stayDays"]):
                    day = days[d]
                    day_str = day.isoformat()
                    if absences.get(str(patient), {}).get(day_str, False):
                        continue
                    if any(abs((day - fixed).days) < interval_days for fixed in fixed_days):
                        continue
                    working = week[day.isoweekday()]
                    if not working["open"]:
                        continue
                    start_work, end_work = clock(working["start"]), clock(working["end"])
                    blocks = [(clock(b["start"]), clock(b["end"])) for b in working.get("breaks", [])]
                    for slot in occupied_by_day[day_str]:
                        if slot.get("patientNumber") is None or slot.get("patientNumber") == patient:
                            t_start = datetime.fromisoformat(slot["startAt"])
                            t_end = datetime.fromisoformat(slot["endAt"])
                            blocks.append((t_start.hour * 60 + t_start.minute,
                                           t_end.hour * 60 + t_end.minute))
                    allowed = [minute for minute in range(start_work, end_work - duration + 1, step)
                               if all(not overlaps(minute, minute + duration, b) for b in blocks)]
                    if not allowed:
                        continue
                    take = model.new_bool_var(f"take_{key}_d{d}")
                    begin = model.new_int_var_from_domain(cp_model.Domain.from_values(allowed), f"begin_{key}_d{d}")
                    opt = model.new_optional_fixed_size_interval_var(begin, duration, take, f"slot_{key}_d{d}")
                    intervals[patient, d].append(opt)
                    item = {"patient": patient, "proc": proc_id, "repeat": repeat,
                            "day": d, "take": take, "begin": begin, "duration": duration,
                            "relax": bool(proc.get("isRelaxing", False)), "severity": plan["severity"]}
                    per_patient_day[patient, d].append(item)
                    per_procedure[patient, proc_id].append(item)
                    chosen.append(item)
                    choices.append(item)
                    delay = model.new_int_var(0, 1440, f"delay_{key}_d{d}")
                    model.add(delay == begin - start_work).only_enforce_if(take)
                    model.add(delay == 0).only_enforce_if(take.negated())
                    late_vars.append(delay)
                model.add(sum(item["take"] for item in choices) == present)
                model.add(day_var == sum(item["day"] * item["take"] for item in choices))
                active.append(present)
                if previous is not None:
                    model.add(present <= previous[0])
                    model.add(day_var >= previous[1] + interval_days).only_enforce_if(present)
                previous = (present, day_var)

            # The catalog's weekly frequency is an upper bound, not a demand
            # to create additional sessions. Check every rolling seven-day span.
            weekly_limit = proc.get("defaultWeeklyCount", 7) or 7
            for offset in range(plan["stayDays"]):
                fixed_in_window = sum(offset <= (fixed - first_date).days < offset + 7 for fixed in fixed_days)
                relevant = [i["take"] for i in per_procedure[patient, proc_id]
                            if offset <= i["day"] < offset + 7]
                if relevant:
                    model.add(sum(relevant) <= max(0, weekly_limit - fixed_in_window))

    # A schedule can be partially feasible. Maximize scheduled sessions first;
    # otherwise we'd get an empty but pleasantly balanced calendar.
    dispersion = []
    patient_gaps = []
    for plan in plans:
        patient = plan["patientNumber"]
        for d in range(plan["stayDays"]):
            entries = per_patient_day[patient, d]
            if not entries:
                continue
            model.add_no_overlap(intervals[patient, d])
            load = model.new_int_var(0, len(entries), f"load_p{patient}_d{d}")
            model.add(load == sum(i["take"] for i in entries))
            squared = model.new_int_var(0, len(entries) ** 2, f"load_squared_p{patient}_d{d}")
            model.add_multiplication_equality(squared, [load, load])
            dispersion.append(squared)

            has_day = model.new_bool_var(f"has_day_p{patient}_d{d}")
            model.add(load >= 1).only_enforce_if(has_day)
            model.add(load == 0).only_enforce_if(has_day.negated())
            begins, ends = [], []
            for e in entries:
                s = model.new_int_var(0, 1440, f"optional_start_p{patient}_d{d}_{e['proc']}_{e['repeat']}")
                finish = model.new_int_var(0, 1440, f"optional_end_p{patient}_d{d}_{e['proc']}_{e['repeat']}")
                model.add(s == e["begin"]).only_enforce_if(e["take"])
                model.add(s == 1440).only_enforce_if(e["take"].negated())
                model.add(finish == e["begin"] + e["duration"]).only_enforce_if(e["take"])
                model.add(finish == 0).only_enforce_if(e["take"].negated())
                begins.append(s)
                ends.append(finish)
            first = model.new_int_var(0, 1440, f"first_p{patient}_d{d}")
            last = model.new_int_var(0, 1440, f"last_p{patient}_d{d}")
            model.add_min_equality(first, begins)
            model.add_max_equality(last, ends)
            gap = model.new_int_var(0, 1440, f"gap_p{patient}_d{d}")
            model.add(gap == last - first - sum(i["duration"] * i["take"] for i in entries)).only_enforce_if(has_day)
            model.add(gap == 0).only_enforce_if(has_day.negated())
            patient_gaps.append(gap)

            if plan["severity"] == "high":
                relaxed = [i for i in entries if i["relax"]]
                normal = [i for i in entries if not i["relax"]]
                for relax in relaxed:
                    for other in normal:
                        model.add(relax["begin"] + relax["duration"] <= other["begin"]).only_enforce_if(
                            [relax["take"], other["take"]])

    model.maximize(5_000_000 * sum(active)
                   - 400 * sum(dispersion)
                   - 4 * sum(patient_gaps)
                   - sum(late_vars))

    solver = cp_model.CpSolver()
    # CP-SAT rarely proves global optimality for the combined date/time model.
    # With a responsive local preview, two seconds is a better default than
    # repeatedly waiting eight seconds for a marginal improvement.
    solver.parameters.max_time_in_seconds = 2.0
    solver.parameters.num_search_workers = 4
    solver.parameters.random_seed = 1
    status = solver.solve(model)
    if status == cp_model.UNKNOWN:
        # An unusually constrained course may need more time to find its
        # first feasible solution. Keep the fast path for ordinary previews,
        # but avoid turning a time-budget hit into a misleading hard failure.
        solver.parameters.max_time_in_seconds = 6.0
        status = solver.solve(model)
    if status not in (cp_model.OPTIMAL, cp_model.FEASIBLE):
        raise RuntimeError(f"CP-SAT produced no feasible schedule (status {solver.status_name(status)})")

    result = []
    for c in chosen:
        if solver.value(c["take"]):
            minute = solver.value(c["begin"])
            begin = datetime.combine(days[c["day"]], datetime.min.time()) + timedelta(minutes=minute)
            end = begin + timedelta(minutes=c["duration"])
            result.append({"patientNumber": c["patient"], "procedureId": c["proc"],
                           "startAt": begin.strftime(STAMP), "endAt": end.strftime(STAMP)})
    result.sort(key=lambda r: (r["startAt"], r["patientNumber"], r["procedureId"]))
    unscheduled = count - len(result)
    delivered = defaultdict(int)
    for item in result:
        delivered[item["patientNumber"], item["procedureId"]] += 1
    missing_patients = sorted({p["patientNumber"] for p in plans for pid in p["procedureIds"]
                               if delivered[p["patientNumber"], pid] < p["procedureCounts"][pid]})
    return {"appointments": result, "scheduledCount": len(result),
            "unscheduledCount": unscheduled, "unscheduledPatientNumbers": missing_patients,
            "optimizerStatus": solver.status_name(status)}


def main() -> None:
    try:
        data = json.load(sys.stdin)
        json.dump(solve(data), sys.stdout, ensure_ascii=False)
        sys.stdout.write("\n")
    except (ValueError, KeyError, TypeError, RuntimeError) as exc:
        print(f"optimizer error: {exc}", file=sys.stderr)
        sys.exit(2)


if __name__ == "__main__":
    main()
