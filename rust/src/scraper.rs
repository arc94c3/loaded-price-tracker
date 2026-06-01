use anyhow::{anyhow, Context, Result};
use regex::Regex;
use rust_decimal::Decimal;
use scraper::{Html, Selector};
use serde_json::Value;
use std::process::Command;
use std::str::FromStr;
use std::thread;
use std::time::Duration;

pub const USER_AGENT: &str =
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 \
     (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36";

// Header set that matches a real Chrome top-level navigation. loaded.com sits
// behind Cloudflare which fingerprints the TLS handshake (JA3/JA4); reqwest's
// rustls/native-tls handshake gets a 403 with CF-Mitigated: challenge
// regardless of headers. Shelling out to the system `curl` binary uses
// OpenSSL/SChannel-based handshakes that Cloudflare accepts, while keeping the
// dependency surface minimal (curl is preinstalled on Windows 10+, macOS, and
// virtually every Linux distro / CI runner).
const BROWSER_HEADERS: &[(&str, &str)] = &[
    (
        "Accept",
        "text/html,application/xhtml+xml,application/xml;q=0.9,\
         image/avif,image/webp,image/apng,*/*;q=0.8",
    ),
    ("Accept-Language", "en-GB,en;q=0.9"),
    ("Sec-Fetch-Site", "none"),
    ("Sec-Fetch-Mode", "navigate"),
    ("Sec-Fetch-User", "?1"),
    ("Sec-Fetch-Dest", "document"),
    ("Upgrade-Insecure-Requests", "1"),
    ("Cache-Control", "no-cache"),
    ("Pragma", "no-cache"),
];

const REQUEST_TIMEOUT_SECS: u64 = 20;
const RETRY_DELAY: Duration = Duration::from_secs(2);

#[derive(Debug, Clone)]
pub struct PriceResult {
    pub price: Decimal,
    pub currency: String,
}

/// Build the reqwest client used for notifier webhooks. (loaded.com fetches go
/// through `curl` instead — see `fetch`.) We still construct a reqwest client
/// because Discord and Telegram webhooks are not Cloudflare-fingerprinted and
/// work fine with rustls.
pub fn build_client() -> Result<reqwest::blocking::Client> {
    // Probe for curl up-front so we fail with a clear message rather than on
    // the first scrape.
    match Command::new("curl").arg("--version").output() {
        Ok(o) if o.status.success() => {}
        Ok(o) => {
            return Err(anyhow!(
                "`curl` is required to fetch loaded.com (Cloudflare TLS fingerprinting). \
                 `curl --version` exited with status {}",
                o.status
            ))
        }
        Err(e) => {
            return Err(anyhow!(
                "`curl` is required to fetch loaded.com but could not be executed: {e}. \
                 Install curl and ensure it is on PATH."
            ))
        }
    }
    reqwest::blocking::Client::builder()
        .user_agent(USER_AGENT)
        .timeout(Duration::from_secs(REQUEST_TIMEOUT_SECS))
        .build()
        .context("Failed to build HTTP client")
}

fn curl_once(url: &str) -> Result<String> {
    let mut cmd = Command::new("curl");
    cmd.arg("--silent")
        .arg("--show-error")
        .arg("--location")
        .arg("--compressed")
        .arg("--max-time")
        .arg(REQUEST_TIMEOUT_SECS.to_string())
        .arg("--fail-with-body")
        .arg("--user-agent")
        .arg(USER_AGENT);
    for (k, v) in BROWSER_HEADERS {
        cmd.arg("-H").arg(format!("{k}: {v}"));
    }
    cmd.arg(url);
    let out = cmd.output().context("Failed to spawn `curl`")?;
    if !out.status.success() {
        let stderr = String::from_utf8_lossy(&out.stderr);
        return Err(anyhow!(
            "curl exited with status {} for {url}: {}",
            out.status,
            stderr.trim()
        ));
    }
    String::from_utf8(out.stdout).context("curl response was not valid UTF-8")
}

fn fetch(_client: &reqwest::blocking::Client, url: &str) -> Result<String> {
    let mut last_err: Option<anyhow::Error> = None;
    for attempt in 0..2 {
        match curl_once(url) {
            Ok(body) => return Ok(body),
            Err(e) => {
                eprintln!("Attempt {} failed for {}: {}", attempt + 1, url, e);
                last_err = Some(e);
            }
        }
        thread::sleep(RETRY_DELAY);
    }
    Err(last_err.unwrap_or_else(|| anyhow!("Unknown fetch failure for {url}")))
}

fn to_decimal(s: &str) -> Option<Decimal> {
    Decimal::from_str(s.trim().replace(',', "").as_str()).ok()
}

fn walk_jsonld(value: &Value, out: &mut Vec<Value>) {
    match value {
        Value::Array(arr) => {
            for v in arr {
                walk_jsonld(v, out);
            }
        }
        Value::Object(map) => {
            out.push(Value::Object(map.clone()));
            if let Some(graph) = map.get("@graph") {
                walk_jsonld(graph, out);
            }
            for v in map.values() {
                if v.is_array() || v.is_object() {
                    walk_jsonld(v, out);
                }
            }
        }
        _ => {}
    }
}

fn node_has_type(node: &Value, target: &str) -> bool {
    match node.get("@type") {
        Some(Value::String(s)) => s == target,
        Some(Value::Array(arr)) => arr.iter().any(|v| v.as_str() == Some(target)),
        _ => false,
    }
}

fn extract_offer(offer: &Value) -> Option<PriceResult> {
    let price_val = offer.get("price").or_else(|| offer.get("lowPrice"))?;
    let price_str = match price_val {
        Value::String(s) => s.clone(),
        Value::Number(n) => n.to_string(),
        _ => return None,
    };
    let price = to_decimal(&price_str)?;
    let currency = offer
        .get("priceCurrency")
        .and_then(|v| v.as_str())
        .unwrap_or("GBP")
        .to_string();
    Some(PriceResult { price, currency })
}

fn parse_jsonld(html: &str) -> Option<PriceResult> {
    let doc = Html::parse_document(html);
    let sel = Selector::parse(r#"script[type="application/ld+json"]"#).ok()?;
    for script in doc.select(&sel) {
        let raw = script.text().collect::<String>();
        if raw.trim().is_empty() {
            continue;
        }
        let Ok(data) = serde_json::from_str::<Value>(&raw) else { continue };
        let mut nodes = Vec::new();
        walk_jsonld(&data, &mut nodes);
        for node in &nodes {
            let is_product = node_has_type(node, "Product");
            let is_offer = node_has_type(node, "Offer");
            if !is_product && !is_offer {
                continue;
            }
            let candidate = if is_product {
                node.get("offers").cloned().unwrap_or(Value::Null)
            } else {
                node.clone()
            };
            let offers: Vec<Value> = match candidate {
                Value::Array(a) => a,
                Value::Object(_) => vec![candidate],
                _ => continue,
            };
            for o in offers {
                if let Some(r) = extract_offer(&o) {
                    return Some(r);
                }
            }
        }
    }
    None
}

fn parse_fallback(html: &str) -> Option<PriceResult> {
    let doc = Html::parse_document(html);
    let selectors = [
        r#"[itemprop="price"]"#,
        ".product-price",
        ".price",
        r#"[class*="price" i]"#,
    ];
    let price_re = Regex::new(r"([£$€])\s*([0-9]+(?:[.,][0-9]{2})?)").ok()?;
    let symbol_to_currency = |s: &str| match s {
        "£" => "GBP",
        "$" => "USD",
        "€" => "EUR",
        _ => "GBP",
    };
    for sel_str in selectors {
        let Ok(sel) = Selector::parse(sel_str) else { continue };
        for el in doc.select(&sel) {
            let content = el
                .value()
                .attr("content")
                .map(|s| s.to_string())
                .unwrap_or_else(|| el.text().collect::<String>().trim().to_string());
            if content.is_empty() {
                continue;
            }
            if let Some(c) = price_re.captures(&content) {
                if let Some(p) = to_decimal(&c[2]) {
                    return Some(PriceResult {
                        price: p,
                        currency: symbol_to_currency(&c[1]).to_string(),
                    });
                }
            }
            if let Some(p) = to_decimal(&content) {
                return Some(PriceResult { price: p, currency: "GBP".into() });
            }
        }
    }
    None
}

pub fn scrape_price(client: &reqwest::blocking::Client, url: &str) -> Result<PriceResult> {
    let html = fetch(client, url)?;
    parse_jsonld(&html)
        .or_else(|| parse_fallback(&html))
        .ok_or_else(|| anyhow!("Could not parse price from {url}"))
}
