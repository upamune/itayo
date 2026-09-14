# itayo

Dawarich **公式 iOS アプリ専用**の、Go + SQLite 単一バイナリ ingest。API は Dawarich 1.14.1 互換ヘッダと公式アプリが使うエンドポイントだけを返す。名前は「いたよ」（Da war ich の系譜）。OwnTracks / Overland / Traccar は扱わない。

## インストール

前提: [mise](https://mise.jdx.dev/)

```bash
git clone https://github.com/upamune/itayo.git
cd itayo
mise trust
mise install
```

環境変数を置く（`.env` はコミットしない）:

```bash
cp .env.example .env
# API_KEY=your-secret を自分の秘密に変える（空だと起動しない）
```

バイナリ:

```bash
mise run build          # dist/itayo（CGO_ENABLED=0）
# または
CGO_ENABLED=0 go build -trimpath -o dist/itayo ./cmd/itayo
```

## 使い方

```bash
export API_KEY=your-secret
export LISTEN_ADDR=:8790
export TIME_ZONE=Asia/Tokyo
export DATABASE_PATH=./itayo.sqlite
./dist/itayo
```

疎通:

```bash
curl -sS http://localhost:8790/api/v1/health
# {"status":"ok"} と X-Dawarich-Version: 1.14.1
```

公式 iOS アプリのサーバ URL には `http://localhost:8790` か、tailnet 上の `http://YOUR_TAILNET_HOST:8790`（例: `http://itayo.example.ts.net:8790`）を指定する。API キーは `API_KEY` と同じ値。ポートは 80 / 443 / 8788 / 8789 を避け、既定の 8790 を使う。`API_KEY` が空だと起動しない。インターネットへ直接晒さず、プライベートネットワークまたは tailnet 上に置く。

認証は `Authorization: Bearer your-secret` を優先する。`?api_key=` も公式クライアント互換のため受け付けるが、アクセスログやリファラにキーが残るので避ける。

| メソッド | パス | 備考 |
| --- | --- | --- |
| GET | `/api/v1/health` | 認証不要。`status=ok` と互換ヘッダ。`X-Dawarich-Response` はキーの有無で文言が変わる（公式 iOS 互換のため維持）。キー探索に使えるので、サービスはプライベートネットワーク / tailnet に置く |
| POST | `/api/v1/points` | 公式 iOS の GeoJSON `locations[]`。不正点は捨てて 200 |
| GET | `/api/v1/points` | `start_at` / `end_at` / `order` / `page` / `per_page` / `slim` |
| GET | `/api/v1/users/me` | stub |
| GET/PATCH | `/api/v1/settings` | stub |

SQLite は `(timestamp, latitude, longitude)` で upsert する。Null Island `(0,0)` は保存しない。

## 開発

```bash
mise run format
mise run lint
mise run test
mise run build
mise run ci
```

CI は format / lint / test / build / pinact。タスク管理は backlog（`backlog task add|list|show|move <id> done`）。
