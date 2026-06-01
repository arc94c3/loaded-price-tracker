"""Evaluate per-product notification rules."""
from __future__ import annotations

from decimal import Decimal
from typing import Any, Dict, List, Optional, Tuple


def _fmt_money(value: Decimal, currency: str) -> str:
    symbol = {"GBP": "£", "USD": "$", "EUR": "€"}.get(currency, "")
    return f"{symbol}{value:.2f}"


def _threshold_key(rule: Dict[str, Any]) -> str:
    return f"threshold:{Decimal(str(rule['at_or_below'])):.2f}"


def evaluate(
    rules: List[Dict[str, Any]],
    old_price: Optional[Decimal],
    new_price: Decimal,
    currency: str,
    armed_rules: Optional[Dict[str, bool]] = None,
) -> Tuple[List[str], Dict[str, bool]]:
    """Evaluate `rules` and return (reasons, updated_armed_state).

    `armed_rules` is the previous arming state (dict of rule key -> armed bool).
    A rule key is treated as armed by default (missing == True).
    """
    armed_state = dict(armed_rules or {})
    reasons: List[str] = []

    for rule in rules:
        t = rule.get("type")

        if t == "any_change":
            if old_price is not None and new_price != old_price:
                reasons.append("price changed")

        elif t == "threshold":
            at_or_below = Decimal(str(rule["at_or_below"]))
            only_once = bool(rule.get("only_once", False))
            key = _threshold_key(rule)
            currently_below = new_price <= at_or_below
            previously_below = old_price is not None and old_price <= at_or_below

            if only_once:
                # Re-arm when price moves above the threshold
                if not currently_below:
                    armed_state[key] = True
                else:
                    is_armed = armed_state.get(key, True)
                    if is_armed and not previously_below:
                        reasons.append(f"hit target {_fmt_money(at_or_below, currency)}")
                        armed_state[key] = False
                    elif is_armed and old_price is None:
                        # First time seeing this product and it's already at/below target
                        reasons.append(f"at target {_fmt_money(at_or_below, currency)}")
                        armed_state[key] = False
            else:
                if currently_below and not previously_below:
                    reasons.append(f"hit target {_fmt_money(at_or_below, currency)}")
                elif currently_below and old_price is None:
                    reasons.append(f"at target {_fmt_money(at_or_below, currency)}")

        elif t == "percent_drop":
            min_percent = Decimal(str(rule["min_percent"]))
            if old_price is not None and old_price > 0 and new_price < old_price:
                drop_pct = (old_price - new_price) / old_price * Decimal(100)
                if drop_pct >= min_percent:
                    reasons.append(f"-{drop_pct:.1f}% drop")

    # De-duplicate while preserving order
    seen = set()
    deduped = []
    for r in reasons:
        if r not in seen:
            deduped.append(r)
            seen.add(r)
    return deduped, armed_state
