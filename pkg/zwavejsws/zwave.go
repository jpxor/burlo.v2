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

package zwavejsws

import (
	"burlo/v2/pkg/logger"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ---------- Types ----------
// SEE: https://github.com/zwave-js/zwave-js-server#api

// Response from zwave-js
type Response struct {
	Type string `json:"type"`

	// result type
	MessageId string          `json:"messageId,omitempty"`
	Success   bool            `json:"success,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`

	// event type
	Event json.RawMessage `json:"event,omitempty"`
}

// Result from a "start_listening" command
type Result struct {
	State State `json:"state"`
}

type State struct {
	Controller struct {
		HomeID uint32 `json:"homeId"`
	} `json:"controller"`
	Nodes json.RawMessage `json:"nodes"`
}

// Node represents a zwave-js node
type Node struct {
	Name     string          `json:"name"`
	Location string          `json:"location"`
	NodeID   int             `json:"nodeId"`
	Values   json.RawMessage `json:"values"`

	DeviceClass struct {
		Generic struct {
			Key   int    `json:"key"`
			Label string `json:"label"`
		} `json:"generic"`
	} `json:"deviceClass"`

	CommandClasses []struct {
		Name     string `json:"name"`
		ID       int    `json:"id"`
		Version  int    `json:"version,omitempty"`
		IsSecure bool   `json:"isSecure,omitempty"`
	} `json:"commandClasses"`
}

// Value represents a parsed value from a node
type Value struct {
	CCVersion        int      `json:"ccVersion"`
	CommandClass     int      `json:"commandClass"`
	CommandClassName string   `json:"commandClassName"`
	Endpoint         int      `json:"endpoint"`
	Metadata         Metadata `json:"metadata"`
	Property         any      `json:"property"`
	PropertyName     string   `json:"propertyName"`
	PropertyKey      any      `json:"propertyKey,omitempty"`
	PropertyKeyName  string   `json:"propertyKeyName,omitempty"`
	Value            any      `json:"value"`
}

// Metadata provides additional info about a Value
type Metadata struct {
	Label     string `json:"label,omitempty"`
	Readable  bool   `json:"readable,omitempty"`
	Writeable bool   `json:"writeable,omitempty"`
	Type      string `json:"type,omitempty"`
	Unit      string `json:"unit,omitempty"`

	CCSpecific struct {
		// setpoints
		SetpointType int `json:"setpointType,omitempty"`

		// multilevel sensors
		SensorType int     `json:"sensorType,omitempty"`
		Scale      float64 `json:"scale,omitempty"`
	} `json:"ccSpecific"`
}

// Event represents a parsed zwave-js event
type Event struct {
	Type   string          `json:"event"`
	NodeID int             `json:"nodeId,omitempty"`
	Source string          `json:"source,omitempty"`
	Args   json.RawMessage `json:"args,omitempty"`
}

// UpdatedValue are the Event Args for "value updated" events
type UpdatedValue struct {
	CommandClass     int    `json:"commandClass"`
	CommandClassName string `json:"commandClassName"`
	Endpoint         int    `json:"endpoint"`
	NewValue         any    `json:"newValue"`
	PrevValue        any    `json:"prevValue"`
	Property         string `json:"property"`
	PropertyName     string `json:"propertyName"`
	PropertyKey      any    `json:"propertyKey,omitempty"`
	PropertyKeyName  string `json:"propertyKeyName,omitempty"`
}

type UpdatedMetadata struct {
	CommandClass     int      `json:"commandClass"`
	CommandClassName string   `json:"commandClassName"`
	Endpoint         int      `json:"endpoint"`
	Property         string   `json:"property"`
	PropertyName     string   `json:"propertyName"`
	Metadata         Metadata `json:"metadata"`
}

// Client manages websocket communication
type Client struct {
	url       string
	conn      *websocket.Conn
	mu        sync.Mutex
	onState   func(State)
	onEvent   func(Event)
	retryWait time.Duration
	log       *logger.Logger
}

// ---------- Public API ----------

func NewClient(url string) *Client {
	return &Client{
		url:       url,
		retryWait: 5 * time.Second,
		log:       logger.New("ZWaveJS   "),
	}
}

// OnState sets the callback when current state is received
func (c *Client) OnState(fn func(State)) {
	c.onState = fn
}

// OnEvent sets the callback when an event is received
func (c *Client) OnEvent(fn func(Event)) {
	c.onEvent = fn
}

// SendCommand sends a generic command to zwave-js
func (c *Client) SendCommand(msg interface{}) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.conn.WriteJSON(msg)
}

// SetValue sets a value on a node
func (c *Client) SetValue(nodeID int, commandClass int, property string, value interface{}) error {
	cmd := map[string]interface{}{
		"command": "node.set_value",
		"nodeId":  nodeID,
		"args": map[string]interface{}{
			"commandClass": commandClass,
			"property":     property,
			"value":        value,
		},
	}
	return c.SendCommand(cmd)
}

// Connect starts the connection loop
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// already connected
	if c.conn != nil {
		return nil
	}

	conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
	if err != nil {
		c.log.Error("zwave: connect failed: %v (%v), retrying in %s", err, c.url, c.retryWait)
		return err
	}

	// When the context is cancelled, close the websocket to unblock reads
	go func() {
		<-ctx.Done()
		c.Close()
	}()

	// Initialize
	err = conn.WriteJSON(map[string]any{
		"messageId":     "initialize",
		"command":       "initialize",
		"schemaVersion": 1})

	if err != nil {
		c.log.Error("zwave command: initialize failed: %v", err)
		return err
	}

	// Start listening
	err = conn.WriteJSON(map[string]any{
		"messageId": "start_listening",
		"command":   "start_listening",
	})

	if err != nil {
		c.log.Error("zwave command: start_listening failed: %v", err)
		return err
	}

	c.conn = conn
	c.log.Info("Connected")
	return nil
}

// Close stops the client
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		tmpConn := c.conn
		c.conn = nil
		tmpConn.Close()
		c.log.Info("Closed")
	}
}

func (c *Client) ListenNext() error {
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		if c.conn == nil {
			return nil // was closed
		}
		c.log.Error("zwave-js ws ReadMessage: %v", err)
		return err
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		c.log.Error("Unmarshal of zwave-js message: %v", err)
		return err
	}

	switch resp.Type {
	case "result":
		c.handleResponse(resp)
		return nil

	case "event":
		c.handleEvent(resp)
		return nil

	default:
		c.log.Info("unhandled zwave-js message type: %s", resp.Type)
		return nil
	}
}

// ---------- Internal ----------

// handleResponse processes "result" type messages
func (c *Client) handleResponse(resp Response) {
	if resp.MessageId == "start_listening" {
		if !resp.Success {
			c.log.Fatal("fatal: start_listening failed")
		}

		var result Result
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			c.log.Fatal("failed Unmarshal of zwave-js start_listening result: %v", err)
		}

		if c.onState != nil {
			c.onState(result.State)
		}

		return
	}

	// other result types can be handled here if needed
	if !resp.Success {
		c.log.Error("messageId '%s' failed", resp.MessageId)
	} else {
		c.log.Info("messageId '%s' succeeded", resp.MessageId)
	}
}

// handleEvent processes "event" type messages
func (c *Client) handleEvent(resp Response) {
	if c.onEvent == nil {
		return
	}
	var event Event
	if err := json.Unmarshal(resp.Event, &event); err != nil {
		c.log.Error("Unmarshal of zwave-js Event: %v", err)
		return
	}
	c.onEvent(event)
}
