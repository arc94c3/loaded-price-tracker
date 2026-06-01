use anyhow::{Context, Result};
use chrono::Utc;
use rust_decimal::Decimal;
use serde_json::{json, Map, Value};
use std::collections::BTreeMap;
use std::fs;
use std::path::Path;

const MAX_HISTORY: usize = 200;

pub type History = BTreeMap<String, Value>;

pub fn load_history(path: &Path) -> Result<History> {
    if !path.exists() {
        return Ok(BTreeMap::new());
    }
    let raw = fs::read_to_string(path)
        .with_context(|| format!("Reading history from {}", path.display()))?;
    if raw.trim().is_empty() {
        return Ok(BTreeMap::new());
    }
    let value: Value = serde_json::from_str(&raw).unwrap_or(Value::Object(Default::default()));
    let map = value.as_object().cloned().unwrap_or_default();
    Ok(map.into_iter().collect())
}

pub fn save_history(path: &Path, history: &History) -> Result<bool> {
    let mut obj = Map::new();
    for (k, v) in history {
        obj.insert(k.clone(), v.clone());
    }
    let mut text = serde_json::to_string_pretty(&Value::Object(obj))?;
    text.push('\n');
    if path.exists() {
        if let Ok(existing) = fs::read_to_string(path) {
            if existing == text {
                return Ok(false);
            }
        }
    }
    fs::write(path, text).with_context(|| format!("Writing {}", path.display()))?;
    Ok(true)
}

pub fn now_iso() -> String {
    Utc::now().format("%Y-%m-%dT%H:%M:%SZ").to_string()
}

pub fn get_entry<'a>(history: &'a History, product_id: &str) -> Option<&'a Value> {
    history.get(product_id)
}

pub fn current_price(entry: &Value) -> Option<Decimal> {
    let s = entry.get("current_price")?.as_str()?;
    s.parse::<Decimal>().ok()
}

pub fn armed_rules(entry: &Value) -> BTreeMap<String, bool> {
    entry
        .get("armed_rules")
        .and_then(|v| v.as_object())
        .map(|m| {
            m.iter()
                .filter_map(|(k, v)| v.as_bool().map(|b| (k.clone(), b)))
                .collect()
        })
        .unwrap_or_default()
}

pub fn update_entry(
    history: &mut History,
    product_id: &str,
    new_price: Decimal,
    currency: &str,
    armed: &BTreeMap<String, bool>,
) {
    let now = now_iso();
    let existing = history.remove(product_id).unwrap_or_else(|| json!({}));
    let mut obj = existing.as_object().cloned().unwrap_or_default();

    let mut hist = obj
        .get("history")
        .and_then(|v| v.as_array())
        .cloned()
        .unwrap_or_default();
    hist.push(json!({ "price": new_price.to_string(), "at": now }));
    if hist.len() > MAX_HISTORY {
        let start = hist.len() - MAX_HISTORY;
        hist = hist[start..].to_vec();
    }

    obj.insert("current_price".into(), Value::String(new_price.to_string()));
    obj.insert("currency".into(), Value::String(currency.into()));
    obj.insert("last_checked".into(), Value::String(now));
    obj.insert("history".into(), Value::Array(hist));
    let armed_obj: Map<String, Value> = armed
        .iter()
        .map(|(k, v)| (k.clone(), Value::Bool(*v)))
        .collect();
    obj.insert("armed_rules".into(), Value::Object(armed_obj));

    history.insert(product_id.into(), Value::Object(obj));
}

pub fn touch_last_checked(history: &mut History, product_id: &str) {
    if let Some(entry) = history.get_mut(product_id) {
        if let Some(obj) = entry.as_object_mut() {
            obj.insert("last_checked".into(), Value::String(now_iso()));
        }
    }
}
