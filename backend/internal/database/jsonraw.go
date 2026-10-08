package database

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

// JSONRawMessage mirrors json.RawMessage but also implements sql.Scanner and
// driver.Valuer so it can round-trip through every DB driver we touch: Postgres
// returns jsonb as []byte, SQLite returns the same column as string. Plain
// json.RawMessage only satisfies the json codec, so scanning out of SQLite
// (used in tests) blows up with "unsupported Scan, storing driver.Value type
// string into type *json.RawMessage".
//
// Behaviour for JSON marshalling is identical to json.RawMessage — this type
// is a drop-in replacement at the model boundary, with an explicit
// conversion (json.RawMessage(value)) at the API surface if the response
// shape uses the stdlib type.
type JSONRawMessage []byte

// MarshalJSON returns m as the JSON encoding of m.
func (m JSONRawMessage) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

// UnmarshalJSON sets *m to a copy of data.
func (m *JSONRawMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("database.JSONRawMessage: UnmarshalJSON on nil pointer")
	}
	*m = append((*m)[0:0], data...)
	return nil
}

// Scan accepts both []byte (Postgres jsonb) and string (SQLite TEXT) and
// copies the raw JSON bytes into m. Nil from the driver becomes a nil slice.
func (m *JSONRawMessage) Scan(value any) error {
	if value == nil {
		*m = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*m = append((*m)[0:0], v...)
	case string:
		*m = append((*m)[0:0], v...)
	default:
		return fmt.Errorf("database.JSONRawMessage: cannot scan %T", value)
	}
	return nil
}

// Value emits the raw bytes; Postgres (jsonb) and SQLite (TEXT) both accept
// this form, and nil maps to SQL NULL.
func (m JSONRawMessage) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return []byte(m), nil
}

// Static interface assertions catch regressions if the contract slips.
var (
	_ json.Marshaler   = JSONRawMessage(nil)
	_ json.Unmarshaler = (*JSONRawMessage)(nil)
	_ driver.Valuer    = JSONRawMessage(nil)
)
