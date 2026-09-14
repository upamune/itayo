# ultra-ship: フェーズ詳細

`SKILL.md` の表の各行を、実際に叩くコマンドとレビュアーへのプロンプトまで落としたもの。`$S` は `scripts/checkpoint.py`、`$R` は `scripts/reviewers.py`、`<base>` は base branch、`<skills>` はスキルのインストール先（通常 `~/.claude/skills`、無ければ `~/.agents/skills`）。

## レビュアー台帳（`$R`）

```
$R list                                  # 優先順に ✓/✗ と理由
$R pick --role review --n 5              # 使える review 役を上から N 件
$R pick --role apply --n 1               # 使える apply 役（書き込み可）を 1 件
$R run <id> --role review --prompt-file P --skill <SKILL.md> --out F   # 外部 CLI を読み取り専用で実行（既定 30 分で打ち切り、--timeout で変更）
$R host                                  # claude-code / codex / cursor / unknown
```

| 順 | id | 中身 | 条件 | 目安時間 |
| --- | --- | --- | --- | --- |
| 1 | `opencode:glm-5.3-flash` | `opencode2 run --model ollama-cloud/glm-5.3-flash`（review は `--agent plan`、apply は `--agent build`） | 個人機かつ `ollama show glm-5.3-flash:cloud` が通る | 約 1 分 |
| 2 | `opencode:deepseek-v4-flash` | 同上 DeepSeek V4 Flash | 同上 | 約 20 秒 |
| 3 | `cursor:grok-4.6` | `cursor-agent --output-format json --model cursor-grok-4.6-high`（review は `--mode=plan`、apply は `-f`）。`text` 形式は長い依頼で最終出力が落ちるので json 固定 | `cursor-agent` がある | 約 4〜5 分 |
| 4 | `codex:gpt-5.6-sol` | `codex exec -m gpt-5.6-sol`（review は `--sandbox read-only`、apply は `--full-auto`） | `codex` とログイン | 約 2 分 |
| 5 | `claude:opus-5` | `claude -p --model opus`（apply は `sonnet`） | ホストが Claude Code でないとき | 数分 |
| 6 | `host` | ホスト自身のサブエージェント。常に使える | (なし) | 数分 |

実測（deslop 基準で 2 ファイルをレビュー）: GLM 59 秒で 4 件（全部妥当）、DeepSeek 16 秒で 1 件、Cursor（Grok 4.6 high）255 秒で 4 件（全部妥当）、Codex 108 秒で 1 件。Cursor は遅いが指摘の質が高く、Codex / Claude の使用量を食わないので Codex より前。`cursor-grok-4.6-high-fast` は 212 秒で指摘本文を返さなかった（前置きだけで終わる）ので使わない。時間は `$R list` にも出るが、**どれも小さい差分（2 ファイル）での値**で、差分が大きければそのぶん伸びる。上限は `run` の `--timeout`（既定 1800 秒 = 30 分）で、そこまでは待つ。短くしない。30 分で返らなければそのレビュアーを `blocked` 扱いにして次のレビュアーで同じラウンドをやり直す。

- 個人機判定は `$USER == upamune` か `ULTRA_SHIP_PERSONAL=1`。会社機では 1・2 が落ちて Cursor → Codex → Claude → host になる
- `ULTRA_SHIP_REVIEWERS=codex:gpt-5.6-sol,host` のように順序と対象を上書きできる
- 各社の Cloud 環境（外部 CLI が無い）では `pick` が `host` だけを返す。その場合はラウンドごとに新しいサブエージェントを起動して同じ手順を回す（Claude Code なら `Agent` ツール、Codex / Cursor ならそれぞれのサブエージェント機構）
- `host` は `$R run` できない。プロンプトファイルの中身をそのままサブエージェントに渡す
- **スキル定義はパスではなく本文で渡す**。外部 CLI はリポジトリ外（`~/.claude/skills`）を読めないことがある（opencode2 は自動拒否）。`$R run --skill <SKILL.md>` が本文をプロンプト先頭に埋め込む。`host` に渡すときも SKILL.md を読んで本文を貼る

## 0. 再開

```bash
$S status 2>/dev/null   # 無ければ init から
```

`status` があるときは、`done` / `skipped` 以外の最初のフェーズから始める。ただし次を実状態で確認してから:

- `commit`: `git status --porcelain` が空か
- `merge`: `git merge HEAD` が「Already up to date」か、`.git/MERGE_HEAD` が残っていないか
- `pr`: `gh pr view --json url` が通るか（通れば `values.pr_url` を埋めて `done` に更新）
- `ci`: `gh pr checks` の結果

記録と実状態が食い違えば実状態を正とし、`$S log` に何を直したか残す。

## 1. commit

```bash
git status --porcelain
git diff --stat
```

- 変更を論理単位（機能 / テスト / 生成物 / 設定）に分けてコミットする。コミットメッセージはその repo の慣習（言語・prefix・本文の形式）に合わせる
- `.env`、鍵、巨大バイナリ、ローカル専用ファイルが混ざっていたら除外して報告する
- `z/` は `.git/info/exclude` に入っているので通常は含まれない。それでも `git status --short | grep '^A.*z/'` で混入が無いことを確認してからコミットする。混ざっていたら `git rm --cached -r z/`

## 2. merge

```bash
git fetch origin <base>
git merge origin/<base>
```

- 衝突が出たら `<skills>/resolving-merge-conflicts/SKILL.md` の手順に従う（両方の意図を保つ、`--abort` しない、検査を通して merge コミット）
- 衝突なしで進んだ場合も検査（typecheck / lint / test）は 1 回通す。検査コマンドは `package.json` / `Makefile` / `mise.toml` / CI 定義から見つける

## 3〜6. レビュー反復

最初に `$R pick --role review --n 9` でレビュアーの順番を決め、`$S set reviewers "<id,id,...>"` に記録する。各スキルについて、次のループを回す:

```
reviewers = pick の結果（ラウンド r では reviewers[(r-1) % len] を使う）
round = 1
loop:
  $S phase review:<id> in_progress
  reviewer = reviewers[(round-1) % len]
  レビュープロンプトを z/<branch>/reviews/<id>-r<round>.prompt.md に書く
  reviewer が host なら 新しいサブエージェントに渡す、それ以外は
    $R run <reviewer> --role review --prompt-file ... --skill <skills>/<skill>/SKILL.md --out z/<branch>/reviews/<id>-r<round>.<reviewer>.md
  指摘を読み、「自明に適用してよい」ものを選ぶ（N = 有効な指摘数、M = 適用する数）
  M > 0 なら 適用役（下記）に渡して直させ、検査を通してコミット（本文にレビュアーと採用内容を書く）
  指摘 1 件ごとに {id, where, finding, fix, safe, applied, note} を z/<branch>/reviews/<id>-r<round>.json に書く
    note には「どう直したか」または「なぜ見送ったか」を必ず書く（後から振り返るのはここ）
  $S round review:<id> --reviewer <reviewer> --findings z/<branch>/reviews/<id>-r<round>.json --summary "..." --commit <sha>
  N == 0（または残りが「未適用と判断したもの」だけ）なら $S phase review:<id> done → 次のスキルへ
  round == 4 なら $S phase review:<id> blocked --note "上限。残: ..." → 次のスキルへ
  round += 1
```

ラウンドごとにレビュアーを変えるのは、同じ目で見直すと前ラウンドの結論を繰り返すだけになるため。前ラウンドの指摘一覧は渡さず、「未適用と判断した指摘」だけを渡して再提案させない。

適用役は `$R pick --role apply --n 1` で選ぶ。`host` が返ったら、ホストの安いサブエージェント（Claude Code なら `Agent` ツールで `model: sonnet`、Codex / Cursor ならその環境のサブエージェント）に渡す。適用役が外部 CLI の場合、その実行中はホストは作業ツリーを触らない。

### レビュープロンプト（`<...>` を埋める）

```
あなたはレビュー担当。上のスキル定義の基準で現在のブランチの変更をレビューする。ファイルは編集しない。指摘だけを返す。

対象 diff: git diff origin/<base>...HEAD（コミット一覧: git log origin/<base>..HEAD --oneline）
<code-review のときだけ: fixed point は origin/<base>。spec は <PR 本文 / issue / spec ファイルのパス>。無ければ Spec 軸は「no spec available」で報告>

数えない指摘: 既に repo の規約で許容されているもの、ツールが自動で直すもの、次の「再提案しない」一覧にあるもの
再提案しない: <過去に未適用と判断した指摘の一覧、無ければ「なし」>

最後に次の形式だけで返答する（余計な前置き不要）:
findings: <有効な指摘の総数>
items:
- [F1] <ファイル:行> <指摘> / fix: <直し方 1 行> / safe: yes|no（yes = 挙動・公開 API・テストの意味を変えずに直せる）
- [F2] ...
```

### 適用プロンプト

```
次の指摘を作業ツリーに適用する。挙動・公開 API・テストの意味は変えない。指摘の意図が不明なら適用せず報告する。

指摘: <safe: yes の項目だけを貼る>
適用後、プロジェクトの検査（typecheck / lint / test）を実行して通す。通らなければ自分の修正を戻す。コミットはしない。

最後に次の形式だけで返答する:
applied: <適用した数>
changed_files: <変更したファイル一覧>
not_applied: <適用しなかった項目と理由。無ければ none>
checks: <実行した検査コマンドと結果>
```

「有効な指摘」はスキルの基準で挙げる価値があるもの。`safe: no` の指摘は適用せず、最終報告の「採用しなかった指摘」に回す（設計判断が必要なものは人に委ねる）。

### 順番の意図

1. **thermo-nuclear**: 構造の作り直しが必要なら最初にやる。後のフェーズで整えたコードを捨てないため
2. **simplify**: 構造が固まった上で、読みやすさと一貫性を上げる
3. **deslop**: 細かい AI 由来の癖（過剰コメント、不要 try/catch、`any` キャスト、深いネスト）を落とす
4. **code-review**: 規約（Standards）と spec の両軸で最終確認。ここで構造の指摘が出て適用した場合だけ、3 に戻ってもう 1 周（1 回まで）

## 7. pr

```bash
gh pr view --json url,baseRefName,state 2>/dev/null
```

- 無ければ `git push -u origin HEAD` してから `<skills>/make-pr-easy-to-review/SKILL.md` に従って作る。**指示が無い限り `gh pr create --draft`**（ready にするのは人の判断）。タイトル・TL;DR・core ファイルと mechanical ファイルの区別・リスク・テスト方法を本文に入れる
- あれば同じスキルで本文を今の diff に合わせて更新する。履歴の書き換えは、まだ push していないコミットに限る（push 済みなら書き換えない）
- `$S set pr_url <url>`

## 8. canvas

`<skills>/pr-review-canvas/SKILL.md` に従って PR の説明 HTML を作る。生成した HTML は `z/<sanitized_branch>/pr-review.html` にもコピーし、`$S set canvas_path <パス>`。ローカルサーバーを立てた場合はその URL も `$S set canvas_url <url>`。ユーザーへの説明はこの HTML を軸にし、レビュー反復で何を変えたかも「レビューで直した点」として書き添える。

## 9. ci

`<skills>/loop-on-ci/SKILL.md` に従う。要点:

```bash
gh pr checks --json name,bucket,state,workflow,link
gh pr checks --watch --fail-fast
gh run view <run-id> --log-failed
```

- 失敗 1 件ごとに原因を 1 つに絞って直し、検査を通してコミット・push
- 明らかに無関係で base では直っている失敗なら、base を merge し直す
- flaky なら 1 回だけ再実行し、証拠を `$S log` に残す
- green になったら `$S phase ci done` と `$S set ci_status green`

## 10. 完了判定

```bash
<このスキルのディレクトリ>/scripts/verify.py          # 全部 ✓ で exit 0
<このスキルのディレクトリ>/scripts/verify.py --no-ci  # CI を待たずに手前まで確認
```

チェックポイント（全フェーズ done/skipped、各ラウンドの指摘に note、safe な指摘の未処理なし）と実状態（作業ツリーがクリーン、origin/<base> 取り込み済み、push 済み、PR が open、canvas ファイルあり、CI 全 pass）を突き合わせる。✗ が残っていれば、その項目のフェーズに戻る。最終報告は `verify.py` の出力を貼ってから書く。
