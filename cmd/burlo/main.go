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

package main

import (
	"burlo/v2/internal/config"
	"burlo/v2/internal/controller"
	"burlo/v2/internal/dx2w"
	"burlo/v2/internal/emoncms"
	"burlo/v2/internal/phidgets"
	"burlo/v2/internal/thermostat"
	"burlo/v2/internal/weather"
	"burlo/v2/pkg/appctx"
	"burlo/v2/pkg/eventbus"
	"burlo/v2/pkg/logger"
	"burlo/v2/pkg/modbus"
	"burlo/v2/pkg/rootserv"
	"burlo/v2/pkg/service"
	"burlo/v2/pkg/sysmon"
	"fmt"
	"os"
	"path/filepath"
)

func main() {

	rootdir := os.Getenv("PROJECT_ROOT")
	if rootdir == "" {
		rootdir = "."
	}

	logger.Init(filepath.Join(rootdir, "var/logs/burlo.log"))

	appConf := config.LoadFile(filepath.Join(rootdir, "var/config/burlo.json"))
	modbusConf := modbus.LoadConfig(filepath.Join(rootdir, "var/config/dx2w.modbus.yml"))

	fmt.Println(filepath.Join(rootdir, "var/logs/burlo.log"))
	fmt.Println(filepath.Join(rootdir, "var/config/burlo.json"))
	fmt.Println(filepath.Join(rootdir, "var/config/dx2w.modbus.yml"))

	// use conf to pass eventbus to whoever needs it
	appConf.EventBus = eventbus.New()
	appConf.DataDir = filepath.Join(rootdir, "var/cache")
	appConf.RootDir = rootdir

	ctx, ctxCancel := appctx.New()

	// init services
	server := rootserv.New(":80")
	sysMonitorService := sysmon.New()
	phidgetsService := phidgets.New(appConf)
	controllerService := controller.New(appConf)
	dx2wModbusService := dx2w.New(modbusConf, appConf)
	thermostatService := thermostat.NewZWaveThermostat(appConf)
	weatherService := weather.NewLocalDX2W(dx2wModbusService, appConf)
	dataLoggerService := emoncms.New(controllerService, dx2wModbusService, appConf)

	// attach web handler enabled services
	server.Attach("/logger", "Logger", logger.WebService())
	server.Attach("/monitor", "System Monitor", sysMonitorService)
	server.Attach("/phidgets", "Phidgets State", phidgetsService)
	server.Attach("/dx2wModbus", "DX2W Modbus Registers", dx2wModbusService)
	server.Attach("/thermostat", "Virtual Thermostat with ZWave", thermostatService)
	server.Attach("/weather", "Weather Data", weatherService)

	// start runnable services
	exitCh := service.Start(ctx, ctxCancel, []service.Runnable{
		phidgetsService,
		dx2wModbusService,
		controllerService,
		thermostatService,
		weatherService,
		dataLoggerService,
		server,
	})

	// waits for all services to stop
	os.Exit(<-exitCh)
}
