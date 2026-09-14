---
name: uv-script
description: uv の PEP 723 インラインメタデータで、単一ファイルで完結する Python スクリプトを書く・直す・実行する。依存を入れるときは `[tool.uv] exclude-newer` を必ず付けて再現性を保つ。「Python でスクリプトを書いて」「uv でスクリプト」「単発の Python を書いて」「この .py に依存を足して」「`# /// script` のファイルを直して」「uv run で動くようにして」と言われたら使う。pyproject.toml を持つパッケージやアプリの開発には使わない。
---

# uv-script

`python foo.py` で動かす単発のスクリプトを、venv や requirements.txt を作らずに書く。依存とバージョンは **スクリプト先頭のインラインメタデータ**（PEP 723、`# /// script` ブロック）に書き、`uv run` が実行のたびに環境を用意する。

原則:

- **1 ファイルで完結させる**。依存・Python バージョン・`exclude-newer` はすべてメタデータに入れ、`pip install` / `uv venv` / `uv pip` / requirements.txt は使わない
- **依存が 1 つでもあるなら `[tool.uv] exclude-newer` を必ず書く**。これが無いと後日実行したときに解決結果が変わる。値はスクリプトを書いた時点の UTC 時刻（RFC 3339）
- **`exclude-newer` を先に書いてから `uv add --script` する**。`uv add --script --exclude-newer` はメタデータに日付を書いてくれない（uv 0.12 時点）ので手で書く。先に書いておけば `uv add` が付ける下限バージョンもその日付に揃う
- **`dependencies = []` は空でも必須**。`requires-python` は `>=3.X` で明示する
- **`uv run --with` は使い捨ての実験専用**。ファイルとして残すスクリプトには使わない
- 書き上げたら **必ず `uv run` で一度動かす**

コマンドの詳細と細かい挙動は [references/cheatsheet.md](references/cheatsheet.md)。

## 手順

### 1. 雛形を作る

```bash
uv init --script <name>.py --python 3.12
```

`requires-python`、空の `dependencies`、`main()` 付きの雛形ができる。既存ファイルに足す場合はこのステップを飛ばし、先頭にブロックを手で書く。Python のバージョンは、指定が無ければ uv が既定で選ぶ最新の安定版（`uv python list` で確認）にする。

### 2. `exclude-newer` を書く（依存がある場合は必須）

```bash
date -u +%Y-%m-%dT%H:%M:%SZ
```

で得た時刻を、メタデータの `dependencies` の後ろに `[tool.uv]` テーブルとして書く:

```python
# /// script
# requires-python = ">=3.12"
# dependencies = []
# [tool.uv]
# exclude-newer = "2026-08-23T12:34:56Z"
# ///
```

依存を使わないスクリプトでも付けてよい（害は無い）。後から依存を足すときに書き忘れないので、迷ったら付ける。

### 3. 依存を足す

```bash
uv add --script <name>.py 'requests<3' rich
```

`dependencies` に追記され、`exclude-newer` の日付時点で解決できる下限（`>=x.y.z`）が付く。`[tool.uv]` テーブルは保持される。上限は必要なときだけ自分で書く（`'requests<3'` のように）。

手で `dependencies` に書いてもよいが、その場合も `exclude-newer` を先に入れておく。

### 4. 本体を書く

- `def main() -> None:` と `if __name__ == "__main__": main()` の形にする
- 引数は `argparse`、標準ライブラリで済むものは依存に入れない
- 外部 API を叩く・ファイルを書く処理は、引数で対象を受け取り、`--dry-run` を付けられる形にする
- CLI として PATH に置くなら shebang `#!/usr/bin/env -S uv run --script` を 1 行目に書いて `chmod +x`（拡張子無しでもよい）

### 5. 実行して確認する

```bash
uv run <name>.py [args...]
```

- `pyproject.toml` のあるディレクトリでも、インラインメタデータがあればプロジェクトの依存は無視される（`--no-project` 不要）。メタデータが無いスクリプトを project 内で動かすときだけ `uv run --no-project`
- 拡張子が `.py` でない、または stdin から渡すときは `uv run --script <file>` / `uv run -`
- 依存のツリーは `uv tree --script <name>.py`

完了条件: `# /// script` ブロックに `requires-python`・`dependencies`・（依存があるなら）`[tool.uv] exclude-newer` があり、`uv run` が通っている。

## 既存スクリプトを直すとき

- `exclude-newer` が無いのに依存があるなら、**まず今日の時刻で `exclude-newer` を足してから**依存を足す。既存の依存の解決結果は今日時点の最新に固定される
- 依存を更新したいときは `exclude-newer` の日付を進める（消さない）。`.lock` があれば `uv lock --script` で取り直す
- `uv run --with` や `pip install` 前提のコメントが残っていたらメタデータに移す

## ロックする（任意）

リポジトリに入れて複数人・CI で動かすスクリプトは `uv lock --script <name>.py` で隣に `<name>.py.lock` を作ってコミットする。以降の `uv run` / `uv add --script` はロックを使う。`exclude-newer` だけで十分な個人用スクリプトでは作らなくてよい。
