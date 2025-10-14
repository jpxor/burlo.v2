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

package events

import (
	"burlo/v2/pkg/eventbus"
	"time"
)

var (
	TopicWeather    eventbus.Topic = "weather"
	TopicThermostat eventbus.Topic = "thermostat"
)

type WeatherUpdate struct {
	TemperatureC    float64
	TemperatureNext float64
	Humidity        float64
	Precipitation   float64
	Time            time.Time
}

type ThermostatUpdate struct {
	TemperatureC float64
	SetpointC    float64
	Humidity     float64
	Mode         int
	State        int
}
