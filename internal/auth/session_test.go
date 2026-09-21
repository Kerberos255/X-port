package auth

import (
	"testing"
	"time"
)

func TestCreatePrunesExpiredSessions(t *testing.T) {
	s := NewSessions(time.Hour)
	s.values["expired"] = Session{Username: "old", Expires: time.Now().Add(-time.Minute)}
	s.values["live"] = Session{Username: "keep", Expires: time.Now().Add(time.Hour)}
	if _, err := s.Create("new"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.values["expired"]; ok {
		t.Fatal("expired session was not pruned")
	}
	if _, ok := s.values["live"]; !ok {
		t.Fatal("live session was pruned")
	}
	if len(s.values) != 2 {
		t.Fatalf("unexpected session count: %d", len(s.values))
	}
}
