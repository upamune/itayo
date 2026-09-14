#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12"
# dependencies = []
# [tool.uv]
# exclude-newer = "2026-08-30T07:45:12Z"
# ///
"""ultra-ship のレビュアー台帳。使える CLI / モデルを優先順に並べ、選んで、実行する。

  reviewers.py list                       # 台帳を優先順に表示（✓ 使える / ✗ 理由）
  reviewers.py pick [--role review|apply] [--n N] [--exclude ID,...] [--json]
  reviewers.py run <id> --role review|apply --prompt-file P [--skill SKILL.md] [--out F] [--cwd DIR] [--timeout SEC]
  reviewers.py host                       # ultra-ship を動かしているホストを表示

役割:
  review: 読み取り専用で指摘だけ返す
  apply : 作業ツリーに書き込んで指摘を適用する（安いモデルに振る）

`host` は ultra-ship を動かしているエージェント自身のサブエージェント。常に使え、外部 CLI が
一つも無い環境（Cursor Cloud / Claude Cloud / Codex Cloud）では pick がこれだけを返す。
run は host には対応しない（ホストの Agent 機構でスキル側が回す）。

run の注意:
  外部 CLI はリポジトリ外のファイル（~/.claude/skills 等）を読めないことがある（opencode2 は自動拒否）。
  スキル定義は --skill で渡すと本文がプロンプトの先頭に埋め込まれる。既定タイムアウトは 30 分。

環境変数:
  ULTRA_SHIP_REVIEWERS=id,id,...   台帳の順序と対象を上書き
  ULTRA_SHIP_PERSONAL=1            個人機扱い（既定は $USER == upamune）
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

PERSONAL_USER = "upamune"
HOST_ID = "host"
OPENCODE2 = ["mise", "x", "npm:@opencode-ai/cli@beta", "--", "opencode2"]


@dataclass
class Reviewer:
    id: str
    label: str
    personal_only: bool
    review: list[str] | None
    apply: list[str] | None
    check: str  # 可用性チェックの種類
    eta: str  # レビュー 1 回の目安時間（小さい差分での実測。差分が大きければ伸びる）

    def command(self, role: str) -> list[str] | None:
        return self.review if role == "review" else self.apply


# 上から優先。{prompt} はプロンプト本文に置き換える。
LEDGER: list[Reviewer] = [
    Reviewer(
        "opencode:glm-5.3-flash", "OpenCode v2 / Ollama Cloud GLM-5.3 Flash", True,
        [*OPENCODE2, "run", "--agent", "plan", "--model", "ollama-cloud/glm-5.3-flash", "{prompt}"],
        [*OPENCODE2, "run", "--agent", "build", "--model", "ollama-cloud/glm-5.3-flash", "{prompt}"],
        "opencode:glm-5.3-flash:cloud",
        "約 1 分",
    ),
    Reviewer(
        "opencode:deepseek-v4-flash", "OpenCode v2 / Ollama Cloud DeepSeek V4 Flash", True,
        [*OPENCODE2, "run", "--agent", "plan", "--model", "ollama-cloud/deepseek-v4-flash", "{prompt}"],
        [*OPENCODE2, "run", "--agent", "build", "--model", "ollama-cloud/deepseek-v4-flash", "{prompt}"],
        "opencode:deepseek-v4-flash:cloud",
        "約 20 秒",
    ),
    Reviewer(
        "cursor:grok-4.6", "Cursor CLI / Grok 4.6 (high)", False,
        ["cursor-agent", "-p", "--mode=plan", "--output-format", "json", "--model", "cursor-grok-4.6-high", "{prompt}"],
        ["cursor-agent", "-p", "-f", "--output-format", "json", "--model", "cursor-grok-4.6-high", "{prompt}"],
        "cursor",
        "約 4〜5 分",
    ),
    Reviewer(
        "codex:gpt-5.6-sol", "Codex CLI / GPT-5.6 Sol", False,
        ["codex", "exec", "-m", "gpt-5.6-sol", "--sandbox", "read-only", "{prompt}"],
        ["codex", "exec", "-m", "gpt-5.6-sol", "--full-auto", "{prompt}"],
        "codex",
        "約 2 分",
    ),
    Reviewer(
        "claude:opus-5", "Claude Code / Opus 5（apply は Sonnet 5）", False,
        ["claude", "-p", "--model", "opus", "--permission-mode", "plan", "{prompt}"],
        ["claude", "-p", "--model", "sonnet", "--permission-mode", "acceptEdits", "{prompt}"],
        "claude",
        "数分",
    ),
]


HOST = Reviewer(HOST_ID, "ホスト自身のサブエージェント（Agent ツール等）", False, None, None, "host", "数分")


def is_personal() -> bool:
    return os.environ.get("ULTRA_SHIP_PERSONAL") == "1" or os.environ.get("USER") == PERSONAL_USER


def detect_host() -> str:
    if os.environ.get("CLAUDECODE"):
        return "claude-code"
    if os.environ.get("CODEX_SANDBOX") or os.environ.get("CODEX_THREAD_ID"):
        return "codex"
    if os.environ.get("CURSOR_AGENT"):
        return "cursor"
    return "unknown"


def run_quiet(cmd: list[str], timeout: int = 60) -> subprocess.CompletedProcess[str] | None:
    try:
        return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return None


def availability(r: Reviewer) -> tuple[bool, str]:
    if r.personal_only and not is_personal():
        return False, "個人機のみ（ULTRA_SHIP_PERSONAL=1 で解禁）"
    kind, _, model = r.check.partition(":")
    if kind == "cursor":
        if not shutil.which("cursor-agent"):
            return False, "cursor-agent が無い"
        return True, "cursor-agent"
    if kind == "opencode":
        if not shutil.which("mise"):
            return False, "mise が無い"
        if not shutil.which("ollama"):
            return False, "ollama が無い"
        p = run_quiet(["ollama", "show", model])
        if p is None or p.returncode != 0:
            return False, f"ollama に {model} が無い（ollama pull {model}）"
        return True, f"opencode2 + {model}"
    if kind == "codex":
        if not shutil.which("codex"):
            return False, "codex が無い"
        if not (Path.home() / ".codex" / "auth.json").exists():
            return False, "codex 未ログイン"
        return True, "codex"
    if kind == "claude":
        if detect_host() == "claude-code":
            return False, "Claude Code 内では host を使う"
        if not shutil.which("claude"):
            return False, "claude が無い"
        return True, "claude"
    if kind == "host":
        return True, detect_host()
    return False, f"unknown check: {r.check}"


def ordered_ledger() -> list[Reviewer]:
    override = os.environ.get("ULTRA_SHIP_REVIEWERS")
    if not override:
        return [*LEDGER, HOST]
    by_id = {r.id: r for r in [*LEDGER, HOST]}
    ids = [s.strip() for s in override.split(",") if s.strip()]
    unknown = [i for i in ids if i not in by_id]
    if unknown:
        sys.exit(f"ULTRA_SHIP_REVIEWERS に未知の id: {', '.join(unknown)}（候補: {', '.join(by_id)}）")
    return [by_id[i] for i in ids]


def cmd_list() -> None:
    print(f"host={detect_host()} personal={is_personal()}  （~時間は小さい差分での実測。大きい差分では伸びる。上限は run の --timeout、既定 30 分）")
    for i, r in enumerate(ordered_ledger(), 1):
        ok, why = availability(r)
        mark = "✓" if ok else "✗"
        print(f"{i}. {mark} {r.id:28} {r.label}  [{why}]  ~{r.eta}")


def pick(n: int, exclude: set[str]) -> list[Reviewer]:
    out = []
    for r in ordered_ledger():
        if r.id in exclude:
            continue
        if availability(r)[0]:
            out.append(r)
        if len(out) >= n:
            break
    if not out and HOST_ID not in exclude:
        out.append(HOST)
    return out


def cmd_pick(role: str, n: int, exclude: set[str], as_json: bool) -> None:
    chosen = pick(n, exclude)
    if as_json:
        print(json.dumps([{"id": r.id, "label": r.label, "eta": r.eta, "command": r.command(role)} for r in chosen], ensure_ascii=False, indent=2))
    else:
        for r in chosen:
            print(r.id)
    if not chosen:
        sys.exit(1)


def build_prompt(prompt_file: Path, skill: Path | None) -> str:
    prompt = prompt_file.read_text()
    if skill is None:
        return prompt
    return f"## スキル定義（{skill.name}）\n\n{skill.read_text().strip()}\n\n## 依頼\n\n{prompt}"


def extract_result(stdout: str) -> str:
    """cursor-agent の --output-format json は {"type": "result", "result": "..."} を返す。それ以外はそのまま。"""
    try:
        d = json.loads(stdout)
    except json.JSONDecodeError:
        return stdout
    return d["result"] if isinstance(d, dict) and isinstance(d.get("result"), str) else stdout


def cmd_run(rid: str, role: str, prompt_file: Path, skill: Path | None, out: Path | None, cwd: Path | None, timeout: int) -> None:
    if rid == HOST_ID:
        sys.exit("host は run できない。ホストのサブエージェント機構でプロンプトを実行する")
    r = next((x for x in LEDGER if x.id == rid), None)
    if r is None:
        sys.exit(f"unknown reviewer: {rid}")
    ok, why = availability(r)
    if not ok:
        sys.exit(f"{rid} は使えない: {why}")
    cmd = r.command(role)
    prompt = build_prompt(prompt_file, skill)
    argv = [prompt if a == "{prompt}" else a for a in cmd]
    print(f"$ {' '.join(a if a != prompt else '<prompt>' for a in argv)}", file=sys.stderr)
    try:
        p = subprocess.run(argv, cwd=cwd, capture_output=True, text=True, timeout=timeout, stdin=subprocess.DEVNULL)
    except subprocess.TimeoutExpired:
        sys.exit(f"{rid} が {timeout} 秒で応答しなかった")
    text = extract_result(p.stdout) if p.stdout.strip() else p.stderr
    if out:
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(text if text.endswith("\n") else text + "\n")
        print(out)
    else:
        print(text)
    if p.returncode != 0:
        if text != p.stderr:
            print(p.stderr, file=sys.stderr)
        sys.exit(p.returncode)


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("list")
    sub.add_parser("host")
    p = sub.add_parser("pick")
    p.add_argument("--role", choices=("review", "apply"), default="review")
    p.add_argument("--n", type=int, default=1)
    p.add_argument("--exclude", default="")
    p.add_argument("--json", action="store_true")
    p = sub.add_parser("run")
    p.add_argument("id")
    p.add_argument("--role", choices=("review", "apply"), required=True)
    p.add_argument("--prompt-file", type=Path, required=True)
    p.add_argument("--skill", type=Path, help="SKILL.md。本文をプロンプト先頭に埋め込む")
    p.add_argument("--out", type=Path)
    p.add_argument("--cwd", type=Path)
    p.add_argument("--timeout", type=int, default=1800, help="秒（既定 1800）")
    a = ap.parse_args()

    if a.cmd == "list":
        cmd_list()
    elif a.cmd == "host":
        print(detect_host())
    elif a.cmd == "pick":
        cmd_pick(a.role, a.n, {s.strip() for s in a.exclude.split(",") if s.strip()}, a.json)
    elif a.cmd == "run":
        cmd_run(a.id, a.role, a.prompt_file, a.skill, a.out, a.cwd, a.timeout)


if __name__ == "__main__":
    main()
