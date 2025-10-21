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

package pictrl

import (
	"burlo/v2/pkg/logger"
	"math"
	"time"
)

type PIController struct {
	Kp, Ki      float64
	intErr      float64
	OutputMin   float64
	OutputMax   float64
	Deadband    float64
	DecayFactor float64 // range [0,1]
	AntiWindup  bool

	log      *logger.Logger
	lastTime time.Time
}

// Update returns the PI output in °C adjustment
func (pi *PIController) Update(setpoint, measurement float64) float64 {
	now := time.Now()
	dt := now.Sub(pi.lastTime).Seconds()

	// Skip integration if called too soon or first run
	if dt <= 0 {
		dt = 0
	}
	if pi.lastTime.IsZero() {
		dt = 0
	}
	pi.lastTime = now

	// Compute error with deadband
	err := setpoint - measurement
	if math.Abs(err) < pi.Deadband {
		pi.log.Debug("within deadband, no error")
		err = 0
	}

	// --- Integral term ---
	if dt > 0 {
		pi.intErr += err * dt
	}

	// Apply time-based decay: decayFactor = fraction that remains after 1 s
	// For example: 0.99 means lose 1% per second.
	if pi.DecayFactor > 0 && pi.DecayFactor < 1.0 && dt > 0 {
		// Convert decayFactor from per-second rate to elapsed-time equivalent
		pi.intErr *= math.Pow(pi.DecayFactor, dt)
	}

	// --- Compute raw output ---
	output := pi.Kp*err + pi.Ki*pi.intErr

	// --- Clamp and optional anti-windup ---
	clamped := false
	if output > pi.OutputMax {
		output = pi.OutputMax
		clamped = true
	} else if output < pi.OutputMin {
		output = pi.OutputMin
		clamped = true
	}

	if clamped && pi.AntiWindup && dt > 0 {
		// Roll back the last integral step to prevent windup
		pi.intErr -= err * dt
	}

	pi.log.Debug("dt=%.2fs, err=%.2f°C, intErr=%.2f, output=%.2f°C", dt, err, pi.intErr, output)
	return output
}

// --- Fluent "With" setters ---

func NewPIController(kp, ki float64) *PIController {
	return &PIController{
		Kp:  kp,
		Ki:  ki,
		log: logger.New("PI Control"),
	}
}

func (pi *PIController) WithOutputLimits(min, max float64) *PIController {
	pi.OutputMin = min
	pi.OutputMax = max
	return pi
}

func (pi *PIController) WithDeadband(db float64) *PIController {
	pi.Deadband = db
	return pi
}

func (pi *PIController) WithDecay(factor float64) *PIController {
	pi.DecayFactor = factor
	return pi
}

func (pi *PIController) WithAntiWindup(enabled bool) *PIController {
	pi.AntiWindup = enabled
	return pi
}
