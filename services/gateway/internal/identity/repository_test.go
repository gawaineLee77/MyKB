package identity

import (
	"encoding/json"
	"testing"
)

func TestMissingProviderGroupsPersistAsArray(t *testing.T) {
	for _, groups := range [][]string{nil, {}, {"  "}} {
		claims := normalizeClaims(Claims{Groups: groups})
		raw, err := json.Marshal(claims.Groups)
		if err != nil || string(raw) != "[]" {
			t.Fatalf("missing groups must satisfy the PostgreSQL array constraint: %s, %v", raw, err)
		}
	}
}
