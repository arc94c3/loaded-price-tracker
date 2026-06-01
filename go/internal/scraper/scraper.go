package scraper

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/shopspring/decimal"
)

const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// Header set matching a Chrome top-level navigation. loaded.com sits behind
// Cloudflare which fingerprints the TLS handshake (JA3/JA4); Go's crypto/tls
// stack is detected and gets a 403 with CF-Mitigated: challenge regardless of
// headers. We shell out to the system `curl` binary, whose OpenSSL/SChannel
// handshake Cloudflare accepts. curl is preinstalled on Windows 10+, macOS,
// and virtually every Linux distro / CI runner.
var browserHeaders = map[string]string{
	"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9," +
		"image/avif,image/webp,image/apng,*/*;q=0.8",
	"Accept-Language":           "en-GB,en;q=0.9",
	"Sec-Fetch-Site":            "none",
	"Sec-Fetch-Mode":            "navigate",
	"Sec-Fetch-User":            "?1",
	"Sec-Fetch-Dest":            "document",
	"Upgrade-Insecure-Requests": "1",
	"Cache-Control":             "no-cache",
	"Pragma":                    "no-cache",
}

const (
	requestTimeout = 20 * time.Second
	retryDelay     = 2 * time.Second
)

type PriceResult struct {
	Price    decimal.Decimal
	Currency string
}

// HTTPClient is a placeholder kept for API compatibility — fetches go through
// `curl`. It is returned by NewClient so the rest of the codebase doesn't have
// to change shape.
type HTTPClient struct{}

func NewClient() *HTTPClient {
	return &HTTPClient{}
}

// EnsureCurl verifies that the `curl` binary is on PATH. Callers (main) should
// invoke this at startup so users get a clear error rather than a per-product
// failure.
func EnsureCurl() error {
	out, err := exec.Command("curl", "--version").Output()
	if err != nil {
		return fmt.Errorf("`curl` is required to fetch loaded.com (Cloudflare TLS fingerprinting) "+
			"but could not be executed: %w. Install curl and ensure it is on PATH", err)
	}
	if len(out) == 0 {
		return fmt.Errorf("`curl --version` produced no output; install curl and ensure it is on PATH")
	}
	return nil
}

func curlOnce(url string) (string, error) {
	args := []string{
		"--silent", "--show-error", "--location", "--compressed",
		"--max-time", strconv.Itoa(int(requestTimeout.Seconds())),
		"--fail-with-body",
		"--user-agent", UserAgent,
	}
	for k, v := range browserHeaders {
		args = append(args, "-H", fmt.Sprintf("%s: %s", k, v))
	}
	args = append(args, url)
	out, err := exec.Command("curl", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("curl exited with status %d for %s: %s",
				ee.ExitCode(), url, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("failed to spawn curl for %s: %w", url, err)
	}
	return string(out), nil
}

func fetch(_ *HTTPClient, url string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		body, err := curlOnce(url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		time.Sleep(retryDelay)
	}
	return "", fmt.Errorf("failed to fetch %s: %w", url, lastErr)
}

func toDecimal(s string) (decimal.Decimal, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, false
	}
	return d, true
}

func walkJSONLD(v interface{}, out *[]map[string]interface{}) {
	switch x := v.(type) {
	case []interface{}:
		for _, item := range x {
			walkJSONLD(item, out)
		}
	case map[string]interface{}:
		*out = append(*out, x)
		if g, ok := x["@graph"]; ok {
			walkJSONLD(g, out)
		}
		for _, val := range x {
			switch val.(type) {
			case []interface{}, map[string]interface{}:
				walkJSONLD(val, out)
			}
		}
	}
}

func hasType(node map[string]interface{}, target string) bool {
	t, ok := node["@type"]
	if !ok {
		return false
	}
	switch v := t.(type) {
	case string:
		return v == target
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && s == target {
				return true
			}
		}
	}
	return false
}

func extractOffer(o map[string]interface{}) (*PriceResult, bool) {
	var priceStr string
	if p, ok := o["price"]; ok {
		priceStr = fmt.Sprintf("%v", p)
	} else if p, ok := o["lowPrice"]; ok {
		priceStr = fmt.Sprintf("%v", p)
	} else {
		return nil, false
	}
	d, ok := toDecimal(priceStr)
	if !ok {
		return nil, false
	}
	currency := "GBP"
	if c, ok := o["priceCurrency"].(string); ok && c != "" {
		currency = c
	}
	return &PriceResult{Price: d, Currency: currency}, true
}

func parseJSONLD(doc *goquery.Document) *PriceResult {
	var result *PriceResult
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		raw := strings.TrimSpace(s.Text())
		if raw == "" {
			return true
		}
		var data interface{}
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			return true
		}
		var nodes []map[string]interface{}
		walkJSONLD(data, &nodes)
		for _, node := range nodes {
			isProduct := hasType(node, "Product")
			isOffer := hasType(node, "Offer")
			if !isProduct && !isOffer {
				continue
			}
			var candidates []map[string]interface{}
			var src interface{} = node
			if isProduct {
				src = node["offers"]
			}
			switch v := src.(type) {
			case map[string]interface{}:
				candidates = []map[string]interface{}{v}
			case []interface{}:
				for _, item := range v {
					if m, ok := item.(map[string]interface{}); ok {
						candidates = append(candidates, m)
					}
				}
			}
			for _, o := range candidates {
				if r, ok := extractOffer(o); ok {
					result = r
					return false
				}
			}
		}
		return true
	})
	return result
}

var (
	priceRe       = regexp.MustCompile(`([£$€])\s*([0-9]+(?:[.,][0-9]{2})?)`)
	symbolToCurr  = map[string]string{"£": "GBP", "$": "USD", "€": "EUR"}
	fallbackSels  = []string{`[itemprop="price"]`, `.product-price`, `.price`, `[class*="price" i]`}
)

func parseFallback(doc *goquery.Document) *PriceResult {
	var result *PriceResult
	for _, sel := range fallbackSels {
		doc.Find(sel).EachWithBreak(func(_ int, s *goquery.Selection) bool {
			content, _ := s.Attr("content")
			if content == "" {
				content = strings.TrimSpace(s.Text())
			}
			if content == "" {
				return true
			}
			if m := priceRe.FindStringSubmatch(content); m != nil {
				if d, ok := toDecimal(m[2]); ok {
					curr := symbolToCurr[m[1]]
					if curr == "" {
						curr = "GBP"
					}
					result = &PriceResult{Price: d, Currency: curr}
					return false
				}
			}
			if d, ok := toDecimal(content); ok {
				result = &PriceResult{Price: d, Currency: "GBP"}
				return false
			}
			return true
		})
		if result != nil {
			break
		}
	}
	return result
}

func ScrapePrice(client *HTTPClient, url string) (*PriceResult, error) {
	html, err := fetch(client, url)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	if r := parseJSONLD(doc); r != nil {
		return r, nil
	}
	if r := parseFallback(doc); r != nil {
		return r, nil
	}
	return nil, fmt.Errorf("could not parse price from %s", url)
}
