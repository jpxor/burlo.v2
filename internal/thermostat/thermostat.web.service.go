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
	"burlo/v2/pkg/logger"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type WebAppRequest struct {
	Command string  `json:"command"`
	DeltaC  float64 `json:"delta,omitempty"`
	Mode    float64 `json:"mode,omitempty"`
}

type WebAppState struct {
	TemperatureC float64 `json:"temperature"`
	SetpointC    float64 `json:"setpoint"`
	Humidity     float64 `json:"humidity"`
	Mode         int     `json:"mode"`
	State        int     `json:"state"`
}

type ClientSync struct {
	clients map[*websocket.Conn]bool
	mutex   sync.Mutex
}

var clients = ClientSync{clients: make(map[*websocket.Conn]bool)}

func (c *ClientSync) broadcast(pm *websocket.PreparedMessage, log *logger.Logger) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for ws := range c.clients {
		if err := ws.WritePreparedMessage(pm); err != nil {
			log.Error("failed to write message: %v", err)
			ws.Close()
			delete(c.clients, ws)
		}
	}
}

func (c *ClientSync) add(ws *websocket.Conn) {
	c.mutex.Lock()
	c.clients[ws] = true
	c.mutex.Unlock()
}

func (c *ClientSync) remove(ws *websocket.Conn) {
	c.mutex.Lock()
	delete(c.clients, ws)
	c.mutex.Unlock()
}

func (c *ClientSync) closeAll() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for ws := range c.clients {
		ws.Close()
		delete(c.clients, ws)
	}
}

func (vt *VirtThermostat) buildHTTPHandler() http.Handler {
	assetsDir := filepath.Join(vt.rootDir, "internal/thermostat/www")
	mux := http.NewServeMux()
	mux.HandleFunc("/", vt.serveRoot(assetsDir))
	mux.HandleFunc("/ws", vt.serveWebSockets(vt.clientQueue))
	return mux
}

func (vt *VirtThermostat) serveRoot(assetsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		// If the user requests "/", serve the main thermostat.html file.
		path := r.URL.Path
		if path == "/" || path == "" {
			http.ServeFile(w, r, filepath.Join(assetsDir, "thermostat.html"))
			return
		}

		// Otherwise, serve any file within the www directory.
		fs := http.FileServer(http.Dir(assetsDir))
		http.StripPrefix("/", fs).ServeHTTP(w, r)
	}
}

func (vt *VirtThermostat) serveWebSockets(msgQueue chan WebAppRequest) http.HandlerFunc {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			vt.log.Debug("checking origin: %s", origin)
			if origin == "" {
				return false
			}
			if strings.Contains(origin, "localhost") {
				return true
			}
			return strings.Contains(origin, r.Host)
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			vt.log.Error("failed to upgrade websocket: %v", err)
			return
		}
		clients.add(ws)
		defer func() {
			clients.remove(ws)
			ws.Close()
		}()

		select {
		case msgQueue <- WebAppRequest{Command: "broadcast"}:
		default:
		}

		var req WebAppRequest
		for {
			if err := ws.ReadJSON(&req); err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					break
				}
				vt.log.Error("failed ws ReadJSON: %v", err)
				break
			}
			select {
			case msgQueue <- req:
			default:
				vt.log.Debug("clientQueue is full; dropping client message")
			}
		}
	}
}

func webAppBroadcast(msg WebAppState) {
	log := logger.New("ThermostatWeb")
	data, err := json.Marshal(msg)
	if err != nil {
		log.Error("failed to marshal broadcast: %v", err)
		return
	}
	pm, err := websocket.NewPreparedMessage(websocket.TextMessage, data)
	if err != nil {
		log.Error("failed to prepare message: %v", err)
		return
	}
	clients.broadcast(pm, log)
}
