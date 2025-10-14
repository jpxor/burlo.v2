// Copyright (C) 2025 Josh Simonot
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package weather

import (
	"burlo/v2/internal/config"
	"burlo/v2/internal/dx2w"
	"burlo/v2/internal/events"
	"burlo/v2/pkg/eventbus"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"
)

// ----- Types for history and events -----

// Entry is one history point.
type Entry struct {
	Time  time.Time `json:"time"`
	TempC float64   `json:"temp_c"`
}

// Weather publishes this payload on updates.
type WeatherUpdate struct {
	Time  time.Time `json:"time"`
	TempC float64   `json:"temp_c"`
	Note  string    `json:"note,omitempty"`
}

// ----- Weather service -----

type Weather struct {
	eb          *eventbus.Bus
	poll        time.Duration
	threshold   float64 // delta in degC that triggers save+publish
	dx2wService *dx2w.HistoryService

	mu        sync.RWMutex
	history   []Entry
	lastSaved *Entry
}

// New creates a Weather service. poll is how often to poll the device (e.g. 30s).
// threshold is temperature delta in degC that triggers saving and publish; pass 0.33.
func NewLocalDX2W(dx2wService *dx2w.HistoryService, appConf *config.Config) *Weather {
	poll := time.Duration(appConf.Weather.PollIntervalSeconds) * time.Second
	// threshold := appConf.Weather.ThresholdDegC
	threshold := 0.33

	if poll <= 0 {
		poll = 30 * time.Second
	}

	if threshold <= 0 {
		threshold = 0.33
	}

	return &Weather{
		eb:          appConf.EventBus,
		poll:        poll,
		threshold:   threshold,
		dx2wService: dx2wService,
		history:     make([]Entry, 0, 1024),
	}
}

func (w *Weather) Run(ctx context.Context) {
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()

	w.pollOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.pollOnce(ctx)
		}
	}
}

// pollOnce reads temperature, decides whether to save/publish, and maintains history.
func (w *Weather) pollOnce(ctx context.Context) error {
	now := time.Now()

	temp, err := dx2w.GetMedianOutdoorAirTempC(w.dx2wService, 5*time.Minute)
	if err != nil {
		// only expected error is "no data". ignore and let next poll try again.
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// determine if should save
	shouldSave := false
	if w.lastSaved == nil {
		shouldSave = true
	} else {
		delta := temp - w.lastSaved.TempC
		if delta < 0 {
			delta = -delta
		}
		if delta >= w.threshold {
			shouldSave = true
		}
	}

	if shouldSave {
		entry := Entry{Time: now, TempC: temp}
		w.history = append(w.history, entry)
		w.lastSaved = &entry

		// prune history older than 24h
		cutoff := now.Add(-24 * time.Hour)
		idx := sort.Search(len(w.history), func(i int) bool {
			return !w.history[i].Time.Before(cutoff)
		})
		if idx > 0 {
			w.history = append([]Entry(nil), w.history[idx:]...)
		}

		// publish event (non-blocking best-effort)
		w.eb.Publish(events.TopicWeather, events.WeatherUpdate{
			Time:         entry.Time,
			TemperatureC: entry.TempC,
		})
	}

	return nil
}

// ----- HTTP Handler -----

// This service implements http.Handler. It exposes two endpoints:
//  - GET /            -> HTML page containing a simple chart (uses Chart.js from CDN)
//  - GET /api/history -> JSON array of history entries
// You can mount it under any path (e.g. /weather/). If mounted under a prefix, the
// handler will still work because it responds only to the suffixes above.

var htmlPage = `<!doctype html>
<html>
<head>
<meta charset="utf-8" />
<title>Outdoor Temperature (24h)</title>
<style>
body { font-family: system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial; padding: 24px }
.container { max-width: 900px; margin: 0 auto }
.card { border-radius: 8px; padding: 16px; box-shadow: 0 2px 6px rgba(0,0,0,0.08) }
</style>
</head>
<body>
<div class="container">
<h1>Outdoor Temperature (last 24h)</h1>
<div class="card">
<canvas id="chart" width="860" height="300"></canvas>
</div>
<p>Auto-updates every 30s.</p>
</div>

<!-- Chart.js from CDN -->
<script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
<script>
async function fetchData() {
  const res = await fetch('./api/history');
  const a = await res.json();
  return a.map(x => ({t: new Date(x.time), y: x.temp_c}));
}

let chart;
async function render() {
  const data = await fetchData();
  const ctx = document.getElementById('chart').getContext('2d');
  const labels = data.map(d => d.t.toLocaleTimeString());
  const values = data.map(d => d.y);
  if (!chart) {
    chart = new Chart(ctx, {
      type: 'line',
      data: {
        labels,
        datasets: [{ label: '°C', data: values, tension: 0.2 }]
      },
      options: { scales: { x: { display: true }, y: { beginAtZero: false } } }
    });
  } else {
    chart.data.labels = labels;
    chart.data.datasets[0].data = values;
    chart.update();
  }
}

render();
setInterval(render, 30_000);
</script>
</body>
</html>`

// ServeHTTP implements http.Handler. It responds to "/" and "/api/history". If you want
// additional endpoints (csv, svg) you can add them here.
func (w *Weather) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "", "/":
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = rw.Write([]byte(htmlPage))
	case "/api/history":
		w.mu.RLock()
		hist := make([]Entry, len(w.history))
		copy(hist, w.history)
		w.mu.RUnlock()

		rw.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(rw)
		enc.SetIndent("", "  ")
		_ = enc.Encode(hist)
	default:
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("not found"))
	}
}

// ----- Helpers for integration -----

// LastSaved returns the last saved entry (copy) or nil if none.
func (w *Weather) LastSaved() *Entry {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.lastSaved == nil {
		return nil
	}
	c := *w.lastSaved
	return &c
}

// History returns a copy of the current history.
func (w *Weather) History() []Entry {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]Entry, len(w.history))
	copy(out, w.history)
	return out
}

// ManualPoll forces an immediate poll (useful for diagnostics). It runs synchronously.
func (w *Weather) ManualPoll(ctx context.Context) error {
	return w.pollOnce(ctx)
}
