// Package jsonutil holds small JSON helpers shared by qbox packages.
package jsonutil

import (
	"encoding/json"
	"errors"
)

// SetObjectTag sets the "tag" field of a JSON object. It rejects null and
// non-object values instead of panicking on a nil map.
func SetObjectTag(raw json.RawMessage, tag string) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, errors.New("значение не является JSON-объектом")
	}
	encoded, err := json.Marshal(tag)
	if err != nil {
		return nil, err
	}
	obj["tag"] = encoded
	return json.Marshal(obj)
}
