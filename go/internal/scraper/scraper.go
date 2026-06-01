package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/shopspring/decimal"
)

const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

const (
	requestTimeout = 20 * time.Second
	retryDelay     = 2 * time.Second
)

type PriceResult struct {
	Price    decimal.Decimal
	Currency string
}

func NewClient() *http.Client {
	return &http.Client{Timeout: requestTimeout}
}

func fetch(client *http.Client, url string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		req.Header.Set("Accept-Language", "en-GB,en;q=0.9")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
			time.Sleep(retryDelay)
			continue
		}
		if readErr != nil {
			lastErr = readErr
			time.Sleep(retryDelay)
			continue
		}
		return string(body), nil
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

func ScrapePrice(client *http.Client, url string) (*PriceResult, error) {
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
