#!/usr/bin/env python3

from __future__ import annotations

import argparse
import re
import sys
from collections import Counter, defaultdict
from pathlib import Path
from typing import Iterable


HEADING_RE = re.compile(r"^(#{1,6})\s+(.*)$")
MARKDOWN_LINK_RE = re.compile(r"\[[^\]]+\]\(([^)]+)\)")
HTML_HREF_RE = re.compile(r"""href=["']([^"']+)["']""")


def github_slugify(text: str) -> str:
    text = re.sub(r"<[^>]+>", "", text)
    text = text.strip().lower()
    text = re.sub(r"[^\w\s-]", "", text)
    text = re.sub(r"\s+", "-", text)
    return text.strip("-")


def build_anchor_set(path: Path) -> set[str]:
    slugs: list[str] = []
    counts: Counter[str] = Counter()
    in_fence = False
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.rstrip("\n")
        if line.startswith("```"):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        match = HEADING_RE.match(line)
        if not match:
            continue
        slug = github_slugify(match.group(2))
        if not slug:
            continue
        suffix = counts[slug]
        counts[slug] += 1
        if suffix:
            slug = f"{slug}-{suffix}"
        slugs.append(slug)
    return set(slugs)


def iter_doc_files(inputs: Iterable[Path]) -> list[Path]:
    files: list[Path] = []
    for raw in inputs:
        path = raw.resolve()
        if path.is_dir():
            files.extend(sorted(p for p in path.rglob("*.md") if p.is_file()))
            continue
        if path.suffix == ".md" and path.is_file():
            files.append(path)
    seen: set[Path] = set()
    ordered: list[Path] = []
    for path in files:
        if path in seen:
            continue
        seen.add(path)
        ordered.append(path)
    return ordered


def extract_links(path: Path) -> list[str]:
    links: list[str] = []
    in_fence = False
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.startswith("```"):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        links.extend(MARKDOWN_LINK_RE.findall(line))
        links.extend(HTML_HREF_RE.findall(line))
    return links


def is_external(link: str) -> bool:
    return link.startswith(("http://", "https://", "mailto:"))


def main() -> int:
    parser = argparse.ArgumentParser(description="Check local markdown links and anchors.")
    parser.add_argument("paths", nargs="+", help="Markdown files or directories to validate")
    args = parser.parse_args()

    repo_root = Path.cwd().resolve()
    files = iter_doc_files(Path(p) for p in args.paths)
    anchors = {path: build_anchor_set(path) for path in files}
    errors: defaultdict[Path, list[str]] = defaultdict(list)

    for path in files:
        for link in extract_links(path):
            if is_external(link):
                continue
            target, _, anchor = link.partition("#")
            if not target:
                target_path = path
            else:
                target_path = (path.parent / target).resolve()
            if not target_path.exists():
                errors[path].append(f"missing target: {link}")
                continue
            if target_path.is_dir():
                errors[path].append(f"directory target is not allowed: {link}")
                continue
            if target_path.suffix != ".md" and anchor:
                errors[path].append(f"anchor target is not markdown: {link}")
                continue
            if anchor and target_path.suffix == ".md":
                known = anchors.get(target_path)
                if known is None:
                    known = build_anchor_set(target_path)
                    anchors[target_path] = known
                if anchor not in known:
                    errors[path].append(f"missing anchor: {link}")

    if errors:
        for path in sorted(errors):
            print(repo_root.joinpath(path.relative_to(repo_root)))
            for message in errors[path]:
                print(f"  - {message}")
        return 1

    print("OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
