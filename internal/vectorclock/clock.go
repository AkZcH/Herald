package vectorclock

import "encoding/json"

type VectorClock map[string]uint64

func New() VectorClock {
	return make(VectorClock)
}

func FromJSON(data []byte) (VectorClock, error) {
	vc := New()
	if len(data) == 0 || string(data) == "{}" {
		return vc, nil
	}
	if err := json.Unmarshal(data, &vc); err != nil {
		return nil, err
	}
	return vc, nil
}

func (vc VectorClock) Increment(nodeID string) VectorClock {
	next := vc.clone()
	next[nodeID]++
	return next
}

func (vc VectorClock) Merge(other VectorClock) VectorClock {
	next := vc.clone()
	for k, v := range other {
		if v > next[k] {
			next[k] = v
		}
	}
	return next
}

func (vc VectorClock) HappensBefore(other VectorClock) bool {
	less := false
	for k, v := range vc {
		if v > other[k] {
			return false
		}
		if v < other[k] {
			less = true
		}
	}
	for k := range other {
		if _, ok := vc[k]; !ok {
			less = true
		}
	}
	return less
}

func (vc VectorClock) Concurrent(other VectorClock) bool {
	return !vc.HappensBefore(other) && !other.HappensBefore(vc)
}

func (vc VectorClock) ToJSON() ([]byte, error) {
	return json.Marshal(vc)
}

func (vc VectorClock) clone() VectorClock {
	next := make(VectorClock, len(vc))
	for k, v := range vc {
		next[k] = v
	}
	return next
}