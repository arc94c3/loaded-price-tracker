# Getting Started — Hosting on GitHub Actions

A friendly, click-by-click guide to running your price monitor on GitHub's
free scheduled CI. Aimed at people who've never used GitHub Actions before.

**Time required:** about 5–10 minutes.
**What you'll need:** a GitHub account and *either* a Discord channel you can
post to *or* a Telegram account.
**Cost:** free. GitHub Actions includes generous monthly minutes for public
repos.

> Prefer to run on your own machine (Raspberry Pi, NAS, laptop)? See
> [GETTING_STARTED_LOCAL.md](GETTING_STARTED_LOCAL.md) instead.

---

## Step 1 — Fork the repo

A "fork" is your own personal copy of the project on GitHub.

1. Go to the repo page on GitHub.
2. Click the **Fork** button (top right).
3. Leave all settings as default and click **Create fork**.

You now have your own copy at `github.com/<your-username>/loaded-price-tracker`.

> **Want it private?** GitHub doesn't allow forking into a private repo
> directly. See [the private setup notes](#optional-private-setup) at the
> bottom of this guide.

---

## Step 2 — Enable GitHub Actions

By default, forks have Actions disabled (a safety measure). You need to turn
them on.

1. On your fork, click the **Actions** tab.
2. You'll see a yellow banner: *"Workflows aren't being run on this forked repository."*
3. Click the green button: **I understand my workflows, go ahead and enable them.**

---

## Step 3 — Get your notification credentials

You only need **one** of these (Discord or Telegram). You can also set up both.

### Discord (easier)

1. Open Discord. Pick a channel where you want notifications to appear.
2. Click the channel name → **Edit Channel** → **Integrations** → **Webhooks** → **New Webhook**.
3. Optionally give it a name and avatar.
4. Click **Copy Webhook URL**. Save this somewhere safe — you'll paste it in Step 4.

### Telegram

1. In Telegram, search for **@BotFather** and open the chat.
2. Send `/newbot`. Follow the prompts to give your bot a name.
3. BotFather replies with a **bot token** that looks like `1234567890:ABCdef...`. Save it.
4. Search for your new bot in Telegram and **send it any message** (e.g. "hi"). This is required before the bot can message you back.
5. In a browser, visit:
   ```
   https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates
   ```
   Replace `<YOUR_TOKEN>` with the token from step 3.
6. In the JSON response, find `"chat":{"id":...}`. That number is your **chat id**. Save it.

---

## Step 4 — Add your secrets to GitHub

This is how the Action knows where to send notifications. Secrets are
encrypted and never visible to anyone but you — even if your fork is public.

1. On your fork, go to **Settings** → **Secrets and variables** → **Actions**.
2. Click **New repository secret**.
3. Add each secret you have:

| Name | Value |
|---|---|
| `DISCORD_WEBHOOK_URL` | The webhook URL from Step 3 (Discord) |
| `TELEGRAM_BOT_TOKEN` | The bot token from Step 3 (Telegram) |
| `TELEGRAM_CHAT_ID` | The chat id from Step 3 (Telegram) |

> You only need to add the ones for the channel(s) you're using.

---

## Step 5 — Allow the Action to commit

The Action saves price history back to the repo, so it needs write permission.

1. On your fork, go to **Settings** → **Actions** → **General**.
2. Scroll to **Workflow permissions** at the bottom.
3. Select **Read and write permissions**.
4. Click **Save**.

---

## Step 6 — Add some products to track

You don't need to install anything. You can edit the file right on GitHub.

1. On your fork, click **products.json**.
2. Click the ✏️ pencil icon (top right) to edit.
3. Replace the contents with something like:

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

4. Scroll down and click **Commit changes**.

### Notification rule cheat sheet

| Rule | What it does |
|---|---|
| `{"type":"any_change"}` | Notify on any price change, up or down |
| `{"type":"threshold","at_or_below":25}` | Notify when the price drops to £25 or below |
| `{"type":"threshold","at_or_below":25,"only_once":true}` | Same, but only notify once until price rises back above £25 |
| `{"type":"percent_drop","min_percent":15}` | Notify on drops of 15% or more vs the last check |

You can combine multiple rules per product — any of them firing triggers a
notification.

---

## Step 7 — Run it for the first time

This also "wakes up" the scheduled cron, which won't fire on a fork until the
workflow has been triggered manually at least once.

1. Go to the **Actions** tab.
2. Click **Check prices** in the left sidebar.
3. Click the **Run workflow** dropdown on the right.
4. Leave the branch as `main` and click the green **Run workflow** button.
5. Refresh the page after a few seconds — you'll see a new run with a yellow
   dot (running) that turns green (success) or red (failed).
6. Click the run to see logs.

If the run is green, you're done! You'll get a notification immediately if any
of your products meet a rule. Otherwise, you'll start getting notifications as
prices change.

---

## What happens next

- The Action runs **every 6 hours** automatically.
- Each run:
  - Fetches the current price of each product.
  - Compares it to the last known price stored in `history.json`.
  - Sends notifications if any rules fire.
  - Commits the updated `history.json` back to your repo (you'll see commits
    from `github-actions[bot]`).
- You can change the schedule by editing the `cron:` line in
  `.github/workflows/check-prices.yml`. For example, `"0 */2 * * *"` would
  check every 2 hours.

---

## Adding more products later

Just repeat **Step 6** — edit `products.json`, commit. The next run will pick
it up.

If you prefer the command line, install one of the implementations
([Python, Rust, or Go](README.md#quick-start)) and use the `add` command —
e.g. `python -m src.main add --url "..." --name "..." --rule threshold:25:once`.

---

## Troubleshooting

**The run failed with "Permission denied" when committing**
You skipped Step 5. Go back and set workflow permissions to read/write.

**No notifications are arriving**
- Check the Action logs — did it actually find a price?
- Test your webhook/bot manually. For Discord, paste the webhook URL into a
  tool like [Discohook](https://discohook.org/). For Telegram, send a test
  with `curl`:
  ```
  curl -X POST "https://api.telegram.org/bot<TOKEN>/sendMessage" \
       -d "chat_id=<CHAT_ID>&text=test"
  ```

**The scraper can't find the price**
loaded.com may have changed its page structure. Open an issue with a product
URL and we'll fix the parser.

**The cron isn't firing**
GitHub disables scheduled workflows on inactive forks after ~60 days.
Trigger the workflow manually (Step 7) to wake it up again.

---

## Optional: private setup

If you don't want anyone to see your wishlist:

1. Don't fork. Instead, on your local machine:
   ```bash
   git clone <this-repo-url> loaded-price-tracker
   cd loaded-price-tracker
   git remote remove origin
   ```
2. Create a **new empty private repo** on GitHub.
3. Add it as the new origin and push:
   ```bash
   git remote add origin <your-new-private-repo-url>
   git push -u origin main
   ```
4. Continue from Step 2 above on your new private repo.

To pull future updates from the original project, add it as an "upstream"
remote and pull from it:
```bash
git remote add upstream <original-repo-url>
git pull upstream main
```
