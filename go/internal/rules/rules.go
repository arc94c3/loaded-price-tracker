package rules

import (
	"fmt"

	"github.com/shopspring/decimal"

	"loaded-go/internal/config"
)

func currencySymbol(c string) string {
	switch c {
	case "GBP":
		return "£"
	case "USD":
		return "$"
	case "EUR":
		return "€"
	}
	return ""
}

func fmtMoney(v decimal.Decimal, currency string) string {
	return fmt.Sprintf("%s%s", currencySymbol(currency), v.StringFixed(2))
}

func thresholdKey(target decimal.Decimal) string {
	return fmt.Sprintf("threshold:%s", target.StringFixed(2))
}

// Evaluate returns the list of reason strings that fired and the updated
// armed-state map. `oldPrice` may be nil (first time we see this product).
func Evaluate(
	productRules []config.Rule,
	oldPrice *decimal.Decimal,
	newPrice decimal.Decimal,
	currency string,
	armedPrev map[string]bool,
) ([]string, map[string]bool) {
	armed := map[string]bool{}
	for k, v := range armedPrev {
		armed[k] = v
	}
	var reasons []string

	for _, r := range productRules {
		switch r.Type {
		case "any_change":
			if oldPrice != nil && !newPrice.Equal(*oldPrice) {
				reasons = append(reasons, "price changed")
			}
		case "threshold":
			if r.AtOrBelow == nil {
				continue
			}
			target := decimal.NewFromFloat(*r.AtOrBelow)
			key := thresholdKey(target)
			currentlyBelow := newPrice.LessThanOrEqual(target)
			previouslyBelow := oldPrice != nil && oldPrice.LessThanOrEqual(target)

			if r.OnlyOnce {
				if !currentlyBelow {
					armed[key] = true
				} else {
					isArmed, ok := armed[key]
					if !ok {
						isArmed = true
					}
					if isArmed && !previouslyBelow {
						reasons = append(reasons, fmt.Sprintf("hit target %s", fmtMoney(target, currency)))
						armed[key] = false
					} else if isArmed && oldPrice == nil {
						reasons = append(reasons, fmt.Sprintf("at target %s", fmtMoney(target, currency)))
						armed[key] = false
					}
				}
			} else if currentlyBelow && !previouslyBelow {
				reasons = append(reasons, fmt.Sprintf("hit target %s", fmtMoney(target, currency)))
			} else if currentlyBelow && oldPrice == nil {
				reasons = append(reasons, fmt.Sprintf("at target %s", fmtMoney(target, currency)))
			}
		case "percent_drop":
			if r.MinPercent == nil {
				continue
			}
			minP := decimal.NewFromFloat(*r.MinPercent)
			if oldPrice != nil && oldPrice.GreaterThan(decimal.Zero) && newPrice.LessThan(*oldPrice) {
				dropPct := oldPrice.Sub(newPrice).Div(*oldPrice).Mul(decimal.NewFromInt(100))
				if dropPct.GreaterThanOrEqual(minP) {
					reasons = append(reasons, fmt.Sprintf("-%s%% drop", dropPct.StringFixed(1)))
				}
			}
		}
	}

	seen := map[string]bool{}
	out := reasons[:0]
	for _, r := range reasons {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out, armed
}
