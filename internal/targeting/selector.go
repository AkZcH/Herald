package targeting

import (
	"fmt"
	"strings"
)

type Predicate struct {
	Key   string
	Value string
}

type Selector []Predicate

func Parse(raw string) (Selector, error) {
	if raw == "" {
		return Selector{}, nil
	}

	var selector Selector
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, "=")
		if idx == -1 {
			return nil, fmt.Errorf("invalid selector %q: missing '='", part)
		}
		key := strings.TrimSpace(part[:idx])
		value := strings.TrimSpace(part[idx+1:])
		if key == "" {
			return nil, fmt.Errorf("invalid selector %q: empty key", part)
		}
		selector = append(selector, Predicate{Key: key, Value: value})
	}
	return selector, nil
}

func (s Selector) Matches(labels map[string]string) bool {
	for _, p := range s {
		v, ok := labels[p.Key]
		if !ok || v != p.Value {
			return false
		}
	}
	return true
}

func (s Selector) IsEmpty() bool {
	return len(s) == 0
}