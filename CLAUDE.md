# itayo

Dawarich 公式 iOS アプリ専用の、Go + SQLite 単一バイナリ ingest。Dawarich 1.14.1 互換のヘッダと公式アプリ用 API を返す。OwnTracks / Overland / Traccar は扱わない。

## よく使うコマンド

```bash
mise run format
mise run format:check
mise run lint
mise run test
mise run build
mise run ci
```

起動は `mise run build` のあと `API_KEY=your-secret ./dist/itayo`。`API_KEY` が空だと起動しない。既定は `LISTEN_ADDR=:8790`、`TIME_ZONE=Asia/Tokyo`。認証は Bearer を優先する。

## 規約

- ツールは mise。グローバルの `go install` / `npm i -g` は使わない
- GitHub Actions の `uses:` は pinact で SHA に pin する。更新は `pinact run -update`
- タスクは backlog: `backlog task add|list|show|move <id> done`
- バイナリは `CGO_ENABLED=0`（pure Go SQLite）
- 公開リポジトリなので、実ホスト名・tailnet 名・トークン・個人パスをコード / ドキュメント / テスト / コミットに書かない。例は `http://localhost:8790` と `http://itayo.example.ts.net:8790`、`API_KEY=your-secret` だけにする

## ディレクトリ

```
cmd/itayo/           # エントリポイント
internal/config/     # 環境変数
internal/store/      # SQLite
internal/ingest/     # 公式 iOS GeoJSON
internal/httpapi/    # Dawarich 互換 HTTP
internal/version/    # X-Dawarich-Version
```
