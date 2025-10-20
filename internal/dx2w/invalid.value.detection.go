package dx2w

import (
	"fmt"
	"time"
)

var valueErrorChecks = map[string]func(name string, value float32, history map[string][]HistoryEntry) error{
	"hp_output_kw":           hp_output_kw_check,
	"mix_water_temp":         water_temp_check,
	"return_water_temp":      water_temp_check,
	"hot_water_min_temp":     water_temp_check,
	"hp_entering_water_temp": water_temp_check,
	"hp_exiting_water_temp":  water_temp_check,
	"buffer_tank_setpoint":   water_temp_check,
	"outside_air_temp":       outside_air_temp_check,
}

func invalidValueErrorDetection(name string, value any, history map[string][]HistoryEntry) error {
	if value == nil {
		return fmt.Errorf("value is nil")
	}
	floatValue, ok := value.(float32)
	if !ok {
		return fmt.Errorf("value is not a float32")
	}
	check, ok := valueErrorChecks[name]
	if !ok {
		return nil // no checks
	}
	return check(name, floatValue, history)
}

func hp_output_kw_check(name string, outputKW float32, history map[string][]HistoryEntry) error {
	if outputKW == 0 {
		return nil
	}
	if outputKW < 0 {
		return fmt.Errorf("negative output KW")
	}
	if outputKW > 15 {
		return fmt.Errorf("output KW exceeds expected maximum (expected ~12KW max)")
	}
	inputKWHistory := history["hp_input_kw"]
	if len(inputKWHistory) > 0 {
		inputKWValue := inputKWHistory[len(inputKWHistory)-1].Value
		if inputKWValue != nil {
			inputKW, ok := inputKWValue.(float32)
			if ok {
				if inputKW < 100 {
					return fmt.Errorf("can't have output KW with no input KW")
				}
				if outputKW > 8*inputKW {
					return fmt.Errorf("can't have output KW with COP > 8")
				}
			}
		}
	}
	return nil
}

func water_temp_check(name string, tempF float32, history map[string][]HistoryEntry) error {
	if tempF < 68 { // 20C
		return fmt.Errorf("water temp too low")
	}
	if tempF > 122 { // 50C
		return fmt.Errorf("water temp too high")
	}
	return nil
}

func outside_air_temp_check(name string, tempF float32, history map[string][]HistoryEntry) error {
	if tempF < -58 { // -50°C
		return fmt.Errorf("air temp too low")
	}
	if tempF > 122 { // 50°C
		return fmt.Errorf("air temp too high")
	}

	airTempHistory := history["outside_air_temp"]
	if len(airTempHistory) == 0 {
		return nil // no prior data to compare
	}

	const maxChangeF = 27.0 // 15°C ≈ 27°F
	const maxInterval = 8 * time.Minute

	latest := airTempHistory[len(airTempHistory)-1]
	prevTemp, ok := latest.Value.(float32)
	if !ok {
		return nil // no valid previous temperature
	}

	delta := tempF - prevTemp
	if delta < 0 {
		delta = -delta
	}

	dt := time.Since(latest.Timestamp)
	if dt < maxInterval && delta > maxChangeF {
		return fmt.Errorf("air temp changed too fast: Δ%.1f°F in %v", delta, dt.Truncate(time.Second))
	}

	return nil
}
