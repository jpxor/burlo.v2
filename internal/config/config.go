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

package config

import (
	"burlo/v2/pkg/eventbus"
	"encoding/json"
	"log"
	"os"
)

type WeatherConfig struct {
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`

	// Poll frequency
	PollIntervalSeconds int `json:"poll_interval_seconds"`
}

type ThermostatConfig struct {
	MaxSetpointC float64 `json:"max_setpoint_c"`
	MinSetpointC float64 `json:"min_setpoint_c"`

	// Add Z-Wave conn info if applicable
	ZWaveAddr     string `json:"zwave_addr"`
	ZWaveDeviceId int    `json:"zwave_deviceId"`
}

type PhidgetsConfig struct {
	HTTPAddr          string `json:"http_addr"`
	CirculatorChannel int    `json:"circulator_channel"`
	CirculatorHubPort int    `json:"circulator_hubport"`
}

type DX2WConfig struct {
	ModbusAddr          string `json:"modbus_addr"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
}

type ControllerConfig struct {
}

type DataLoggerConfig struct {
	EmonCMSAddr     string `json:"emoncms_addr"`
	EmonCMSApiKey   string `json:"emoncms_apikey"`
	IntervalSeconds int    `json:"interval_seconds"`
}

type Config struct {
	Weather    WeatherConfig    `json:"weather"`
	Thermostat ThermostatConfig `json:"thermostat"`
	Phidgets   PhidgetsConfig   `json:"phidgets"`
	DX2W       DX2WConfig       `json:"dx2w"`
	Controller ControllerConfig `json:"controller"`
	DataLogger DataLoggerConfig `json:"datalogger"`

	// not loaded from file, but added here to
	// pass to all services alongside config
	EventBus *eventbus.Bus
}

func LoadFile(path string) *Config {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open config: %v", err)
	}
	defer f.Close()
	var c Config
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		log.Fatalf("decode config: %v", err)
	}
	// apply defaults
	if c.Weather.PollIntervalSeconds == 0 {
		c.Weather.PollIntervalSeconds = 300
	}
	if c.DX2W.PollIntervalSeconds == 0 {
		c.DX2W.PollIntervalSeconds = 15
	}
	if c.DataLogger.IntervalSeconds == 0 {
		c.DataLogger.IntervalSeconds = 60
	}
	if c.Thermostat.MaxSetpointC == 0 {
		c.Thermostat.MaxSetpointC = 32
	}
	if c.Thermostat.MinSetpointC == 0 {
		c.Thermostat.MinSetpointC = 12
	}
	return &c
}
