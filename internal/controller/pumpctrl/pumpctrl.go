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

package pumpctrl

import (
	"burlo/v2/pkg/logger"
	"context"
	"math"
	"time"
)

// Actuator is a callback interface to switch the pump
type Actuator func(on bool) error

// Controller controls duty-cycle operation of a pump
type Controller struct {
	actuate   Actuator
	percent   float64 // requested run % (0–100)
	currentOn bool

	lastChange time.Time
	cancel     context.CancelFunc
	updateCh   chan float64

	log *logger.Logger
}

// Actuator creates a new duty-cycle pump controller
func NewActuator(actuate Actuator) *Controller {
	return &Controller{
		actuate:  actuate,
		updateCh: make(chan float64, 1),
		log:      logger.New("PumpCtrl"),
	}
}

// SetDutyCycle changes the target duty cycle (0–100)
func (c *Controller) SetDutyCycle(p float64) {
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	select {
	case c.updateCh <- p:
	default:
		<-c.updateCh
		c.updateCh <- p
	}
}

// Run starts the control loop until context is cancelled
func (c *Controller) Run(ctx context.Context) {
	ctx, c.cancel = context.WithCancel(ctx)
	defer c.cancel()

	ticker := time.NewTicker(time.Minute) // check once a minute
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.setPump(false)
			return

		case p := <-c.updateCh:
			c.log.Debug("new duty cycle: %.1f%%", p)
			c.percent = p

		case now := <-ticker.C:
			c.tick(now)
		}
	}
}

// tick evaluates whether pump should be ON at this moment
func (c *Controller) tick(now time.Time) {
	if c.percent <= 0 {
		c.setPump(false)
		return
	}
	if c.percent >= 100 {
		c.setPump(true)
		return
	}

	minOn := 5.0 // minutes
	minOff := 2.0

	runMinutes := math.Max(60.0*c.percent/100.0, minOn)
	c.log.Debug("tick: expected runMinutes: %.1f minutes per hour", runMinutes)

	// distribute cycles evenly in the hour
	cycles := max(int(math.Floor(60.0/(runMinutes+minOff))), 1)
	c.log.Debug("tick: expected cycles: %d per hour", cycles)

	cycleLength := 60.0 / float64(cycles) // minutes
	onLength := runMinutes / float64(cycles)

	// Enforce minimum ON/OFF times
	if onLength < minOn {
		onLength = minOn
		cycleLength = onLength * 60.0 / runMinutes
	}

	offLength := cycleLength - onLength
	if offLength < minOff {
		offLength = minOff
		cycleLength = onLength + offLength
	}

	// Where are we in the current cycle?
	minOfHour := float64(now.Minute()) + float64(now.Second())/60.0
	cyclePos := math.Mod(minOfHour, cycleLength)

	shouldBeOn := cyclePos < onLength
	c.setPump(shouldBeOn)
}

// setPump calls actuator if state changes and respects min ON/OFF times
func (c *Controller) setPump(on bool) {
	if c.currentOn == on {
		return
	}
	// enforce min ON/OFF durations
	elapsed := time.Since(c.lastChange)
	if c.currentOn && elapsed < 5*time.Minute {
		return // still in min ON
	}
	if !c.currentOn && elapsed < 2*time.Minute {
		return // still in min OFF
	}

	c.currentOn = on
	c.lastChange = time.Now()

	if err := c.actuate(on); err != nil {
		c.log.Error("actuator error: %v", err)
	}
}
