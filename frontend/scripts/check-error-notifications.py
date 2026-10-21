"""Browser checks for centralized, plain-language error notifications.

Run with local Next.js frontend: python scripts/check-error-notifications.py
Requires Python Playwright and Chrome. All errors are mocked in the browser.
"""

import sys

from playwright.sync_api import sync_playwright


BASE_URL = (sys.argv[1] if len(sys.argv) > 1 else "http://localhost:3000").rstrip("/")


def check_procedure_blocked(browser):
    page = browser.new_page(viewport={"width": 1440, "height": 900})
    try:
        def refuse_delete(route):
            if route.request.method == "DELETE":
                route.fulfill(
                    status=409,
                    content_type="application/json",
                    body='{"error":"procedure is used in one or more patient plans"}',
                )
            else:
                route.continue_()

        page.route("**/api/backend/v1/procedures/*", refuse_delete)
        page.goto(BASE_URL + "/schedule/procedures?date=2026-10-21", wait_until="domcontentloaded")
        button = page.get_by_role("button", name="Удалить Процедура 1", exact=True).first
        button.click()
        notice = page.locator('[data-slot="toast"]')
        notice.first.wait_for()
        assert "Процедуру пока нельзя удалить" in notice.first.inner_text()
        assert "Сначала уберите" in notice.first.inner_text()
        assert notice.first.locator("button[aria-label='Закрыть уведомление']").count() == 1
        button.click()
        page.wait_for_timeout(150)
        assert notice.count() == 1, "repeated blocked request should not flood notifications"
        print("OK: blocked deletion is clear, dismissible and deduplicated")
    finally:
        page.close()


def check_calendar_load_error(browser, network_failure=False):
    page = browser.new_page(viewport={"width": 390, "height": 780})
    try:
        def fail_load(route):
            if network_failure:
                route.abort("failed")
            else:
                route.fulfill(status=503, content_type="application/json", body='{"error":"database unavailable"}')

        page.route("**/api/backend/v1/appointments?*", fail_load)
        page.goto(BASE_URL + "/?date=2026-10-21", wait_until="domcontentloaded")
        notice = page.locator('[data-slot="toast"]')
        notice.first.wait_for(timeout=15000)
        expected = "Не удалось связаться с программой" if network_failure else "Программа временно недоступна"
        assert expected in notice.first.inner_text()
        assert "backend" not in notice.first.inner_text().lower()
        assert notice.first.bounding_box()["width"] <= 390
        print("OK: " + ("connection loss" if network_failure else "unavailable service") + " on mobile")
    finally:
        page.close()


def main():
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(channel="chrome", headless=True)
        try:
            check_procedure_blocked(browser)
            check_calendar_load_error(browser)
            check_calendar_load_error(browser, network_failure=True)
        finally:
            browser.close()


if __name__ == "__main__":
    main()
