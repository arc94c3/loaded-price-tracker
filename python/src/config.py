"""Load and validate products.json."""
from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, List
from urllib.parse import urlparse


class ConfigError(Exception):
    pass


VALID_RULE_TYPES = {"any_change", "threshold", "percent_drop"}
DEFAULT_NOTIFY: List[Dict[str, Any]] = [{"type": "any_change"}]


@dataclass
class Product:
    id: str
    name: str
    url: str
    tags: List[str] = field(default_factory=list)
    notify: List[Dict[str, Any]] = field(default_factory=lambda: list(DEFAULT_NOTIFY))


def _validate_rule(rule: Dict[str, Any], product_id: str) -> Dict[str, Any]:
    if not isinstance(rule, dict):
        raise ConfigError(f"{product_id}: rule must be an object")
    t = rule.get("type")
    if t not in VALID_RULE_TYPES:
        raise ConfigError(f"{product_id}: unknown rule type '{t}'")
    if t == "threshold":
        if not isinstance(rule.get("at_or_below"), (int, float)):
            raise ConfigError(f"{product_id}: threshold rule needs numeric 'at_or_below'")
        if "only_once" in rule and not isinstance(rule["only_once"], bool):
            raise ConfigError(f"{product_id}: 'only_once' must be boolean")
    if t == "percent_drop":
        if not isinstance(rule.get("min_percent"), (int, float)):
            raise ConfigError(f"{product_id}: percent_drop needs numeric 'min_percent'")
    return rule


def _validate_product(raw: Dict[str, Any]) -> Product:
    for key in ("id", "name", "url"):
        if not raw.get(key) or not isinstance(raw[key], str):
            raise ConfigError(f"Product missing required string field '{key}': {raw}")
    parsed = urlparse(raw["url"])
    if parsed.scheme not in ("http", "https") or not parsed.netloc:
        raise ConfigError(f"{raw['id']}: invalid URL '{raw['url']}'")
    tags = raw.get("tags", []) or []
    if not isinstance(tags, list) or any(not isinstance(t, str) for t in tags):
        raise ConfigError(f"{raw['id']}: 'tags' must be a list of strings")
    notify_raw = raw.get("notify")
    if notify_raw is None:
        notify = [dict(r) for r in DEFAULT_NOTIFY]
    else:
        if not isinstance(notify_raw, list) or not notify_raw:
            raise ConfigError(f"{raw['id']}: 'notify' must be a non-empty list")
        notify = [_validate_rule(r, raw["id"]) for r in notify_raw]
    return Product(
        id=raw["id"],
        name=raw["name"],
        url=raw["url"],
        tags=tags,
        notify=notify,
    )


def load_config(path: Path) -> List[Product]:
    if not path.exists():
        raise ConfigError(f"Config file not found: {path}")
    with path.open("r", encoding="utf-8") as f:
        data = json.load(f)
    products_raw = data.get("products", [])
    if not isinstance(products_raw, list):
        raise ConfigError("'products' must be a list")
    products = [_validate_product(p) for p in products_raw]
    seen = set()
    for p in products:
        if p.id in seen:
            raise ConfigError(f"Duplicate product id: {p.id}")
        seen.add(p.id)
    return products


def save_config(path: Path, products: List[Product]) -> None:
    data = {
        "products": [
            {
                "id": p.id,
                "name": p.name,
                "url": p.url,
                "tags": p.tags,
                "notify": p.notify,
            }
            for p in products
        ]
    }
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


_SLUG_RE = re.compile(r"[^a-z0-9]+")


def slugify(name: str) -> str:
    slug = _SLUG_RE.sub("-", name.lower()).strip("-")
    return slug or "product"
