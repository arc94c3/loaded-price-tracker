package main

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spf13/pflag"

	"loaded-go/internal/banner"
	"loaded-go/internal/config"
	"loaded-go/internal/notifier"
	"loaded-go/internal/rules"
	"loaded-go/internal/scraper"
	"loaded-go/internal/storage"
)

const politeDelay = 1500 * time.Millisecond

func projectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "products.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

func loadEnvFile(dir string) {
	f, err := os.Open(filepath.Join(dir, ".env"))
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if _, set := os.LookupEnv(k); !set {
			_ = os.Setenv(k, v)
		}
	}
}

func parseRule(spec string) (config.Rule, error) {
	parts := strings.Split(spec, ":")
	switch parts[0] {
	case "any_change":
		return config.Rule{Type: "any_change"}, nil
	case "threshold":
		if len(parts) < 2 {
			return config.Rule{}, fmt.Errorf("threshold rule needs a value: %s", spec)
		}
		v, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return config.Rule{}, err
		}
		r := config.Rule{Type: "threshold", AtOrBelow: &v}
		if len(parts) >= 3 && parts[2] == "once" {
			r.OnlyOnce = true
		}
		return r, nil
	case "percent_drop":
		if len(parts) < 2 {
			return config.Rule{}, fmt.Errorf("percent_drop rule needs a value: %s", spec)
		}
		v, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return config.Rule{}, err
		}
		return config.Rule{Type: "percent_drop", MinPercent: &v}, nil
	}
	return config.Rule{}, fmt.Errorf("unknown rule type: %s", parts[0])
}

func cmdWatch(root string, dryRun bool, productFilter string, intervalMin int) int {
	if intervalMin < 1 {
		intervalMin = 1
	}
	d := time.Duration(intervalMin) * time.Minute
	fmt.Printf("Watch mode: checking every %d minute(s). Ctrl-C to stop.\n", intervalMin)
	for {
		_ = cmdCheck(root, dryRun, productFilter)
		fmt.Printf("Next check in %d minute(s).\n", intervalMin)
		time.Sleep(d)
	}
}

func cmdCheck(root string, dryRun bool, productFilter string) int {
	productsPath := filepath.Join(root, "products.json")
	historyPath := filepath.Join(root, "history.json")

	products, err := config.LoadConfig(productsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Config error:", err)
		return 2
	}
	if productFilter != "" {
		filtered := products[:0]
		for _, p := range products {
			if p.ID == productFilter {
				filtered = append(filtered, p)
			}
		}
		products = filtered
		if len(products) == 0 {
			fmt.Fprintf(os.Stderr, "No product with id %q\n", productFilter)
			return 2
		}
	}

	history, err := storage.Load(historyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "History load error:", err)
		return 2
	}

	client := scraper.NewClient()
	if err := scraper.EnsureCurl(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	notif := &notifier.Notifier{
		Client:         &http.Client{Timeout: 20 * time.Second},
		DiscordWebhook: os.Getenv("DISCORD_WEBHOOK_URL"),
		TelegramToken:  os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID: os.Getenv("TELEGRAM_CHAT_ID"),
		DryRun:         dryRun,
	}

	var failures []string
	changes := 0

	for i, p := range products {
		if i > 0 {
			time.Sleep(politeDelay)
		}
		fmt.Printf("Checking %s\n", p.Name)
		result, err := scraper.ScrapePrice(client, p.URL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Scrape failed for %s: %v\n", p.ID, err)
			failures = append(failures, fmt.Sprintf("%s (%s): %v", p.Name, p.ID, err))
			storage.TouchLastChecked(history, p.ID)
			continue
		}

		entry := storage.GetEntry(history, p.ID)
		var oldPrice *decimal.Decimal
		if d, ok := storage.CurrentPrice(entry); ok {
			oldPrice = &d
		}
		armedPrev := storage.ArmedRules(entry)

		reasons, armedNew := rules.Evaluate(p.Notify, oldPrice, result.Price, result.Currency, armedPrev)
		storage.UpdateEntry(history, p.ID, result.Price, result.Currency, armedNew)

		if len(reasons) > 0 {
			changes++
			change := &notifier.PriceChange{
				ProductName: p.Name,
				ProductURL:  p.URL,
				OldPrice:    oldPrice,
				NewPrice:    result.Price,
				Currency:    result.Currency,
				Reasons:     reasons,
			}
			notif.Notify(change)
		} else {
			fmt.Printf("No notification needed for %s (price %s)\n", p.ID, result.Price)
		}
	}

	notif.NotifyErrorSummary(failures)

	if !dryRun {
		wrote, err := storage.Save(historyPath, history)
		if err != nil {
			fmt.Fprintln(os.Stderr, "History save error:", err)
			return 1
		}
		if wrote {
			fmt.Println("History updated")
		} else {
			fmt.Println("History unchanged")
		}
	} else {
		fmt.Println("[dry-run] history not saved")
	}

	fmt.Printf("Done. %d product(s) checked, %d notification(s), %d failure(s).\n",
		len(products), changes, len(failures))
	if len(failures) > 0 {
		return 1
	}
	return 0
}

func cmdAdd(root, productURL, name string, tags []string, ruleSpecs []string) int {
	u, err := url.Parse(productURL)
	if err != nil || !strings.Contains(strings.ToLower(u.Host), "loaded.com") {
		fmt.Fprintf(os.Stderr, "URL must be on loaded.com, got: %s\n", productURL)
		return 2
	}
	productsPath := filepath.Join(root, "products.json")
	products, err := config.LoadConfig(productsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Config error:", err)
		return 2
	}
	for _, p := range products {
		if p.URL == productURL {
			fmt.Fprintln(os.Stderr, "A product with this URL is already tracked.")
			return 2
		}
	}

	fmt.Printf("Test-scraping %s ...\n", productURL)
	if err := scraper.EnsureCurl(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	client := scraper.NewClient()
	result, err := scraper.ScrapePrice(client, productURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not scrape the page:", err)
		return 2
	}
	fmt.Printf("Parsed current price: %s%s\n", result.Currency, result.Price)

	base := config.Slugify(name)
	newID := base
	existing := map[string]bool{}
	for _, p := range products {
		existing[p.ID] = true
	}
	for n := 2; existing[newID]; n++ {
		newID = fmt.Sprintf("%s-%d", base, n)
	}

	var rs []config.Rule
	if len(ruleSpecs) == 0 {
		rs = []config.Rule{{Type: "any_change"}}
	} else {
		for _, s := range ruleSpecs {
			r, err := parseRule(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
			rs = append(rs, r)
		}
	}

	products = append(products, config.Product{
		ID: newID, Name: name, URL: productURL, Tags: tags, Notify: rs,
	})
	if err := config.SaveConfig(productsPath, products); err != nil {
		fmt.Fprintln(os.Stderr, "Save failed:", err)
		return 1
	}
	fmt.Printf("Added %q (id: %s) with %d rule(s).\n", name, newID, len(rs))
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, `loaded-go — Loaded.com price monitor (Go)

Usage:
  loaded-go [--no-banner] check [--dry-run] [--product ID] [--watch] [--interval MIN]
  loaded-go [--no-banner] add --url URL --name NAME [--tags ...] [--rule SPEC ...]

Rule SPECs:
  any_change | threshold:25 | threshold:25:once | percent_drop:15`)
}

func main() {
	root := projectRoot()
	loadEnvFile(root)
	// Also try go/ subdir for .env
	loadEnvFile(filepath.Join(root, "go"))

	args := os.Args[1:]
	noBanner := false
	// Strip --no-banner from any position.
	out := args[:0]
	for _, a := range args {
		if a == "--no-banner" {
			noBanner = true
			continue
		}
		out = append(out, a)
	}
	args = out

	if len(args) == 0 {
		args = []string{"check"}
	}

	if !noBanner {
		banner.Print()
	}

	switch args[0] {
	case "check":
		fs := pflag.NewFlagSet("check", pflag.ExitOnError)
		dryRun := fs.Bool("dry-run", false, "don't send or persist anything")
		product := fs.String("product", "", "check a single product by id")
		watch := fs.Bool("watch", false, "run continuously, repeating every --interval minutes")
		interval := fs.Int("interval", 360, "minutes between checks when --watch is set")
		_ = fs.Parse(args[1:])
		if *watch {
			os.Exit(cmdWatch(root, *dryRun, *product, *interval))
		}
		os.Exit(cmdCheck(root, *dryRun, *product))
	case "add":
		fs := pflag.NewFlagSet("add", pflag.ExitOnError)
		url := fs.String("url", "", "product URL on loaded.com (required)")
		name := fs.String("name", "", "display name (required)")
		tags := fs.StringSlice("tags", nil, "optional tags")
		rule := fs.StringSlice("rule", nil, "notification rule, repeatable")
		_ = fs.Parse(args[1:])
		if *url == "" || *name == "" {
			usage()
			os.Exit(2)
		}
		os.Exit(cmdAdd(root, *url, *name, *tags, *rule))
	case "-h", "--help", "help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
		usage()
		os.Exit(2)
	}
}
