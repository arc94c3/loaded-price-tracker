"""Load and save price history."""
from __future__ import annotations

import json
from copy import deepcopy
from datetime import datetime, timezone
from decimal import Decimal
from pathlib import Path
from typing import Any, Dict, Optional


def _utcnow_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def load_history(path: Path) -> Dict[str, Any]:
    if not path.exists():
        return {}
    with path.open("r", encoding="utf-8") as f:
        try:
            return json.load(f)
        except json.JSONDecodeError:
            return {}


def save_history(path: Path, history: Dict[str, Any]) -> bool:
    """Write history to disk if it differs from the on-disk copy. Returns True if written."""
    serialised = json.dumps(history, indent=2, sort_keys=True, default=str) + "\n"
    if path.exists():
        existing = path.read_text(encoding="utf-8")
        if existing == serialised:
            return False
    path.write_text(serialised, encoding="utf-8")
    return True


def get_entry(history: Dict[str, Any], product_id: str) -> Optional[Dict[str, Any]]:
    return history.get(product_id)


def update_entry(
    history: Dict[str, Any],
    product_id: str,
    new_price: Decimal,
    currency: str,
    armed_rules: Optional[Dict[str, bool]] = None,
    max_history: int = 200,
) -> Dict[str, Any]:
    """Append the new price to the product's history and return the updated entry."""
    entry = deepcopy(history.get(product_id) or {})
    now = _utcnow_iso()
    entry.setdefault("history", [])
    entry["history"].append({"price": str(new_price), "at": now})
    if len(entry["history"]) > max_history:
        entry["history"] = entry["history"][-max_history:]
    entry["current_price"] = str(new_price)
    entry["currency"] = currency
    entry["last_checked"] = now
    if armed_rules is not None:
        entry["armed_rules"] = armed_rules
    history[product_id] = entry
    return entry


def touch_last_checked(history: Dict[str, Any], product_id: str) -> None:
    entry = history.get(product_id)
    if entry is None:
        return
    entry["last_checked"] = _utcnow_iso()
