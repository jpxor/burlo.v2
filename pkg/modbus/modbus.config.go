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

package modbus

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Modbus     ModbusConfig           `yaml:"modbus"`
	PollGroups map[string]int         `yaml:"poll_groups"`
	Registers  map[string]RegisterDef `yaml:"registers"`
}

type ModbusConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	SlaveID byte   `yaml:"slave_id"`
	Timeout int    `yaml:"timeout"` // seconds
}

type RegisterDef struct {
	Address     uint16  `yaml:"address"`
	Type        string  `yaml:"type"`      // "holding" // not implemented: "input", "coil", "discrete"
	DataType    string  `yaml:"data_type"` // "uint16", "int16", "bool", "float32" // not implemented: "uint32", "int32",
	Scale       float64 `yaml:"scale"`     // scaling factor (if set, interprets int16 value as scaled float)
	Offset      float64 `yaml:"offset"`    // offset value
	Description string  `yaml:"description"`
	Writable    bool    `yaml:"writable"`
	Group       string  `yaml:"group,omitempty"`
}

func LoadConfig(filename string) *Config {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	return &config
}
