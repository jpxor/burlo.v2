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

package lwtctrl

import (
	"burlo/v2/pkg/logger"
	"time"
)

// Actuator is a callback interface to switch the pump
type Actuator func(tempC float64) error

type Controller struct {
	actuator Actuator
	log      *logger.Logger
}

func NewActuator(actuator Actuator) *Controller {
	return &Controller{
		actuator: actuator,
		log:      logger.New("LWTControl"),
	}
}

func (c *Controller) SetTargetLWT(tempC float64) {
	const maxRetries = 3
	const retryDelay = 500 * time.Millisecond

	for i := range maxRetries {
		if err := c.actuator(tempC); err != nil {
			c.log.Error("attempt %d/%d: %v", i+1, maxRetries, err)
			time.Sleep(retryDelay)
			continue
		}
		// success
		return
	}

	// all retries failed
	c.log.Error("write failed after %d attempts", maxRetries)
}
