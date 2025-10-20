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

package modbus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
)

// ReadTyped reads a register value and converts it into the requested type T.
// Supported T: float32, float64, int16, uint16, int, bool
func ReadTyped[T any](c *Client, name string) (T, error) {
	var zero T

	val, err := c.ReadValue(name)
	if err != nil {
		return zero, err
	}

	switch any(zero).(type) {

	case float32:
		f32, ok := val.(float32)
		if !ok {
			return zero, fmt.Errorf("cannot convert %T to float32", val)
		}
		return any(f32).(T), nil

	case float64:
		f32, ok := val.(float32)
		if !ok {
			return zero, fmt.Errorf("cannot convert %T to float64", val)
		}
		return any(float64(f32)).(T), nil

	case int16:
		switch v := val.(type) {
		case float32:
			return any(int16(math.Round(float64(v)))).(T), nil
		case int16:
			return any(v).(T), nil
		default:
			return zero, fmt.Errorf("cannot convert %T to int16", val)
		}

	case uint16:
		switch v := val.(type) {
		case float32:
			return any(uint16(math.Round(float64(v)))).(T), nil
		case int:
			return any(v).(T), nil
		default:
			return zero, fmt.Errorf("cannot convert %T to uint16", val)
		}

	case int:
		switch v := val.(type) {
		case float32:
			return any(int(math.Round(float64(v)))).(T), nil
		case int16:
			return any(int(v)).(T), nil
		default:
			return zero, fmt.Errorf("cannot convert %T to int", val)
		}

	case uint:
		switch v := val.(type) {
		case float32:
			return any(int(math.Round(float64(v)))).(T), nil
		case uint16:
			return any(int(v)).(T), nil
		default:
			return zero, fmt.Errorf("cannot convert %T to int", val)
		}

	case bool:
		b, ok := val.(bool)
		if !ok {
			return zero, fmt.Errorf("cannot convert %T to bool", val)
		}
		return any(b).(T), nil

	default:
		return zero, fmt.Errorf("unsupported type parameter %T", zero)
	}
}

// ReadValue reads a register by name and returns its decoded value as `any`.
// Supported return types:
//   - float32 (for float32 or scaled int16/uint16 registers)
//   - int16   (for int16 registers without scaling)
//   - uint16  (for uint16 registers without scaling)
//   - bool    (for bool registers)
func (c *Client) ReadValue(name string) (any, error) {
	regDef, ok := c.config.Registers[name]
	if !ok {
		return nil, fmt.Errorf("register %q not configured", name)
	}

	var valf64 float64
	var raw []byte
	var err error

	nregisters := c.registerCountFromDataType(regDef.DataType)
	raw, err = c.ReadRegisters(c.ctx, regDef.Address, nregisters)

	if err != nil {
		return nil, fmt.Errorf("register read failed for %s: %w", name, err)
	}

	if len(raw) < int(nregisters*2) {
		return nil, fmt.Errorf("register %q returned insufficient data", name)
	}

	switch regDef.DataType {
	case "float32":
		valf64 = float64(bytesToFloat32(raw))
		if regDef.Scale == 0 {
			return float32(valf64), nil
		}

	case "int16":
		valf64 = float64(bytesToInt16(raw))
		if regDef.Scale == 0 {
			return int16(valf64), nil
		}

	case "uint16":
		valf64 = float64(bytesToUint16(raw))
		if regDef.Scale == 0 {
			return uint16(valf64), nil
		}

	case "bool", "binary":
		return bytesToUint16(raw) != 0, nil

	default:
		return nil, fmt.Errorf("unsupported data type %q for register %q", regDef.DataType, name)
	}

	// if requires scaling, always return float32
	valf64 = valf64*regDef.Scale + regDef.Offset
	return float32(valf64), nil
}

// WriteValue writes a Go value into a named register.
// Accepted input types:
//   - float64 (for float32 and scaled int16 registers)
//   - int     (for int16/uint16 registers)
//   - bool    (for bool registers)
func (c *Client) WriteValue(name string, value any) error {
	regDef, ok := c.config.Registers[name]
	if !ok {
		return fmt.Errorf("register %q not configured", name)
	}

	c.log.Info("WriteRegister '%s' <- %v", name, value)

	// all 16 & 32 bit numeric values can be represented by a float64
	valf64, err := toFloat64(value)
	if err != nil {
		return fmt.Errorf("value is not a numeric or bool type (got %T)", value)
	}

	if regDef.Scale != 0 {
		valf64 = (valf64 - regDef.Offset) / regDef.Scale
	}

	var raw []byte
	var nregisters uint16

	switch regDef.DataType {
	case "float32":
		if valf64 > math.MaxFloat32 || valf64 < -math.MaxFloat32 {
			return fmt.Errorf("value %v out of float32 range for register %q", valf64, name)
		}
		raw = float32ToBytes(float32(valf64))
		nregisters = 2

	case "int16":
		ival := int64(math.Round(valf64))
		if ival < math.MinInt16 || ival > math.MaxInt16 {
			return fmt.Errorf("value %v out of int16 range for register %q", valf64, name)
		}
		raw = uint16ToBytes(uint16(ival))
		nregisters = 1

	case "uint16":
		ival := uint64(math.Round(valf64))
		if ival > math.MaxUint16 {
			return fmt.Errorf("value %v out of uint16 range for register %q", valf64, name)
		}
		raw = uint16ToBytes(uint16(ival))
		nregisters = 1

	case "bool":
		if valf64 != 0 {
			raw = uint16ToBytes(math.MaxUint16)
		} else {
			raw = uint16ToBytes(0)
		}
		nregisters = 1

	default:
		return fmt.Errorf("unsupported data type %q for register %q", regDef.DataType, name)
	}

	_, err = c.client.WriteMultipleRegisters(c.ctx, regDef.Address, nregisters, raw)
	if err != nil {
		return fmt.Errorf("failed to write register %q: %w", name, err)
	}
	return nil
}

func (c *Client) registerCountFromDataType(dt string) uint16 {
	switch dt {
	case "uint16", "int16":
		return 1
	case "float32":
		return 2
	case "bool":
		return 1
	default:
		c.log.Fatal("fatal: unhandled registerCountFromDataType: %q", dt)
		return 0
	}
}

func bytesToUint16(b []byte) uint16 {
	return binary.BigEndian.Uint16(b)
}

func bytesToInt16(b []byte) int16 {
	return int16(binary.BigEndian.Uint16(b))
}

func uint16ToBytes(v uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, v)
	return buf
}

func bytesToFloat32(data []byte) float32 {
	var floatVal float32
	buf := bytes.NewReader(data)

	// assumed bigendian
	err := binary.Read(buf, binary.BigEndian, &floatVal)
	if err != nil {
		log.Fatalln("bytesToFloat32:", err)
	}

	return floatVal
}

func float32ToBytes(f float32) []byte {
	bits := math.Float32bits(f)
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, bits)
	return bytes
}

// toFloat64 attempts to convert an interface{} value into a float64.
// It supports int, uint, float types, and returns an error otherwise.
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int8:
		return float64(n), nil
	case int16:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case uint:
		return float64(n), nil
	case uint8:
		return float64(n), nil
	case uint16:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	case bool:
		if n {
			return 1.0, nil
		} else {
			return 0.0, nil
		}
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}
