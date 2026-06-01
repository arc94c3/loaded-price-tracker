"""Loaded Price Monitor — entrypoint."""
from __future__ import annotations

import argparse
import logging
import os
import sys
import time
from decimal import Decimal
from pathlib import Path
from typing import List, Optional
from urllib.parse import urlparse

from . import storage
from .banner import print_banner
from .config import ConfigError, Product, load_config, save_config, slugify
from .notifier import PriceChange, notify, notify_error_summary
from .rules import evaluate
from .scraper import ScrapeError, scrape_price

PYTHON_ROOT = Path(__file__).resolve().parent.parent
ROOT = PYTHON_ROOT.parent
PRODUCTS_PATH = ROOT / "products.json"
HISTORY_PATH = ROOT / "history.json"

POLITE_DELAY_SEC = 1.5

log = logging.getLogger("loaded")


def _load_env_file() -> None:
    env_path = PYTHON_ROOT / ".env"
    if not env_path.exists():
        return
    for line in env_path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        os.environ.setdefault(k.strip(), v.strip().strip('"').strip("'"))


def _secrets():
    return (
        os.environ.get("DISCORD_WEBHOOK_URL") or None,
        os.environ.get("TELEGRAM_BOT_TOKEN") or None,
        os.environ.get("TELEGRAM_CHAT_ID") or None,
    )


def cmd_check(args: argparse.Namespace) -> int:
    if getattr(args, "watch", False):
        return _watch_loop(args)
    return _check_once(args)


def _watch_loop(args: argparse.Namespace) -> int:
    interval_sec = max(1, args.interval) * 60
    log.info("Watch mode: checking every %d minute(s). Ctrl-C to stop.", args.interval)
    last_rc = 0
    try:
        while True:
            last_rc = _check_once(args)
            log.info("Next check in %d minute(s).", args.interval)
            time.sleep(interval_sec)
    except KeyboardInterrupt:
        log.info("Watch stopped.")
        return last_rc


def _check_once(args: argparse.Namespace) -> int:
    try:
        products = load_config(PRODUCTS_PATH)
    except ConfigError as e:
        log.error("Config error: %s", e)
        return 2

    if args.product:
        products = [p for p in products if p.id == args.product]
        if not products:
            log.error("No product with id '%s'", args.product)
            return 2

    history = storage.load_history(HISTORY_PATH)
    discord, tg_token, tg_chat = _secrets()
    failures: List[str] = []
    changes_to_notify = 0

    for i, product in enumerate(products):
        if i > 0:
            time.sleep(POLITE_DELAY_SEC)
        log.info("Checking %s", product.name)
        try:
            result = scrape_price(product.url)
        except ScrapeError as e:
            log.error("Scrape failed for %s: %s", product.id, e)
            failures.append(f"{product.name} ({product.id}): {e}")
            storage.touch_last_checked(history, product.id)
            continue

        old_entry = storage.get_entry(history, product.id)
        old_price: Optional[Decimal] = None
        if old_entry and old_entry.get("current_price") is not None:
            old_price = Decimal(str(old_entry["current_price"]))
        armed_prev = (old_entry or {}).get("armed_rules") or {}

        reasons, armed_new = evaluate(
            product.notify, old_price, result.price, result.currency, armed_prev
        )

        storage.update_entry(
            history,
            product.id,
            result.price,
            result.currency,
            armed_rules=armed_new,
        )

        if reasons:
            changes_to_notify += 1
            change = PriceChange(
                product_name=product.name,
                product_url=product.url,
                old_price=old_price,
                new_price=result.price,
                currency=result.currency,
                reasons=reasons,
            )
            notify(change, discord, tg_token, tg_chat, dry_run=args.dry_run)
        else:
            log.info("No notification needed for %s (price %s)", product.id, result.price)

    if failures and not args.dry_run:
        notify_error_summary(failures, discord, tg_token, tg_chat)

    if not args.dry_run:
        wrote = storage.save_history(HISTORY_PATH, history)
        log.info("History %s", "updated" if wrote else "unchanged")
    else:
        log.info("[dry-run] history not saved")

    log.info(
        "Done. %d product(s) checked, %d notification(s), %d failure(s).",
        len(products), changes_to_notify, len(failures),
    )
    return 0 if not failures else 1


def _parse_rule_arg(spec: str) -> dict:
    parts = spec.split(":")
    t = parts[0]
    if t == "any_change":
        return {"type": "any_change"}
    if t == "threshold":
        if len(parts) < 2:
            raise SystemExit(f"threshold rule needs a value, got: {spec}")
        rule = {"type": "threshold", "at_or_below": float(parts[1])}
        if len(parts) >= 3 and parts[2] == "once":
            rule["only_once"] = True
        return rule
    if t == "percent_drop":
        if len(parts) < 2:
            raise SystemExit(f"percent_drop rule needs a value, got: {spec}")
        return {"type": "percent_drop", "min_percent": float(parts[1])}
    raise SystemExit(f"Unknown rule type: {t}")


def cmd_add(args: argparse.Namespace) -> int:
    parsed = urlparse(args.url)
    if "loaded.com" not in (parsed.netloc or "").lower():
        log.error("URL must be on loaded.com, got: %s", args.url)
        return 2

    try:
        products = load_config(PRODUCTS_PATH)
    except ConfigError as e:
        log.error("Config error: %s", e)
        return 2

    if any(p.url == args.url for p in products):
        log.error("A product with this URL is already tracked.")
        return 2

    log.info("Test-scraping %s ...", args.url)
    try:
        result = scrape_price(args.url)
    except ScrapeError as e:
        log.error("Could not scrape the page: %s", e)
        return 2
    log.info("Parsed current price: %s%s", result.currency, result.price)

    base_id = slugify(args.name)
    new_id = base_id
    existing_ids = {p.id for p in products}
    suffix = 2
    while new_id in existing_ids:
        new_id = f"{base_id}-{suffix}"
        suffix += 1

    rules = [_parse_rule_arg(r) for r in args.rule] if args.rule else [{"type": "any_change"}]
    tags = args.tags or []

    product = Product(id=new_id, name=args.name, url=args.url, tags=tags, notify=rules)
    products.append(product)
    save_config(PRODUCTS_PATH, products)
    log.info("Added '%s' (id: %s) with %d rule(s).", args.name, new_id, len(rules))
    return 0


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="loaded", description="Loaded.com price monitor")
    p.add_argument("--no-banner", action="store_true", help="Suppress ASCII banner")
    sub = p.add_subparsers(dest="cmd")

    check = sub.add_parser("check", help="Check all tracked products (default)")
    check.add_argument("--dry-run", action="store_true", help="Don't send or persist anything")
    check.add_argument("--product", help="Check a single product by id")
    check.add_argument(
        "--watch", action="store_true",
        help="Run continuously, repeating every --interval minutes",
    )
    check.add_argument(
        "--interval", type=int, default=360,
        help="Minutes between checks when --watch is set (default: 360 = 6h)",
    )
    check.set_defaults(func=cmd_check)

    add = sub.add_parser("add", help="Add a product to the tracker")
    add.add_argument("--url", required=True, help="Product URL on loaded.com")
    add.add_argument("--name", required=True, help="Display name")
    add.add_argument("--tags", nargs="*", default=[], help="Optional tags")
    add.add_argument(
        "--rule", action="append", default=[],
        help="Notification rule (repeatable). Forms: any_change | threshold:25 | threshold:25:once | percent_drop:15",
    )
    add.set_defaults(func=cmd_add)

    return p


def main(argv: Optional[List[str]] = None) -> int:
    _load_env_file()
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        datefmt="%H:%M:%S",
    )

    parser = build_parser()
    args = parser.parse_args(argv)
    cmd = args.cmd or "check"

    show_banner = (
        not args.no_banner
        and sys.stdout.isatty()
        and not (cmd == "check" and getattr(args, "dry_run", False))
    )
    if show_banner:
        print_banner()

    if cmd == "check" and not hasattr(args, "func"):
        args = parser.parse_args(["check", *(argv or [])])
        args.no_banner = True  # already shown (or skipped) above
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
