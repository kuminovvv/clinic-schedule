"""Проверка ссылок, навигационных якорей и согласованности статических страниц.

Запуск: pnpm run build && pnpm run test:integrity
"""

from __future__ import annotations

import json
import sys
from collections import Counter
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urljoin, urlparse


ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "out"
ORIGIN = "https://example.test"


class PageParser(HTMLParser):
    def __init__(self):
        super().__init__()
        self.ids: list[str] = []
        self.links: list[str] = []
        self.assets: list[str] = []
        self.descriptions: list[str] = []
        self.h1_count = 0

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if attrs.get("id"):
            self.ids.append(attrs["id"])
        if tag == "a" and "href" in attrs:
            self.links.append(attrs["href"])
        if tag in {"img", "script"} and attrs.get("src"):
            self.assets.append(attrs["src"])
        if tag == "link" and attrs.get("href"):
            self.assets.append(attrs["href"])
        if tag == "meta" and attrs.get("name") == "description":
            self.descriptions.append(attrs.get("content", ""))
        if tag == "h1":
            self.h1_count += 1


def output_file(path: str) -> Path:
    path = path.removeprefix("/")
    candidate = OUT / path
    if candidate.is_file():
        return candidate
    return candidate / "index.html"


def check() -> int:
    if not (OUT / "index.html").exists():
        print("Не найден out/index.html — сначала выполните pnpm run build")
        return 2

    content = json.loads((ROOT / "src/shared/config/site-content.json").read_text(encoding="utf-8"))
    catalog = content["catalog"]
    works = [*catalog["omsuCases"], catalog["gazpromCase"], *catalog["personalCases"], *catalog["technicalCases"]]
    slugs = [work["slug"] for work in works]
    problems: list[str] = []

    if len(slugs) != len(set(slugs)):
        problems.append("Повторяющиеся slug в каталоге")
    if catalog["intro"]["counterValue"] != f"{len(slugs)} кейсов":
        problems.append("Счётчик кейсов не соответствует фактическому каталогу")

    pages = {}
    for path in OUT.rglob("index.html"):
        url_path = "/" if path == OUT / "index.html" else "/" + path.parent.relative_to(OUT).as_posix() + "/"
        parser = PageParser()
        parser.feed(path.read_text(encoding="utf-8"))
        pages[url_path] = parser
        if parser.h1_count != 1:
            problems.append(f"{url_path}: заголовков H1: {parser.h1_count}")
        if not any(value.strip() for value in parser.descriptions):
            problems.append(f"{url_path}: отсутствует description")
        duplicates = [item for item, n in Counter(parser.ids).items() if n > 1]
        if duplicates:
            problems.append(f"{url_path}: повторяющиеся ID: {duplicates}")

    for slug in slugs:
        if f"/projects/{slug}/" not in pages:
            problems.append(f"Не экспортирован кейс /projects/{slug}/")

    all_case_urls = {f"/projects/{slug}/" for slug in slugs}
    for item in content["home"]["projects"]["items"]:
        if f"/projects/{item['slug']}/" not in all_case_urls:
            problems.append(f"С главной нет кейса для {item['slug']}")
    for item in content["stackPage"]["examples"]:
        if not item["href"] or item["href"].rstrip("/") + "/" not in all_case_urls:
            problems.append(f"В стеке нет страницы для {item['title']}")

    for route, parser in pages.items():
        for href in parser.links + parser.assets:
            if not href or href.startswith(("mailto:", "tel:", "data:", "#__")):
                continue
            url = urlparse(urljoin(ORIGIN + route, href))
            if url.netloc != urlparse(ORIGIN).netloc:
                continue
            destination = output_file(url.path)
            if not destination.is_file():
                problems.append(f"{route}: отсутствует ресурс {href}")
                continue
            if url.fragment and url.path.rstrip("/") + "/" in pages:
                ids = pages[url.path.rstrip("/") + "/"].ids
                if url.fragment not in ids:
                    problems.append(f"{route}: не существует якоря {href}")

    for item in problems:
        print("ERROR:", item)
    print(f"Проверено {len(pages)} HTML-страниц и {len(slugs)} кейсов. Ошибок: {len(problems)}")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(check())
