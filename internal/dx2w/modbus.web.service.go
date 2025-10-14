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
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// Value represents the current register value for display
type Value struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Value       any    `json:"value"`
	Error       string `json:"error,omitempty"`
	Writable    bool   `json:"writable"`
}

// GetHandler returns an http.Handler for the history service.
func (s *HistoryService) NewServeMux() http.Handler {
	mux := http.NewServeMux()

	// serve api
	mux.HandleFunc("/api/values", s.handleAPIValues)
	mux.HandleFunc("/api/write", s.handleAPIWrite)
	mux.HandleFunc("/api/history", s.handleAPIHistory)

	// Serve the www directory
	mux.Handle("/", http.FileServer(http.Dir("internal/dx2w/www")))
	return mux
}

func (s *HistoryService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := sync.OnceValue(s.NewServeMux)()
	mux.ServeHTTP(w, r)
}

// ---------- API Endpoints ----------

// handleAPIValues returns the latest value from all registers.
func (s *HistoryService) handleAPIValues(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	latest := s.LatestAll()
	values := make(map[string]Value)

	for name, entry := range latest {
		regCfg, ok := s.registers[name]
		if !ok {
			continue
		}
		values[name] = Value{
			ID:          name,
			Description: regCfg.Description,
			Value:       entry.Value,
			Error:       entry.Error,
			Writable:    regCfg.Writable,
		}
	}
	if err := json.NewEncoder(w).Encode(values); err != nil {
		s.log.Error("failed to encode latest values: %v", err)
	}
}

// handleAPIHistory returns all history entries for a given register.
func (s *HistoryService) handleAPIHistory(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing 'id' parameter", http.StatusBadRequest)
		s.log.Error("handleAPIHistory called without 'id' parameter")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.ListAll(id)); err != nil {
		s.log.Error("failed to encode history for id %s: %v", id, err)
	}
}

func (s *HistoryService) handleAPIWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		s.log.Error("handleAPIWrite called with invalid method: %s", r.Method)
		return
	}

	var req struct {
		ID    string `json:"id"`
		Value any    `json:"value"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		s.log.Error("failed to read request body: %v", err)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		s.log.Error("invalid request JSON: %v", err)
		return
	}

	if req.Value == nil || req.ID == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		s.log.Error("handleAPIWrite: null or empty ID/value: %+v", req)
		return
	}

	regCfg, ok := s.registers[req.ID]
	if !ok || !regCfg.Writable {
		http.Error(w, "register not writable", http.StatusForbidden)
		s.log.Error("register not writable: %s", req.ID)
		return
	}

	if err := s.client.WriteValue(req.ID, req.Value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		s.log.Error("modbus WriteValue error for %s: %v", req.ID, err)
		return
	}

	// Immediately re-read the same register to verify the change
	go func() {
		// small delay to let device settle (some Modbus slaves need it)
		time.Sleep(300 * time.Millisecond)

		val, err := s.client.ReadValue(req.ID)
		if err != nil {
			s.log.Error("post-write poll failed for %s: %v", req.ID, err)
			return
		}

		entry := HistoryEntry{
			Timestamp: time.Now(),
			Value:     val,
		}

		s.mu.Lock()
		s.history[req.ID] = append(s.history[req.ID], entry)
		s.mu.Unlock()
	}()

	s.log.Info("updated modbus register: %+v", req)
	w.Write(body)
}
