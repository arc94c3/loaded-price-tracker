"""Fetch and parse product prices from loaded.com."""
from __future__ import annotations

import json
import logging
import re
import time
from dataclasses import dataclass
from decimal import Decimal, InvalidOperation
from typing import Optional

from curl_cffi import requests
from curl_cffi.requests.exceptions import RequestException
from bs4 import BeautifulSoup

# Cloudflare in front of loaded.com fingerprints the TLS handshake (JA3/JA4),
# so a stock urllib3/OpenSSL client is challenged regardless of headers.
# curl_cffi impersonates a real Chrome TLS ClientHello which passes the
# managed challenge cleanly.
IMPERSONATE = "chrome"

log = logging.getLogger(__name__)

USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/131.0.0.0 Safari/537.36"
)

# Cloudflare in front of loaded.com challenges requests that don't look like a
# real browser. The full set of Sec-Fetch / Upgrade-Insecure-Requests headers
# below mirrors what Chrome sends for a top-level navigation and is enough to
# pass the default managed challenge without solving it.
BROWSER_HEADERS = {
    "User-Agent": USER_AGENT,
    "Accept": (
        "text/html,application/xhtml+xml,application/xml;q=0.9,"
        "image/avif,image/webp,image/apng,*/*;q=0.8"
    ),
    "Accept-Language": "en-GB,en;q=0.9",
    "Accept-Encoding": "gzip, deflate, br",
    "Sec-Fetch-Site": "none",
    "Sec-Fetch-Mode": "navigate",
    "Sec-Fetch-User": "?1",
    "Sec-Fetch-Dest": "document",
    "Upgrade-Insecure-Requests": "1",
    "Cache-Control": "no-cache",
    "Pragma": "no-cache",
}

REQUEST_TIMEOUT = 20
RETRY_DELAY = 2.0


class ScrapeError(Exception):
    """Raised when a product page cannot be fetched or parsed."""


@dataclass
class PriceResult:
    price: Decimal
    currency: str


def _fetch(url: str, session: Optional[requests.Session] = None) -> str:
    sess = session or requests.Session()
    last_err: Optional[Exception] = None
    for attempt in range(2):
        try:
            resp = sess.get(
                url,
                headers=BROWSER_HEADERS,
                timeout=REQUEST_TIMEOUT,
                allow_redirects=True,
                impersonate=IMPERSONATE,
            )
            resp.raise_for_status()
            return resp.text
        except RequestException as e:
            last_err = e
            log.warning("Fetch attempt %d failed for %s: %s", attempt + 1, url, e)
            time.sleep(RETRY_DELAY)
    raise ScrapeError(f"Failed to fetch {url}: {last_err}")


def _to_decimal(value) -> Optional[Decimal]:
    if value is None:
        return None
    try:
        return Decimal(str(value).strip().replace(",", ""))
    except (InvalidOperation, ValueError):
        return None


def _parse_jsonld(soup: BeautifulSoup) -> Optional[PriceResult]:
    for tag in soup.find_all("script", type="application/ld+json"):
        raw = tag.string or tag.get_text() or ""
        if not raw.strip():
            continue
        try:
            data = json.loads(raw)
        except json.JSONDecodeError:
            continue
        for node in _walk_jsonld(data):
            if not isinstance(node, dict):
                continue
            t = node.get("@type")
            types = t if isinstance(t, list) else [t]
            if "Product" not in types and "Offer" not in types:
                continue
            offer = node.get("offers") if "Product" in types else node
            offers = offer if isinstance(offer, list) else [offer] if offer else []
            for o in offers:
                if not isinstance(o, dict):
                    continue
                price = _to_decimal(o.get("price") or o.get("lowPrice"))
                currency = o.get("priceCurrency") or "GBP"
                if price is not None:
                    return PriceResult(price=price, currency=str(currency))
    return None


def _walk_jsonld(data):
    if isinstance(data, list):
        for item in data:
            yield from _walk_jsonld(item)
    elif isinstance(data, dict):
        yield data
        if "@graph" in data:
            yield from _walk_jsonld(data["@graph"])
        for v in data.values():
            if isinstance(v, (list, dict)):
                yield from _walk_jsonld(v)


_PRICE_RE = re.compile(r"([£$€])\s*([0-9]+(?:[.,][0-9]{2})?)")
_CURRENCY_SYMBOLS = {"£": "GBP", "$": "USD", "€": "EUR"}


def _parse_fallback(soup: BeautifulSoup) -> Optional[PriceResult]:
    selectors = [
        '[itemprop="price"]',
        ".product-price",
        ".price",
        '[class*="price" i]',
    ]
    for sel in selectors:
        for el in soup.select(sel):
            content = el.get("content") or el.get_text(" ", strip=True)
            if not content:
                continue
            m = _PRICE_RE.search(content)
            if m:
                price = _to_decimal(m.group(2))
                if price is not None:
                    return PriceResult(
                        price=price,
                        currency=_CURRENCY_SYMBOLS.get(m.group(1), "GBP"),
                    )
            price = _to_decimal(content)
            if price is not None:
                return PriceResult(price=price, currency="GBP")
    return None


def scrape_price(url: str, session: Optional[requests.Session] = None) -> PriceResult:
    """Fetch `url` and return the current price. Raises ScrapeError on failure."""
    html = _fetch(url, session=session)
    soup = BeautifulSoup(html, "html.parser")
    result = _parse_jsonld(soup) or _parse_fallback(soup)
    if result is None:
        raise ScrapeError(f"Could not parse price from {url}")
    return result
