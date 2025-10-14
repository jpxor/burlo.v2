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

package phidgets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type DigitalOutRequest struct {
	Name        string `json:"name"`
	TargetState bool   `json:"target_state"`
	Channel     int    `json:"channel"`
	HubPort     int    `json:"hub_port"`
}

type VoltageOutRequest struct {
	Name        string  `json:"name"`
	TargetState float64 `json:"target_state"`
	Channel     int     `json:"channel"`
	HubPort     int     `json:"hub_port"`
}

type DigitalInRequest struct {
	Name    string `json:"name"`
	Channel int    `json:"channel"`
	HubPort int    `json:"hub_port"`
	Webhook string `json:"webhook"`
}

func postJSON(url string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("HTTP POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func SetDigitalOutput(serverURL, name string, state bool, channel, hubPort int) error {
	return postJSON(fmt.Sprintf("%s/phidgets/digital_out", serverURL), DigitalOutRequest{
		Name:        name,
		TargetState: state,
		Channel:     channel,
		HubPort:     hubPort,
	})
}

func SetVoltageOutput(serverURL, name string, voltage float64, channel, hubPort int) error {
	return postJSON(fmt.Sprintf("%s/phidgets/voltage_out", serverURL), VoltageOutRequest{
		Name:        name,
		TargetState: voltage,
		Channel:     channel,
		HubPort:     hubPort,
	})
}

func OpenDigitalInput(serverURL, name string, channel, hubPort int, webhookURL string) error {
	return postJSON(fmt.Sprintf("%s/phidgets/digital_in", serverURL), DigitalInRequest{
		Name:    name,
		Channel: channel,
		HubPort: hubPort,
		Webhook: webhookURL,
	})
}
