package slack

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPost(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := Post(srv.URL, "hello drift"); err != nil {
		t.Fatalf("post: %v", err)
	}
	if !strings.Contains(got, `"text":"hello drift"`) {
		t.Errorf("unexpected payload: %s", got)
	}
}

func TestPostNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "invalid_token")
	}))
	defer srv.Close()

	err := Post(srv.URL, "x")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got %v", err)
	}
}
