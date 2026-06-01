# Cloudflare on loaded.com

loaded.com sits behind **Cloudflare**, which means scraping isn't just a
matter of sending a real-looking `User-Agent`. Cloudflare fingerprints the
**TLS handshake** itself (JA3/JA4) and the HTTP/2 frame ordering, so a stock
Python `requests`, Go `net/http`, or Rust `reqwest` client gets a `403
Forbidden` with `CF-Mitigated: challenge` regardless of how perfect its
headers look.

## How this project handles it

| Implementation | TLS strategy |
|---|---|
| **Python** | [`curl_cffi`](https://github.com/lexiforest/curl_cffi) with `impersonate="chrome"` — embeds a patched libcurl that replays Chrome's exact ClientHello. |
| **Rust** | Shells out to the system `curl` binary (OpenSSL/SChannel handshake passes). `reqwest` is still used for Discord/Telegram webhooks. |
| **Go** | Shells out to the system `curl` binary. `net/http` is still used for Discord/Telegram webhooks. |

Cloudflare's current challenge profile on loaded.com is **permissive** — it
only checks the TLS/HTTP fingerprint and a sensible header set. There is no
JavaScript challenge, no Turnstile widget, no cookie-bound clearance step.
Once you look like a real browser at the connection layer, every page returns
HTTP 200 normally.

## Why we don't use [native-tls / rustls / SChannel]

- **`reqwest` with `rustls-tls`**: 403 (rustls ClientHello is distinctive).
- **`reqwest` with `native-tls`** (SChannel on Windows): 403.
- **Go `net/http` defaults** (crypto/tls): 403.

The system `curl` binary is preinstalled on Windows 10+, macOS, every major
Linux distro, and all GitHub-hosted runners, so taking it as a runtime
dependency for the Rust and Go implementations is essentially free.

## What to do if Cloudflare ever escalates

If loaded.com switches their Cloudflare profile to "interactive challenge" or
Turnstile, these are the escalation options in increasing order of effort:

1. **Add `--http2`/`--tls-max 1.3`** to the `curl` invocation, or switch the
   `impersonate` profile in Python to a newer Chrome (`chrome131`, `chrome133`,
   etc.).
2. **Rust**: switch from shelling out to [`wreq`](https://crates.io/crates/wreq)
   (async, requires Tokio) which natively impersonates browser TLS + HTTP/2.
3. **Go**: switch to
   [`utls`](https://github.com/refraction-networking/utls) with a custom
   `http.RoundTripper`, or to
   [`cycletls`](https://github.com/Danny-Dasilva/CycleTLS).
4. **Last resort**: drive a real headless browser (Playwright / Puppeteer)
   that solves the challenge in JavaScript. Heavy, slow, but uncatchable.

## Verifying it still works

The quickest sanity check is to run the `add` subcommand against the homepage:

```sh
# Python
python -m src.main --no-banner add --url 'https://www.loaded.com/' --name 'probe'

# Rust
cargo run -- add --url 'https://www.loaded.com/' --name 'probe'

# Go
go run ./cmd/loaded-go --no-banner add --url 'https://www.loaded.com/' --name 'probe'
```

All three should print `Parsed current price: GBP…` followed by
`Added 'probe'`. If you see `HTTP 403` or `CF-Mitigated: challenge`,
Cloudflare has tightened up and you need to escalate per the list above.
Don't forget to remove `probe` from `products.json` afterwards.
