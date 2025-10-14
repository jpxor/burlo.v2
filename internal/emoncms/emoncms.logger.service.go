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

package emoncms

import (
	"burlo/v2/internal/config"
	"burlo/v2/internal/controller"
	"burlo/v2/internal/dx2w"
	"burlo/v2/pkg/logger"
	"burlo/v2/pkg/service"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type loggerService struct {
	addr       string
	apiKey     string
	interval   time.Duration
	log        *logger.Logger
	dx2wSrv    *dx2w.HistoryService
	controller *controller.Controller
}

func New(controller *controller.Controller, dx2wSrv *dx2w.HistoryService, appConfig *config.Config) service.Runnable {
	return &loggerService{
		addr:     appConfig.DataLogger.EmonCMSAddr,
		apiKey:   appConfig.DataLogger.EmonCMSApiKey,
		interval: time.Duration(appConfig.DataLogger.IntervalSeconds) * time.Second,
		log:      logger.New("DataLogger"),

		dx2wSrv:    dx2wSrv,
		controller: controller,
	}
}

func (c *loggerService) emoncmsInputPost(node string, data map[string]float64) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		c.log.Error("json.Marshal: %v", err)
		return err
	}

	request := fmt.Sprintf("%s/input/post?node=%s&apikey=%s&fulljson=%s",
		c.addr, node, c.apiKey, string(bytes))

	resp, err := http.Get(request)
	if err != nil {
		c.log.Error("http.Get: %v", err)
		return err
	}
	resp.Body.Close()
	return nil
}

var dx2wKeys = []string{
	"BUFFER_FLOW", "BUFFER_TANK_SETPOINT", "BUFFER_TANK_TEMP", "COMPRESSOR_CALL", "HOT_WATER_MIN_TEMP",
	"HP_CIRCULATOR", "HP_ENTERING_WATER_TEMP", "HP_EXITING_WATER_TEMP", "HP_INPUT_KW",
	"HP_OUTPUT_KW", "MIX_WATER_TEMP", "RETURN_WATER_TEMP", "OUTSIDE_AIR_TEMP",
}

func anyAsNumber(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case bool:
		if val {
			return 1, true
		} else {
			return 0, true
		}
	}
	return 0, false
}

func (c *loggerService) filter(keys []string, allData map[string]dx2w.HistoryEntry) map[string]float64 {
	result := make(map[string]float64)
	for _, key := range keys {
		if entry, ok := allData[strings.ToLower(key)]; ok && entry.Value != nil {
			// Safely type-assert
			if val, ok := anyAsNumber(entry.Value); ok {
				result[key] = val
			} else {
				c.log.Error("invalid type for key %q: %T", key, entry.Value)
			}
		} else {
			c.log.Error("missing or invalid data for key %q", key)
		}
	}
	return result
}

func (c *loggerService) getEmoncmsData() map[string]map[string]float64 {
	return map[string]map[string]float64{
		"dx2w":       c.filter(dx2wKeys, c.dx2wSrv.LatestAll()),
		"controller": c.controller.GetData(),
	}
}

func (c *loggerService) tick() {
	for node, nodeData := range c.getEmoncmsData() {
		err := c.emoncmsInputPost(node, nodeData)
		if err != nil {
			c.log.Error("emoncmsInputPost: %v", err)
		}
	}
}

func (c *loggerService) Run(ctx context.Context) {
	c.log.Info("Running...")
	defer c.log.Info("Stopped.")

	tick := time.NewTicker(c.interval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			c.tick()
		}
	}
}
