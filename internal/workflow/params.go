package workflow

import (
	"fmt"
	"time"
)

// ParamHelper provides typed getters for safe access to map[string]any params.
type ParamHelper struct {
	params map[string]any
}

// NewParamHelper creates a ParamHelper from a params map.
func NewParamHelper(params map[string]any) *ParamHelper {
	if params == nil {
		params = make(map[string]any)
	}
	return &ParamHelper{params: params}
}

// String returns the string value for key, or defaultVal if not found or wrong type.
func (p *ParamHelper) String(key, defaultVal string) string {
	v, ok := p.params[key]
	if !ok {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}

// Int returns the int value for key, or defaultVal if not found or wrong type.
// Handles both int and float64 (common from YAML/JSON unmarshaling).
func (p *ParamHelper) Int(key string, defaultVal int) int {
	v, ok := p.params[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return defaultVal
	}
}

// Bool returns the bool value for key, or defaultVal if not found or wrong type.
func (p *ParamHelper) Bool(key string, defaultVal bool) bool {
	v, ok := p.params[key]
	if !ok {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// Duration returns a time.Duration parsed from a string value for key,
// or defaultVal if not found, wrong type, or unparseable.
func (p *ParamHelper) Duration(key string, defaultVal time.Duration) time.Duration {
	v, ok := p.params[key]
	if !ok {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

// Has returns true if the key exists in the params map.
func (p *ParamHelper) Has(key string) bool {
	_, ok := p.params[key]
	return ok
}

// Raw returns the raw value for a key, or nil if not found.
func (p *ParamHelper) Raw(key string) any {
	return p.params[key]
}

// Validate checks that all keys in the params map are in the allowed set.
// Returns an error listing any unknown keys.
func (p *ParamHelper) Validate(allowed map[string]bool) error {
	for k := range p.params {
		if !allowed[k] {
			return fmt.Errorf("unknown parameter %q", k)
		}
	}
	return nil
}
