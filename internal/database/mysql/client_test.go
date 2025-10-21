package mysql

import (
	"testing"
	"time"
)

func TestFormatQuery(t *testing.T) {
	ts := time.Date(2024, 5, 1, 12, 30, 0, 0, time.UTC)
	query, err := formatQuery("INSERT INTO users VALUES (?, ?, ?)", "alice", 42, ts)
	if err != nil {
		t.Fatalf("formatQuery returned error: %v", err)
	}

	expected := "INSERT INTO users VALUES ('alice', 42, '2024-05-01 12:30:00')"
	if query != expected {
		t.Fatalf("unexpected formatted query: %s", query)
	}
}

func TestFormatQueryArgumentCount(t *testing.T) {
	if _, err := formatQuery("SELECT ?", 1, 2); err == nil {
		t.Fatalf("expected error when too many args provided")
	}

	if _, err := formatQuery("SELECT ?"); err == nil {
		t.Fatalf("expected error when not enough args provided")
	}
}
