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
	"fmt"
	"time"
)

func SetOutdoorAirDesignTempC(c float64) error {
	return ModbusClient.WriteValue("outdoor_air_design_temp", c_to_f(c))
}

func SetHotWaterDesignTempC(c float64) error {
	if c < 20 || c > 50 {
		return fmt.Errorf("temperature out of range")
	}
	return ModbusClient.WriteValue("hot_water_design_temp", c_to_f(c))
}

func SetHotWaterMinTempC(c float64) error {
	if c < 20 || c > 50 {
		return fmt.Errorf("temperature out of range")
	}
	return ModbusClient.WriteValue("hot_water_min_temp", c_to_f(c))
}

func SetHotWaterDifferentialTempC(c float64) error {
	if c < 1 || c > 20 {
		return fmt.Errorf("temperature out of range")
	}
	return ModbusClient.WriteValue("hot_water_min_temp", c*9.0/5.0)
}

func GetMedianOutdoorAirTempC(s *HistoryService, interval time.Duration) (float64, error) {
	tempF, err := s.Median("outside_air_temp", interval)
	return f_to_c(tempF), err
}

func c_to_f(c float64) float64 {
	return c*(9.0/5.0) + 32.0
}

func f_to_c(f float64) float64 {
	return (f - 32.0) * (5.0 / 9.0)
}
