# GitHub App とリポジトリ設定

tagpr に渡すトークンは GitHub App のインストールトークンにする。`GITHUB_TOKEN` との違い:

| | `GITHUB_TOKEN` | GitHub App トークン |
| --- | --- | --- |
| 作ったタグで `on.push.tags` のワークフローが起動する | しない | する |
| リリース PR の `pull_request` ワークフロー | 手動承認待ち | 自動で走る |
| 必要な設定 | workflow `permissions` + repo の「Allow GitHub Actions to create and approve pull requests」 | App の権限とインストール |
| コミット・PR の作者 | `github-actions[bot]` | `<app-slug>[bot]` |

tagpr はコミットを GitHub API 経由で作る（Verified commit になる）ので、`git config user.name` 等の設定は要らない。

## 1. App を作る

ブラウザ操作が要るので、ユーザーに依頼する。URL:

- 個人: `https://github.com/settings/apps/new`
- Organization: `https://github.com/organizations/<org>/settings/apps/new`

設定値:

| 項目 | 値 |
| --- | --- |
| GitHub App name | `<owner>-tagpr` など（グローバルで一意） |
| Homepage URL | リポジトリ URL で可 |
| Webhook | **Active のチェックを外す** |
| Repository permissions | **Contents: Read and write**、**Pull requests: Read and write**、**Issues: Read-only**（Metadata: Read-only は自動） |
| Where can this GitHub App be installed? | Only on this account |

作成後の App 設定ページで:

1. **Client ID** を控える（`Iv1.` または `Iv23` で始まる文字列。古い `App ID` の数字でも動くが Client ID を使う）
2. **Private keys > Generate a private key** で `.pem` をダウンロードする

## 2. インストールする

App 設定ページの **Install App** から owner を選び、**Only select repositories** で対象リポジトリを選ぶ。後から対象を増やすときは `https://github.com/settings/installations`（org なら `https://github.com/organizations/<org>/settings/installations`）で Configure。

確認:

```bash
gh api user/installations -q '.installations[] | "\(.app_slug)\t\(.account.login)"'
gh api user/installations/<installation_id>/repositories -q '.repositories[].full_name'
```

`installation_id` は `gh api user/installations -q '.installations[] | "\(.id)\t\(.app_slug)"'` で分かる。

App の権限を後から増やした場合、インストール側で承認するまで新しい権限は効かない（`create-github-app-token` で未承認の `permission-*` を要求するとエラー）。

## 3. vars / secrets に登録する

```bash
gh variable set TAGPR_APP_CLIENT_ID --body "<Client ID>"
gh secret set TAGPR_APP_PRIVATE_KEY < ~/Downloads/<app-name>.<date>.private-key.pem
rm ~/Downloads/<app-name>.<date>.private-key.pem   # 手元に残さない
gh variable list
gh secret list
```

同じ App を org 内の複数リポジトリで使うなら org レベルに置く:

```bash
gh variable set TAGPR_APP_CLIENT_ID --org <org> --visibility selected --repos <repo1>,<repo2> --body "<Client ID>"
gh secret set TAGPR_APP_PRIVATE_KEY --org <org> --visibility selected --repos <repo1>,<repo2> < key.pem
```

## 4. ワークフローでトークンを発行する

```yaml
- uses: actions/create-github-app-token@v3
  id: app-token
  with:
    client-id: ${{ vars.TAGPR_APP_CLIENT_ID }}
    private-key: ${{ secrets.TAGPR_APP_PRIVATE_KEY }}
    permission-contents: write
    permission-pull-requests: write
    permission-issues: read
```

- `owner` / `repositories` を省略すると、トークンはワークフローを実行中のリポジトリだけに絞られる。monorepo でも同じリポジトリ内なので不要
- `permission-*` で必要最小限に絞る。省略するとインストールの全権限を継承する
- トークンは 1 時間で失効し、ジョブ終了時に revoke される（別ジョブには渡せない。`skip-token-revoke: true` で残せるが tagpr では不要）
- 出力: `token`、`installation-id`、`app-slug`

`actions/checkout` にも同じトークンを渡し、`persist-credentials: false` にする。tagpr は `env.GITHUB_TOKEN` のトークンで push と API 呼び出しをする。

## 5. リポジトリ設定

「Settings > Actions > General > Workflow permissions」を `gh api` で操作する:

```bash
# 現状
gh api repos/{owner}/{repo}/actions/permissions/workflow
# Allow GitHub Actions to create and approve pull requests を ON
gh api -X PUT repos/{owner}/{repo}/actions/permissions/workflow -F can_approve_pull_request_reviews=true
```

- `default_workflow_permissions` は触らない（`read` のままでよい）。ワークフローの `permissions:` ブロックで必要な分だけ上げられる
- `can_approve_pull_request_reviews` は `GITHUB_TOKEN` で PR を作る場合にだけ必要。App トークンでは不要だが、フォールバックや他のワークフローのために `true` にしておく
- Organization 側の設定が優先される。repo で `true` にできないときは `gh api orgs/{org}/actions/permissions/workflow` を確認し、org 管理者に `-X PUT ... -F can_approve_pull_request_reviews=true` を依頼する

マージ方式:

```bash
gh repo view --json mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed
gh repo edit --enable-squash-merge     # または --enable-merge-commit
```

tagpr は Rebase and merge に対応していない。

## 補足: コミット作者を App の bot にしたい他の step がある場合

tagpr 自体は不要だが、同じワークフロー内で `git commit` する step があるなら:

```yaml
- id: app-user
  run: echo "id=$(gh api "/users/${APP_SLUG}[bot]" --jq .id)" >> "$GITHUB_OUTPUT"
  env:
    GH_TOKEN: ${{ steps.app-token.outputs.token }}
    APP_SLUG: ${{ steps.app-token.outputs.app-slug }}
- run: |
    git config user.name "${APP_SLUG}[bot]"
    git config user.email "${APP_USER_ID}+${APP_SLUG}[bot]@users.noreply.github.com"
  env:
    APP_SLUG: ${{ steps.app-token.outputs.app-slug }}
    APP_USER_ID: ${{ steps.app-user.outputs.id }}
```
