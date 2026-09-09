package history

import (
	"strings"

	"leeway/internal/model"
)

// Attach writes omitempty deltas onto ok windows/cost. Missing previous
// observations stay absent (never a synthetic 0). seen is last-seen strictly
// before this Output's generatedAt, or the previous watch tick.
func Attach(out *model.Output, seen map[Key]Obs) {
	if out == nil || seen == nil {
		return
	}
	for i := range out.Providers {
		pr := &out.Providers[i]
		for j := range pr.Accounts {
			ar := &pr.Accounts[j]
			if ar.Status != "ok" || ar.Snapshot == nil {
				continue
			}
			sib := siblingPercentJumped(pr.ID, ar.Name, ar.Snapshot.Windows, seen)
			for k := range ar.Snapshot.Windows {
				win := &ar.Snapshot.Windows[k]
				prev, ok := seen[Key{pr.ID, ar.Name, win.ID}]
				if !ok {
					continue
				}
				win.Delta = windowDelta(prev, obsFromWindow(*win), sib)
			}
			c := ar.Snapshot.Cost
			if c == nil || c.Amount == nil || costDuplicatesWindow(*c, ar.Snapshot.Windows) {
				continue
			}
			prev, ok := seen[Key{pr.ID, ar.Name, costWindow}]
			if !ok {
				continue
			}
			cur := Obs{ID: costWindow, Used: cloneF64(c.Amount), Limit: cloneF64(c.Limit), Currency: c.Currency}
			if d := windowDelta(prev, cur, sib); d != nil {
				c.Delta = &model.CostDelta{Amount: d.Used, Reset: d.Reset}
			}
		}
	}
}

func siblingPercentJumped(provider, account string, windows []model.RateWindow, seen map[Key]Obs) bool {
	for _, w := range windows {
		if w.UsedPercent == nil {
			continue
		}
		prev, ok := seen[Key{provider, account, w.ID}]
		if !ok {
			continue
		}
		if resetsAtJumped(prev, obsFromWindow(w)) {
			return true
		}
	}
	return false
}

func windowDelta(prev, curr Obs, siblingPercentJumped bool) *model.WindowDelta {
	if isReset(prev, curr, siblingPercentJumped) {
		return &model.WindowDelta{Reset: true}
	}
	if prev.UsedPercent != nil && curr.UsedPercent != nil {
		d := *curr.UsedPercent - *prev.UsedPercent
		if abs(d) < ppOmit {
			return nil
		}
		return &model.WindowDelta{UsedPercent: &d}
	}
	if prev.Used != nil && curr.Used != nil {
		d := *curr.Used - *prev.Used
		if abs(d) < amtOmit {
			return nil
		}
		return &model.WindowDelta{Used: &d}
	}
	return nil
}

func isReset(prev, curr Obs, siblingPercentJumped bool) bool {
	if resetsAtJumped(prev, curr) {
		return true
	}
	if !remainingBalance(curr) &&
		prev.ResetsAt == nil && curr.ResetsAt == nil &&
		siblingPercentJumped && dropped(prev, curr) {
		return true
	}
	if prev.UsedPercent != nil && curr.UsedPercent != nil {
		if *prev.UsedPercent-*curr.UsedPercent >= 50 {
			return true
		}
	}
	if !remainingBalance(curr) &&
		prev.Used != nil && curr.Used != nil && prev.Limit != nil &&
		*prev.Used > 0 && *curr.Used < usedNearZero {
		return true
	}
	return false
}

func resetsAtJumped(prev, curr Obs) bool {
	return prev.ResetsAt != nil && curr.ResetsAt != nil &&
		curr.ResetsAt.After(prev.ResetsAt.Add(resetSlack))
}

func dropped(prev, curr Obs) bool {
	if prev.UsedPercent != nil && curr.UsedPercent != nil {
		return *curr.UsedPercent < *prev.UsedPercent
	}
	if prev.Used != nil && curr.Used != nil {
		return *curr.Used < *prev.Used
	}
	return false
}

func remainingBalance(o Obs) bool {
	if o.UsedPercent != nil {
		return false
	}
	return o.Used != nil && o.Limit == nil && !strings.HasPrefix(o.ID, dayPrefix)
}

func obsFromWindow(w model.RateWindow) Obs {
	return Obs{
		ID:          w.ID,
		UsedPercent: w.UsedPercent,
		Used:        w.Used,
		Limit:       w.Limit,
		Currency:    w.Currency,
		ResetsAt:    w.ResetsAt,
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
