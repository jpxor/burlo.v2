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

package dx2w

import (
	"burlo/v2/internal/config"
	"burlo/v2/pkg/logger"
	"burlo/v2/pkg/modbus"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var ModbusClient *modbus.Client

const snapshotFilename = "dx2w_history.json.gz"

type HistoryEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Value     any       `json:"value,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type HistoryService struct {
	client       *modbus.Client
	history      map[string][]HistoryEntry
	mu           sync.RWMutex
	registers    map[string]modbus.RegisterDef
	log          *logger.Logger
	ctx          context.Context
	modbusConfig *modbus.Config
	snapshotFile string
	rootDir      string
}

func New(modbusConfig *modbus.Config, appConfig *config.Config) *HistoryService {
	sync.OnceFunc(func() {
		ModbusClient = modbus.NewClient(context.Background(), modbusConfig)
	})()

	s := &HistoryService{
		client:       ModbusClient,
		modbusConfig: modbusConfig,
		history:      make(map[string][]HistoryEntry),
		registers:    modbusConfig.Registers,
		log:          logger.New("DX2WModbus"),
		snapshotFile: filepath.Join(appConfig.DataDir, snapshotFilename),
		rootDir:      appConfig.RootDir,
	}

	s.loadFromDisk()
	return s
}

func (s *HistoryService) Run(ctx context.Context) {
	s.log.Info("Running...")
	s.ctx = ctx

	// Group registers by poll group
	grouped := make(map[string][]string)
	for name, reg := range s.registers {
		group := reg.Group
		if group == "" {
			group = "default"
		}
		grouped[group] = append(grouped[group], name)
	}

	// Get group intervals
	groupIntervals := s.modbusConfig.PollGroups
	if len(groupIntervals) == 0 {
		groupIntervals = map[string]int{"default": 660} // fallback
	}

	var wg sync.WaitGroup
	for group, names := range grouped {
		intervalSec, ok := groupIntervals[group]
		if !ok {
			intervalSec = groupIntervals["default"]
		}
		wg.Add(1)
		go func(group string, names []string, interval time.Duration) {
			defer wg.Done()
			s.runGroupPoller(ctx, group, names, interval)
		}(group, names, time.Duration(intervalSec)*time.Second)
	}

	// snapshot save loop
	snapshotTicker := time.NewTicker(15 * time.Minute)
	defer snapshotTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.saveToDisk()
			s.log.Info("Stopped")
			wg.Wait()
			return
		case <-snapshotTicker.C:
			s.saveToDisk()
		}
	}
}

func (s *HistoryService) runGroupPoller(ctx context.Context, group string, names []string, interval time.Duration) {
	s.log.Info("Starting group %q poller (every %v)", group, interval)
	s.pollRegisters(names)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("Stopping group %q poller", group)
			return
		case <-ticker.C:
			start := time.Now()
			s.pollRegisters(names)
			s.log.Debug("group %q: polled %d registers, finished in %v", group, len(names), time.Since(start))
		}
	}
}

func (s *HistoryService) pollRegisters(names []string) {
	for _, name := range names {
		entry := HistoryEntry{Timestamp: time.Now()}
		val, err := s.client.ReadValue(name)
		s.log.Debug("ReadValue %s : %+v | %+v", name, val, err)

		s.mu.Lock()

		if err != nil {
			err = invalidValueErrorDetection(name, val, s.history)
			s.log.Error("Invalid value detected: %s (%v): %v", name, val, err)
		}

		if err != nil {
			// Copy previous history slice (may be empty)
			var prevValue any
			if len(s.history[name]) > 0 {
				prevValue = s.history[name][len(s.history[name])-1].Value
			}
			entry.Error = err.Error()
			entry.Value = prevValue
		} else {
			entry.Value = val
		}

		s.history[name] = append(s.history[name], entry)

		// Trim history older than 24h
		cutoff := time.Now().Add(-24 * time.Hour)
		entries := s.history[name]
		idx := 0
		for i, e := range entries {
			if e.Timestamp.After(cutoff) {
				idx = i
				break
			}
		}
		s.history[name] = entries[idx:]
		s.mu.Unlock()

		select {
		case <-s.ctx.Done():
			return
		default:
			continue
		}
	}
}

func (s *HistoryService) saveToDisk() {
	s.mu.RLock()
	copyMap := make(map[string][]HistoryEntry, len(s.history))
	totalEntries := 0
	entriesPerRegister := 0
	for k, v := range s.history {
		copyMap[k] = append([]HistoryEntry(nil), v...)
		totalEntries += len(v)

		if entriesPerRegister == 0 {
			entriesPerRegister = len(v)
		}
	}
	s.mu.RUnlock()

	// Marshal to JSON in memory to get uncompressed size
	jsonData, err := json.Marshal(copyMap)
	if err != nil {
		s.log.Error("failed to marshal snapshot to JSON: %v", err)
		return
	}

	tmpPath := s.snapshotFile + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		s.log.Error("failed to create temp snapshot file: %v", err)
		return
	}
	defer file.Close()

	gz := gzip.NewWriter(file)
	enc := json.NewEncoder(gz)
	enc.SetIndent("", "  ")
	if err := enc.Encode(copyMap); err != nil {
		s.log.Error("failed to encode snapshot: %v", err)
		gz.Close()
		return
	}
	if err := gz.Close(); err != nil {
		s.log.Error("failed to close gzip: %v", err)
		return
	}
	if err := file.Sync(); err != nil {
		s.log.Error("failed to fsync snapshot: %v", err)
	}
	file.Close()
	if err := os.Rename(tmpPath, s.snapshotFile); err != nil {
		s.log.Error("failed to rename snapshot file: %v", err)
		return
	}
	s.log.Debug("snapshot saved: %d total entries (%d bytes to disk)", entriesPerRegister, len(jsonData))
}

func (s *HistoryService) loadFromDisk() {
	path := filepath.Clean(s.snapshotFile)
	file, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			s.log.Error("failed to open history snapshot: %v", err)
		}
		return
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		s.log.Error("failed to open gzip: %v", err)
		return
	}
	defer gz.Close()

	var data map[string][]HistoryEntry
	if err := json.NewDecoder(gz).Decode(&data); err != nil {
		s.log.Error("failed to decode snapshot: %v", err)
		return
	}

	s.mu.Lock()
	s.history = data
	s.mu.Unlock()
	s.log.Info("history restored from snapshot (%d registers)", len(data))
}

func (s *HistoryService) ListAll(name string) []HistoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]HistoryEntry(nil), s.history[name]...)
}

func (s *HistoryService) LatestAll() map[string]HistoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	latest := make(map[string]HistoryEntry)
	for name, entries := range s.history {
		if len(entries) > 0 {
			latest[name] = entries[len(entries)-1]
		}
	}
	return latest
}

func (s *HistoryService) Mean(name string, interval time.Duration) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.history[name]
	if len(entries) == 0 {
		return 0, fmt.Errorf("no history for register %q", name)
	}

	cutoff := time.Now().Add(-interval)
	sum := 0.0
	count := 0
	for _, e := range entries {
		if e.Timestamp.Before(cutoff) || e.Error != "" || e.Value == nil {
			continue
		}
		val, ok := toFloat64(e.Value)
		if !ok {
			continue
		}
		sum += val
		count++
	}
	if count == 0 {
		return 0, fmt.Errorf("no numeric values for register %q in the last %s", name, interval)
	}
	return sum / float64(count), nil
}

func (s *HistoryService) PercentOn(name string, interval time.Duration) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.history[name]
	if len(entries) == 0 {
		return 0, fmt.Errorf("no history for register %q", name)
	}

	cutoff := time.Now().Add(-interval)
	total := 0
	onCount := 0
	for _, e := range entries {
		if e.Timestamp.Before(cutoff) || e.Error != "" || e.Value == nil {
			continue
		}
		total++
		switch v := e.Value.(type) {
		case bool:
			if v {
				onCount++
			}
		case int, int16, uint16, float32, float64:
			num, ok := toFloat64(v)
			if ok && num != 0 {
				onCount++
			}
		default:
			total--
		}
	}

	if total <= 0 {
		return 0, fmt.Errorf("no valid entries for register %q in the last %s", name, interval)
	}
	return (float64(onCount) / float64(total)) * 100, nil
}

func (s *HistoryService) Median(name string, interval time.Duration) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.history[name]
	if len(entries) == 0 {
		return 0, fmt.Errorf("no history for register %q", name)
	}

	cutoff := time.Now().Add(-interval)
	var nums []float64
	for _, e := range entries {
		if e.Timestamp.Before(cutoff) || e.Error != "" || e.Value == nil {
			continue
		}
		val, ok := toFloat64(e.Value)
		if !ok {
			continue
		}
		nums = append(nums, val)
	}

	if len(nums) == 0 {
		return 0, fmt.Errorf("no numeric values for register %q in the last %s", name, interval)
	}

	sort.Float64s(nums)
	mid := len(nums) / 2
	if len(nums)%2 == 0 {
		return (nums[mid-1] + nums[mid]) / 2, nil
	}
	return nums[mid], nil
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int16:
		return float64(val), true
	case uint16:
		return float64(val), true
	default:
		return 0, false
	}
}
