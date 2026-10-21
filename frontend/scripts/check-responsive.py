"""Проверка статического экспорта на горизонтальные и текстовые переполнения.

Запуск: pnpm run build; python scripts/check-responsive.py
Требования: Python 3, pip install playwright, установленный Chrome или Playwright Chromium.
"""

from __future__ import annotations

import sys
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Thread

from playwright.sync_api import Error, sync_playwright


OUT = Path(__file__).resolve().parents[1] / "out"
WIDTHS = (320, 360, 375, 390, 430, 580, 640, 768, 900, 1024, 1280, 1440, 1920)
THEMES = ("light", "dark")

CHECK = """
() => {
  const width = window.innerWidth;
  const issues = [];
  const overflow = document.documentElement.scrollWidth - width;
  if (overflow > 2) issues.push("Горизонтальная прокрутка: +" + overflow + "px");

  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  let node;
  while ((node = walker.nextNode())) {
    const value = node.textContent.trim();
    if (!value) continue;

    const parent = node.parentElement;
    const style = getComputedStyle(parent);
    if (style.display === "none" || style.visibility === "hidden") continue;

    const range = document.createRange();
    range.selectNodeContents(node);
    for (const rect of range.getClientRects()) {
      if (!rect.width || !rect.height) continue;
      let container = parent;
      while (container && !["ARTICLE", "SECTION", "ASIDE", "FOOTER"].includes(container.tagName)) {
        container = container.parentElement;
      }
      const bounds = container?.getBoundingClientRect();
      const outside = rect.left < -2 || rect.right > width + 2 ||
        (bounds && (rect.left < bounds.left - 2 || rect.right > bounds.right + 2));
      if (outside) {
        issues.push("Текст за границей: " + value.slice(0, 48) + " (" +
          Math.round(rect.left) + "…" + Math.round(rect.right) + "px)");
      }

      // Текст не должен исчезать под overflow:hidden/clip у своих предков.
      for (let ancestor = parent; ancestor && ancestor !== document.body; ancestor = ancestor.parentElement) {
        const css = getComputedStyle(ancestor);
        if (!["hidden", "clip"].includes(css.overflowX) &&
            !["hidden", "clip"].includes(css.overflowY)) continue;
        const box = ancestor.getBoundingClientRect();
        if (rect.right > box.right + 2 || rect.left < box.left - 2 ||
            rect.top < box.top - 2 || rect.bottom > box.bottom + 2) {
          issues.push("Обрезанный текст: " + value.slice(0, 48));
          break;
        }
      }
    }
  }
  return [...new Set(issues)].slice(0, 8);
}
"""


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(OUT), **kwargs)

    def log_message(self, *_args):
        pass


class QuietServer(ThreadingHTTPServer):
    def handle_error(self, *_args):
        pass


def run() -> int:
    if not (OUT / "index.html").exists():
        print("Сначала выполните pnpm run build: нет Frontend/out/index.html")
        return 2

    routes = [
        "/" + str(path.parent.relative_to(OUT)).replace("\\", "/").strip("./") + "/"
        for path in OUT.rglob("index.html")
    ]
    routes = sorted(set("/" if route == "//" else route for route in routes))
    server = QuietServer(("127.0.0.1", 0), Handler)
    Thread(target=server.serve_forever, daemon=True).start()
    failures = []

    try:
        with sync_playwright() as playwright:
            try:
                browser = playwright.chromium.launch(channel="chrome", headless=True)
            except Error:
                browser = playwright.chromium.launch(headless=True)
            try:
                for theme in THEMES:
                    context = browser.new_context(viewport={"width": 1440, "height": 900})
                    context.add_init_script(
                        f"localStorage.setItem('portfolio-theme', '{theme}')"
                    )
                    context.route("**/*__next*", lambda route: route.abort())
                    page = context.new_page()
                    try:
                        for width in WIDTHS:
                            page.set_viewport_size({"width": width, "height": 900})
                            for route in routes:
                                response = page.goto(
                                    f"http://127.0.0.1:{server.server_port}{route}",
                                    wait_until="domcontentloaded",
                                )
                                page.evaluate("document.fonts.ready")
                                problems = page.evaluate(CHECK)
                                if response.status != 200 or problems:
                                    failures.append(
                                        (theme, width, route, response.status, problems)
                                    )
                    finally:
                        context.close()
            finally:
                browser.close()
    finally:
        server.shutdown()
        server.server_close()

    count = len(routes) * len(WIDTHS) * len(THEMES)
    for theme, width, route, status, problems in failures[:30]:
        print(f"{theme} {width}px {route} HTTP {status}: {'; '.join(problems)}")
    print(f"Проверено {count} комбинаций ({len(routes)} маршрутов): ошибок {len(failures)}")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(run())
