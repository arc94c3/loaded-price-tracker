package storage

import (
	"encoding/json"
	"os"
	"time"

	"github.com/shopspring/decimal"
)

const maxHistory = 200

type HistoryEntry struct {
	CurrentPrice string            `json:"current_price"`
	Currency     string            `json:"currency"`
	LastChecked  string            `json:"last_checked"`
	History      []PricePoint      `json:"history"`
	ArmedRules   map[string]bool   `json:"armed_rules"`
}

type PricePoint struct {
	Price string `json:"price"`
	At    string `json:"at"`
}

type History map[string]*HistoryEntry

func NowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

func Load(path string) (History, error) {
	h := History{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return h, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return h, nil
	}
	if err := json.Unmarshal(data, &h); err != nil {
		// Tolerate malformed file by starting fresh.
		return History{}, nil
	}
	return h, nil
}

// Save writes the history JSON in deterministic (sorted-key) order. Returns
// true if the file actually changed on disk.
func Save(path string, h History) (bool, error) {
	// Go's json encoder sorts map[string]X keys alphabetically — deterministic.
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == string(data) {
			return false, nil
		}
	}
	return true, os.WriteFile(path, data, 0o644)
}

func GetEntry(h History, productID string) *HistoryEntry {
	return h[productID]
}

func CurrentPrice(e *HistoryEntry) (decimal.Decimal, bool) {
	if e == nil || e.CurrentPrice == "" {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(e.CurrentPrice)
	if err != nil {
		return decimal.Zero, false
	}
	return d, true
}

func ArmedRules(e *HistoryEntry) map[string]bool {
	if e == nil || e.ArmedRules == nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(e.ArmedRules))
	for k, v := range e.ArmedRules {
		out[k] = v
	}
	return out
}

func UpdateEntry(h History, productID string, newPrice decimal.Decimal, currency string, armed map[string]bool) {
	now := NowISO()
	entry := h[productID]
	if entry == nil {
		entry = &HistoryEntry{}
	}
	entry.History = append(entry.History, PricePoint{Price: newPrice.String(), At: now})
	if len(entry.History) > maxHistory {
		entry.History = entry.History[len(entry.History)-maxHistory:]
	}
	entry.CurrentPrice = newPrice.String()
	entry.Currency = currency
	entry.LastChecked = now
	entry.ArmedRules = armed
	if entry.ArmedRules == nil {
		entry.ArmedRules = map[string]bool{}
	}
	h[productID] = entry
}

func TouchLastChecked(h History, productID string) {
	if e := h[productID]; e != nil {
		e.LastChecked = NowISO()
	}
}
