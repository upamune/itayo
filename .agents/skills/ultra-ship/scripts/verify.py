#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12"
# dependencies = []
# [tool.uv]
# exclude-newer = "2026-08-30T07:45:12Z"
# ///
"""ultra-ship の完了判定。チェックポイントの記録と git / gh の実状態を突き合わせ、全部通れば exit 0。

  verify.py            # 人向けの表
  verify.py --json     # 機械向け
  verify.py --no-ci    # CI の確認を飛ばす（PR 作成直後など）

判定項目:
  checkpoint  z/<branch>/ultra-ship.html があり、全フェーズが done か skipped
  findings    レビューの各ラウンドで、指摘ごとに note があり、最終ラウンドに「safe なのに未適用」が無い
  worktree    未コミットの変更が無く、merge / rebase の途中でない
  base        origin/<base> を取り込み済み（origin/<base> が HEAD の祖先）
  pushed      HEAD が origin/<branch> と一致
  pr          PR が存在し open（draft かどうかは表示のみ）
  canvas      values.canvas_path のファイルが存在
  ci          gh pr checks が全部 pass（--no-ci で省略）
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

STATE_RE = re.compile(r'<script id="ultra-ship-state" type="application/json">(.*?)</script>', re.S)


def sh(*args: str) -> tuple[int, str]:
    p = subprocess.run(args, capture_output=True, text=True)
    return p.returncode, (p.stdout or p.stderr).strip()


def ok(*args: str) -> bool:
    return sh(*args)[0] == 0


class Report:
    def __init__(self) -> None:
        self.items: list[dict] = []

    def add(self, name: str, passed: bool, detail: str) -> None:
        self.items.append({"name": name, "ok": passed, "detail": detail})

    @property
    def passed(self) -> bool:
        return all(i["ok"] for i in self.items)


def load_state(root: Path, root_dir: str, branch: str) -> dict | None:
    safe = re.sub(r"[^A-Za-z0-9._-]+", "_", branch).strip("_") or "detached"
    path = root / root_dir / safe / "ultra-ship.html"
    if not path.exists():
        return None
    m = STATE_RE.search(path.read_text())
    return json.loads(m.group(1)) if m else None


def check_checkpoint(r: Report, state: dict | None) -> None:
    if state is None:
        r.add("checkpoint", False, "z/<branch>/ultra-ship.html が無い、または壊れている")
        return
    bad = [f"{p['id']}={p['status']}" for p in state["phases"] if p["status"] not in ("done", "skipped")]
    r.add("checkpoint", not bad, "全フェーズ done/skipped" if not bad else "未完了: " + ", ".join(bad))


def check_findings(r: Report, state: dict | None) -> None:
    if state is None:
        r.add("findings", False, "チェックポイントが無い")
        return
    problems = []
    for p in state["phases"]:
        if not p["id"].startswith("review:"):
            continue
        if p["status"] == "skipped":
            continue
        if not p["rounds"]:
            problems.append(f"{p['id']}: ラウンド記録なし")
            continue
        for rd in p["rounds"]:
            for f in rd["findings"]:
                if not f.get("note"):
                    problems.append(f"{p['id']} r{rd['n']} {f['id']}: note なし")
        last = p["rounds"][-1]
        pending = [f["id"] for f in last["findings"] if f.get("safe") and not f.get("applied")]
        if pending:
            problems.append(f"{p['id']} r{last['n']}: safe なのに未適用 {', '.join(pending)}")
    r.add("findings", not problems, "全ラウンドに記録があり、safe な指摘は処理済み" if not problems else "; ".join(problems))


def check_git(r: Report, root: Path, branch: str, base: str | None) -> None:
    _, status = sh("git", "status", "--porcelain")
    in_merge = (root / ".git" / "MERGE_HEAD").exists() or (root / ".git" / "rebase-merge").exists() or (root / ".git" / "rebase-apply").exists()
    r.add("worktree", not status and not in_merge, "クリーン" if not status and not in_merge else ("merge/rebase 途中" if in_merge else f"未コミット: {len(status.splitlines())} 件"))

    if base:
        merged = ok("git", "merge-base", "--is-ancestor", f"origin/{base}", "HEAD")
        r.add("base", merged, f"origin/{base} を取り込み済み" if merged else f"origin/{base} が HEAD の祖先でない（merge が必要）")
    else:
        r.add("base", False, "base が不明")

    rc, remote = sh("git", "rev-parse", "--verify", "--quiet", f"origin/{branch}")
    _, head = sh("git", "rev-parse", "HEAD")
    if rc != 0:
        r.add("pushed", False, f"origin/{branch} が無い（未 push）")
    else:
        r.add("pushed", remote == head, "HEAD == origin/" + branch if remote == head else f"HEAD {head[:8]} != origin/{branch} {remote[:8]}")


def check_pr(r: Report, want_ci: bool) -> None:
    rc, out = sh("gh", "pr", "view", "--json", "url,state,isDraft,baseRefName")
    if rc != 0:
        r.add("pr", False, "PR が無い")
        if want_ci:
            r.add("ci", False, "PR が無いので確認不能")
        return
    pr = json.loads(out)
    is_open = pr["state"] == "OPEN"
    r.add("pr", is_open, f"{pr['url']} ({'draft' if pr['isDraft'] else 'ready'}, base {pr['baseRefName']})" if is_open else f"state={pr['state']}")
    if not want_ci:
        return
    rc, out = sh("gh", "pr", "checks", "--json", "name,bucket,state")
    if rc != 0 and not out.startswith("["):
        r.add("ci", False, out.splitlines()[0] if out else "gh pr checks 失敗")
        return
    checks = json.loads(out) if out else []
    if not checks:
        r.add("ci", False, "チェックが 1 つも無い")
        return
    bad = [f"{c['name']}={c['bucket']}" for c in checks if c["bucket"] not in ("pass", "skipping")]
    r.add("ci", not bad, f"{len(checks)} 件すべて pass" if not bad else ", ".join(bad))


def check_canvas(r: Report, root: Path, state: dict | None) -> None:
    path = (state or {}).get("values", {}).get("canvas_path")
    if not path:
        r.add("canvas", False, "values.canvas_path が無い")
        return
    p = Path(path)
    if not p.is_absolute():
        p = root / p
    r.add("canvas", p.exists(), str(p) if p.exists() else f"{p} が無い")


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--no-ci", action="store_true")
    ap.add_argument("--root", default="z")
    a = ap.parse_args()

    rc, top = sh("git", "rev-parse", "--show-toplevel")
    if rc != 0:
        sys.exit("git リポジトリではない")
    root = Path(top)
    _, branch = sh("git", "rev-parse", "--abbrev-ref", "HEAD")
    state = load_state(root, a.root, branch)
    base = (state or {}).get("base")

    r = Report()
    check_checkpoint(r, state)
    check_findings(r, state)
    check_git(r, root, branch, base)
    check_pr(r, not a.no_ci)
    check_canvas(r, root, state)

    if a.json:
        print(json.dumps({"ok": r.passed, "branch": branch, "base": base, "items": r.items}, ensure_ascii=False, indent=2))
    else:
        print(f"ultra-ship verify: {branch} → {base or '?'}")
        for i in r.items:
            print(f"  {'✓' if i['ok'] else '✗'} {i['name']:11} {i['detail']}")
        print("  => " + ("完了" if r.passed else "未完了"))
    sys.exit(0 if r.passed else 1)


if __name__ == "__main__":
    main()
