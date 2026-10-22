import unittest
from collections import Counter
from datetime import date, timedelta

from cpsat_course import solve


def week():
    return [
        {"weekday": day, "open": day <= 5, "start": "09:00", "end": "16:00",
         "breaks": [{"start": "13:00", "end": "14:00"}] if day <= 5 else []}
        for day in range(1, 8)
    ]


def fixture(counts=(3, 3, 2, 3, 2, 3), stay=14):
    ids = [f"p{i}" for i in range(len(counts))]
    return {
        "startDate": "2026-10-22",
        "settings": {"timeStepMinutes": 10, "workWeek": week()},
        "plans": [{"patientNumber": 1, "severity": "medium", "stayDays": stay,
                   "procedureIds": ids,
                   "procedureCounts": dict(zip(ids, counts))}],
        "procedures": {
            pid: {"durationMinutes": 30, "preparationMinutes": 20,
                  "minIntervalDays": 1, "defaultWeeklyCount": 7, "isRelaxing": False}
            for pid in ids
        },
        "occupied": [], "absences": {},
    }


class CourseSATTest(unittest.TestCase):
    def test_two_week_course_spread_and_compact(self):
        data = fixture()
        output = solve(data)
        sessions = output["appointments"]
        self.assertEqual(output["scheduledCount"], 16)
        self.assertEqual(output["unscheduledCount"], 0)
        days = Counter(item["startAt"][:10] for item in sessions)
        self.assertGreaterEqual(len(days), 7, f"Insufficiently distributed: {days}")
        self.assertLessEqual(max(days.values()), 3, f"Overloaded days: {days}")
        for day, count in days.items():
            self.assertLessEqual(date.fromisoformat(day), date(2026, 11, 4))
            self.assertLessEqual(date.fromisoformat(day).weekday(), 4)
        for i, session in enumerate(sessions):
            self.assertFalse("13:00" <= session["startAt"][11:] < "14:00")
            for previous in sessions[:i]:
                if previous["startAt"][:10] == session["startAt"][:10]:
                    self.assertTrue(session["startAt"] >= previous["endAt"] or
                                    previous["startAt"] >= session["endAt"])

    def test_absence_busy_and_weekly_limit(self):
        data = fixture(counts=(3,), stay=14)
        data["procedures"]["p0"]["minIntervalDays"] = 2
        data["procedures"]["p0"]["defaultWeeklyCount"] = 2
        data["absences"] = {"1": {"2026-10-22": True, "2026-10-23": True}}
        data["occupied"] = [{"startAt": "2026-10-26T09:00", "endAt": "2026-10-26T12:00",
                             "patientNumber": 1, "procedureId": "other", "sourcePlanDate": ""}]
        output = solve(data)
        self.assertEqual(output["scheduledCount"], 3)
        starts = [date.fromisoformat(item["startAt"][:10]) for item in output["appointments"]]
        self.assertTrue(all(day >= date(2026, 10, 26) for day in starts))
        self.assertTrue(all(abs((a - b).days) >= 2 for i, a in enumerate(starts) for b in starts[:i]))
        for item in output["appointments"]:
            if item["startAt"].startswith("2026-10-26"):
                self.assertGreaterEqual(item["startAt"], "2026-10-26T12:00")

    def test_existing_fixed_session_respected(self):
        data = fixture(counts=(2,), stay=14)
        data["procedures"]["p0"]["minIntervalDays"] = 4
        data["occupied"] = [{"startAt": "2026-10-22T09:00", "endAt": "2026-10-22T09:50",
                             "patientNumber": 1, "procedureId": "p0", "sourcePlanDate": "2026-10-22"}]
        output = solve(data)
        self.assertEqual(output["scheduledCount"], 2)
        self.assertTrue(all(item["startAt"][:10] >= "2026-10-26" for item in output["appointments"]))

    def test_impossible_plan_reports_unscheduled(self):
        data = fixture(counts=(3,), stay=4)
        data["procedures"]["p0"]["minIntervalDays"] = 4
        output = solve(data)
        self.assertEqual(output["scheduledCount"], 1)
        self.assertEqual(output["unscheduledCount"], 2)
        self.assertEqual(output["unscheduledPatientNumbers"], [1])

    def test_high_severity_relax_first(self):
        data = fixture(counts=(1, 1), stay=1)
        data["plans"][0]["severity"] = "high"
        data["procedures"]["p1"]["isRelaxing"] = True
        output = solve(data)
        self.assertEqual(output["scheduledCount"], 2)
        self.assertEqual(output["appointments"][0]["procedureId"], "p1")


if __name__ == "__main__":
    unittest.main()
