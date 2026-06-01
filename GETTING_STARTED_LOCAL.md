# Getting Started — Hosting Locally

Run the monitor on your own hardware — a Raspberry Pi, a home server, a NAS,
or even your laptop. Nothing leaves your machine except the outbound HTTPS
requests to loaded.com and your notification service(s).

**Best for:** people who already self-host things, want full privacy, or
don't want to involve GitHub at all.
**Time required:** about 10–20 minutes.
**You'll need:** any always-on (or frequently-on) machine with Python 3.10+,
Rust, or Go installed.

> Prefer the zero-install GitHub Actions route? See
> [GETTING_STARTED_GITHUB.md](GETTING_STARTED_GITHUB.md) instead.

---

## Step 1 — Get the code

```bash
git clone https://github.com/arc94c3/loaded-price-tracker.git
cd loaded-price-tracker
```

> If you want your `products.json` / `history.json` kept private, point
> `git remote set-url origin` at your own private repo, or just don't push.

---

## Step 2 — Pick an implementation and install

You only need **one** of these. They are interchangeable — same config files,
same history format.

### Python (easiest to hack on)

```bash
cd python
python3 -m venv .venv
source .venv/bin/activate          # Windows: .venv\Scripts\Activate.ps1
pip install -r requirements.txt
cd ..
```

### Rust (single static binary)

```bash
cd rust
cargo build --release
# Binary is at rust/target/release/loaded-rs (or .exe on Windows)
cd ..
```

### Go (single static binary)

```bash
cd go
go build -o ../loaded-go ./cmd/loaded-go
cd ..
```

---

## Step 3 — Set up notifications

You only need **one** of Discord or Telegram. See
[GETTING_STARTED_GITHUB.md#step-3](GETTING_STARTED_GITHUB.md#step-3--get-your-notification-credentials)
for how to obtain the webhook URL / bot token / chat id.

Create a `.env` file (the Python and Rust impls load it automatically; for Go,
the env vars must be exported by your shell or service unit):

```bash
# .env  (place inside python/ for Python, inside rust/ for Rust)
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...
TELEGRAM_BOT_TOKEN=1234567890:ABCdef...
TELEGRAM_CHAT_ID=123456789
```

> A sample is at `python/.env.example`. Never commit your real `.env` —
> it's already in `.gitignore`.

---

## Step 4 — Add some products

Edit `products.json` directly (see the cheat sheet in the GitHub guide for
notification rules), or use the CLI `add` command, e.g.:

```bash
# Python
cd python && python -m src.main add \
    --url "https://loaded.com/products/elden-ring-pc" \
    --name "Elden Ring (PC)" \
    --rule threshold:25:once --rule percent_drop:15

# Rust
./rust/target/release/loaded-rs add \
    --url "https://loaded.com/products/elden-ring-pc" \
    --name "Elden Ring (PC)" \
    --rule threshold:25:once --rule percent_drop:15

# Go
./loaded-go add \
    --url "https://loaded.com/products/elden-ring-pc" \
    --name "Elden Ring (PC)" \
    --rule threshold:25:once --rule percent_drop:15
```

---

## Step 5 — Do a dry-run test

This checks every tracked product once but doesn't send notifications or
write history:

```bash
# Python
cd python && python -m src.main check --dry-run

# Rust
./rust/target/release/loaded-rs check --dry-run

# Go
./loaded-go check --dry-run
```

Drop `--dry-run` once you're happy — that will send the first round of
notifications (for any rules that already match) and write `history.json`.

---

## Step 6 — Schedule it

Pick **one** of the options below. The watch loop (6a) is simplest and
self-contained. Cron / systemd / Task Scheduler (6b–d) are more "Unix-y" and
survive reboots automatically.

### 6a — Built-in watch mode (works everywhere)

Every implementation has a `--watch` flag that loops forever, sleeping between
checks. Run it under `nohup`, `tmux`, `screen`, a systemd unit, a Windows
Service wrapper, or whatever you usually use to keep a process alive.

```bash
# Defaults to checking every 360 minutes (6 hours)
loaded-rs check --watch

# Or customise:
loaded-rs check --watch --interval 120     # every 2 hours
python -m src.main check --watch --interval 60
./loaded-go check --watch --interval 30
```

Press Ctrl-C to stop. The Python impl handles this cleanly; the Rust/Go impls
just terminate the loop.

### 6b — Cron (Linux / macOS / Raspberry Pi)

Open your crontab:

```bash
crontab -e
```

Add a line. Example: run every 6 hours, using the Rust binary, logging to a file:

```cron
0 */6 * * * cd /home/pi/loaded-price-tracker && \
  . ./.env && \
  ./rust/target/release/loaded-rs --no-banner check \
  >> /home/pi/loaded-price-tracker/run.log 2>&1
```

> Cron has no environment by default. Either inline the env vars, or source a
> file as above. A ready-to-edit sample is in `examples/cron.sample`.

### 6c — systemd timer (Raspberry Pi OS / Debian / Ubuntu / Fedora)

Two-unit setup: a `.service` that runs once, and a `.timer` that schedules it.
Samples are in `examples/`:

- `examples/loaded-price-monitor.service`
- `examples/loaded-price-monitor.timer`
- `examples/loaded-price-monitor.env` (referenced by `EnvironmentFile=`)

Install (as the user who will run the monitor):

```bash
cd loaded-price-tracker
mkdir -p ~/.config/systemd/user
cp examples/loaded-price-monitor.{service,timer} ~/.config/systemd/user/
cp examples/loaded-price-monitor.env ~/.config/loaded-price-monitor.env
chmod 600 ~/.config/loaded-price-monitor.env
$EDITOR ~/.config/loaded-price-monitor.env                       # fill in your secrets
$EDITOR ~/.config/systemd/user/loaded-price-monitor.service      # fix paths

systemctl --user daemon-reload
systemctl --user enable --now loaded-price-monitor.timer
systemctl --user list-timers loaded-price-monitor.timer
journalctl --user -u loaded-price-monitor.service -f             # follow logs
```

> Want it to run when nobody is logged in? `sudo loginctl enable-linger $USER`.
> Or install as a **system** unit under `/etc/systemd/system/` instead.

### 6d — Windows Task Scheduler

A PowerShell helper is at `examples/Register-LoadedPriceMonitor.ps1`. Edit the
paths at the top of the script, then from an elevated PowerShell:

```powershell
cd C:\path\to\loaded-price-tracker
.\examples\Register-LoadedPriceMonitor.ps1
```

This registers a task that runs every 6 hours and survives reboots. To remove:

```powershell
Unregister-ScheduledTask -TaskName "Loaded Price Monitor" -Confirm:$false
```

Manual alternative: Task Scheduler → Create Task → Trigger "On a schedule,
every 6 hours" → Action: start `loaded-rs.exe` with argument `--no-banner check`,
working directory set to your repo root, with the three env vars set on the
Action.

### 6e — macOS launchd

A sample plist is at `examples/com.loaded.price-monitor.plist`. Copy it to
`~/Library/LaunchAgents/`, edit the paths and env vars, then:

```bash
launchctl load ~/Library/LaunchAgents/com.loaded.price-monitor.plist
launchctl list | grep loaded
```

---

## Raspberry Pi quick-start

The Pi is the canonical "leave it running" target. The path of least
resistance:

```bash
# On Raspberry Pi OS Bookworm or later
sudo apt update && sudo apt install -y python3-venv git

git clone https://github.com/arc94c3/loaded-price-tracker.git
cd loaded-price-tracker/python
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt

cp .env.example .env
nano .env                     # paste your webhook/bot creds

python -m src.main add --url "..." --name "..." --rule threshold:25:once

# Then either:
python -m src.main check --watch --interval 360       # foreground, kill with Ctrl-C
# ...or set up systemd (Step 6c) for proper background operation
```

For a permanent setup, follow **Step 6c** (systemd timer) above. The Pi will
then check prices on schedule, persist across reboots, and you can `journalctl`
to see logs.

---

## Updating later

```bash
cd loaded-price-tracker
git pull
# Re-install deps if requirements changed:
( cd python && source .venv/bin/activate && pip install -r requirements.txt )
# Or rebuild Rust/Go binary
( cd rust && cargo build --release )
( cd go && go build -o ../loaded-go ./cmd/loaded-go )
```

If you're tracking products in your own private fork, periodically merge from
upstream:

```bash
git remote add upstream https://github.com/arc94c3/loaded-price-tracker.git
git fetch upstream
git merge upstream/main
```

---

## Troubleshooting

**`history.json` keeps showing as modified in `git status`**
That's expected — the monitor updates it every run. Either commit it
periodically (`git add history.json && git commit -m 'history'`), add it to
your local `.gitignore`, or ignore it via `git update-index --skip-worktree
history.json`.

**Notifications never arrive**
Check the run log. If the scraper succeeded but nothing fired, your rules
probably aren't matching yet. Try adding a `{"type":"any_change"}` rule
temporarily to confirm the notification pipeline works.

**systemd timer not firing**
- `systemctl --user list-timers` to see next run time.
- `journalctl --user -u loaded-price-monitor.service` for errors.
- Make sure `loginctl enable-linger $USER` is set if you want it to run while
  logged out.

**Scraper fails ("could not find price")**
loaded.com may have changed page structure. Open an issue with a product URL.
