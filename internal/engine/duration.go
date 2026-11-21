package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

type Duration struct {
	time.Duration
}

func (d Duration) MarshalText() ([]byte, error) {
	if d.Duration == 0 {
		return []byte("0s"), nil
	}
	return []byte(d.Duration.String()), nil
}

func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("parse duration: %w", err)
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		return d.UnmarshalText([]byte(text))
	}
	var seconds float64
	if err := json.Unmarshal(data, &seconds); err != nil {
		return fmt.Errorf("duration must be a Go duration string or seconds: %w", err)
	}
	nanoseconds := seconds * float64(time.Second)
	if math.IsInf(nanoseconds, 0) ||
		nanoseconds >= float64(math.MaxInt64) ||
		nanoseconds < float64(math.MinInt64) {
		return fmt.Errorf("duration seconds out of range")
	}
	d.Duration = time.Duration(nanoseconds)
	return nil
}
