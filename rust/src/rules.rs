use crate::config::Rule;
use rust_decimal::Decimal;
use rust_decimal_macros::dec;
use std::collections::BTreeMap;

fn currency_symbol(c: &str) -> &str {
    match c {
        "GBP" => "£",
        "USD" => "$",
        "EUR" => "€",
        _ => "",
    }
}

fn fmt_money(value: Decimal, currency: &str) -> String {
    format!("{}{:.2}", currency_symbol(currency), value)
}

fn threshold_key(at_or_below: Decimal) -> String {
    format!("threshold:{:.2}", at_or_below)
}

pub fn evaluate(
    rules: &[Rule],
    old_price: Option<Decimal>,
    new_price: Decimal,
    currency: &str,
    armed_prev: &BTreeMap<String, bool>,
) -> (Vec<String>, BTreeMap<String, bool>) {
    let mut armed = armed_prev.clone();
    let mut reasons: Vec<String> = Vec::new();

    for rule in rules {
        match rule {
            Rule::AnyChange => {
                if let Some(old) = old_price {
                    if new_price != old {
                        reasons.push("price changed".into());
                    }
                }
            }
            Rule::Threshold { at_or_below, only_once } => {
                let target = Decimal::try_from(*at_or_below).unwrap_or(dec!(0));
                let key = threshold_key(target);
                let currently_below = new_price <= target;
                let previously_below = matches!(old_price, Some(p) if p <= target);

                if *only_once {
                    if !currently_below {
                        armed.insert(key.clone(), true);
                    } else {
                        let is_armed = armed.get(&key).copied().unwrap_or(true);
                        if is_armed && !previously_below {
                            reasons.push(format!("hit target {}", fmt_money(target, currency)));
                            armed.insert(key, false);
                        } else if is_armed && old_price.is_none() {
                            reasons.push(format!("at target {}", fmt_money(target, currency)));
                            armed.insert(key, false);
                        }
                    }
                } else if currently_below && !previously_below {
                    reasons.push(format!("hit target {}", fmt_money(target, currency)));
                } else if currently_below && old_price.is_none() {
                    reasons.push(format!("at target {}", fmt_money(target, currency)));
                }
            }
            Rule::PercentDrop { min_percent } => {
                let min = Decimal::try_from(*min_percent).unwrap_or(dec!(0));
                if let Some(old) = old_price {
                    if old > dec!(0) && new_price < old {
                        let drop_pct = (old - new_price) / old * dec!(100);
                        if drop_pct >= min {
                            reasons.push(format!("-{:.1}% drop", drop_pct));
                        }
                    }
                }
            }
        }
    }

    // Dedupe preserving order.
    let mut seen = std::collections::HashSet::new();
    let deduped: Vec<String> = reasons
        .into_iter()
        .filter(|r| seen.insert(r.clone()))
        .collect();
    (deduped, armed)
}
