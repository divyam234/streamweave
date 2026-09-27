package db

import (
	"strings"
	"testing"
)

func TestSchemaValidationAndRendering(t *testing.T) {
	for _, name := range []string{"tenant_a", "Other2"} {
		if _, err := Schema(name); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"a.b", "bad;DROP TABLE users", "has space"} {
		if _, err := Schema(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	query := qualified{prefix: Quote("tenant_a") + "."}.render("SELECT * FROM " + Marker + "providers")
	if !strings.Contains(query, `"tenant_a".providers`) {
		t.Fatalf("unexpected query: %s", query)
	}
}
