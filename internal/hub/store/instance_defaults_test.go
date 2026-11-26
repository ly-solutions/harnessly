package store

import (
	"context"
	"testing"
)

// An installation that starts with nobody keeps projects without a row closed, and goes on doing so after
// people come; one that had people before the choice stays as it was.
func TestUnrecordedDefaultIsChosenOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		people bool
		closed bool
	}{{"new", false, true}, {"older", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := openTest(t).S()
			person := func(email string) {
				if err := s.CreateUser(ctx, &User{Email: email}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.people {
				person("anna@example.com")
			}
			for i := range 3 {
				closed, err := s.DesignUnrecordedClosed(ctx)
				if err != nil || closed != tc.closed {
					t.Fatalf("call %d: closed = %v, %v; want %v", i, closed, err, tc.closed)
				}
				// People who come after the choice do not change it.
				person(NewID() + "@example.com")
			}
		})
	}
}
