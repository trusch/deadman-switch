package config

import (
	"encoding/json"
	"errors"
	"time"
)

type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*d = Duration(value)
		return nil
	case string:
		var err error
		dur, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(dur)
		return nil
	default:
		return errors.New("invalid duration")
	}
}
