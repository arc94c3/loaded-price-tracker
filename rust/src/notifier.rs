use anyhow::Result;
use chrono::Utc;
use rust_decimal::Decimal;
use rust_decimal_macros::dec;
use serde_json::json;

pub struct PriceChange {
    pub product_name: String,
    pub product_url: String,
    pub old_price: Option<Decimal>,
    pub new_price: Decimal,
    pub currency: String,
    pub reasons: Vec<String>,
}

impl PriceChange {
    fn symbol(&self) -> &str {
        match self.currency.as_str() {
            "GBP" => "£",
            "USD" => "$",
            "EUR" => "€",
            _ => "",
        }
    }

    fn fmt_money(&self, v: Decimal) -> String {
        format!("{}{:.2}", self.symbol(), v)
    }

    fn delta(&self) -> Option<Decimal> {
        self.old_price.map(|o| self.new_price - o)
    }

    fn percent(&self) -> Option<Decimal> {
        let old = self.old_price?;
        if old == dec!(0) {
            return None;
        }
        Some((self.new_price - old) / old * dec!(100))
    }

    fn arrow(&self) -> &str {
        match self.delta() {
            None => "🆕",
            Some(d) if d < dec!(0) => "🔻",
            Some(d) if d > dec!(0) => "🔺",
            _ => "•",
        }
    }

    pub fn headline(&self) -> String {
        let base = if let Some(old) = self.old_price {
            let d = self.delta().unwrap_or(dec!(0));
            let p = self.percent().unwrap_or(dec!(0));
            let sign = if d < dec!(0) { "-" } else { "+" };
            format!(
                "{} {}: {} → {} ({}{}, {}{:.1}%)",
                self.arrow(),
                self.product_name,
                self.fmt_money(old),
                self.fmt_money(self.new_price),
                sign,
                self.fmt_money(d.abs()),
                sign,
                p.abs(),
            )
        } else {
            format!(
                "{} {}: {} (new)",
                self.arrow(),
                self.product_name,
                self.fmt_money(self.new_price)
            )
        };
        if self.reasons.is_empty() {
            base
        } else {
            format!("{} — {}", base, self.reasons.join(", "))
        }
    }
}

fn send_discord(client: &reqwest::blocking::Client, webhook: &str, change: &PriceChange) -> Result<()> {
    let color = match change.delta() {
        Some(d) if d < dec!(0) => 0xE7_4C_3C,
        Some(d) if d > dec!(0) => 0xF3_9C_12,
        _ => 0x34_98_DB,
    };
    let mut fields = vec![];
    if let Some(old) = change.old_price {
        fields.push(json!({ "name": "Previous", "value": change.fmt_money(old), "inline": true }));
    }
    fields.push(json!({ "name": "Current", "value": change.fmt_money(change.new_price), "inline": true }));
    if !change.reasons.is_empty() {
        fields.push(json!({ "name": "Why", "value": change.reasons.join(", "), "inline": false }));
    }
    let payload = json!({
        "embeds": [{
            "title": change.product_name,
            "url": change.product_url,
            "color": color,
            "description": change.headline(),
            "timestamp": Utc::now().to_rfc3339(),
            "fields": fields,
        }]
    });
    client.post(webhook).json(&payload).send()?.error_for_status()?;
    Ok(())
}

fn md_escape(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for ch in s.chars() {
        if matches!(ch, '_' | '*' | '[' | ']' | '`') {
            out.push('\\');
        }
        out.push(ch);
    }
    out
}

fn send_telegram(
    client: &reqwest::blocking::Client,
    token: &str,
    chat_id: &str,
    change: &PriceChange,
) -> Result<()> {
    let text = format!(
        "*{}*\n{}\n[View product]({})",
        md_escape(&change.product_name),
        md_escape(&change.headline()),
        change.product_url
    );
    let url = format!("https://api.telegram.org/bot{}/sendMessage", token);
    client
        .post(url)
        .json(&json!({
            "chat_id": chat_id,
            "text": text,
            "parse_mode": "Markdown",
            "disable_web_page_preview": false,
        }))
        .send()?
        .error_for_status()?;
    Ok(())
}

pub struct Notifier<'a> {
    pub client: &'a reqwest::blocking::Client,
    pub discord_webhook: Option<String>,
    pub telegram_token: Option<String>,
    pub telegram_chat_id: Option<String>,
    pub dry_run: bool,
}

impl<'a> Notifier<'a> {
    pub fn notify(&self, change: &PriceChange) {
        if self.dry_run {
            println!("[dry-run] {}", change.headline());
            return;
        }
        if let Some(webhook) = &self.discord_webhook {
            if let Err(e) = send_discord(self.client, webhook, change) {
                eprintln!("Discord notification failed for {}: {}", change.product_name, e);
            }
        }
        if let (Some(token), Some(chat_id)) = (&self.telegram_token, &self.telegram_chat_id) {
            if let Err(e) = send_telegram(self.client, token, chat_id, change) {
                eprintln!("Telegram notification failed for {}: {}", change.product_name, e);
            }
        }
    }

    pub fn notify_error_summary(&self, failures: &[String]) {
        if failures.is_empty() || self.dry_run {
            return;
        }
        let text = format!(
            "⚠️ Loaded Price Monitor: {} product(s) failed:\n{}",
            failures.len(),
            failures.iter().map(|f| format!("- {}", f)).collect::<Vec<_>>().join("\n")
        );
        if let Some(webhook) = &self.discord_webhook {
            let _ = self
                .client
                .post(webhook)
                .json(&json!({ "content": text }))
                .send()
                .and_then(|r| r.error_for_status());
        }
        if let (Some(token), Some(chat_id)) = (&self.telegram_token, &self.telegram_chat_id) {
            let url = format!("https://api.telegram.org/bot{}/sendMessage", token);
            let _ = self
                .client
                .post(url)
                .json(&json!({ "chat_id": chat_id, "text": text }))
                .send()
                .and_then(|r| r.error_for_status());
        }
    }
}
