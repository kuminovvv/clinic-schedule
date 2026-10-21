"""Browser regression for document-driven scrolling of sticky viewports.

Run: python scripts/check-page-driven-scroll.py [http://localhost:3000]
Requires: playwright (Python) and Chrome.
"""

import sys

from playwright.sync_api import sync_playwright


BASE_URL = (sys.argv[1] if len(sys.argv) > 1 else "http://localhost:3000").rstrip("/")
ROUTES = (
    ("/schedule/procedures?date=2026-10-21", "catalog"),
    ("/?date=2026-10-21", "calendar"),
)
VIEWPORTS = ((1600, 900), (900, 900), (390, 780))


def metrics(page, kind):
    return page.evaluate(
        """(kind) => {
          const track = kind === "catalog"
            ? document.querySelector('[aria-label="Каталог процедур"]')
            : document.querySelector('main.sticky')?.parentElement
          if (!track) return null
          const panel = track.firstElementChild
          const viewport = panel.querySelector('[class*="overflow-hidden"]')
          const content = [...track.querySelectorAll("*")].find(
            (element) => element.style.transform.startsWith("translateY(")
          )
          const rect = panel.getBoundingClientRect()
          const viewportRect = viewport?.getBoundingClientRect()
          const rows = [...track.querySelectorAll("article")]
          return {
            scrollY: Math.round(window.scrollY),
            screenHeight: innerHeight,
            documentHeight: document.scrollingElement.scrollHeight,
            documentWidth: document.scrollingElement.scrollWidth,
            screenWidth: innerWidth,
            panelTop: Math.round(rect.top),
            panelBottom: Math.round(rect.bottom),
            panelHeight: Math.round(rect.height),
            viewportBottom: Math.round(viewportRect?.bottom ?? 0),
            innerScrollTop: viewport?.scrollTop ?? -1,
            transform: content?.style.transform,
            lastRowBottom: Math.round(rows.at(-1)?.getBoundingClientRect().bottom ?? 0),
            rowCount: rows.length,
          }
        }""",
        kind,
    )


def run_case(browser, route, kind, width, height):
    page = browser.new_page(viewport={"width": width, "height": height})
    try:
        page.goto(BASE_URL + route, wait_until="domcontentloaded")
        selector = '[aria-label="Каталог процедур"]' if kind == "catalog" else "main.sticky"
        page.wait_for_function(
            """selector => {
              const node = document.querySelector(selector)
              const track = selector === "main.sticky" ? node?.parentElement : node
              return Boolean(track?.style.height)
            }""",
            arg=selector,
        )
        before = metrics(page, kind)
        assert before is not None, f"{kind}: no scroll track"
        assert before["documentWidth"] <= width, f"{kind}: horizontal overflow"
        assert before["panelBottom"] <= height, f"{kind}: viewport overflow"
        if kind == "calendar":
            geometry = page.evaluate(
                """() => {
                  const day = document.querySelector("[data-calendar-day]")
                  const content = day.parentElement
                  const viewport = content.parentElement
                  const header = viewport.previousElementSibling
                  return {
                    headerBottom: Math.round(header.getBoundingClientRect().bottom),
                    viewportTop: Math.round(viewport.getBoundingClientRect().top),
                    headerTop: Math.round(header.getBoundingClientRect().top),
                    innerScrollTop: viewport.scrollTop,
                  }
                }"""
            )
            assert abs(geometry["headerBottom"] - geometry["viewportTop"]) <= 1, (
                f"calendar: time grid must start directly under the weekday header: {geometry}"
            )
            assert "24:00" not in page.locator("main.sticky").inner_text(), (
                "calendar: 24:00 is not a valid wall clock time"
            )

        page.mouse.move(width // 2, min(height - 100, 450))
        page.mouse.wheel(0, 350)
        page.wait_for_timeout(300)
        middle = metrics(page, kind)
        assert middle["scrollY"] > before["scrollY"], f"{kind}: page did not scroll"
        assert middle["innerScrollTop"] == 0, f"{kind}: nested scrolling"
        assert middle["transform"] != before["transform"], f"{kind}: content did not move"
        assert middle["panelBottom"] <= height, f"{kind}: panel extended below viewport"

        page.evaluate("window.scrollTo(0, document.scrollingElement.scrollHeight)")
        page.wait_for_timeout(250)
        bottom = metrics(page, kind)
        if kind == "catalog" and bottom["rowCount"] > 0:
            assert bottom["lastRowBottom"] <= bottom["viewportBottom"] + 2, (
                f"{kind}: last row is unreachable"
            )
        if kind == "calendar":
            bottom_grid = page.evaluate(
                """() => {
                    const day = document.querySelector("[data-calendar-day]")
                    const content = day.parentElement
                    const viewport = content.parentElement
                    const header = viewport.previousElementSibling
                    return {
                      gridBottom: content.getBoundingClientRect().bottom,
                      viewportBottom: viewport.getBoundingClientRect().bottom,
                      headerTop: header.getBoundingClientRect().top,
                    }
                }"""
            )
            assert abs(bottom_grid["gridBottom"] - bottom_grid["viewportBottom"]) <= 2, (
                f"calendar: end of the day is not aligned with viewport bottom: {bottom_grid}"
            )
            assert abs(bottom_grid["headerTop"] - geometry["headerTop"]) <= 1, (
                "calendar: weekday header should remain fixed when scrolling"
            )

        page.set_viewport_size({"width": width, "height": height - 150})
        page.wait_for_timeout(250)
        page.evaluate("window.scrollTo(0, document.scrollingElement.scrollHeight)")
        page.wait_for_timeout(250)
        resized = metrics(page, kind)
        assert resized["panelBottom"] <= height - 150, f"{kind}: resize overflow"
        assert resized["documentWidth"] <= width, f"{kind}: resize horizontal overflow"
        assert resized["innerScrollTop"] == 0, f"{kind}: resize nested scrolling"
        print(f"OK {kind}, {width}x{height}: page scroll, sticky panel, end, resize")
    finally:
        page.close()


def main():
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(channel="chrome", headless=True)
        try:
            for route, kind in ROUTES:
                for width, height in VIEWPORTS:
                    run_case(browser, route, kind, width, height)
        finally:
            browser.close()


if __name__ == "__main__":
    main()
