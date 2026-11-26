package main

import (
	"net/http/httptest"
	"testing"
)

// The routine of a chat is a list with ?chat=; without it the list stays the list.
func TestRoutinesCallOfChat(t *testing.T) {
	c, ok := routinesCall(httptest.NewRequest("GET", platformAPI+"/routines?chat=conv-1&limit=5", nil))
	if !ok || c.Op != "chat" || c.ID != "conv-1" || c.Limit != 5 {
		t.Fatalf("chat: %+v %v", c, ok)
	}
	c, ok = routinesCall(httptest.NewRequest("GET", platformAPI+"/routines", nil))
	if !ok || c.Op != "list" || c.ID != "" {
		t.Fatalf("list: %+v %v", c, ok)
	}
}
