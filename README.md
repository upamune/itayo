# itayo

Dawarich **公式 iOS アプリ専用**の、Go + SQLite 単一バイナリ ingest。API は Dawarich 1.14.1 互換ヘッダと、公式アプリが接続・連続トラッキング・履歴表示に使うエンドポイントを返す。名前は「いたよ」（Da war ich の系譜）。OwnTracks / Overland / Traccar は扱わない。

公式 iOS のソースは公開されていない。契約は [Freika/dawarich](https://github.com/Freika/dawarich) の swagger・コントローラ・CHANGELOG と [Dawarich for iOS](https://dawarich.app/docs/dawarich-for-ios/) から取っている。

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
export USER_EMAIL=itayo@example.com
./dist/itayo
```

疎通:

```bash
curl -sS http://localhost:8790/api/v1/health
# {"status":"ok"} と X-Dawarich-Version: 1.14.1
```

公式 iOS アプリのサーバ URL には `http://localhost:8790` か、プライベートネットワーク上の `http://YOUR_TAILNET_HOST:8790`（例: `http://itayo.example.ts.net:8790`）を指定する。API キーは `API_KEY` と同じ値。ポートは 80 / 443 / 8788 / 8789 を避け、既定の 8790 を使う。`API_KEY` が空だと起動しない。インターネットへ直接晒さず、プライベートネットワークまたは tailnet 上に置く。

`LISTEN_ADDR` の例:

| 値 | 意味 |
| --- | --- |
| `127.0.0.1:8790` | このホストのループバックだけ |
| `:8790` | このホストの全インタフェース |

SIGTERM / SIGINT で 10 秒以内に graceful shutdown する。

## 認証

公式 Dawarich 互換のため、次の **両方** を受け付ける。クエリ `?api_key=` は削除しない。比較は SHA-256 のあと constant-time。キーはログに出さない。

| 方式 | 例 | 推奨 |
| --- | --- | --- |
| `Authorization: Bearer` | `Authorization: Bearer your-secret` | **こちらを使う** |
| クエリ | `?api_key=your-secret` | 公式クライアント互換のため残す。新規の呼び出しでは使わない |

クエリにキーを置くと、リバースプロキシやサーバのアクセスログ、リファラに平文で残る。手で叩くときや自前クライアントでは必ず Bearer にする。

```bash
curl -sS -H 'Authorization: Bearer your-secret' http://localhost:8790/api/v1/users/me
```

すべての応答に `X-Dawarich-Version` と `X-Dawarich-Response` が付く。後者はキーの有無で `Hey, I'm alive!` / `Hey, I'm alive and authenticated!` と変わる（公式 iOS の Test Connection 用）。

## 公式 iOS が使う API

| メソッド | パス | iOS での用途 | 実装 |
| --- | --- | --- | --- |
| GET | `/api/v1/health` | Test Connection。認証不要 | 本体 |
| POST | `/api/v1/points` | 連続トラッキング / アップロード。GeoJSON `locations[]`。不正点は捨てて 200 | 本体 |
| GET | `/api/v1/points` | 地図の Server + device。`start_at` / `end_at` / `order` / `page` / `per_page` / `slim` / bbox | 本体 |
| GET | `/api/v1/points/tracked_months` | 地図の日リボン / カレンダー | 本体 |
| GET | `/api/v1/users/me` | 接続後のユーザ。永続 identity と実 settings | 本体 |
| GET/PATCH | `/api/v1/settings` | `maps.distance_unit` など。permit list + maps の deep merge | 本体 |
| GET/PATCH | `/api/v1/settings/mobile` | 端末間の Settings sync。last-write-wins | 本体 |
| GET | `/api/v1/plan` | Pro フラグ。単一ユーザなので self-hosted 相当の pro | 本体 |
| GET | `/api/v1/insights` と `/details` | Insights タブ。点から距離・ヒートマップ・ストリークを計算。国/都市は 0（ジオコーディングなし） | 本体 |
| GET | `/api/v1/stats` | Insights の Stats サブタブ。点から年次距離 | 本体 |

SQLite は `(timestamp, latitude, longitude)` で upsert する。Null Island `(0,0)` は保存しない。bbox は `min_latitude` / `max_latitude` / `min_longitude` / `max_longitude`。不正な bbox は 400。

`GET /users/me` の `features` は Dawarich の `Api::UserSerializer` と同じ `reverse_geocoding` と `family` だけを返す。どちらもこのバイナリでは false。`self_hosted: true` のような stub は返さない。

## スコープ外（公式 iOS がサーバに依存しない、または別基盤が要る）

これらは 404 のままにする。将来やるなら issue を切る。

| 対象 | 理由 |
| --- | --- |
| GET/POST `/api/v1/visits` と batch | 公式 iOS は端末側で visit を検出する。[visits-and-places](https://dawarich.app/docs/features/visits-and-places/) |
| GET `/api/v1/timeline` | 主に Web Map v2。iOS の履歴は points | 
| Digests API | Sidekiq 相当の非同期生成が要る |
| Demo data / Auth Apple・Google / Family / Photos | クラウド・マルチユーザ・外部サービス |
| OwnTracks / Overland / Traccar | 公式 iOS 専用のため削除済み。戻さない |

## systemd

プレースホルダ付きユニットは `contrib/systemd/`。

```bash
sudo useradd --system --home /var/lib/itayo --shell /usr/sbin/nologin itayo
sudo mkdir -p /var/lib/itayo /etc/itayo
sudo cp dist/itayo /usr/local/bin/itayo
sudo cp contrib/systemd/itayo.env.example /etc/itayo/itayo.env
sudo chmod 600 /etc/itayo/itayo.env
# API_KEY を自分の秘密に変える
sudo cp contrib/systemd/itayo.service /etc/systemd/system/itayo.service
sudo systemctl daemon-reload
sudo systemctl enable --now itayo
```

## バックアップとリストア

WAL モード。コピーする前にチェックポイントするか、SQLite の backup API を使う。

```bash
# 稼働中（推奨）
sqlite3 /var/lib/itayo/itayo.sqlite "VACUUM INTO '/var/backups/itayo.sqlite'"

# 停止中
sqlite3 /var/lib/itayo/itayo.sqlite "PRAGMA wal_checkpoint(TRUNCATE);"
cp /var/lib/itayo/itayo.sqlite /var/backups/itayo.sqlite
```

リストアはサービスを止めてからファイルを戻す。`.sqlite-wal` / `.sqlite-shm` が残っている停止コピーを戻すときは、3 ファイルとも同じ時点のものを揃える。スキーマ版は `schema_migrations` テーブル。

## 開発

```bash
mise run format
mise run lint
mise run test
mise run build
mise run ci
```

CI は format / lint / test / build / pinact。タスク管理は backlog（`backlog task add|list|show|move <id> done`）。
