package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type PriceChange struct {
	ProductName string
	ProductURL  string
	OldPrice    *decimal.Decimal
	NewPrice    decimal.Decimal
	Currency    string
	Reasons     []string
}

func (c *PriceChange) symbol() string {
	switch c.Currency {
	case "GBP":
		return "£"
	case "USD":
		return "$"
	case "EUR":
		return "€"
	}
	return ""
}

func (c *PriceChange) fmtMoney(v decimal.Decimal) string {
	return fmt.Sprintf("%s%s", c.symbol(), v.StringFixed(2))
}

func (c *PriceChange) delta() *decimal.Decimal {
	if c.OldPrice == nil {
		return nil
	}
	d := c.NewPrice.Sub(*c.OldPrice)
	return &d
}

func (c *PriceChange) percent() *decimal.Decimal {
	if c.OldPrice == nil || c.OldPrice.IsZero() {
		return nil
	}
	p := c.NewPrice.Sub(*c.OldPrice).Div(*c.OldPrice).Mul(decimal.NewFromInt(100))
	return &p
}

func (c *PriceChange) arrow() string {
	d := c.delta()
	if d == nil {
		return "🆕"
	}
	if d.IsNegative() {
		return "🔻"
	}
	if d.IsPositive() {
		return "🔺"
	}
	return "•"
}

func (c *PriceChange) Headline() string {
	var base string
	if c.OldPrice == nil {
		base = fmt.Sprintf("%s %s: %s (new)", c.arrow(), c.ProductName, c.fmtMoney(c.NewPrice))
	} else {
		d := c.delta()
		p := c.percent()
		sign := "+"
		if d.IsNegative() {
			sign = "-"
		}
		base = fmt.Sprintf(
			"%s %s: %s → %s (%s%s, %s%s%%)",
			c.arrow(),
			c.ProductName,
			c.fmtMoney(*c.OldPrice),
			c.fmtMoney(c.NewPrice),
			sign,
			c.fmtMoney(d.Abs()),
			sign,
			p.Abs().StringFixed(1),
		)
	}
	if len(c.Reasons) > 0 {
		base += " — " + strings.Join(c.Reasons, ", ")
	}
	return base
}

type Notifier struct {
	Client          *http.Client
	DiscordWebhook  string
	TelegramToken   string
	TelegramChatID  string
	DryRun          bool
}

func (n *Notifier) Notify(c *PriceChange) {
	if n.DryRun {
		fmt.Printf("[dry-run] %s\n", c.Headline())
		return
	}
	if n.DiscordWebhook != "" {
		if err := n.sendDiscord(c); err != nil {
			fmt.Fprintf(stderrPrint(), "Discord notification failed for %s: %v\n", c.ProductName, err)
		}
	}
	if n.TelegramToken != "" && n.TelegramChatID != "" {
		if err := n.sendTelegram(c); err != nil {
			fmt.Fprintf(stderrPrint(), "Telegram notification failed for %s: %v\n", c.ProductName, err)
		}
	}
}

func (n *Notifier) NotifyErrorSummary(failures []string) {
	if len(failures) == 0 || n.DryRun {
		return
	}
	text := fmt.Sprintf("⚠️ Loaded Price Monitor: %d product(s) failed:\n", len(failures))
	for _, f := range failures {
		text += "- " + f + "\n"
	}
	if n.DiscordWebhook != "" {
		_ = n.postJSON(n.DiscordWebhook, map[string]string{"content": text})
	}
	if n.TelegramToken != "" && n.TelegramChatID != "" {
		url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.TelegramToken)
		_ = n.postJSON(url, map[string]string{"chat_id": n.TelegramChatID, "text": text})
	}
}

func (n *Notifier) sendDiscord(c *PriceChange) error {
	color := 0x3498DB
	if d := c.delta(); d != nil {
		if d.IsNegative() {
			color = 0xE74C3C
		} else if d.IsPositive() {
			color = 0xF39C12
		}
	}
	fields := []map[string]interface{}{}
	if c.OldPrice != nil {
		fields = append(fields, map[string]interface{}{
			"name": "Previous", "value": c.fmtMoney(*c.OldPrice), "inline": true,
		})
	}
	fields = append(fields, map[string]interface{}{
		"name": "Current", "value": c.fmtMoney(c.NewPrice), "inline": true,
	})
	if len(c.Reasons) > 0 {
		fields = append(fields, map[string]interface{}{
			"name": "Why", "value": strings.Join(c.Reasons, ", "), "inline": false,
		})
	}
	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{{
			"title":       c.ProductName,
			"url":         c.ProductURL,
			"color":       color,
			"description": c.Headline(),
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
			"fields":      fields,
		}},
	}
	return n.postJSON(n.DiscordWebhook, payload)
}

func mdEscape(s string) string {
	for _, ch := range []string{"_", "*", "[", "]", "`"} {
		s = strings.ReplaceAll(s, ch, "\\"+ch)
	}
	return s
}

func (n *Notifier) sendTelegram(c *PriceChange) error {
	text := fmt.Sprintf("*%s*\n%s\n[View product](%s)",
		mdEscape(c.ProductName), mdEscape(c.Headline()), c.ProductURL)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.TelegramToken)
	return n.postJSON(url, map[string]interface{}{
		"chat_id":                  n.TelegramChatID,
		"text":                     text,
		"parse_mode":               "Markdown",
		"disable_web_page_preview": false,
	})
}

func (n *Notifier) postJSON(url string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// Indirection so tests could swap; avoids importing os here.
func stderrPrint() *stderrWriter { return &stderrWriter{} }

type stderrWriter struct{}

func (*stderrWriter) Write(p []byte) (int, error) { return fmt.Print(string(p)) }
