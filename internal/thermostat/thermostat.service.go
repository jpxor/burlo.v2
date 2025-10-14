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

package thermostat

import (
	"context"
	"net/http"

	"burlo/v2/internal/config"
	"burlo/v2/internal/events"
	"burlo/v2/pkg/eventbus"
	"burlo/v2/pkg/logger"
	"burlo/v2/pkg/service"
)

type VirtThermostat struct {
	conf        config.ThermostatConfig
	evBus       *eventbus.Bus
	clientQueue chan WebAppRequest

	backend Backend
	data    vtData
	log     *logger.Logger

	httpHandler http.Handler
}

type vtData struct {
	TemperatureC float64
	SetpointC    float64
	Humidity     float64
	Mode         VTMode
	State        VTState
}

type VTMode int
type VTState int

const (
	Mode_OFF          VTMode = 0
	Mode_HEAT         VTMode = 1
	Mode_SETBACK_HEAT VTMode = 11
)

const (
	State_IDLE   VTState = 0
	State_ACTIVE VTState = 1
)

type BackendUpdate struct {
	Property  string
	Value     any
	Broadcast bool
}

type Backend interface {
	service.Runnable
	SetMode(mode VTMode) error
	SetSetpoint(c float64) error
	Updates() <-chan BackendUpdate
}

func NewZWaveThermostat(conf *config.Config) *VirtThermostat {
	log := logger.New("Thermostat")

	backend := NewZWaveBackend(conf, log)
	vt := &VirtThermostat{
		conf:        conf.Thermostat,
		evBus:       conf.EventBus,
		clientQueue: make(chan WebAppRequest, 8),
		backend:     backend,
		log:         log,
	}
	vt.httpHandler = vt.buildHTTPHandler()
	return vt
}

func (vt *VirtThermostat) Run(ctx context.Context) {
	vt.log.Info("starting virtual thermostat")

	go vt.backend.Run(ctx)
	backendUpdates := vt.backend.Updates()

	vt.clientQueue <- WebAppRequest{Command: "broadcast"}

	for {
		select {
		case <-ctx.Done():
			vt.log.Info("stopping virtual thermostat")
			return

		case msg, ok := <-backendUpdates:
			if !ok {
				vt.log.Info("backend updates channel closed")
				return
			}

			switch msg.Property {
			case "temperature":
				vt.data.TemperatureC = msg.Value.(float64)
			case "setpoint":
				vt.data.SetpointC = msg.Value.(float64)
			case "humidity":
				vt.data.Humidity = msg.Value.(float64)
			case "mode":
				vt.data.Mode = msg.Value.(VTMode)
			case "state":
				vt.data.State = msg.Value.(VTState)
			case "none":
				// ignore
			default:
				vt.log.Error("unhandled backend update: %+v", msg)
				continue
			}

			if !msg.Broadcast {
				continue
			}

		case req := <-vt.clientQueue:
			vt.log.Debug("msg from client: %+v", req)
			switch req.Command {
			case "broadcast":
				// just forward
			case "change_setpoint":
				vt.data.SetpointC = vt.deltaSetpoint(req.DeltaC)
				if err := vt.backend.SetSetpoint(vt.data.SetpointC); err != nil {
					vt.log.Error("SetSetpoint failed: %v", err)
				}
			case "toggle_mode":
				vt.data.Mode = vt.data.Mode.toggle()
				if err := vt.backend.SetMode(vt.data.Mode); err != nil {
					vt.log.Error("SetMode failed: %v", err)
				}
			default:
				continue
			}
		}

		state := WebAppState{
			TemperatureC: vt.data.TemperatureC,
			SetpointC:    vt.data.SetpointC,
			Humidity:     vt.data.Humidity,
			Mode:         int(vt.data.Mode),
			State:        int(vt.data.State),
		}

		go webAppBroadcast(state)

		if vt.evBus != nil {
			vt.evBus.Publish(events.TopicThermostat, events.ThermostatUpdate{
				TemperatureC: vt.data.TemperatureC,
				SetpointC:    vt.data.SetpointC,
				Humidity:     vt.data.Humidity,
				Mode:         int(vt.data.Mode),
				State:        int(vt.data.State),
			})
		}
	}
}

func (vt *VirtThermostat) deltaSetpoint(delta float64) float64 {
	newSetpoint := vt.data.SetpointC + delta
	if newSetpoint > vt.conf.MaxSetpointC {
		newSetpoint = vt.conf.MaxSetpointC
	} else if newSetpoint < vt.conf.MinSetpointC {
		newSetpoint = vt.conf.MinSetpointC
	}
	return newSetpoint
}

func (m VTMode) toggle() VTMode {
	if m == Mode_HEAT {
		return Mode_OFF
	}
	return Mode_HEAT
}

func (vt *VirtThermostat) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vt.httpHandler.ServeHTTP(w, r)
}
