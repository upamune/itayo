#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12"
# dependencies = []
# [tool.uv]
# exclude-newer = "2026-08-30T07:45:12Z"
# ///
"""ultra-ship スキルの進捗チェックポイント。

状態は z/<sanitized_branch>/ultra-ship.html の中に JSON として埋め込む。
HTML は人が眺める進捗ページ、埋め込み JSON はエージェントが再開に使う。
レビューは「何回目に、誰が、何を指摘し、何をどう直したか」を 1 件ずつ残す。

使い方:
  checkpoint.py init --base main            # 作成（既存なら状態を保持したまま再描画）
  checkpoint.py status                      # 埋め込み JSON を stdout に出す（再開用）
  checkpoint.py phase <id> <status> [--note TEXT]
  checkpoint.py round <phase-id> --reviewer ID --findings findings.json [--summary TEXT] [--commit SHA]
  checkpoint.py set <key> <value>           # pr_url, canvas_path など任意のキー
  checkpoint.py log TEXT                    # 時系列ログを 1 行追加
  checkpoint.py path                        # HTML のパスを表示

findings.json は指摘の配列。1 件の形:
  {"id": "F1", "where": "src/a.ts:42", "finding": "指摘", "fix": "直し方",
   "safe": true, "applied": true, "note": "どう直したか / なぜ見送ったか"}
指摘ゼロのラウンドは [] を渡す。
"""

from __future__ import annotations

import argparse
import html
import json
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

PHASES = [
    ("commit", "コミット", "未コミットの変更を論理単位でコミット"),
    ("merge", "base merge", "base branch を取り込み、衝突を解消"),
    ("review:thermo", "thermo-nuclear", "構造・抽象の大きな見直し"),
    ("review:simplify", "simplify", "挙動を変えずに読みやすく"),
    ("review:deslop", "deslop", "AI 由来の冗長さ・不要な防御を除去"),
    ("review:code-review", "code-review", "規約と spec の両軸で最終確認"),
    ("pr", "PR", "PR を作成・整備"),
    ("canvas", "canvas", "pr-review-canvas で変更を説明"),
    ("ci", "CI", "CI が green になるまで修正"),
]
STATUSES = ("pending", "in_progress", "done", "skipped", "blocked")
STATUS_LABEL = {"pending": "未着手", "in_progress": "作業中", "done": "完了", "skipped": "スキップ", "blocked": "保留"}
STATE_TAG_ID = "ultra-ship-state"
STATE_RE = re.compile(r'<script id="' + STATE_TAG_ID + r'" type="application/json">(.*?)</script>', re.S)


def sh(*args: str) -> str:
    return subprocess.check_output(args, text=True).strip()


def now() -> str:
    return datetime.now(timezone.utc).astimezone().isoformat(timespec="seconds")


def repo_root() -> Path:
    return Path(sh("git", "rev-parse", "--show-toplevel"))


def branch_name() -> str:
    return sh("git", "rev-parse", "--abbrev-ref", "HEAD")


def sanitize(name: str) -> str:
    return re.sub(r"[^A-Za-z0-9._-]+", "_", name).strip("_") or "detached"


def checkpoint_path(root_dir: str, filename: str) -> Path:
    return repo_root() / root_dir / sanitize(branch_name()) / filename


def ensure_excluded(root_dir: str) -> None:
    exclude = repo_root() / ".git" / "info" / "exclude"
    line = f"/{root_dir}/"
    existing = exclude.read_text() if exclude.exists() else ""
    if line not in existing.splitlines():
        exclude.parent.mkdir(parents=True, exist_ok=True)
        with exclude.open("a") as f:
            if existing and not existing.endswith("\n"):
                f.write("\n")
            f.write(line + "\n")


def new_state(base: str) -> dict:
    return {
        "branch": branch_name(),
        "base": base,
        "created_at": now(),
        "updated_at": now(),
        "phases": [
            {"id": pid, "title": title, "desc": desc, "status": "pending", "note": "", "rounds": []}
            for pid, title, desc in PHASES
        ],
        "values": {},
        "log": [],
    }


def load(path: Path) -> dict | None:
    if not path.exists():
        return None
    m = STATE_RE.search(path.read_text())
    if not m:
        sys.exit(f"{path} に {STATE_TAG_ID} が見つからない。壊れているなら削除して init し直す")
    return json.loads(m.group(1))


def save(path: Path, state: dict) -> None:
    state["updated_at"] = now()
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(render(state))


def find_phase(state: dict, pid: str) -> dict:
    for p in state["phases"]:
        if p["id"] == pid:
            return p
    sys.exit(f"unknown phase: {pid}（候補: {', '.join(p['id'] for p in state['phases'])}）")


def load_findings(path: Path) -> list[dict]:
    data = json.loads(path.read_text())
    if not isinstance(data, list):
        sys.exit("findings.json は配列にする")
    out = []
    for i, f in enumerate(data, 1):
        for key in ("safe", "applied"):
            if not isinstance(f.get(key, False), bool):
                sys.exit(f"findings[{i - 1}].{key} は JSON の true/false にする（{f[key]!r}）")
        if not f.get("note"):
            sys.exit(f"findings[{i - 1}].note が空。どう直したか / なぜ見送ったかを書く")
        out.append({
            "id": str(f.get("id") or f"F{i}"),
            "where": str(f.get("where", "")),
            "finding": str(f.get("finding", "")),
            "fix": str(f.get("fix", "")),
            "safe": f.get("safe", False),
            "applied": f.get("applied", False),
            "note": str(f["note"]),
        })
    return out


CSS = """
@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&family=Newsreader:opsz,wght@6..72,400;6..72,500;6..72,600&display=swap');
:root {
  --paper: oklch(98% 0.006 85); --ink: oklch(22% 0.015 60); --muted: oklch(52% 0.015 60);
  --rule: oklch(88% 0.01 80); --panel: oklch(96% 0.008 85);
  --done: oklch(58% 0.14 150); --work: oklch(66% 0.16 65); --block: oklch(58% 0.19 25); --skip: oklch(70% 0.01 60);
  --mono: "IBM Plex Mono", ui-monospace, SFMono-Regular, Menlo, monospace;
  --serif: "Newsreader", "Hiragino Mincho ProN", "Yu Mincho", Georgia, serif;
}
@media (prefers-color-scheme: dark) {
  :root { --paper: oklch(18% 0.01 60); --ink: oklch(92% 0.01 85); --muted: oklch(65% 0.012 70);
          --rule: oklch(30% 0.012 60); --panel: oklch(22% 0.01 60); --skip: oklch(50% 0.01 60); }
}
* { box-sizing: border-box; }
body { margin: 0; background: var(--paper); color: var(--ink); font-family: var(--mono); font-size: 13px; line-height: 1.6; }
main { max-width: 1040px; margin: 0 auto; padding: 48px 32px 96px; }
header { display: grid; grid-template-columns: 1fr auto; gap: 24px; align-items: end; border-bottom: 2px solid var(--ink); padding-bottom: 20px; }
.kicker { text-transform: uppercase; letter-spacing: .18em; font-size: 11px; color: var(--muted); }
h1 { font-family: var(--serif); font-weight: 500; font-size: 40px; line-height: 1.1; margin: 6px 0 0; letter-spacing: -.01em; }
h1 .arrow { color: var(--muted); margin: 0 .25em; }
.stamp { text-align: right; color: var(--muted); font-size: 12px; }
.stamp b { display: block; color: var(--ink); font-size: 28px; font-family: var(--serif); font-weight: 500; line-height: 1; margin-bottom: 6px; }
.track { display: grid; grid-template-columns: repeat(PHASE_COUNT, 1fr); gap: 4px; margin: 20px 0 40px; }
.track span { height: 6px; background: var(--rule); }
.track .done { background: var(--done); } .track .in_progress { background: var(--work); }
.track .blocked { background: var(--block); } .track .skipped { background: var(--skip); }
h2 { font-family: var(--serif); font-weight: 500; font-size: 22px; margin: 48px 0 12px; }
ol.phases { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--rule); }
.phase { border-bottom: 1px solid var(--rule); }
.phase > summary { display: grid; grid-template-columns: 36px 160px 1fr auto; gap: 16px; padding: 14px 0; align-items: baseline; cursor: pointer; list-style: none; }
.phase > summary::-webkit-details-marker { display: none; }
.phase > summary .n { color: var(--muted); }
.phase > summary .n::before { content: "▸ "; font-size: 10px; }
.phase[open] > summary .n::before { content: "▾ "; }
.phase .detail { padding: 0 0 14px 212px; }
.phase .name { font-weight: 600; }
.phase .name small { display: block; font-weight: 400; color: var(--muted); font-size: 11px; }
.phase .body { min-width: 0; }
.phase .desc { color: var(--muted); }
.phase.pending > summary { cursor: default; } .phase.pending > summary .n::before { content: "  "; }
.pill { font-size: 11px; padding: 2px 8px; border: 1px solid currentColor; border-radius: 999px; white-space: nowrap; }
.pending .pill { color: var(--muted); } .in_progress .pill { color: var(--work); } .done .pill { color: var(--done); }
.blocked .pill { color: var(--block); } .skipped .pill { color: var(--skip); }
.pending .name, .skipped .name { color: var(--muted); }
details.round { margin-top: 10px; border: 1px solid var(--rule); background: var(--panel); }
details.round + details.round { margin-top: 6px; }
details.round summary { cursor: pointer; padding: 8px 12px; display: grid; grid-template-columns: 70px 1fr auto auto; gap: 16px; list-style: none; }
details.round summary::-webkit-details-marker { display: none; }
details.round summary .rv { color: var(--muted); }
details.round summary .stat { color: var(--muted); }
details.round summary .stat b { color: var(--ink); font-weight: 500; }
details.round[open] summary { border-bottom: 1px solid var(--rule); }
.round .summary-text { padding: 8px 12px 0; color: var(--muted); }
table.findings { width: 100%; border-collapse: collapse; margin: 8px 0 0; }
table.findings th { text-align: left; font-weight: 500; color: var(--muted); font-size: 11px; padding: 6px 12px; border-bottom: 1px solid var(--rule); }
table.findings td { padding: 8px 12px; vertical-align: top; border-bottom: 1px solid var(--rule); }
table.findings tr:last-child td { border-bottom: 0; }
td.id { color: var(--muted); white-space: nowrap; }
td.where { white-space: nowrap; color: var(--muted); }
td.mark { white-space: nowrap; }
.yes { color: var(--done); } .no { color: var(--block); }
.fix { color: var(--muted); }
.fix::before { content: "→ "; }
.empty { padding: 10px 12px; color: var(--muted); }
table.kv { border-collapse: collapse; }
table.kv th { text-align: left; font-weight: 500; color: var(--muted); padding: 4px 24px 4px 0; vertical-align: top; }
table.kv td { padding: 4px 0; word-break: break-all; }
a { color: inherit; text-decoration-color: var(--muted); text-underline-offset: 3px; } a:hover { text-decoration-color: var(--ink); }
ul.log { list-style: none; margin: 0; padding: 0; }
ul.log li { display: grid; grid-template-columns: 200px 1fr; gap: 16px; padding: 4px 0; border-bottom: 1px dotted var(--rule); }
ul.log time { color: var(--muted); }
.muted { color: var(--muted); }
@media (max-width: 720px) {
  main { padding: 32px 16px 64px; } h1 { font-size: 28px; }
  .phase > summary { grid-template-columns: 28px 1fr auto; } .phase .detail { padding-left: 0; }
  details.round summary { grid-template-columns: 60px 1fr; }
  ul.log li { grid-template-columns: 1fr; gap: 0; }
}
"""


def render_round(r: dict, open_: bool) -> str:
    e = html.escape
    n_f = len(r["findings"])
    n_a = sum(f["applied"] for f in r["findings"])
    commit = f'<span class="stat">commit <b>{e(r["commit"][:10])}</b></span>' if r.get("commit") else '<span class="stat"></span>'
    rows = "".join(
        f'<tr><td class="id">{e(f["id"])}</td><td class="where">{e(f["where"])}</td>'
        f'<td>{e(f["finding"])}{"<div class=fix>" + e(f["fix"]) + "</div>" if f["fix"] else ""}</td>'
        f'<td class="mark">{"<span class=yes>safe</span>" if f["safe"] else "<span class=no>unsafe</span>"}</td>'
        f'<td class="mark">{"<span class=yes>適用</span>" if f["applied"] else "<span class=no>見送り</span>"}</td>'
        f'<td>{e(f["note"])}</td></tr>'
        for f in r["findings"]
    )
    table = (
        f'<table class="findings"><thead><tr><th>#</th><th>場所</th><th>指摘 / 直し方</th><th>安全</th><th>採用</th><th>どう直したか / 理由</th></tr></thead>'
        f'<tbody>{rows}</tbody></table>'
        if rows else '<div class="empty">有効な指摘なし</div>'
    )
    summary = f'<div class="summary-text">{e(r["summary"])}</div>' if r.get("summary") else ""
    return (
        f'<details class="round"{" open" if open_ else ""}><summary><span>round {r["n"]}</span><span class="rv">{e(r["reviewer"])}</span>'
        f'<span class="stat">指摘 <b>{n_f}</b> / 採用 <b>{n_a}</b></span>{commit}</summary>'
        f'{summary}{table}</details>'
    )


def value_cell(v: object) -> str:
    text = html.escape(str(v))
    return f'<a href="{text}">{text}</a>' if str(v).startswith("http") else text


def render(state: dict) -> str:
    e = html.escape
    phases = state["phases"]
    done = sum(p["status"] in ("done", "skipped") for p in phases)
    track = "".join(f'<span class="{p["status"]}"></span>' for p in phases)
    items = []
    for i, p in enumerate(phases, 1):
        active = p["status"] in ("in_progress", "blocked")
        last = len(p["rounds"])
        rounds = "".join(render_round(r, active and r["n"] == last) for r in p["rounds"])
        note = f'<div class="note">{e(p["note"])}</div>' if p["note"] else ""
        detail = f'<div class="detail">{note}{rounds}</div>' if (note or rounds) else ""
        items.append(
            f'<li><details class="phase {p["status"]}"{" open" if active else ""}><summary><span class="n">{i:02d}</span>'
            f'<span class="name">{e(p["title"])}<small>{e(p["id"])}</small></span>'
            f'<div class="body"><div class="desc">{e(p["desc"])}</div></div>'
            f'<span class="pill">{STATUS_LABEL[p["status"]]}</span></summary>{detail}</details></li>'
        )
    values = "".join(f"<tr><th>{e(k)}</th><td>{value_cell(v)}</td></tr>" for k, v in state["values"].items())
    log = "".join(f'<li><time>{e(l["at"])}</time><span>{e(l["message"])}</span></li>' for l in state["log"])
    state_json = json.dumps(state, ensure_ascii=False, indent=2).replace("</", "<\\/")
    return f"""<!doctype html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ultra-ship: {e(state['branch'])}</title>
<style>{CSS.replace("PHASE_COUNT", str(len(PHASES)))}</style>
</head>
<body>
<main>
<header>
  <div>
    <div class="kicker">ultra-ship checkpoint</div>
    <h1>{e(state['branch'])}<span class="arrow">→</span>{e(state['base'])}</h1>
  </div>
  <div class="stamp"><b>{done}<span class="muted">/{len(phases)}</span></b>更新 {e(state['updated_at'])}<br>作成 {e(state['created_at'])}</div>
</header>
<div class="track">{track}</div>
<ol class="phases">{''.join(items)}</ol>
<h2>値</h2>
<table class="kv">{values or '<tr><td class="muted">（なし）</td></tr>'}</table>
<h2>ログ</h2>
<ul class="log">{log or '<li class="muted">（なし）</li>'}</ul>
</main>
<script id="{STATE_TAG_ID}" type="application/json">
{state_json}
</script>
</body>
</html>
"""


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", default="z", help="チェックポイントを置くディレクトリ（既定 z）")
    ap.add_argument("--file", default="ultra-ship.html", help="ファイル名（既定 ultra-ship.html）")
    sub = ap.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("init")
    p.add_argument("--base", required=True, help="base branch 名（例: main）")

    sub.add_parser("status")
    sub.add_parser("path")

    p = sub.add_parser("phase")
    p.add_argument("id")
    p.add_argument("status", choices=STATUSES)
    p.add_argument("--note", default=None)

    p = sub.add_parser("round")
    p.add_argument("id")
    p.add_argument("--reviewer", required=True, help="reviewers.py の id（例: cursor:grok-4.6, host）")
    p.add_argument("--findings", type=Path, required=True, help="findings.json")
    p.add_argument("--summary", default="")
    p.add_argument("--commit", default="", help="適用をコミットした SHA")

    p = sub.add_parser("set")
    p.add_argument("key")
    p.add_argument("value")

    p = sub.add_parser("log")
    p.add_argument("message")

    a = ap.parse_args()
    path = checkpoint_path(a.root, a.file)

    if a.cmd == "path":
        print(path)
        return

    if a.cmd == "init":
        ensure_excluded(a.root)
        state = load(path)
        if state is None:
            state = new_state(a.base)
            state["log"].append({"at": now(), "message": "チェックポイントを作成"})
        elif state["base"] != a.base:
            state["base"] = a.base
            state["log"].append({"at": now(), "message": f"base を {a.base} に変更"})
        save(path, state)
        print(path)
        return

    state = load(path)
    if state is None:
        sys.exit(f"{path} が無い。先に `checkpoint.py init --base <base>` を実行する")

    if a.cmd == "status":
        print(json.dumps(state, ensure_ascii=False, indent=2))
        return

    if a.cmd == "phase":
        ph = find_phase(state, a.id)
        ph["status"] = a.status
        if a.note is not None:
            ph["note"] = a.note
        state["log"].append({"at": now(), "message": f"{a.id} → {a.status}" + (f": {a.note}" if a.note else "")})
    elif a.cmd == "round":
        ph = find_phase(state, a.id)
        findings = load_findings(a.findings)
        n = len(ph["rounds"]) + 1
        ph["rounds"].append({
            "n": n, "at": now(), "reviewer": a.reviewer, "summary": a.summary, "commit": a.commit, "findings": findings,
        })
        applied = sum(f["applied"] for f in findings)
        state["log"].append({"at": now(), "message": f"{a.id} round {n} ({a.reviewer}): 指摘 {len(findings)} / 採用 {applied}"})
    elif a.cmd == "set":
        state["values"][a.key] = a.value
        state["log"].append({"at": now(), "message": f"{a.key} = {a.value}"})
    elif a.cmd == "log":
        state["log"].append({"at": now(), "message": a.message})

    save(path, state)
    print(path)


if __name__ == "__main__":
    main()
