# uv スクリプト チートシート

uv 0.12 系で確認。公式: https://docs.astral.sh/uv/guides/scripts/

## インラインメタデータ（PEP 723）

```python
#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12"
# dependencies = [
#     "httpx>=0.27",
#     "rich",
# ]
# [tool.uv]
# exclude-newer = "2026-08-23T12:34:56Z"
# ///
```

- ブロックは `# /// script` で始まり `# ///` で終わる。各行は `# ` で始める。shebang を使うならブロックより前の 1 行目に置く
- `requires-python`: `>=3.X`。無いときは uv の既定 Python で動くが、明示する
- `dependencies`: **空でも必須**。PEP 508 の指定子（`'requests<3'`、`'rich>=13,<14'`、`'pkg[extra]'`）が書ける
- `[tool.uv]`: `exclude-newer` のほか、`[[tool.uv.index]]`（`url = "..."`）で別 index、`[tool.uv.sources]` で git / path 指定ができる
- `exclude-newer` は RFC 3339（`2006-12-02T02:07:43Z`）。この時刻より後に PyPI に上がった配布物を候補から外す。`uv run` / `uv add --script` / `uv lock --script` / `uv tree --script` すべてが尊重する

## コマンド

| やること | コマンド | 備考 |
| --- | --- | --- |
| 雛形 | `uv init --script x.py --python 3.12` | `requires-python` と空の `dependencies`、`main()` が入る |
| 依存追加 | `uv add --script x.py 'requests<3' rich` | メタデータが無ければブロックごと作る。`[tool.uv]` は保持される。`--exclude-newer` を渡しても日付はメタデータに書かれないので手で書く |
| 依存削除 | `uv remove --script x.py rich` | |
| 実行 | `uv run x.py [args]` | `.py` なら `--script` 省略可。`--python 3.11` で Python を差し替え |
| 実行（拡張子なし / stdin） | `uv run --script ./tool` / `echo 'print(1)' \| uv run -` | |
| 使い捨て実験 | `uv run --with rich --with 'httpx<1' x.py` | ファイルに残すスクリプトでは使わない |
| 依存ツリー | `uv tree --script x.py` | |
| ロック | `uv lock --script x.py` | 隣に `x.py.lock`。以降の run / add / export / tree が使う |
| requirements 出力 | `uv export --script x.py` | |
| 別 index | `uv add --index https://example.com/simple --script x.py pkg` | `[[tool.uv.index]]` がメタデータに入る |

## project との関係

- `pyproject.toml` のあるディレクトリで `uv run x.py` すると、通常は先にプロジェクトをインストールして、その環境で動く
- **インラインメタデータがあるスクリプトはプロジェクトを無視する**（`--no-project` 不要）
- メタデータの無いスクリプトをプロジェクト内で単体で動かすときは `uv run --no-project x.py`（`--no-project` はスクリプト名より前）
- `uv run --with` をプロジェクト内で使うと、プロジェクト依存 + `--with` になる

## `exclude-newer` の運用

- 新規に書く: `date -u +%Y-%m-%dT%H:%M:%SZ` の値を書く。依存を足す前に書く
- 依存を更新する: 日付を進めて `uv run`（`.lock` があれば `uv lock --script`）。消すと「最新」が毎回変わる
- 環境変数 `UV_EXCLUDE_NEWER` や `--exclude-newer` でも同じ効果が出るが、スクリプト単体で再現できなくなるのでメタデータに書く
- 書いた日付より新しいパッケージが必要になった（新 API を使いたい等）ら、そのときに日付を進める

## Python バージョン

- `requires-python` を満たす Python が無ければ uv がダウンロードする
- `uv run --python 3.11 x.py` で一時的に差し替え。常用するなら `requires-python` を変える
- 3.12 以降の構文（`type X = ...` など）を使うなら `requires-python = ">=3.12"` を忘れない

## トラブルシュート

- `ModuleNotFoundError`: メタデータの `dependencies` に入っていない。`uv add --script` で足す。`# /// script` の綴りや各行の `# ` が崩れていないかも見る
- 依存を足したら解決に失敗する: `exclude-newer` が古すぎて要求バージョンが存在しない。日付を進めるか、バージョン指定を緩める
- プロジェクト内でプロジェクトの依存が勝手に入る: メタデータが無い。付けるか `--no-project`
- Windows で GUI スクリプト: 拡張子を `.pyw` にすると `pythonw` で動く
