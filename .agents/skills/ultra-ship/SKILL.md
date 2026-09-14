---
name: ultra-ship
description: 実装が終わったブランチを「コミット → base merge と衝突解消 → 4 種のレビューを Cursor / OpenCode / Codex / Claude / ホストのサブエージェントをローテーションして指摘ゼロまで反復 → PR 作成・整備 → pr-review-canvas で説明 → CI が green になるまで修正」まで一気に持っていく。進捗は z/<branch>/ultra-ship.html にチェックポイントとして残し、途中から再開できる。
disable-model-invocation: true
---

# ultra-ship

実装済みのブランチを、レビュー可能で CI が通る PR にするまでを 1 回の呼び出しで終わらせる。既存スキルの組み合わせで、順番と「終了条件」をここで固定する。各フェーズの具体的な手順、レビュアー台帳、レビュアーと適用役に渡すプロンプトは [references/phases.md](references/phases.md)。

原則:

- **チェックポイント優先**。始める前に必ず `checkpoint.py status` を試し、あれば `done` 以外の最初のフェーズから再開する。ただし記録を鵜呑みにせず、git / gh の実状態（未コミット差分、merge 中か、PR の有無、CI 状態）と突き合わせてから進む
- **フェーズの節目ごとに必ずチェックポイントを更新する**。`phase <id> in_progress` で入り、`done` / `skipped` / `blocked` で抜ける。レビューは 1 ラウンドごとに `round` で「誰が、何を指摘し、何をどう直したか（見送ったなら理由）」を指摘 1 件単位で記録する。後から振り返る材料はここにしか残らない
- **自明に良い指摘は勝手に採用する**。挙動を変えない整理、命名、重複除去、不要コメント削除、規約違反の修正は聞かずに直す。挙動やスコープが変わるもの、spec と矛盾するものだけ「採用しなかった指摘」として最後に報告する
- **レビューは毎ラウンド別のレビュアー**で行う。`scripts/reviewers.py` の台帳（OpenCode+GLM-5.3 Flash → OpenCode+DeepSeek V4 Flash → Cursor+Grok 4.6 → Codex → Claude → `host`。1 ラウンド数分から、差分が大きければ最大 30 分待つ）を上から順にローテーションし、前ラウンドの文脈を引き継がせない。Claude / Codex は台帳の下位なので、上位モデルで指摘が尽きれば呼ばれない（使用量のオフロード）。外部 CLI が一つも無い環境（各社の Cloud）では `host`（ホスト自身のサブエージェント）だけで回す。有効な指摘がゼロになったらそのスキルは終了、上限は各 4 ラウンド（超えたら `blocked` にして理由を書き、次へ進む）
- **レビュー（読み取り専用）と適用（書き込み）を分ける**。外部レビュアーは指摘だけ返し、適用は `reviewers.py pick --role apply` で選んだ安い書き込み可能なレビュアー、無ければホストの安いサブエージェント（Claude Code なら `model: sonnet`）がやる。作業ツリーに同時に書くエージェントは常に 1 つ
- **コードを触ったラウンドの後は必ずプロジェクトの検査（typecheck / lint / test）を通してからコミット**する
- **`z/` 以下（チェックポイント、レビュー結果、canvas の HTML）は絶対にコミットしない**。`checkpoint.py init` が `.git/info/exclude` に `/z/` を入れるが、`git add -A` や `git add -f` で混ざらないよう、コミット前に `git status --short` に `z/` が無いことを確認する。混ざっていたら `git rm --cached -r z/` で外す
- 破壊的な git 操作（`push --force`、`reset --hard`、`--no-verify`）はしない。履歴の書き換えは make-pr-easy-to-review の手順どおり、未 push のコミットに限る

## 手順

```
S=<このスキルのディレクトリ>/scripts/checkpoint.py
R=<このスキルのディレクトリ>/scripts/reviewers.py
$S init --base <base>      # 既存なら状態を保持したまま再描画。z/ は .git/info/exclude に入れる
$S status                  # 再開時はまずこれを読む
$R list                    # 使えるレビュアーを確認し、$S set reviewers "<id,id,...>" で記録
```

base は `gh pr view --json baseRefName` があればそれ、無ければ `gh repo view --json defaultBranchRef -q .defaultBranchRef.name`。

| # | phase id | やること | 使うスキル | 終了条件 |
| --- | --- | --- | --- | --- |
| 1 | `commit` | `git status` で漏れを確認し、論理単位でコミット。生成物・秘密情報が混ざっていないか見る | (なし) | 作業ツリーがクリーン |
| 2 | `merge` | `git fetch origin <base>` して `git merge origin/<base>`。衝突したら解消して検査を通す | resolving-merge-conflicts | merge コミット済みで検査が通る |
| 3 | `review:thermo` | 構造・抽象の大きな見直し。構造の問題を最初に潰す | thermo-nuclear-code-quality-review | 有効な指摘 0 |
| 4 | `review:simplify` | 挙動を変えずに読みやすく | simplify | 有効な指摘 0 |
| 5 | `review:deslop` | AI 由来の冗長さ・不要な防御・コメントを除去 | deslop | 有効な指摘 0 |
| 6 | `review:code-review` | 規約（Standards）と spec の両軸で最終確認。fixed point は `origin/<base>` の merge-base | code-review | 有効な指摘 0 |
| 7 | `pr` | PR が無ければ push して **draft で**作成（指示が無い限り ready にしない）、あれば説明を更新。レビュー観点の案内を付ける | make-pr-easy-to-review | PR URL が `values.pr_url` に入る |
| 8 | `canvas` | PR の変更内容を HTML で説明し、`z/<branch>/pr-review.html` にも保存する | pr-review-canvas | `values.canvas_path` が入る |
| 9 | `ci` | チェックを見て失敗を直し、push して green まで繰り返す | loop-on-ci | 全チェック green |
| 10 | (verify) | `scripts/verify.py` で全項目を突き合わせる | (なし) | exit 0 |

3〜6 は順番に意味がある。構造（thermo）を直してから整える（simplify）、その後に細かい slop を落とし（deslop）、最後に規約と spec に照らす（code-review）。code-review の指摘で構造が変わったら 3 に戻す（上限 1 回）。

レビュアー（外部 CLI でもホストのサブエージェントでも）には必ず次を渡す: 使うスキルの `SKILL.md` の本文（`$R run --skill` が埋め込む。外部 CLI はリポジトリ外を読めないことがあり、`disable-model-invocation: true` のスキルは Skill ツールでも呼べない）、diff の範囲（`git diff origin/<base>...HEAD`）、「指摘のみ・編集しない」こと、返答フォーマット（指摘の一覧と、それぞれが自明に適用してよいものか）。適用役には指摘一覧と「挙動が変わるものは適用せず報告」の線引きを渡す。プロンプト例は [references/phases.md](references/phases.md)。

## 最後の報告

チェックポイントを `$S status` で読み直して、次を短くまとめる:

- PR URL と canvas の HTML パス
- 各レビューのラウンド数、どのレビュアーが見たか、採用した主な変更
- 採用しなかった指摘とその理由
- CI の最終状態と、直した失敗
- `blocked` にしたフェーズがあればその理由と、人が判断すべきこと

完了条件: `scripts/verify.py` が全項目 ✓ で exit 0（チェックポイント・作業ツリー・base 取り込み・push・PR・canvas・CI を機械的に突き合わせる）。✗ が残っていたら該当フェーズに戻り、勝手に「完了」と報告しない。
