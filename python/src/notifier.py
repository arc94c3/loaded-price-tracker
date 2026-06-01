"""Discord and Telegram notification senders."""
from __future__ import annotations

import logging
from dataclasses import dataclass
from datetime import datetime, timezone
from decimal import Decimal
from typing import List, Optional

import requests

log = logging.getLogger(__name__)

CURRENCY_SYMBOLS = {"GBP": "£", "USD": "$", "EUR": "€"}


@dataclass
class PriceChange:
    product_name: str
    product_url: str
    old_price: Optional[Decimal]
    new_price: Decimal
    currency: str
    reasons: List[str]

    @property
    def symbol(self) -> str:
        return CURRENCY_SYMBOLS.get(self.currency, "")

    @property
    def delta(self) -> Optional[Decimal]:
        if self.old_price is None:
            return None
        return self.new_price - self.old_price

    @property
    def percent(self) -> Optional[Decimal]:
        if self.old_price is None or self.old_price == 0:
            return None
        return (self.new_price - self.old_price) / self.old_price * Decimal(100)

    @property
    def arrow(self) -> str:
        d = self.delta
        if d is None:
            return "🆕"
        if d < 0:
            return "🔻"
        if d > 0:
            return "🔺"
        return "•"

    def fmt_money(self, value: Decimal) -> str:
        return f"{self.symbol}{value:.2f}"

    def headline(self) -> str:
        if self.old_price is None:
            base = f"{self.arrow} {self.product_name}: {self.fmt_money(self.new_price)} (new)"
        else:
            d = self.delta or Decimal(0)
            p = self.percent or Decimal(0)
            sign = "-" if d < 0 else "+"
            base = (
                f"{self.arrow} {self.product_name}: "
                f"{self.fmt_money(self.old_price)} → {self.fmt_money(self.new_price)} "
                f"({sign}{self.fmt_money(abs(d))}, {sign}{abs(p):.1f}%)"
            )
        if self.reasons:
            base += " — " + ", ".join(self.reasons)
        return base


def _send_discord(webhook_url: str, change: PriceChange) -> None:
    color = 0xE74C3C if (change.delta or Decimal(0)) < 0 else 0x3498DB
    if (change.delta or Decimal(0)) > 0:
        color = 0xF39C12
    embed = {
        "title": change.product_name,
        "url": change.product_url,
        "color": color,
        "description": change.headline(),
        "timestamp": datetime.now(timezone.utc).isoformat(),
    }
    fields = []
    if change.old_price is not None:
        fields.append({"name": "Previous", "value": change.fmt_money(change.old_price), "inline": True})
    fields.append({"name": "Current", "value": change.fmt_money(change.new_price), "inline": True})
    if change.reasons:
        fields.append({"name": "Why", "value": ", ".join(change.reasons), "inline": False})
    embed["fields"] = fields

    payload = {"embeds": [embed]}
    resp = requests.post(webhook_url, json=payload, timeout=15)
    resp.raise_for_status()


def _send_telegram(bot_token: str, chat_id: str, change: PriceChange) -> None:
    text = (
        f"*{_md_escape(change.product_name)}*\n"
        f"{_md_escape(change.headline())}\n"
        f"[View product]({change.product_url})"
    )
    url = f"https://api.telegram.org/bot{bot_token}/sendMessage"
    resp = requests.post(
        url,
        json={
            "chat_id": chat_id,
            "text": text,
            "parse_mode": "Markdown",
            "disable_web_page_preview": False,
        },
        timeout=15,
    )
    resp.raise_for_status()


def _md_escape(text: str) -> str:
    for ch in ("_", "*", "[", "]", "`"):
        text = text.replace(ch, "\\" + ch)
    return text


def notify(
    change: PriceChange,
    discord_webhook: Optional[str],
    telegram_token: Optional[str],
    telegram_chat_id: Optional[str],
    dry_run: bool = False,
) -> None:
    """Send notification to all configured channels. Failures in one don't block the other."""
    if dry_run:
        log.info("[dry-run] %s", change.headline())
        return

    if discord_webhook:
        try:
            _send_discord(discord_webhook, change)
            log.info("Discord notified for %s", change.product_name)
        except Exception as e:
            log.error("Discord notification failed for %s: %s", change.product_name, e)

    if telegram_token and telegram_chat_id:
        try:
            _send_telegram(telegram_token, telegram_chat_id, change)
            log.info("Telegram notified for %s", change.product_name)
        except Exception as e:
            log.error("Telegram notification failed for %s: %s", change.product_name, e)


def notify_error_summary(
    failures: List[str],
    discord_webhook: Optional[str],
    telegram_token: Optional[str],
    telegram_chat_id: Optional[str],
) -> None:
    if not failures:
        return
    text = f"⚠️ Loaded Price Monitor: {len(failures)} product(s) failed:\n" + "\n".join(
        f"- {f}" for f in failures
    )
    if discord_webhook:
        try:
            requests.post(discord_webhook, json={"content": text}, timeout=15).raise_for_status()
        except Exception as e:
            log.error("Discord error summary failed: %s", e)
    if telegram_token and telegram_chat_id:
        try:
            requests.post(
                f"https://api.telegram.org/bot{telegram_token}/sendMessage",
                json={"chat_id": telegram_chat_id, "text": text},
                timeout=15,
            ).raise_for_status()
        except Exception as e:
            log.error("Telegram error summary failed: %s", e)
