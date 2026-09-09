// Package history records successful usage probes and attaches per-window
// deltas against the last observation of the same (provider, account, window).
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"leeway/internal/auth"
	"leeway/internal/model"
)

const (
	fileName     = "history.jsonl"
	maxRecords   = 200
	recordVer    = 1
	costWindow   = "cost"
	dayPrefix    = "day-"
	ppOmit       = 0.05
	amtOmit      = 0.005
	usedNearZero = 0.005
	resetSlack   = 5 * time.Minute
)

// Key identifies one window across runs.
type Key struct {
	Provider string
	Account  string
	Window   string
}

// Obs is one numeric observation of a window (no secrets, no titles).
type Obs struct {
	ID          string
	UsedPercent *float64
	Used        *float64
	Limit       *float64
	Currency    string
	ResetsAt    *time.Time
}

// Meta describes the usage invocation that produced an Output.
type Meta struct {
	Watch          bool
	ProviderFilter string
	AccountFilter  string
	Skip           bool
}

// Point is one sample in a history series.
type Point struct {
	T           time.Time `json:"t"`
	UsedPercent *float64  `json:"usedPercent,omitempty"`
	Used        *float64  `json:"used,omitempty"`
	Limit       *float64  `json:"limit,omitempty"`
	Currency    string    `json:"currency,omitempty"`
	Reset       bool      `json:"reset,omitempty"`
}

// Series is last-N observations of one window, oldest first.
type Series struct {
	Provider string  `json:"provider"`
	Account  string  `json:"account"`
	Window   string  `json:"window"`
	Points   []Point `json:"points"`
}

type fileRecord struct {
	V              int         `json:"v"`
	TS             time.Time   `json:"ts"`
	GeneratedAt    time.Time   `json:"generatedAt"`
	Watch          bool        `json:"watch,omitempty"`
	ProviderFilter string      `json:"providerFilter,omitempty"`
	AccountFilter  string      `json:"accountFilter,omitempty"`
	Entries        []fileEntry `json:"entries"`
}

type fileEntry struct {
	Provider string       `json:"provider"`
	Account  string       `json:"account"`
	Windows  []fileWindow `json:"windows"`
}

type fileWindow struct {
	ID          string     `json:"id"`
	UsedPercent *float64   `json:"usedPercent,omitempty"`
	Used        *float64   `json:"used,omitempty"`
	Limit       *float64   `json:"limit,omitempty"`
	Currency    string     `json:"currency,omitempty"`
	ResetsAt    *time.Time `json:"resetsAt,omitempty"`
}

// Path is LEEWAY_CACHE/history.jsonl.
func Path() (string, error) {
	dir, err := auth.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// LastSeenFromOutput copies numeric window values out of out. It ignores Delta.
func LastSeenFromOutput(out *model.Output) map[Key]Obs {
	seen := map[Key]Obs{}
	if out == nil {
		return seen
	}
	for _, rec := range projectEntries(out) {
		for _, w := range rec.Windows {
			seen[Key{rec.Provider, rec.Account, w.ID}] = obsFromFileWindow(w)
		}
	}
	return seen
}

// LoadLastSeenBefore returns the last ok observation of each key with
// generatedAt strictly before t. Missing file is an empty map, not an error.
func LoadLastSeenBefore(t time.Time) (map[Key]Obs, error) {
	recs, err := loadRecords()
	if err != nil {
		return nil, err
	}
	seen := map[Key]Obs{}
	for _, rec := range recs {
		if !rec.GeneratedAt.Before(t) {
			continue
		}
		for _, e := range rec.Entries {
			for _, w := range e.Windows {
				seen[Key{e.Provider, e.Account, w.ID}] = obsFromFileWindow(w)
			}
		}
	}
	return seen, nil
}

// Record appends a whitelist projection of ok accounts. Cache replays (same
// generatedAt already on disk) and Meta.Skip are no-ops. Write failures are
// returned for the caller to log; they must not fail the probe.
func Record(out *model.Output, meta Meta) error {
	if meta.Skip || out == nil {
		return nil
	}
	rec := project(out, meta)
	if rec == nil {
		return nil
	}
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return withLock(f, func() error {
		recs, err := readRecords(f)
		if err != nil {
			return err
		}
		for _, old := range recs {
			if old.GeneratedAt.Equal(out.GeneratedAt) {
				return nil
			}
		}
		recs = append(recs, *rec)
		if len(recs) > maxRecords {
			recs = recs[len(recs)-maxRecords:]
		}
		return rewrite(f, recs)
	})
}

// Collect returns last n points per window, oldest first. n<=0 means all
// retained samples. Filters match provider id and account tag exactly.
func Collect(n int, providerFilter, accountFilter string) ([]Series, error) {
	recs, err := loadRecords()
	if err != nil {
		return nil, err
	}
	type acc struct {
		points []Point
		prev   Obs
		has    bool
	}
	grouped := map[Key]*acc{}
	var keys []Key
	for _, rec := range recs {
		for _, e := range rec.Entries {
			if providerFilter != "" && e.Provider != providerFilter {
				continue
			}
			if accountFilter != "" && e.Account != accountFilter {
				continue
			}
			sib := false
			for _, w := range e.Windows {
				if w.UsedPercent == nil {
					continue
				}
				k := Key{e.Provider, e.Account, w.ID}
				g, ok := grouped[k]
				if !ok || !g.has {
					continue
				}
				if resetsAtJumped(g.prev, obsFromFileWindow(w)) {
					sib = true
					break
				}
			}
			for _, w := range e.Windows {
				k := Key{e.Provider, e.Account, w.ID}
				g, ok := grouped[k]
				if !ok {
					g = &acc{}
					grouped[k] = g
					keys = append(keys, k)
				}
				cur := obsFromFileWindow(w)
				pt := Point{
					T:           rec.GeneratedAt,
					UsedPercent: cloneF64(w.UsedPercent),
					Used:        cloneF64(w.Used),
					Limit:       cloneF64(w.Limit),
					Currency:    w.Currency,
				}
				if g.has {
					pt.Reset = isReset(g.prev, cur, sib)
				}
				g.points = append(g.points, pt)
				g.prev = cur
				g.has = true
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		if a.Account != b.Account {
			return a.Account < b.Account
		}
		return a.Window < b.Window
	})
	out := make([]Series, 0, len(keys))
	for _, k := range keys {
		pts := grouped[k].points
		if n > 0 && len(pts) > n {
			pts = pts[len(pts)-n:]
			if len(pts) > 0 {
				pts[0].Reset = false
			}
		}
		if len(pts) == 0 {
			continue
		}
		out = append(out, Series{Provider: k.Provider, Account: k.Account, Window: k.Window, Points: pts})
	}
	return out, nil
}

func loadRecords() ([]fileRecord, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readRecords(f)
}

func readRecords(f *os.File) ([]fileRecord, error) {
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	var recs []fileRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec fileRecord
		if err := json.Unmarshal(line, &rec); err != nil || rec.V != recordVer {
			continue
		}
		recs = append(recs, rec)
	}
	return recs, sc.Err()
}

func rewrite(f *os.File, recs []fileRecord) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, rec := range recs {
		if err := enc.Encode(rec); err != nil {
			return err
		}
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	_, err := f.Write(buf.Bytes())
	return err
}

func obsFromFileWindow(w fileWindow) Obs {
	return Obs{
		ID:          w.ID,
		UsedPercent: cloneF64(w.UsedPercent),
		Used:        cloneF64(w.Used),
		Limit:       cloneF64(w.Limit),
		Currency:    w.Currency,
		ResetsAt:    cloneTime(w.ResetsAt),
	}
}

func cloneF64(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneTime(p *time.Time) *time.Time {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func project(out *model.Output, meta Meta) *fileRecord {
	entries := projectEntries(out)
	if len(entries) == 0 {
		return nil
	}
	return &fileRecord{
		V:              recordVer,
		TS:             time.Now().UTC(),
		GeneratedAt:    out.GeneratedAt,
		Watch:          meta.Watch,
		ProviderFilter: meta.ProviderFilter,
		AccountFilter:  meta.AccountFilter,
		Entries:        entries,
	}
}

func projectEntries(out *model.Output) []fileEntry {
	if out == nil {
		return nil
	}
	var entries []fileEntry
	for _, pr := range out.Providers {
		for _, ar := range pr.Accounts {
			if ar.Status != "ok" || ar.Snapshot == nil {
				continue
			}
			var wins []fileWindow
			for _, w := range ar.Snapshot.Windows {
				fw := fileWindow{
					ID:          w.ID,
					UsedPercent: cloneF64(w.UsedPercent),
					Used:        cloneF64(w.Used),
					Limit:       cloneF64(w.Limit),
					Currency:    w.Currency,
					ResetsAt:    cloneTime(w.ResetsAt),
				}
				if fw.UsedPercent == nil && fw.Used == nil && fw.ResetsAt == nil {
					continue
				}
				wins = append(wins, fw)
			}
			if c := ar.Snapshot.Cost; c != nil && c.Amount != nil && !costDuplicatesWindow(*c, ar.Snapshot.Windows) {
				wins = append(wins, fileWindow{
					ID:       costWindow,
					Used:     cloneF64(c.Amount),
					Limit:    cloneF64(c.Limit),
					Currency: c.Currency,
				})
			}
			if len(wins) == 0 {
				continue
			}
			entries = append(entries, fileEntry{Provider: pr.ID, Account: ar.Name, Windows: wins})
		}
	}
	return entries
}

func costDuplicatesWindow(c model.Cost, windows []model.RateWindow) bool {
	if c.Amount == nil {
		return false
	}
	for _, win := range windows {
		if win.Used != nil && win.Title == c.Period &&
			win.Currency == c.Currency && *win.Used == *c.Amount {
			return true
		}
	}
	return false
}
