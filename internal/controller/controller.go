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

package controller

import (
	"burlo/v2/internal/config"
	"burlo/v2/internal/controller/pictrl"
	"burlo/v2/internal/controller/pumpctrl"
	"burlo/v2/internal/controller/pumpctrl/lwtctrl"
	"burlo/v2/internal/dx2w"
	"burlo/v2/internal/events"
	"burlo/v2/internal/phidgets"
	"burlo/v2/pkg/logger"
	"context"
	"math"
)

var (
	// LWT
	maxSupplyTemp float64 = 45 // °C
	minSupplyTemp float64 = 27 // °C

	// home envelope
	heatLossCoefficient float64 = 0.2102 // kW/°C
	heatLossBaseDt      float64 = 4.5    // °C

	// hydronics
	flowRate    float64 = 0.293 // L/s (== Kg/s)
	h2oConst    float64 = 4.186 // kJ/(kg·°C)
	radHeatCoef float64 = 0.395 // kW/°C
)

type Controller struct {
	conf *config.Config
	log  *logger.Logger

	// actuators
	pumpCtrl *pumpctrl.Controller
	lwtCtrl  *lwtctrl.Controller

	// weather
	outdoorTemp float64

	// thermostat
	indoorTemp float64
	setpoint   float64
	modeOn     bool
	stateOn    bool

	// targets & controls
	lwtTarget   float64
	cdutyTarget float64
	correction  float64
	pic         *pictrl.PIController

	// validity flags
	hasWeather    bool
	hasThermostat bool
}

func (s *Controller) GetData() map[string]float64 {
	data := map[string]float64{}
	if s.hasWeather && s.hasThermostat {
		data["indoor_air_temp"] = s.indoorTemp
		data["outdoor_air_temp"] = s.outdoorTemp
		data["setpoint"] = s.setpoint
		data["lwtTarget"] = s.lwtTarget
		data["circ_target_duty"] = s.cdutyTarget
		data["tstat_call"] = 0
		if s.stateOn {
			data["tstat_call"] = 1
		}
	}
	return data
}

func New(conf *config.Config) *Controller {
	pic := pictrl.NewPIController(1.0, 0.01).
		WithOutputLimits(-4, 4).
		WithDeadband(0.2).
		WithDecay(0.98).
		WithAntiWindup(true)

	setCirculatorState := func(state bool) error {
		return phidgets.SetDigitalOutput(conf.Phidgets.HTTPAddr, "circulator", state,
			conf.Phidgets.CirculatorChannel, conf.Phidgets.CirculatorHubPort)
	}

	setTargetLWT := func(tempC float64) error {
		return dx2w.SetHotWaterMinTempC(tempC)
	}

	return &Controller{
		conf:     conf,
		pic:      pic,
		lwtCtrl:  lwtctrl.NewActuator(setTargetLWT),
		pumpCtrl: pumpctrl.NewActuator(setCirculatorState),
		log:      logger.New("Controller"),
	}
}

func (s *Controller) Run(ctx context.Context) {
	s.log.Info("Running...")
	defer s.log.Info("Stopped")

	weatherEvents, _ := s.conf.EventBus.Subscribe(ctx, events.TopicWeather, true)
	thermostatEvents, _ := s.conf.EventBus.Subscribe(ctx, events.TopicThermostat, true)

	// set this low on start to allow us to control the
	// LWT via min hot water temp, then reset to default
	// value when exiting
	dx2w.SetHotWaterDesignTempC(35)
	defer dx2w.SetHotWaterDesignTempC(42)

	for {
		select {
		case ev := <-weatherEvents:
			s.hasWeather = true
			s.handleWeatherEvent(ev.(events.WeatherUpdate))

		case ev := <-thermostatEvents:
			s.hasThermostat = true
			s.handleThermostatEvent(ev.(events.ThermostatUpdate))

		case <-ctx.Done():
			return
		}
	}
}

func (s *Controller) handleWeatherEvent(ev events.WeatherUpdate) {
	s.outdoorTemp = ev.TemperatureC
	s.recalculate()
}

func (s *Controller) handleThermostatEvent(ev events.ThermostatUpdate) {
	s.indoorTemp = ev.TemperatureC
	s.setpoint = ev.SetpointC
	s.modeOn = ev.Mode > 0
	s.stateOn = ev.State > 0
	s.recalculate()
}

// recalculate updates leaving water temp and circulator duty cycle targets
// everytime indoor or outdoor conditions change (ie. temp or setpoint)
func (s *Controller) recalculate() {

	if !(s.hasWeather && s.hasThermostat) {
		s.log.Info("update: waiting for full data: weather=%v thermostat=%v\n", s.hasWeather, s.hasThermostat)
		return
	}

	if s.modeOn {
		// we use setpoint instead of indoor temp to get the expected lwt/duty
		// for steady-state at setpoint. This will slowly push conditions toward
		// setpoint
		s.lwtTarget, s.cdutyTarget = baselineOperatingState(s.outdoorTemp, s.setpoint)

		// apply a small proportional boost to move towards setpoint slightly
		// faster, and a small integral correction to account for changes to
		// system dynamics
		s.correction = s.pic.Update(s.setpoint, s.indoorTemp)
		s.lwtTarget += s.correction

	} else {
		// if the system mode is OFF: don't run the circulator and
		// let the water cool down to room temp (ie. system not
		// maintaining elevated temps when not needed)
		s.cdutyTarget = 0
		s.correction = 0
		s.lwtTarget = 16
	}

	s.lwtCtrl.SetTargetLWT(s.lwtTarget)
	s.pumpCtrl.SetDutyCycle(s.cdutyTarget)

	s.log.Info("update inputs: room=%0.2f°C, setpoint=%0.2f°C, outdoor=%0.2f°C, ModeHeat=%v\n",
		s.indoorTemp, s.setpoint, s.outdoorTemp, s.modeOn)

	s.log.Info("update outputs: LWT=%0.2f°C, Corr=%0.2f°C, Duty=%0.0f%%\n",
		s.lwtTarget, s.correction, s.cdutyTarget)
}

func baselineOperatingState(outdoorTC, indoorTC float64) (float64, float64) {
	Qloss := expectedHeatLoadKW(indoorTC, outdoorTC)

	requiredGains := Qloss
	supplyTC := targetSupplyTemp(indoorTC, requiredGains)

	supplyTC = math.Max(supplyTC, minSupplyTemp)
	supplyTC = math.Min(supplyTC, maxSupplyTemp)

	Qgain := expectedHeatGainKW(indoorTC, supplyTC)
	circDuty := 100 * math.Min(1.0, math.Max(0.0, Qloss/Qgain))

	return supplyTC, circDuty
}

func expectedHeatLoadKW(roomTC, outdoorTC float64) float64 {
	dT := roomTC - outdoorTC

	if dT < 2.5 {
		return 0
	}
	minKW := 0.35
	return math.Max(heatLossCoefficient*(dT-heatLossBaseDt), minKW)
}

func expectedHeatGainKW(roomTC, supplyTC float64) float64 {
	if supplyTC < roomTC {
		return 0
	}

	advCoef := radHeatCoef / (1 - (radHeatCoef / (2 * h2oConst * flowRate)))
	Q := advCoef * (supplyTC - roomTC)

	return Q
}

func targetSupplyTemp(indoorTC, targetQgain float64) float64 {

	// solve for supply temperature
	advCoef := radHeatCoef / (1 - (radHeatCoef / (2 * h2oConst * flowRate)))
	supplyTC := targetQgain/advCoef + indoorTC

	// enforce minimum: supply cannot be below indoor temperature
	if supplyTC < indoorTC {
		supplyTC = indoorTC
	}

	return supplyTC
}
