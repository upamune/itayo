---
name: tagpr
description: Songmu/tagpr でリリース PR の自動生成とタグ付け・GitHub Release 作成を GitHub Actions に組み込む。GitHub App（actions/create-github-app-token）のトークンで動かし、タグ push から別の公開ワークフローを起動できる形にする。「tagpr を導入して」「リリース PR を自動化したい」「タグ付けとリリースを自動化」「tagpr のワークフローを直して」「release PR が作られない」と言われたら使う。新規・既存どちらのリポジトリにも使える。
---

# tagpr

[tagpr](https://github.com/Songmu/tagpr) は、リリースブランチへの push のたびに「次のバージョンと CHANGELOG を含むリリース PR」を作って追従させ、その PR をマージするとタグと GitHub Release を作るツール。このスキルでは **GitHub App のインストールトークン**で動かす構成に統一する。

原則:

- **トークンは GitHub App**。`actions/create-github-app-token` で都度発行し、`GITHUB_TOKEN` は使わない。`GITHUB_TOKEN` で作ったタグは別ワークフロー（`on.push.tags`）を起動できず、リリース PR 上の `pull_request` ワークフローも手動承認待ちになるため
- **リポジトリ設定も Actions から PR を作れる状態にする**（Settings > Actions > General > Workflow permissions の「Allow GitHub Actions to create and approve pull requests」）。App トークンなら本来不要だが、`gh api` で必ず入れておき、`GITHUB_TOKEN` にフォールバックしても動く状態にする
- **公開（publish / deploy）はタグ push で起動する別ワークフロー**に置き、`workflow_dispatch` で明示タグを渡して再実行できるようにする
- **`uses:` は pinact で SHA に pin**し、`actions/checkout` は `persist-credentials: false`
- **既存の公開済みタグは動かさない**。ベースラインがずれていたら `.tagpr` 側を直す

雛形と詳細は reference に分ける。必要なものだけ読む:

- [references/github-app.md](references/github-app.md): GitHub App の作成・権限・インストール、vars / secrets の登録、リポジトリ設定の `gh api`
- [references/workflows.md](references/workflows.md): `tagpr.yml`、タグ起動の `release.yml`、同一ワークフロー内で公開する変種
- [references/config.md](references/config.md): `.tagpr` の全設定と action の inputs / outputs、トラブルシュート

## 手順

### 1. 現状を調べる

対象リポジトリで次を確認し、結果をメモする:

```bash
git remote -v
gh repo view --json defaultBranchRef,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed
git tag --sort=-v:refname | head        # 最新タグと接頭辞（v / monorepo prefix）
git describe --tags --abbrev=0 2>/dev/null
ls .tagpr .github/release.yml .github/release.yaml CHANGELOG.md 2>/dev/null
ls .github/workflows/
gh api repos/{owner}/{repo}/actions/permissions/workflow
```

見るポイント:

- リリースブランチ（通常 `main`）と、最新リリースタグが指すコミット
- タグ形式（`v` 接頭辞の有無、monorepo なら `tools/v1.2.3` のような prefix）
- バージョンを持つファイル（`version.go`、`package.json`、`action.yml` など）。無ければ tag-only
- 既存の CHANGELOG と `.github/release.yml`
- **タグや GitHub Release やパッケージを作っている既存ワークフロー**。tagpr と二重にタグを打たないよう、どれを置き換え、どれを「タグ起動の公開」として残すかを決める
- マージ方式。tagpr は **Create a merge commit** か **Squash and merge** のみ対応。Rebase しか許していないなら設定を変える

既存のバージョンファイルと最新タグが食い違っていたら、ここで揃える（タグは動かさず、ファイル側を直す）。

完了条件: リリースブランチ・最新タグ・バージョンファイル・既存の公開ワークフローを列挙し、tagpr がタグを作った後に何を起動するかを決めた。

### 2. GitHub App を用意する

[references/github-app.md](references/github-app.md) に従う。要点:

1. App を作る（owner 配下。Webhook は無効、Repository permissions は **Contents: Read and write / Pull requests: Read and write / Issues: Read-only**）
2. 対象リポジトリにインストールする
3. Client ID を `TAGPR_APP_CLIENT_ID`（variable）、秘密鍵を `TAGPR_APP_PRIVATE_KEY`（secret）として登録する

```bash
gh variable set TAGPR_APP_CLIENT_ID --body "<Client ID>"
gh secret set TAGPR_APP_PRIVATE_KEY < ~/Downloads/<app>.private-key.pem
```

App の作成とインストールはブラウザ操作が要るので、URL を示してユーザーに頼み、終わったら `gh api user/installations` で入っていることを確認する。既に tagpr 用 App があればそれを再利用し、対象リポジトリへのインストールだけ足す。

完了条件: `gh variable list` に `TAGPR_APP_CLIENT_ID`、`gh secret list` に `TAGPR_APP_PRIVATE_KEY` が出て、App が対象リポジトリにインストールされている。

### 3. リポジトリ設定を直す

```bash
gh api -X PUT repos/{owner}/{repo}/actions/permissions/workflow -F can_approve_pull_request_reviews=true
gh api repos/{owner}/{repo}/actions/permissions/workflow
```

`can_approve_pull_request_reviews: true` が「Allow GitHub Actions to create and approve pull requests」。Organization 側で禁止されていると repo 単位では有効にできないので、その場合は `gh api orgs/{org}/actions/permissions/workflow` を見て org 管理者に依頼する。

Rebase merge しか許していないなら `gh repo edit --enable-squash-merge` か `--enable-merge-commit` を入れる。

完了条件: `can_approve_pull_request_reviews` が `true` で、merge commit か squash のどちらかが許可されている。

### 4. `.tagpr` と `.github/release.yml` をコミットする

初回実行の自動検出に頼らず、先に書いてレビューできる状態にする。

```ini
[tagpr]
    releaseBranch = main
    versionFile = version.go
    vPrefix = true
```

- `versionFile` は手順 1 で見つけたファイルをカンマ区切りで全部。無ければ `-`（tag-only）
- `vPrefix` は既存タグに合わせる
- monorepo なら `tagPrefix` と、ルートからの相対パスで `versionFile` / `changelogFile` / `releaseYAMLPath`
- CalVer のプロジェクトなら `calendarVersioning`
- 他の仕組みが CHANGELOG を管理しているなら `changelog = false`
- バージョン更新の前後にコマンドが要る（lock ファイル更新、生成物の再生成）なら `command` / `postVersionCommand`

`.github/release.yml` が無ければ最小のものを置く（tagpr が初回に作るが、先に置いた方が diff が読みやすい）:

```yaml
changelog:
  exclude:
    labels: [tagpr, dependencies]
  categories:
    - title: Breaking Changes
      labels: [major, breaking]
    - title: Features
      labels: [minor, enhancement, feature]
    - title: Bug Fixes
      labels: [bug]
    - title: Other Changes
      labels: ["*"]
```

ラベル名は `.tagpr` の `majorLabels` / `minorLabels`（既定 `major` / `minor`）と揃える。設定一覧は [references/config.md](references/config.md)。

完了条件: `.tagpr` がリリースブランチ・バージョンファイル・タグ形式と一致し、`.github/release.yml` がある。

### 5. ワークフローを書く

[references/workflows.md](references/workflows.md) の `tagpr.yml` をそのまま置く。骨子:

```yaml
- uses: actions/create-github-app-token@v3
  id: app-token
  with:
    client-id: ${{ vars.TAGPR_APP_CLIENT_ID }}
    private-key: ${{ secrets.TAGPR_APP_PRIVATE_KEY }}
    permission-contents: write
    permission-pull-requests: write
    permission-issues: read
- uses: actions/checkout@v7
  with:
    token: ${{ steps.app-token.outputs.token }}
    persist-credentials: false
- id: tagpr
  uses: Songmu/tagpr@v1
  env:
    GITHUB_TOKEN: ${{ steps.app-token.outputs.token }}
```

公開側は手順 1 の決定に従う:

- **別ワークフロー（推奨）**: `on.push.tags: ['v*']` と `workflow_dispatch`（`tag` 入力）を持つ `release.yml`。App トークンで作ったタグなので起動する
- **同一ワークフロー**: `if: steps.tagpr.outputs.tag != ''` で続ける。外部クレデンシャル不要だが再実行しづらい

既存の公開ワークフローがタグ push で動いているなら、そのまま残して tagpr 側のタグ作成・Release 作成だけ新ワークフローへ寄せる。既存ワークフローが自前でタグを打っているなら、その step を消す。

書いたら `pinact run` で SHA に pin し、`actionlint` があれば通す。

完了条件: `.github/workflows/tagpr.yml`（と必要なら `release.yml`）があり、`pinact run -check` が 0、タグを作る経路が tagpr の一つだけになっている。

### 6. push して最初のリリース PR を確認する

`.tagpr`、`.github/release.yml`、ワークフローをリリースブランチに push する（PR 経由でもよい）。tagpr が走り、`tagpr-from-<version>` ブランチとリリース PR ができる。

マージ前に次を確認する:

- `base_tag` と CHANGELOG の範囲が、意図した前回リリースの直後から始まっている
- 提案バージョンが期待通り（既定は patch。`tagpr:minor` / `tagpr:major` ラベルか、バージョンファイルの直接編集で変えられる）
- 変更されたのがバージョンファイルと生成物だけで、既存の CHANGELOG が壊れていない
- release notes のカテゴリ・除外が `.github/release.yml` 通り
- マージ後に公開ワークフローが **一度だけ** 走る

範囲や内容がおかしければ PR は開けたまま、リリースブランチ側で `.tagpr` や `release.yml` を直す。tagpr が PR を作り直す。`tagPrefix` やベースラインを変えると別ブランチの PR になるので、古い方は確認後に閉じる。

完了条件: リリース PR の diff と CHANGELOG をユーザーと確認し、マージしてよい状態になっている（マージ自体はユーザーの判断）。

### 7. 切り替えを完了する

リリース PR をマージしたら、次を確認する:

1. 新しいタグがマージコミットを指している（`git fetch --tags && git tag --points-at origin/main`）
2. GitHub Release と CHANGELOG に意図した変更が入っている（`gh release view <tag>`）
3. 公開ワークフローが一度だけ走って成功した（`gh run list --workflow release.yml`）
4. 次の変更を push すると、次のリリース PR が作られる・更新される

成功したら旧プロセスのタグ付け・Release 作成 step を消す。明示タグを受け取れる公開コマンドは復旧用に残す。

完了条件: 旧プロセスにタグを作る経路が残っておらず、次のリリース PR が自動で追従している。

## トラブルシュート

- **リリース PR が作られない**: ワークフローの `permissions` と、手順 3 の `can_approve_pull_request_reviews`。App トークンなら App 側の権限（Contents / Pull requests: write、Issues: read）とインストール対象に対象リポジトリが含まれているか。`create-github-app-token` で `permission-*` を指定している場合、インストールに無い権限を要求するとエラーになる
- **タグを打っても `release.yml` が走らない**: tagpr に渡したトークンが `GITHUB_TOKEN` になっていないか。`env.GITHUB_TOKEN` が `steps.app-token.outputs.token` であることを確認
- **バージョンファイルの検出が違う**: `.tagpr` の `versionFile` を明示する。tag-only なら `-`
- **`.tagpr` をサブディレクトリに置いたらファイルが見つからない**: パスはリポジトリルート基準。`tools/package.json` のように書く
- **最初の PR が全履歴を含む**: 一致するタグが無いと `v0.0.0` から始まる。ベースラインにしたいコミットに正しいタグを打つ（既存タグは動かさない）か、それを受け入れる

詳しくは [references/config.md](references/config.md) の末尾。
