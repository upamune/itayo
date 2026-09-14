# `.tagpr` 設定と action の inputs / outputs

`.tagpr` はリポジトリルートに Git config 形式で置く。どの項目も `TAGPR_*` 環境変数で上書きでき、環境変数が優先される。action の `config` 入力（`TAGPR_CONFIG_FILE`）で別パスの設定ファイルを指定できる。

```ini
[tagpr]
    releaseBranch = main
    versionFile = version.go
    vPrefix = true
```

## パス解決

`versionFile` / `changelogFile` / `releaseYAMLPath` / `template` と、`command` / `postVersionCommand` の作業ディレクトリは **リポジトリルート基準**。`.tagpr` をサブディレクトリに置いても変わらない。`config: tools/.tagpr` なら `versionFile = tools/package.json` と書く。

## 設定一覧

| 設定 | 既定 | 説明 | 環境変数 |
| --- | --- | --- | --- |
| `releaseBranch` | 自動検出 | リリースブランチ。ワークフローの `on.push.branches` と一致させる | `TAGPR_RELEASE_BRANCH` |
| `versionFile` | 自動検出 | バージョンを持つファイル。カンマ区切りで複数可。`-` で tag-only | `TAGPR_VERSION_FILE` |
| `vPrefix` | 自動検出 | タグに `v` を付けるか。バージョンファイルの値には影響しない | `TAGPR_VPREFIX` |
| `tagPrefix` | なし | monorepo 用の接頭辞。`tools` → `tools/v1.2.3` | `TAGPR_TAG_PREFIX` |
| `fixedMajorVersion` | なし | メンテナンスブランチで特定メジャーのタグだけを見る（`1` / `v1`）。`calendarVersioning` とは併用不可 | `TAGPR_FIXED_MAJOR_VERSION` |
| `majorLabels` | `major` | マージ済み PR に付いていたら major bump を提案するラベル（カンマ区切り） | `TAGPR_MAJOR_LABELS` |
| `minorLabels` | `minor` | 同じく minor bump | `TAGPR_MINOR_LABELS` |
| `calendarVersioning` | なし | CalVer。`true` で `YYYY.MM0D.MICRO`、または書式を指定（`YYYY.0M.MICRO` など）。major / minor ラベルは無視される | `TAGPR_CALENDAR_VERSIONING` |
| `changelog` | `true` | CHANGELOG を生成・更新するか | `TAGPR_CHANGELOG` |
| `changelogFile` | `CHANGELOG.md` | CHANGELOG のパス | `TAGPR_CHANGELOG_FILE` |
| `releaseYAMLPath` | `.github/release.yml` / `.yaml` | release notes 設定。無ければ初回に `.github/release.yml` を作る | `TAGPR_RELEASE_YAML_PATH` |
| `command` | なし | バージョンファイル更新の **前** に実行するコマンド | `TAGPR_COMMAND` |
| `postVersionCommand` | なし | バージョンファイル更新の **後** に実行するコマンド | `TAGPR_POST_VERSION_COMMAND` |
| `template` | なし | リリース PR のタイトル・本文の Go text template ファイル。1 行目がタイトル | `TAGPR_TEMPLATE` |
| `templateText` | なし | 同じくインラインのテンプレート。`template` が無いときだけ使われる | `TAGPR_TEMPLATE_TEXT` |
| `commitPrefix` | `[tagpr]` | tagpr が作るコミットの接頭辞 | `TAGPR_COMMIT_PREFIX` |
| `release` | `true` | GitHub Release。`true` で公開、`draft` で下書き、`false` で作らない | `TAGPR_RELEASE` |

`command` / `postVersionCommand` には `TAGPR_CURRENT_VERSION`（例 `v1.2.3`）と `TAGPR_NEXT_VERSION`（例 `v1.3.0`）が渡る。

## バージョン決定の順序（SemVer）

1. `tagPrefix` に一致する最新の SemVer タグを探す。無ければ `v0.0.0` として最初のコミットから比較する
2. 前回リリース以降にマージされた PR のラベルを見て、`majorLabels` / `minorLabels` に一致すればリリース PR に `tagpr:major` / `tagpr:minor` を付ける
3. リリース PR の `tagpr:major` / `tagpr/major` で major、`tagpr:minor` / `tagpr/minor` で minor、無ければ patch。両方あれば major
4. リリース PR 内でバージョンファイルを直接編集していれば、それがラベルより優先される

`dependabot[bot]` の PR のラベルは無視される。tagpr は自分のリリース PR に必ず `tagpr` ラベルを付ける（`.github/release.yml` の `exclude.labels` に入れておく）。

CalVer では、バージョンファイルがあれば PR 作成・更新時に値を計算してファイルに書き、マージされた値がタグになる。`versionFile = -` ならマージ後のタグ作成時に計算する。

## よくある構成

### 複数のバージョンファイル

```ini
[tagpr]
    versionFile = version.go,action.yml
```

### monorepo

```yaml
- uses: Songmu/tagpr@v1
  with:
    config: tools/.tagpr
  env:
    GITHUB_TOKEN: ${{ steps.app-token.outputs.token }}
```

```ini
# tools/.tagpr
[tagpr]
    tagPrefix = tools
    versionFile = tools/package.json
    changelogFile = tools/CHANGELOG.md
    releaseYAMLPath = tools/.github/release.yml
```

タグは `tools/v1.2.3`。公開ワークフローの `on.push.tags` も `tools/v*` にする。

### 旧メジャーの保守ブランチ

```ini
[tagpr]
    releaseBranch = v1
    fixedMajorVersion = 1
```

ワークフローの `on.push.branches` に `v1` を足す（または保守ブランチ用に別ワークフロー）。

### tag-only

```ini
[tagpr]
    versionFile = -
```

CHANGELOG とリリース PR は作られ、マージ後にタグが打たれる。

## GitHub Action

### inputs

| input | 説明 |
| --- | --- |
| `config` | 設定ファイルのパス。既定 `.tagpr` |
| `version` | インストールする tagpr の版。action がテスト済みの既定値を持つので通常は指定しない |

### outputs

| output | 説明 |
| --- | --- |
| `tag` | 作ったタグ。作らなかった実行では空 |
| `pull_request` | 作成・更新したリリース PR の JSON |
| `base_tag` | 比較元のタグ。前回タグが無ければ空 |

GitHub Enterprise では `GITHUB_TOKEN` の代わりに `GH_ENTERPRISE_TOKEN` に渡す。

## トラブルシュート

| 症状 | 見るところ |
| --- | --- |
| リリース PR が作られない | ワークフローの `permissions`（`contents: write` / `pull-requests: write` / `issues: read`）、repo の `can_approve_pull_request_reviews`、App の権限とインストール対象。`create-github-app-token` で要求した `permission-*` がインストールに無いとエラー |
| タグを打っても別ワークフローが走らない | tagpr の `env.GITHUB_TOKEN` が `secrets.GITHUB_TOKEN` になっている。App トークンに差し替える |
| リリース PR の CI が「approval required」で止まる | 同上。App トークンで作った PR なら承認なしで走る |
| 違うバージョンファイルが選ばれた | `versionFile` を明示。tag-only は `-` |
| `.tagpr` をサブディレクトリに置いたらファイルが無いと言われる | パスはルート基準。`tools/...` と書く |
| 最初のリリース PR が全履歴を含む | 一致するタグが無い。`vPrefix` / `tagPrefix` が既存タグと合っているか確認。本当にタグが無ければ、ベースラインのコミットに正しいタグを打つか全履歴を受け入れる |
| 既存 CHANGELOG の構造が崩れた | tagpr は最初の `##` 見出しの前に挿入する。独自構造なら `changelog = false` にして別プロセスに任せる |
| Release PR を Rebase and merge したらタグが打たれない | tagpr は merge commit か squash のみ対応。`gh repo edit --enable-squash-merge` |
