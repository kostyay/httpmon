package browse

import (
	"testing"
)

func TestSessionStopNil(t *testing.T) {
	var s *Session
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop on nil session: %v", err)
	}
}

func TestSessionStopNilCleanup(t *testing.T) {
	s := &Session{}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop with nil cleanup: %v", err)
	}
}
