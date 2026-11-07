package auth

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

type signupCredentials struct {
	input Registration
	err   error
	calls int
}

func (c *signupCredentials) Authenticate(context.Context, string, string) (string, error) {
	return "", ErrUnauthenticated
}
func (c *signupCredentials) Register(_ context.Context, input Registration) (string, error) {
	c.calls++
	c.input = input
	return "user-1", c.err
}

func TestSignupValidation(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{}`, 400},
		{`null`, 400},
		{`{"first_name":"A","last_name":"B","email":"bad","password":"long-password"}`, 400},
		{`{"first_name":"A","last_name":"B","email":"a@example.com","password":"short"}`, 400},
		{`{"first_name":"A","last_name":"B","email":"a@example.com","password":"long-password","role":"admin"}`, 400},
		{`{"first_name":"A","last_name":"B","email":"a@example.com","password":"long-password"}{}`, 400},
	} {
		c := &signupCredentials{}
		h := NewLoginHandler(nil, c)
		r := httptest.NewRequest("POST", "/signup", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.Signup(w, r)
		if w.Code != tc.status || c.calls != 0 {
			t.Fatalf("%s: %d; calls %d", tc.body, w.Code, c.calls)
		}
	}
	c := &signupCredentials{err: ErrEmailExists}
	h := NewLoginHandler(nil, c)
	r := httptest.NewRequest("POST", "/signup", strings.NewReader(`{"first_name":" A ","last_name":" B ","email":" A@EXAMPLE.COM ","password":"long-password"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Signup(w, r)
	if w.Code != 409 || c.input.Email != "a@example.com" || c.input.FirstName != "A" {
		t.Fatalf("duplicate/normalization: %d %+v", w.Code, c.input)
	}
}
