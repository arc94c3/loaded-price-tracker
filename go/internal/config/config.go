package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type Rule struct {
	Type       string   `json:"type"`
	AtOrBelow  *float64 `json:"at_or_below,omitempty"`
	OnlyOnce   bool     `json:"only_once,omitempty"`
	MinPercent *float64 `json:"min_percent,omitempty"`
}

type Product struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Tags   []string `json:"tags"`
	Notify []Rule   `json:"notify"`
}

type Config struct {
	Products []Product `json:"products"`
}

func LoadConfig(path string) ([]Product, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	for i := range cfg.Products {
		if len(cfg.Products[i].Notify) == 0 {
			cfg.Products[i].Notify = []Rule{{Type: "any_change"}}
		}
		if cfg.Products[i].Tags == nil {
			cfg.Products[i].Tags = []string{}
		}
	}
	if err := validate(cfg.Products); err != nil {
		return nil, err
	}
	return cfg.Products, nil
}

func SaveConfig(path string, products []Product) error {
	for i := range products {
		if products[i].Tags == nil {
			products[i].Tags = []string{}
		}
	}
	data, err := json.MarshalIndent(Config{Products: products}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func validate(products []Product) error {
	seen := map[string]bool{}
	for _, p := range products {
		if p.ID == "" || p.Name == "" || p.URL == "" {
			return fmt.Errorf("product missing required field: %+v", p)
		}
		u, err := url.Parse(p.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%s: invalid URL: %s", p.ID, p.URL)
		}
		if seen[p.ID] {
			return fmt.Errorf("duplicate product id: %s", p.ID)
		}
		seen[p.ID] = true
		for _, r := range p.Notify {
			switch r.Type {
			case "any_change":
			case "threshold":
				if r.AtOrBelow == nil {
					return fmt.Errorf("%s: threshold rule needs 'at_or_below'", p.ID)
				}
			case "percent_drop":
				if r.MinPercent == nil {
					return fmt.Errorf("%s: percent_drop rule needs 'min_percent'", p.ID)
				}
			default:
				return fmt.Errorf("%s: unknown rule type %q", p.ID, r.Type)
			}
		}
	}
	return nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(name string) string {
	s := slugRe.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "product"
	}
	return s
}
