# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

BEpusdt — a self-hosted crypto (USDT/USDC/native) payment gateway, descended from `Epusdt` and wire-compatible with it (and with 彩虹易支付). Single static Go binary that embeds a Vue 3 admin SPA and a multi-template cashier site, with on-chain block scanners running as in-process tasks.

## Common commands

Backend (Go 1.24+):

```bash
# Build the binary (matches the Dockerfile/goreleaser invocation)
CGO_ENABLED=0 go build -trimpath \
  -ldflags="-X 'github.com/v03413/bepusdt/app.Version=dev' -s -w -buildid=" \
  -o bepusdt ./main

# Run the gateway (subcommands defined in app/cmd)
./bepusdt start          # main entrypoint; flags also read from .env / env vars
./bepusdt version
./bepusdt reset          # regenerate admin user / password / secret entrance URL

# Tests (sparse — only app/task/notify currently has them)
go test ./...
go test ./app/task/notify/...
```

Frontend (`web/`, pnpm enforced via `preinstall: only-allow pnpm`):

```bash
cd web
pnpm install --frozen-lockfile --shamefully-hoist
pnpm run dev          # Vite dev server, proxies /api to VITE_APP_BASE_URL
pnpm run build:prod   # outputs to web/dist; vite renames index.html -> secure.html
pnpm run lint:eslint
pnpm run lint:prettier
```

Full release build (Docker pipeline): build the frontend, copy `web/dist/*` into `static/secure/`, then build the Go binary — the embed in `static/embep.go` requires `static/secure/` to be populated, and `secure.html` is loaded as a Gin HTML template.

Configuration is done via env vars / `.env` (see `.env.example`): `LISTEN`, `LOG`, `SQLITE`, `MYSQL_DSN`, `POSTGRESQL_DSN`. CLI flags in `app/cmd/flag.go` mirror them.

## Architecture

### Entrypoint and lifecycle

`main/main.go` wires three `urfave/cli` subcommands from `app/cmd`. `start.go` is the only long-running one and orders initialization strictly:

1. `model.Init(sqlite, mysql, postgres)` — picks one of three GORM drivers based on which DSN is non-empty (postgres > mysql > sqlite fallback), runs `AutoMigrate` for `Wallet/Order/NotifyRecord/Conf/Rate`, and seeds defaults via `ConfInit` + `FillDefaultConf` + `RefreshC`.
2. `log.Init` (lumberjack-backed file logger).
3. `task.Init()` — initializes per-chain scanner state (BSC, ETH, Plasma, Polygon, Arbitrum, X Layer, Base) and registers periodic tasks.
4. `task.Start(ctx)` runs each registered `Task` as its own goroutine driven by a `time.Ticker`.
5. `router.Handler()` builds the Gin engine; HTTP server runs in a goroutine and shuts down on SIGINT/SIGTERM with a 5s grace period.

### Trade-type registry — central abstraction

`app/model/registry.go` is the source of truth for every supported chain/token combination. Each entry of `registry` (`UsdtTrc20`, `UsdtErc20`, `UsdcBase`, `EthereumEth`, …) carries:

- `Network` (one of the constants in `app/conf/const.go`: `tron`, `ethereum`, `bsc`, `polygon`, `arbitrum`, `base`, `xlayer`, `plasma`, `aptos`, `solana`)
- `Crypto` (USDT/USDC/TRX/BNB/ETH), `Contract` address, decimal scale
- `AmountRange`, block-explorer URL format, RPC endpoint config key, address case-sensitivity flag

A package-level `init()` derives several lookup maps from this registry (`networkTradesMap`, `contractTradeMap`, `contractDecimalMap`, etc.). When adding a new chain or token, register it here and the rest of the system (template selection, amount validation, explorer links, RPC endpoint resolution) picks it up automatically. The cashier templates in `static/payment/views/` are named to match these trade types (e.g. `usdt.trc20.html`, `usdc.base.html`).

### Tasks / on-chain scanners

`app/task` follows a register-then-run pattern: any file's `init()` (or `*Init()` called from `task.Init()`) calls `task.Register(Task{Duration, Callback})`. Per-chain files (`tron.go`, `ethereum.go`, `bsc.go`, `solana.go`, `aptos.go`, `polygon.go`, `arbitrum.go`, `base.go`, `xlayer.go`, `plasma.go`, `evm.go`) own their block scanning, plus cross-cutting tasks: `notify.go` (callback retries), `rate.go` (FX rate refresh), `transfer.go`, `mqtt.go`. `blockQueueLimit = 100` is intentional — see the comment in `task.go` about why a fixed channel would deadlock height-sync logging.

### HTTP routing — three coexisting API surfaces

`app/router/router.go` mounts everything onto one Gin engine:

- `epusdt.go` → `app/handler/epusdt`: native API (`/api/v1/order/*`, `/api/v1/pay/*`, `/pay/checkout-counter/:trade_id`, `/pay/cashier/:trade_id`, `/pay/check-status/:trade_id`). Drop-in compatible with upstream Epusdt.
- `epay.go` → `app/handler/epay`: 彩虹易支付-compatible `/submit.php`.
- `admin.go` → `app/handler/admin`: backend management (`/api/conf`, `/api/wallet`, `/api/order`, `/api/rate`, `/api/dashboard`). Consumed by the Vue SPA.
- `auth.go` → `app/handler/auth`: login / token / password.
- `static.go` serves either the embedded `static/payment/` templates or a custom path from the `PaymentStaticPath` config (see `docs/payment-template/README.md`); admin SPA HTML lives in the embedded `static/secure/` FS as `secure.html`.

Two-layer admin protection (note: bypassed entirely when `conf.Debug = true` in `app/conf/const.go`):

1. **Secret entrance URL** — `cmd reset` generates a random path; visiting it sets `admin_secure` in the session, which gates every route registered through `PostRegister`/`GetRegister` (they populate `secureRoute`). Routes registered directly with `engine.POST/GET` (epusdt, epay, public cashier endpoints) skip this gate.
2. **Bearer token** — routes registered with `checkAuth=true` additionally require the `Authorization` header to match a token cached in `go-cache`.

Use `PostRegister` / `GetRegister` (not `router.POST` / `router.GET`) when adding admin endpoints so the route correctly enters the auth maps.

### Notifier abstraction

`app/notifier/notifier.go` defines the `Notifier` interface (`Success`, `NotifyFail`, `NonOrderTransfer`, `TronResourceChange`, `Welcome`, `Test`). Implementations: `none.go`, `telegram.go`, `wechat.go`. The active channel is selected at call time from the `NotifierChannel` / `NotifierParams` conf entries; instances are memoized in `notifierMap` keyed by `md5(channel+params)`. Always go through the package-level functions (`notifier.Success(order)` etc.) rather than calling implementations directly.

### Frontend

`web/` is a fork of [SnowAdmin](https://github.com/WangFan-io/SnowAdmin) (Vue 3 + Arco Design + Pinia + Vue Router + vue-i18n). The build emits `dist/secure.html` (renamed from `index.html` by a Vite plugin) plus `dist/assets/`; this entire dist tree is copied into `static/secure/` before the Go embed compiles. The cashier (`static/payment/`) is a separate, server-rendered set of HTML templates parsed by Gin and is not built from `web/`.

## Conventions worth knowing

- All user-facing comments and log messages are in Chinese; preserve that style.
- The version string is injected at build time via `-ldflags '-X github.com/v03413/bepusdt/app.Version=...'`. Don't hardcode it.
- `conf.Debug` (constant in `app/conf/const.go`) bypasses session/token auth — never set true outside local debugging.
- Embedded FS paths in `static/embep.go` (`secure/*`, `payment/*`) must exist at compile time; building without first producing `web/dist` and copying it to `static/secure/` will fail the embed.
- `init()` side effects matter: the registry maps in `app/model/registry.go` and task registrations in `app/task/*.go` rely on package init order — don't restructure these into lazy initialization without auditing the call sites.
