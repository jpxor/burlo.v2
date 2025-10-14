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
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"burlo/v2/internal/config"
	"burlo/v2/pkg/logger"
	"burlo/v2/pkg/zwavejsws"
)

type zWaveThermostatBackend struct {
	zwaveclient *zwavejsws.Client
	updates     chan BackendUpdate
	conf        config.ThermostatConfig
	log         *logger.Logger
	nodeId      int
	units       Unit
}

func NewZWaveBackend(conf *config.Config, log *logger.Logger) Backend {
	backend := &zWaveThermostatBackend{
		log:         log,
		conf:        conf.Thermostat,
		updates:     make(chan BackendUpdate, 8),
		nodeId:      conf.Thermostat.ZWaveDeviceId,
		zwaveclient: zwavejsws.NewClient(conf.Thermostat.ZWaveAddr),
	}
	backend.zwaveclient.OnState(backend.identifyAndInitZWaveThermostat)
	backend.zwaveclient.OnEvent(backend.handleThermostatNodeEvents)
	return backend
}

func (b *zWaveThermostatBackend) Updates() <-chan BackendUpdate {
	return b.updates
}

// find thermostat node and update virtual thermostat state
// with real thermostat values
func (b *zWaveThermostatBackend) identifyAndInitZWaveThermostat(state zwavejsws.State) {

	const ThermostatSetpointCC = 0x43 // 67
	thermostatNodes := []zwavejsws.Node{}

	nodes, err := state.ParseNodes()
	if err != nil {
		b.log.Fatal("failed to parse zwave-js nodes: %v", err)
		return
	}

	// find all thermostat nodes
	for _, node := range nodes {
		if node.DeviceClass.Generic.Label == "Thermostat" {
			thermostatNodes = append(thermostatNodes, node)

		} else {
			for _, cc := range node.CommandClasses {
				if cc.ID == ThermostatSetpointCC {
					thermostatNodes = append(thermostatNodes, node)
					break
				}
			}
		}
	}

	if len(thermostatNodes) == 0 {
		b.log.Fatal("No thermostat nodes found")
	}

	// find node matching configured nodeId, or
	// fallback to first one found
	var tnode zwavejsws.Node

	for _, node := range thermostatNodes {
		if b.nodeId == node.NodeID {
			tnode = node
			break
		}
	}

	if tnode.NodeID == 0 {
		b.nodeId = thermostatNodes[0].NodeID
		tnode = thermostatNodes[0]
	}

	b.log.Info("found %d thermostat node(s)", len(thermostatNodes))
	b.log.Info("using thermostat nodeId %d", tnode.NodeID)
	b.log.Info("zwave node [name=%s, location=%s]", tnode.Name, tnode.Location)

	values, err := tnode.ParseValues()
	if err != nil {
		b.log.Error("failed to parse zwave-js node values: %v", err)
		return
	}

	for _, val := range values {
		if val.CommandClass == 49 && val.PropertyName == "Air temperature" {
			b.log.Info("Current temperature: %v %v", val.Value.(float64), val.Metadata.Unit)
			b.units = fromZwaveThermostatUnits(val.Metadata.Unit)
			b.updates <- BackendUpdate{
				Property: "temperature",
				Value:    fromZwaveThermostatTemp(val.Value.(float64), b.units),
			}
		}
		if val.CommandClass == 49 && val.PropertyName == "Humidity" {
			b.log.Info("Current humidity: %v%%", val.Value.(float64))
			b.updates <- BackendUpdate{
				Property: "humidity",
				Value:    val.Value,
			}
		}
		if val.CommandClass == 67 && val.PropertyName == "setpoint" && val.Metadata.CCSpecific.SetpointType == 1 {
			b.log.Info("Current setpoint: %v %v", val.Value.(float64), val.Metadata.Unit)
			b.units = fromZwaveThermostatUnits(val.Metadata.Unit)
			b.updates <- BackendUpdate{
				Property: "setpoint",
				Value:    fromZwaveThermostatTemp(val.Value.(float64), b.units),
			}
		}
		if val.CommandClass == 66 && val.PropertyName == "state" {
			state := int(val.Value.(float64))
			b.log.Info("Current state: %+v", state)
			b.updates <- BackendUpdate{
				Property: "state",
				Value:    fromZwaveThermostatState(state),
			}
		}
		if val.CommandClass == 64 && val.PropertyName == "mode" {
			mode := int(val.Value.(float64))
			b.log.Info("Current mode: %+v", mode)
			b.updates <- BackendUpdate{
				Property: "mode",
				Value:    fromZwaveThermostatMode(mode),
			}
		}
	}
	b.updates <- BackendUpdate{
		Property:  "none",
		Broadcast: true,
	}
}

// handleThermostatNodeEvents filters events for our thermostat node
func (b *zWaveThermostatBackend) handleThermostatNodeEvents(event zwavejsws.Event) {
	if event.NodeID != b.nodeId {
		return
	}

	switch true {
	case event.IsValueUpdate():
		b.handleValueUpdate(event)

	case event.IsMetadataUpdate():
		b.handleMetadataUpdate(event)

	case event.IsStatisticsUpdate():
		// ignore

	default:
		b.log.Error("zwave-js unhandled event: \n%+v\n", event)
		b.log.Debug("zwave-js event Args: \n%+v\n", string(event.Args))
	}
}

func (b *zWaveThermostatBackend) handleValueUpdate(event zwavejsws.Event) {
	val, err := event.ParseValueUpdated()
	if err != nil {
		b.log.Error("failed to parse zwave-js value update: %v", err)
		return
	}

	switch val.CommandClass {
	case 49: // Multilevel Sensor
		b.handleMultilevelSensor(&val)

	case 66: // Thermostat Operating State
		b.handleThermostatOperatingState(&val)

	case 67: // Thermostat Setpoint
		b.handleThermostatSetpoint(&val)

	case 64: // Thermostat Mode
		b.handleThermostatMode(&val)

	default:
		b.log.Error("zwave-js unhandled value update: %+v", val)
		b.log.Debug("zwave-js unhandled value: \n%+v\n", val)
		return

		// TODO: handle battery update event:
		// 2025/10/10 03:15:13 [Thermostat] ERROR: (thermostat.zwave.backend.go:190)
		// zwave-js unhandled value update: {CommandClass:128 CommandClassName:Battery Endpoint:0 NewValue:95 PrevValue:100 Property:level PropertyName:level PropertyKey:<nil> PropertyKeyName:}
	}
}

// Handles CommandClass 49
func (b *zWaveThermostatBackend) handleMultilevelSensor(val *zwavejsws.UpdatedValue) {
	switch val.Property {
	case "Air temperature":
		b.broadcastUpdate("temperature",
			fromZwaveThermostatTemp(val.NewValue.(float64), b.units))
	case "Humidity":
		b.broadcastUpdate("humidity", val.NewValue)
	}
}

// Handles CommandClass 66
func (b *zWaveThermostatBackend) handleThermostatOperatingState(val *zwavejsws.UpdatedValue) {
	if val.Property != "state" {
		b.log.Error("zwave-js unhandled operating state update: \n%+v\n", val)
		return
	}
	stateNum, ok := parseNumberToInt(val.NewValue)
	if !ok {
		b.log.Error("zwave-js unhandled operating state update: \n%+v\n", val)
		return
	}
	b.broadcastUpdate("state", fromZwaveThermostatState(stateNum))
}

// Handles CommandClass 67
func (b *zWaveThermostatBackend) handleThermostatSetpoint(val *zwavejsws.UpdatedValue) {
	if val.Property != "setpoint" {
		b.log.Error("zwave-js unhandled setpoint update: \n%+v\n", val)
		return
	}
	propertyKey, ok := parseNumberToInt(val.PropertyKey)
	if !ok {
		b.log.Error("zwave-js unhandled setpoint update: \n%+v\n", val)
		return
	}
	switch propertyKey {
	case 1: // heating
		b.broadcastUpdate("setpoint",
			fromZwaveThermostatTemp(val.NewValue.(float64), b.units))
	case 0: // cooling?
		// ignore
	case 11: // away heating (Energy Save Heating)
		// ignore
	default:
		b.log.Error("zwave-js unhandled setpoint update (unknown propertyKey): \n%+v\n", val)
	}
}

// Handles CommandClass 64
func (b *zWaveThermostatBackend) handleThermostatMode(val *zwavejsws.UpdatedValue) {
	modeNum, ok := parseNumberToInt(val.NewValue)
	if !ok {
		b.log.Error("failed to parse thermostat mode: %+v", val)
		return
	}
	b.broadcastUpdate("mode", fromZwaveThermostatMode(modeNum))
}

func parseNumberToInt(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case int:
		return v, true
	case uint:
		return int(v), true
	case string:
		f, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return int(f), err == nil
	default:
		panic(value)
	}
}

func (b *zWaveThermostatBackend) handleMetadataUpdate(event zwavejsws.Event) {
	mdata, err := event.ParseMatadataUpdated()
	if err != nil {
		b.log.Error("failed to parse zwave-js metadata update: %v", err)
		return
	}

	switch mdata.CommandClass {
	case 49: // Multilevel Sensor
		b.handleMultilevelSensorMetadata(&mdata)

	case 67: // Thermostat Setpoint
		// ignore

	default:
		b.log.Error("unhandled metadata update event")
		b.log.Debug("metadata: \n%+v\n", mdata)
		return
	}
}

func (b *zWaveThermostatBackend) handleMultilevelSensorMetadata(mdata *zwavejsws.UpdatedMetadata) {
	switch mdata.Property {
	case "Air temperature":
		b.units = fromZwaveThermostatUnits(mdata.Metadata.Unit)
	case "Humidity":
		// ignore
	}
}

func (b *zWaveThermostatBackend) broadcastUpdate(property string, value any) {
	b.log.Info("update from zwave device: %s = %+v", property, value)
	b.updates <- BackendUpdate{
		Property:  property,
		Value:     value,
		Broadcast: true,
	}
}

func (b *zWaveThermostatBackend) SetMode(m VTMode) error {
	zwtMode := toZwaveThermostatMode(m)
	err := b.zwaveclient.SendCommand(map[string]any{
		"messageId": fmt.Sprintf("set:mode(%d)[%d]", zwtMode, time.Now().UnixNano()),
		"command":   "node.set_value",
		"args": map[string]any{
			"nodeId":       b.nodeId,
			"commandClass": 64,
			"property":     "mode",
			"value":        zwtMode,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send mode: %v", err)
	}
	return nil
}

func (b *zWaveThermostatBackend) SetSetpoint(sp float64) error {
	zwtSetpoint := toZwaveThermostatTemp(sp, b.units)
	err := b.zwaveclient.SendCommand(map[string]any{
		"messageId": fmt.Sprintf("set:setpoint(%.1f)[%d]", zwtSetpoint, time.Now().UnixNano()),
		"command":   "node.set_value",
		"nodeId":    b.nodeId,
		"value":     zwtSetpoint,
		"valueId": map[string]any{
			"commandClass": 67,
			"property":     "setpoint",
			"propertyKey":  1, // heating setpoint

		},
	})
	if err != nil {
		return fmt.Errorf("failed to send setpoint: %v", err)
	}
	return nil
}

func (b *zWaveThermostatBackend) Run(ctx context.Context) {
	b.log.Info("starting Z-Wave backend")
	defer b.log.Info("stopping Z-Wave backend")
	defer close(b.updates)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := b.zwaveclient.Connect(ctx)
			if err != nil {
				b.log.Info("failed to connect to zwaveclient: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			err = b.zwaveclient.ListenNext()
			if err != nil {
				b.zwaveclient.Close()
			}
		}
	}
}

type Unit int

var Unit_Celsius Unit = 0
var Unit_Fahrenheit Unit = 1

func toZwaveThermostatMode(m VTMode) int {
	return int(m)
}

func fromZwaveThermostatMode(m int) VTMode {
	return VTMode(m)
}

func fromZwaveThermostatState(s int) VTState {
	return VTState(s)
}

// toZwaveThermostatTemp convert temp from celcius to `unit`
func toZwaveThermostatTemp(tCel float64, unit Unit) float64 {
	switch unit {
	case Unit_Celsius:
		return tCel
	case Unit_Fahrenheit:
		return (tCel * 9.0 / 5.0) + 32
	default:
		log.Println("unknown unit:", unit)
		return tCel
	}
}

// fromZwaveThermostatTemp convert temp from `unit` to celcius
func fromZwaveThermostatTemp(temp float64, unit Unit) float64 {
	switch unit {
	case Unit_Celsius:
		return temp
	case Unit_Fahrenheit:
		return (temp - 32) * 5.0 / 9.0
	default:
		log.Println("unknown unit:", unit)
		return temp
	}
}

func fromZwaveThermostatUnits(u string) Unit {
	switch true {
	case strings.Contains(u, "C"):
		return Unit_Celsius
	case strings.Contains(u, "F"):
		return Unit_Fahrenheit
	default:
		log.Println("failed to parse zwave unit string:", u)
		return Unit_Celsius
	}
}
