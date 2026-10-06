#!/usr/bin/env python3
"""Collect upstream notices with go-licenses and preserve supplemental notices."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
TARGETS = (
    "darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64",
    "windows/amd64", "windows/arm64",
)


def run(*args, **kwargs):
    return subprocess.check_output(args, cwd=ROOT, text=True, **kwargs)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="fail if notices need updating")
    args = parser.parse_args()
    subprocess.run(["go", "mod", "download"], cwd=ROOT, check=True)
    requirements = json.loads(run("go", "mod", "edit", "-json"))["Require"]
    modules = {}
    notices = {}
    embedded = {}

    for requirement in requirements:
        name = requirement["Path"]
        modules[name] = json.loads(run("go", "list", "-m", "-json", name))
        notices[name + "/LICENSE"] = (Path(modules[name]["Dir"]) / "LICENSE").read_text()

    with tempfile.TemporaryDirectory(prefix="somniloq-notices-") as temporary:
        for target in TARGETS:
            goos, goarch = target.split("/")
            destination = Path(temporary) / goos / goarch
            subprocess.run(
                ["go-licenses", "save", "./cmd/somniloq", "--ignore",
                 "github.com/ryotapoi/somniloq", "--save_path", str(destination)],
                cwd=ROOT, check=True,
                env={**os.environ, "GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"},
            )
            for path in sorted(destination.rglob("*")):
                if path.is_file():
                    notices[path.relative_to(destination).as_posix()] = path.read_text()
            # go-licenses does not detect licenses embedded in generated libc code.
            sources = run(
                "go", "list", "-deps", "-f",
                '{{if .Module}}{{range .GoFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{end}}',
                "./cmd/somniloq",
                env={**os.environ, "GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"},
            )
            libc = Path(modules["modernc.org/libc"]["Dir"])
            for source in sources.splitlines():
                path = Path(source)
                if not source or not path.is_relative_to(libc):
                    continue
                for match in re.finditer(
                    r"(?m)^[ \t]*(?://[^\n]*(?:\n[ \t]*//[^\n]*)*|/\*[\s\S]*?\*/)",
                    path.read_text(),
                ):
                    comment = match.group().strip()
                    if re.search(
                        r"\bcopyright(?:\s|\()|\bpermission\b|\blicen[cs]e\b",
                        comment, re.IGNORECASE,
                    ):
                        label = path.relative_to(libc).as_posix()
                        embedded.setdefault(comment, set()).add(label)

    supplements = {
        "modernc.org/sqlite": ("SQLITE-LICENSE",),
        "modernc.org/libc": ("LICENSE-GO", "musl/COPYRIGHT"),
        "modernc.org/memory": ("LICENSE-MMAP-GO",),
    }
    for name, paths in supplements.items():
        for relative in paths:
            notices[name + "/" + relative] = (Path(modules[name]["Dir"]) / relative).read_text()
    notices["modernc.org/libc/embedded-source-notices"] = (
        "対象ビルドの libc ソース内に保持された通知コメントです。\n"
        "生成ヘッダー由来の通知も含み、実行コードのライセンス分類を示すものではありません。\n\n"
        + "\n\n".join(
            "出典: " + ", ".join(sorted(paths)) + "\n\n" + comment
            for comment, paths in sorted(embedded.items(), key=lambda item: sorted(item[1]))
        )
    )

    goroot = Path(run("go", "env", "GOROOT").strip())
    # Homebrew installs LICENSE beside libexec rather than inside GOROOT.
    go_license = goroot / "LICENSE"
    if not go_license.is_file():
        go_license = goroot.parent / "LICENSE"
    notices["Go/LICENSE"] = go_license.read_text()
    notices["Go/PATENTS"] = (goroot / "PATENTS").read_text()
    for path in sorted((goroot / "src/vendor").rglob("LICENSE")):
        notices["Go/" + path.relative_to(goroot).as_posix()] = path.read_text()

    sections = [
        "THIRD-PARTY NOTICES\n\n"
        "somniloq が利用する外部ソフトウェアのライセンス・著作権通知です。\n"
        "go.mod の直接・間接依存と、各対象環境の CLI 依存を収集しています。\n"
        "実際に含まれるコードはビルド対象により異なります。\n"
        "ライセンス原文は以下に保持しています。\n\n"
        "生成: scripts/update-third-party-notices.py (go-licenses v2.0.1)\n"
        "収集対象: " + ", ".join(TARGETS) + "\n"
        "Go: " + run("go", "env", "GOVERSION").strip() + "\n\n"
        "依存モジュール:\n" + "".join(
            name + " " + modules[name]["Version"] + "\n" for name in sorted(modules)
        )
    ]
    for name, content in sorted(notices.items()):
        sections.append("\n" + "=" * 72 + "\n" + name + "\n" + "=" * 72 + "\n\n" + content.rstrip() + "\n")
    output = "".join(sections)
    destination = ROOT / "THIRD-PARTY-NOTICES.txt"
    if args.check:
        if destination.read_text() != output:
            raise SystemExit("THIRD-PARTY-NOTICES.txt is outdated; regenerate it.")
    else:
        destination.write_text(output)


if __name__ == "__main__":
    main()
