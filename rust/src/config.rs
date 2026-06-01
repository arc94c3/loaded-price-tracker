use anyhow::{anyhow, bail, Context, Result};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashSet;
use std::fs;
use std::path::Path;
use url::Url;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum Rule {
    AnyChange,
    Threshold {
        at_or_below: f64,
        #[serde(default)]
        only_once: bool,
    },
    PercentDrop {
        min_percent: f64,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Product {
    pub id: String,
    pub name: String,
    pub url: String,
    #[serde(default)]
    pub tags: Vec<String>,
    #[serde(default = "default_notify")]
    pub notify: Vec<Rule>,
}

fn default_notify() -> Vec<Rule> {
    vec![Rule::AnyChange]
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Config {
    #[serde(default)]
    pub products: Vec<Product>,
}

pub fn load_config(path: &Path) -> Result<Vec<Product>> {
    let raw = fs::read_to_string(path)
        .with_context(|| format!("Reading config from {}", path.display()))?;
    let cfg: Config = serde_json::from_str(&raw).context("Parsing products.json")?;
    validate(&cfg.products)?;
    Ok(cfg.products)
}

pub fn save_config(path: &Path, products: &[Product]) -> Result<()> {
    let cfg = Config { products: products.to_vec() };
    let mut text = serde_json::to_string_pretty(&cfg)?;
    text.push('\n');
    fs::write(path, text).with_context(|| format!("Writing {}", path.display()))?;
    Ok(())
}

fn validate(products: &[Product]) -> Result<()> {
    let mut seen = HashSet::new();
    for p in products {
        if p.id.is_empty() || p.name.is_empty() || p.url.is_empty() {
            bail!("Product missing required field: {:?}", p);
        }
        let parsed = Url::parse(&p.url).map_err(|e| anyhow!("{}: invalid URL: {e}", p.id))?;
        if !matches!(parsed.scheme(), "http" | "https") {
            bail!("{}: URL must be http(s)", p.id);
        }
        if !seen.insert(&p.id) {
            bail!("Duplicate product id: {}", p.id);
        }
        if p.notify.is_empty() {
            bail!("{}: 'notify' must be non-empty (omit field for default)", p.id);
        }
        for rule in &p.notify {
            match rule {
                Rule::Threshold { at_or_below, .. } if !at_or_below.is_finite() => {
                    bail!("{}: threshold 'at_or_below' must be finite", p.id);
                }
                Rule::PercentDrop { min_percent } if !min_percent.is_finite() => {
                    bail!("{}: percent_drop 'min_percent' must be finite", p.id);
                }
                _ => {}
            }
        }
    }
    Ok(())
}

pub fn slugify(name: &str) -> String {
    let mut out = String::with_capacity(name.len());
    let mut prev_dash = false;
    for ch in name.chars() {
        if ch.is_ascii_alphanumeric() {
            out.push(ch.to_ascii_lowercase());
            prev_dash = false;
        } else if !prev_dash && !out.is_empty() {
            out.push('-');
            prev_dash = true;
        }
    }
    let trimmed = out.trim_matches('-').to_string();
    if trimmed.is_empty() { "product".into() } else { trimmed }
}

/// Pretty-print products.json identically to the Python writer for clean diffs.
pub fn pretty_value(products: &[Product]) -> Value {
    serde_json::to_value(Config { products: products.to_vec() }).expect("serialize")
}
