use anyhow::{anyhow, Result};
use clap::{Parser, Subcommand};
use loaded_rs::{banner, config, notifier, rules, scraper, storage};
use std::io::IsTerminal;
use std::path::PathBuf;
use std::thread;
use std::time::Duration;

const POLITE_DELAY: Duration = Duration::from_millis(1500);

#[derive(Parser)]
#[command(name = "loaded-rs", about = "Loaded.com price monitor (Rust)")]
struct Cli {
    /// Suppress ASCII banner
    #[arg(long, global = true)]
    no_banner: bool,

    #[command(subcommand)]
    command: Option<Cmd>,
}

#[derive(Subcommand)]
enum Cmd {
    /// Check all tracked products (default)
    Check {
        /// Don't send notifications or persist history
        #[arg(long)]
        dry_run: bool,
        /// Check only a single product by id
        #[arg(long)]
        product: Option<String>,
    },
    /// Add a product to the tracker
    Add {
        #[arg(long)]
        url: String,
        #[arg(long)]
        name: String,
        #[arg(long, num_args = 0..)]
        tags: Vec<String>,
        /// Repeatable. Forms: any_change | threshold:25 | threshold:25:once | percent_drop:15
        #[arg(long)]
        rule: Vec<String>,
    },
}

fn project_root() -> PathBuf {
    // Walk up from CWD until we find products.json; fall back to CARGO_MANIFEST_DIR's parent.
    if let Ok(mut cwd) = std::env::current_dir() {
        loop {
            if cwd.join("products.json").exists() {
                return cwd;
            }
            if !cwd.pop() {
                break;
            }
        }
    }
    let manifest = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    manifest.parent().unwrap_or(&manifest).to_path_buf()
}

fn load_env_file() {
    let env_path = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join(".env");
    let Ok(content) = std::fs::read_to_string(&env_path) else { return };
    for line in content.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let Some((k, v)) = line.split_once('=') else { continue };
        let k = k.trim();
        let v = v.trim().trim_matches('"').trim_matches('\'');
        if std::env::var_os(k).is_none() {
            std::env::set_var(k, v);
        }
    }
}

fn secrets() -> (Option<String>, Option<String>, Option<String>) {
    (
        std::env::var("DISCORD_WEBHOOK_URL").ok().filter(|s| !s.is_empty()),
        std::env::var("TELEGRAM_BOT_TOKEN").ok().filter(|s| !s.is_empty()),
        std::env::var("TELEGRAM_CHAT_ID").ok().filter(|s| !s.is_empty()),
    )
}

fn parse_rule(spec: &str) -> Result<config::Rule> {
    let parts: Vec<&str> = spec.split(':').collect();
    match parts[0] {
        "any_change" => Ok(config::Rule::AnyChange),
        "threshold" => {
            if parts.len() < 2 {
                return Err(anyhow!("threshold rule needs a value: {spec}"));
            }
            let value: f64 = parts[1].parse()?;
            let only_once = parts.get(2).map(|s| *s == "once").unwrap_or(false);
            Ok(config::Rule::Threshold { at_or_below: value, only_once })
        }
        "percent_drop" => {
            if parts.len() < 2 {
                return Err(anyhow!("percent_drop rule needs a value: {spec}"));
            }
            Ok(config::Rule::PercentDrop { min_percent: parts[1].parse()? })
        }
        other => Err(anyhow!("Unknown rule type: {other}")),
    }
}

fn cmd_check(dry_run: bool, product_filter: Option<String>) -> Result<i32> {
    let root = project_root();
    let products_path = root.join("products.json");
    let history_path = root.join("history.json");

    let mut products = config::load_config(&products_path)?;
    if let Some(id) = &product_filter {
        products.retain(|p| &p.id == id);
        if products.is_empty() {
            return Err(anyhow!("No product with id '{id}'"));
        }
    }

    let mut history = storage::load_history(&history_path)?;
    let client = scraper::build_client()?;
    let (discord, tg_token, tg_chat) = secrets();
    let notif = notifier::Notifier {
        client: &client,
        discord_webhook: discord,
        telegram_token: tg_token,
        telegram_chat_id: tg_chat,
        dry_run,
    };

    let mut failures: Vec<String> = Vec::new();
    let mut changes = 0usize;

    for (i, product) in products.iter().enumerate() {
        if i > 0 {
            thread::sleep(POLITE_DELAY);
        }
        println!("Checking {}", product.name);
        let result = match scraper::scrape_price(&client, &product.url) {
            Ok(r) => r,
            Err(e) => {
                eprintln!("Scrape failed for {}: {e}", product.id);
                failures.push(format!("{} ({}): {e}", product.name, product.id));
                storage::touch_last_checked(&mut history, &product.id);
                continue;
            }
        };

        let old_entry = storage::get_entry(&history, &product.id);
        let old_price = old_entry.and_then(storage::current_price);
        let armed_prev = old_entry.map(storage::armed_rules).unwrap_or_default();

        let (reasons, armed_new) =
            rules::evaluate(&product.notify, old_price, result.price, &result.currency, &armed_prev);

        storage::update_entry(
            &mut history,
            &product.id,
            result.price,
            &result.currency,
            &armed_new,
        );

        if !reasons.is_empty() {
            changes += 1;
            let change = notifier::PriceChange {
                product_name: product.name.clone(),
                product_url: product.url.clone(),
                old_price,
                new_price: result.price,
                currency: result.currency.clone(),
                reasons,
            };
            notif.notify(&change);
        } else {
            println!("No notification needed for {} (price {})", product.id, result.price);
        }
    }

    notif.notify_error_summary(&failures);

    if !dry_run {
        let wrote = storage::save_history(&history_path, &history)?;
        println!("History {}", if wrote { "updated" } else { "unchanged" });
    } else {
        println!("[dry-run] history not saved");
    }

    println!(
        "Done. {} product(s) checked, {} notification(s), {} failure(s).",
        products.len(), changes, failures.len()
    );
    Ok(if failures.is_empty() { 0 } else { 1 })
}

fn cmd_add(url: String, name: String, tags: Vec<String>, rule_specs: Vec<String>) -> Result<i32> {
    let host = url::Url::parse(&url)?.host_str().unwrap_or("").to_lowercase();
    if !host.contains("loaded.com") {
        return Err(anyhow!("URL must be on loaded.com, got: {url}"));
    }

    let root = project_root();
    let products_path = root.join("products.json");
    let mut products = config::load_config(&products_path)?;

    if products.iter().any(|p| p.url == url) {
        return Err(anyhow!("A product with this URL is already tracked."));
    }

    println!("Test-scraping {} ...", url);
    let client = scraper::build_client()?;
    let result = scraper::scrape_price(&client, &url)?;
    println!("Parsed current price: {}{}", result.currency, result.price);

    let base = config::slugify(&name);
    let mut new_id = base.clone();
    let mut n = 2;
    let existing: std::collections::HashSet<_> = products.iter().map(|p| p.id.clone()).collect();
    while existing.contains(&new_id) {
        new_id = format!("{base}-{n}");
        n += 1;
    }

    let rules = if rule_specs.is_empty() {
        vec![config::Rule::AnyChange]
    } else {
        rule_specs.iter().map(|s| parse_rule(s)).collect::<Result<Vec<_>>>()?
    };

    products.push(config::Product {
        id: new_id.clone(),
        name: name.clone(),
        url,
        tags,
        notify: rules.clone(),
    });
    config::save_config(&products_path, &products)?;
    println!("Added '{}' (id: {}) with {} rule(s).", name, new_id, rules.len());
    Ok(0)
}

fn main() -> Result<()> {
    load_env_file();
    let cli = Cli::parse();

    let is_interactive_check = matches!(&cli.command, Some(Cmd::Check { .. }) | None)
        && std::io::stdout().is_terminal();
    let show_banner = !cli.no_banner && (is_interactive_check || !matches!(&cli.command, Some(Cmd::Check { .. }) | None));
    if show_banner {
        banner::print_banner(false);
    }

    let code = match cli.command.unwrap_or(Cmd::Check { dry_run: false, product: None }) {
        Cmd::Check { dry_run, product } => cmd_check(dry_run, product)?,
        Cmd::Add { url, name, tags, rule } => cmd_add(url, name, tags, rule)?,
    };
    std::process::exit(code);
}
