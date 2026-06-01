# Loaded Price Monitor

Tracks game-key prices on **loaded.com** and posts **Discord** + **Telegram**
notifications when prices change. Three implementations — **Python**, **Rust**,
and **Go** — sharing the same `products.json` config and `history.json` state
files, so you can switch freely between them.

> **New user?** See [GETTING_STARTED.md](GETTING_STARTED.md) — pick between
> [hosting on GitHub Actions](GETTING_STARTED_GITHUB.md) or
> [hosting locally on a Pi / NAS / laptop](GETTING_STARTED_LOCAL.md). The rest
> of this README is the reference.

![Banner: LOADED PRICE MONITOR in cyan-to-magenta ANSI block art on a dark background](docs/banner.png)

<details>
<summary>Plain-text fallback</summary>

```
  ██╗      ██████╗  █████╗ ██████╗ ███████╗██████╗
  ██║     ██╔═══██╗██╔══██╗██╔══██╗██╔════╝██╔══██╗
  ██║     ██║   ██║███████║██║  ██║█████╗  ██║  ██║
  ██║     ██║   ██║██╔══██║██║  ██║██╔══╝  ██║  ██║
  ███████╗╚██████╔╝██║  ██║██████╔╝███████╗██████╔╝
  ╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═════╝ ╚══════╝╚═════╝
  ██████╗ ██████╗ ██╗ ██████╗███████╗    ███╗   ███╗ ██████╗ ███╗   ██╗██╗████████╗ ██████╗ ██████╗
  ██╔══██╗██╔══██╗██║██╔════╝██╔════╝    ████╗ ████║██╔═══██╗████╗  ██║██║╚══██╔══╝██╔═══██╗██╔══██╗
  ██████╔╝██████╔╝██║██║     █████╗      ██╔████╔██║██║   ██║██╔██╗ ██║██║   ██║   ██║   ██║██████╔╝
  ██╔═══╝ ██╔══██╗██║██║     ██╔══╝      ██║╚██╔╝██║██║   ██║██║╚██╗██║██║   ██║   ██║   ██║██╔══██╗
  ██║     ██║  ██║██║╚██████╗███████╗    ██║ ╚═╝ ██║╚██████╔╝██║ ╚████║██║   ██║   ╚██████╔╝██║  ██║
  ╚═╝     ╚═╝  ╚═╝╚═╝ ╚═════╝╚══════╝    ╚═╝     ╚═╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝
  > track game key prices on loaded.com
```

</details>

## Layout

```
loaded-price-tracker/
├─ python/   # loaded — requests + beautifulsoup4 + rich
├─ rust/     # loaded-rs — reqwest + scraper + clap + owo-colors
├─ go/       # loaded-go — net/http + goquery + pflag + fatih/color
├─ products.json   # shared config (committed)
├─ history.json    # shared state (committed by Actions)
└─ .github/workflows/check-prices.yml   # cron-driven Python runner
```

Pick **one** implementation to run locally; they all read/write the same JSON
files in the project root.

| Trait | Python | Rust | Go |
|---|---|---|---|
| Setup speed | fastest (no compile) | slowest first build | fast |
| Runtime | tiny script | single static binary | single static binary |
| Best for | CI / GitHub Actions | distributing to friends | distributing to friends |

The GitHub Action uses Python (no compile step in CI = faster, simpler).

## Quick start

Clone, then pick your implementation.

### Python

```powershell
cd python
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt

python -m src.main check --dry-run
python -m src.main add --url "https://loaded.com/products/<slug>" --name "Elden Ring (PC)" `
    --rule threshold:25:once --rule percent_drop:15
```

### Rust

```powershell
cd rust
cargo build --release
# Binary: rust\target\release\loaded-rs.exe — copy it anywhere on PATH.

.\target\release\loaded-rs.exe check --dry-run
.\target\release\loaded-rs.exe add --url "https://loaded.com/..." --name "Elden Ring (PC)" `
    --rule threshold:25:once --rule percent_drop:15
```

### Go

```powershell
cd go
go build -o loaded-go.exe -ldflags "-s -w" ./cmd/loaded-go

.\loaded-go.exe check --dry-run
.\loaded-go.exe add --url "https://loaded.com/..." --name "Elden Ring (PC)" `
    --rule threshold:25:once --rule percent_drop:15
```

> All three resolve `products.json` / `history.json` by walking up from the
> current directory, so you can run from the project root or from any
> sub-directory.

## Notification rules

Each product in `products.json` has a `notify` list. Rules are OR'd; if several
fire on one run, they are combined into a single notification.

| Type | Example | Fires when |
|---|---|---|
| `any_change` | `{"type":"any_change"}` | Price differs from the last check |
| `threshold` | `{"type":"threshold","at_or_below":25.00,"only_once":true}` | Current price ≤ value. With `only_once`, fires once on crossing; re-arms when price rises back above. |
| `percent_drop` | `{"type":"percent_drop","min_percent":15}` | Drop of ≥ X% vs the previous check |

Default when `notify` is omitted: `[{"type": "any_change"}]`.

### CLI flag syntax (all three implementations)

```
--rule any_change
--rule threshold:25
--rule threshold:25:once
--rule percent_drop:15
```

Repeat `--rule` to combine.

### Editing `products.json` by hand

```json
{
  "products": [
    {
      "id": "elden-ring-pc",
      "name": "Elden Ring (PC)",
      "url": "https://loaded.com/products/elden-ring-pc",
      "tags": ["wishlist"],
      "notify": [
        { "type": "threshold", "at_or_below": 25.00, "only_once": true },
        { "type": "percent_drop", "min_percent": 15 }
      ]
    }
  ]
}
```

## Secrets

Set these as environment variables (locally) or repository **Actions secrets**
(for the scheduled runner):

| Variable | Where to get it |
|---|---|
| `DISCORD_WEBHOOK_URL` | Discord → channel settings → Integrations → Webhooks |
| `TELEGRAM_BOT_TOKEN` | [@BotFather](https://t.me/BotFather) → `/newbot` |
| `TELEGRAM_CHAT_ID` | Message your bot, then visit `https://api.telegram.org/bot<TOKEN>/getUpdates` |

For local dev, each implementation reads its own `.env` file
(`python/.env`, `rust/.env`, `go/.env`) — copy the relevant `.env.example`
and fill in. Only one channel is required; either Discord or Telegram alone
works fine.

## Common commands (CLI parity)

| Action | Python | Rust | Go |
|---|---|---|---|
| Run all checks | `python -m src.main check` | `loaded-rs check` | `loaded-go check` |
| Dry run | `... check --dry-run` | `... check --dry-run` | `... check --dry-run` |
| One product | `... check --product <id>` | `... check --product <id>` | `... check --product <id>` |
| Watch loop | `... check --watch --interval 360` | `... check --watch --interval 360` | `... check --watch --interval 360` |
| Add product | `... add --url ... --name ...` | `... add --url ... --name ...` | `... add --url ... --name ...` |
| Suppress banner | `--no-banner` | `--no-banner` | `--no-banner` |

`--watch` runs `check` on a loop, sleeping `--interval` minutes between runs
(default 360 = 6h). Useful when self-hosting without cron/systemd — see
[GETTING_STARTED_LOCAL.md](GETTING_STARTED_LOCAL.md).

## GitHub Actions setup

1. Push this repo to GitHub.
2. **Settings → Secrets and variables → Actions** — add `DISCORD_WEBHOOK_URL`,
   `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`.
3. **Settings → Actions → General → Workflow permissions** → "Read and write
   permissions" (so the workflow can commit `history.json`).
4. The workflow runs every 6 hours by default. Adjust the cron in
   `.github/workflows/check-prices.yml`, or trigger manually via the Actions tab.

The default workflow uses the **Python** implementation. To switch to Rust or
Go in CI, swap the install / run steps in the workflow — but expect longer
runtimes due to compile time unless you cache `target/` (Rust) or commit the
prebuilt binary (Go).

## Share with a friend

Designed around a **fork-per-user** model. The Action runs on **GitHub's
servers** — your friend's laptop can be off; the cron still fires every 6 hours.

### Step-by-step

1. **Fork the repo** on GitHub.
   - GitHub doesn't allow forking directly into a private repo. Either:
     - Leave the fork **public** — your `products.json` is visible (just game
       URLs), but secrets are encrypted regardless of repo visibility, or
     - Go **private** — clone the repo locally, create a new empty private
       repo on GitHub, push to it. You lose the "fork" relationship but can
       still add the original as a remote and `git pull` updates.
2. **Enable Actions** on the fork — Actions tab → "I understand my workflows,
   go ahead and enable them". (One-time, forks have Actions disabled by default.)
3. **Add secrets** — Settings → Secrets and variables → Actions → New repository
   secret. Add `DISCORD_WEBHOOK_URL`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`.
   Secrets are encrypted at rest, never shown after creation, and never appear
   in logs.
4. **Allow commits from the workflow** — Settings → Actions → General →
   Workflow permissions → "Read and write permissions".
5. **Add products** — edit `products.json` in the GitHub web UI (pencil icon),
   or clone + use the `add` CLI + push.
6. **Trigger once manually** — Actions tab → Check prices → Run workflow.
   This confirms setup works *and* "wakes up" the cron (GitHub's anti-abuse
   measure: scheduled runs on forks don't fire until the workflow has been
   triggered manually at least once).
7. **Done.** It now runs every 6 hours. You'll see commits from
   `github-actions[bot]` updating `history.json`, and notifications appear in
   Discord/Telegram when prices change.

### What your friend actually needs
- A GitHub account (free)
- A Discord webhook URL **or** a Telegram bot token + chat id (either alone works)
- ~5 minutes for one-time setup

No Python/Rust/Go installation required — unless they want to use the `add`
CLI locally instead of editing `products.json` in the browser.

### Privacy at a glance

| Thing | Visible to whom (public fork) |
|---|---|
| Code | Anyone |
| `products.json` (game URLs) | Anyone |
| `history.json` (price log) | Anyone |
| **Secrets** (webhook / token / chat id) | **Only the repo owner — always encrypted** |
| Notifications | Only whoever's in the Discord channel / Telegram chat |

### Pulling upstream updates
If you forked:
```bash
git remote add upstream <this-repo-url>
git pull upstream main
```

### "Just give me a binary"
Build Rust or Go locally and send the resulting `loaded-rs.exe` / `loaded-go.exe`
to your friend along with a `products.json`. They can run it on a schedule with
Windows Task Scheduler / cron — no Python, no toolchain needed.

## Local-only mode
You don't need GitHub Actions at all. Every implementation has a `--watch`
flag for self-hosted scheduling, and `examples/` contains ready-to-use
systemd, cron, launchd, and Windows Task Scheduler templates.

See **[GETTING_STARTED_LOCAL.md](GETTING_STARTED_LOCAL.md)** for the full
Raspberry Pi / NAS / laptop walkthrough.

## Files

- `products.json` — tracked products (committed)
- `history.json` — price history + per-rule arm state (committed by the Action)
- `python/`, `rust/`, `go/` — three implementations of the same tool
- `.github/workflows/check-prices.yml` — cron-driven Python runner
- `examples/` — systemd, cron, launchd, and Windows Task Scheduler templates
  for self-hosting

## Notes

- Be polite: there's a 1.5s delay between requests in all three implementations.
  Don't crank the cron too high.
- The scrapers prefer JSON-LD (`Product`/`Offer`) which is robust to restyling,
  and fall back to CSS selectors if needed.
- Failed products don't abort the run; a summary notification is sent at the
  end listing failures.
- All three write `history.json` in the same format, so switching
  implementations between runs is safe.
