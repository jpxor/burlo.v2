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
	"encoding/json"
	"fmt"
)

func (e Event) IsValueUpdate() bool {
	return e.Type == "value updated"
}

func (e Event) IsStatisticsUpdate() bool {
	return e.Type == "statistics updated"
}

func (e Event) IsMetadataUpdate() bool {
	return e.Type == "metadata updated"
}

// ParseValueUpdated parses a "value updated" event into a Value
func (e Event) ParseValueUpdated() (UpdatedValue, error) {
	if e.Type != "value updated" {
		return UpdatedValue{}, fmt.Errorf("not a value updated event")
	}
	var value UpdatedValue
	if err := json.Unmarshal(e.Args, &value); err != nil {
		err = fmt.Errorf("failed Unmarshal of zwave-js UpdatedValue: %v", err)
		return UpdatedValue{}, err
	}
	return value, nil
}

// ParseMatadataUpdated
func (e Event) ParseMatadataUpdated() (UpdatedMetadata, error) {
	if e.Type != "metadata updated" {
		return UpdatedMetadata{}, fmt.Errorf("not a metadata updated event")
	}
	var value UpdatedMetadata
	if err := json.Unmarshal(e.Args, &value); err != nil {
		err = fmt.Errorf("failed Unmarshal of zwave-js UpdatedMetadata: %v", err)
		return UpdatedMetadata{}, err
	}
	return value, nil
}

func (s State) ParseNodes() ([]Node, error) {
	var nodes []Node
	if err := json.Unmarshal(s.Nodes, &nodes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal zwave-js nodes: %v", err)
	}
	return nodes, nil
}

func (n Node) ParseValues() ([]Value, error) {
	var values []Value
	if err := json.Unmarshal(n.Values, &values); err != nil {
		return nil, fmt.Errorf("failed to unmarshal zwave-js node values: %v", err)
	}
	return values, nil
}
