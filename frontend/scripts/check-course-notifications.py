"""Verify course limits use toasts and never save an unapproved plan.

Run with local Next.js: python scripts/check-course-notifications.py
The browser mocks all PUT and generation POST requests; no data is modified.
"""

import json
import sys

from playwright.sync_api import sync_playwright


BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://localhost:3000").rstrip("/")


def verify(browser, mode):
    page = browser.new_page(viewport={"width": 1200, "height": 850})
    submissions = []

    def mock_write(route):
        body = route.request.post_data_json
        route.fulfill(status=200, content_type="application/json", body=json.dumps(body))

    def mock_generate(route):
        body = route.request.post_data_json
        if body.get("preview"):
            result = (
                {
                    "appointments": [],
                    "scheduledCount": 10,
                    "unscheduledCount": 5,
                    "unscheduledPatientNumbers": [1],
                    "warnings": [],
                }
                if mode == "overflow"
                else {
                    "appointments": [],
                    "scheduledCount": 15,
                    "unscheduledCount": 0,
                    "unscheduledPatientNumbers": [],
                    "warnings": [
                        "Пациент 1: интервал процедуры превышает срок пребывания."
                    ],
                }
            )
        else:
            submissions.append(body)
            result = {"appointments": [], "scheduledCount": 15, "unscheduledCount": 0}
        route.fulfill(status=200, content_type="application/json", body=json.dumps(result, ensure_ascii=False))

    page.route("**/api/backend/v1/procedures", mock_write)
    page.route("**/api/backend/v1/day-plans", mock_write)
    page.route("**/api/backend/v1/schedules/generate", mock_generate)

    try:
        page.goto(BASE + "/schedule/patients?date=2026-10-21", wait_until="domcontentloaded")
        page.get_by_role("button", name="Составить курс процедур").click()
        toast = page.locator('[data-slot="toast"][data-type="constraint"]')
        toast.wait_for(timeout=10000)
        assert "помещаются" in toast.inner_text() if mode == "overflow" else "Проверьте ограничения курса" in toast.inner_text()
        assert "За период пребывания не помещается" not in page.locator("main").inner_text()
        assert toast.locator("button[aria-label='Закрыть уведомление']").count() == 1
        assert not submissions, "generation must never run before explicit approval"

        if mode == "overflow":
            assert "Не удалось разместить 5 сеансов" in toast.inner_text()
            assert toast.locator("button:not([aria-label])").count() == 0
            print("OK: capacity warning is a toast; generation blocked")
        elif mode == "confirmation":
            page.wait_for_timeout(500)
            assert toast.count() == 1, "decision toast must remain visible"
            assert toast.locator("button:not([aria-label])").count() == 1
            toast.locator("button:not([aria-label])").click()
            page.wait_for_function("() => location.pathname === '/'", timeout=10000)
            assert len(submissions) == 1, "approval must generate exactly once"
            print("OK: warning requires approval in toast and creates schedule once")
        else:
            page.get_by_role("spinbutton", name="Срок пребывания пациента в днях").fill("8")
            page.wait_for_timeout(300)
            assert toast.count() == 0, "old confirmation must close when treatment plan changes"
            assert not submissions, "old confirmation cannot be applied to a changed plan"
            print("OK: changed treatment plan invalidates pending approval")
    finally:
        page.close()


def main():
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(channel="chrome", headless=True)
        try:
            verify(browser, "overflow")
            verify(browser, "confirmation")
            verify(browser, "stale")
        finally:
            browser.close()


if __name__ == "__main__":
    main()
