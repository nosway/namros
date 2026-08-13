#!/usr/bin/env python3
"""Fail when Markdown source markers survive into the published documentation.

The public manual sources wrap their bodies in component <div> blocks. Without
the `md_in_html` Markdown extension those bodies publish as raw text: headings
stay as `##`, list items stay as `-`, and inline code keeps its backticks.
`mkdocs build --strict` does not catch this because it only validates links and
navigation, so this check inspects the rendered page bodies instead.
"""

import re
import sys
from pathlib import Path

# Markers that must never appear as literal text in a rendered page body.
PATTERNS = (
    ("ATX heading", re.compile(r"^\s{0,3}#{1,6} \S", re.M)),
    ("list item", re.compile(r"^\s{0,3}[-*] \S", re.M)),
    ("fence", re.compile(r"^\s{0,3}```", re.M)),
    # Rendered inline code and links become tags that TAGS strips, so a
    # surviving pair here means the block was published as raw text.
    ("inline code span", re.compile(r"`[^`\n]+`")),
    ("inline link", re.compile(r"\[[^\]\n]+\]\([^)\n]+\)")),
)

# Material renders page content inside <article>; fall back to the whole file.
ARTICLE = re.compile(r"<article\b[^>]*>(.*?)</article>", re.S)
# Non-visible regions, plus code where `#`, `-`, and ``` are legitimate content.
STRIP = re.compile(r"<(script|style|template|pre|code)\b[^>]*>.*?</\1>", re.S)
TAGS = re.compile(r"<[^>]+>")
IMG_SRC = re.compile(r"<img\b[^>]*?\bsrc=\"([^\"]+)\"")


def page_text(html: str) -> str:
    match = ARTICLE.search(html)
    body = match.group(1) if match else html
    body = STRIP.sub(" ", body)
    return TAGS.sub(" ", body)


def broken_assets(html: str, page: Path, site: Path):
    """Yield local image sources that do not resolve to a published file.

    mkdocs publishes `chapter.md` as `chapter/index.html`, so a relative path
    written for a flat layout resolves one directory short. mkdocs rewrites
    Markdown image paths but leaves raw <img src> attributes untouched, and
    --strict does not validate them.
    """
    for src in IMG_SRC.findall(html):
        if src.startswith(("http://", "https://", "//", "data:", "#")):
            continue
        target = (page.parent / src.split("?")[0].split("#")[0]).resolve()
        if not target.is_file():
            yield src


def main() -> int:
    site = Path(sys.argv[1] if len(sys.argv) > 1 else "site")
    if not site.is_dir():
        print(f"docs-render-check: site directory not found: {site}", file=sys.stderr)
        return 1

    failures = []
    pages = 0
    for html_file in sorted(site.rglob("*.html")):
        if html_file.name == "404.html":
            continue
        pages += 1
        raw = html_file.read_text(encoding="utf-8", errors="ignore")
        text = page_text(raw)
        rel = html_file.relative_to(site)
        for label, pattern in PATTERNS:
            hits = pattern.findall(text)
            if hits:
                failures.append((rel, f"{len(hits)} unrendered {label}(s)"))
        for src in broken_assets(raw, html_file, site):
            failures.append((rel, f"unresolved image: {src}"))

    if not pages:
        print(f"docs-render-check: no pages found under {site}", file=sys.stderr)
        return 1

    if failures:
        print(
            f"docs-render-check: {len(failures)} problem(s) on "
            f"{len({f[0] for f in failures})} of {pages} pages",
            file=sys.stderr,
        )
        for rel, detail in failures[:20]:
            print(f"  {rel}: {detail}", file=sys.stderr)
        if len(failures) > 20:
            print(f"  ... and {len(failures) - 20} more", file=sys.stderr)
        print(
            "docs-render-check: unrendered Markdown usually means mkdocs.yml "
            "lost the md_in_html extension or a component div lost "
            'markdown="1"; an unresolved image usually means a raw <img src> '
            "that mkdocs did not rewrite — use Markdown image syntax instead",
            file=sys.stderr,
        )
        return 1

    print(f"docs-render-check: {pages} pages rendered cleanly")
    return 0


if __name__ == "__main__":
    sys.exit(main())
