package provider

import (
	"sort"
	"time"
)

// Stat is the accumulated call statistics of one provider/model pair
// (generation spec 4.6: 每次调用后统计次数与费用估算; 阶段 3: 本地调用统计).
type Stat struct {
	ProviderID    string  `json:"providerId"`
	Model         string  `json:"model"`
	CallCount     int     `json:"callCount"`
	EstimatedCost float64 `json:"estimatedCost"` // in the provider's currency
	Currency      string  `json:"currency"`
	LastCallAt    string  `json:"lastCallAt,omitempty"`
}

// Stats is the persisted call statistics ledger.
type Stats struct {
	Items []Stat `json:"items"`
}

// RecordCall accumulates one completed call for provider/model with the given
// estimated cost. Costs accumulate in the provider's currency.
func (s *Stats) RecordCall(providerID, model string, cost float64) {
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range s.Items {
		if s.Items[i].ProviderID == providerID && s.Items[i].Model == model {
			s.Items[i].CallCount++
			s.Items[i].EstimatedCost += cost
			s.Items[i].LastCallAt = now
			return
		}
	}
	s.Items = append(s.Items, Stat{
		ProviderID: providerID, Model: model, CallCount: 1,
		EstimatedCost: cost, Currency: Currency(providerID), LastCallAt: now,
	})
}

// ForProvider returns the stats rows of one provider (never nil).
func (s *Stats) ForProvider(providerID string) []Stat {
	var out []Stat
	for _, st := range s.Items {
		if st.ProviderID == providerID {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

// TotalCalls returns the total number of recorded calls.
func (s *Stats) TotalCalls() int {
	n := 0
	for _, st := range s.Items {
		n += st.CallCount
	}
	return n
}
