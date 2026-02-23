package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApplyCORS_Preflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	w := httptest.NewRecorder()

	handled := ApplyCORS(w, req)
	if !handled {
		t.Fatalf("expected ApplyCORS to handle OPTIONS request")
	}
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status %d got %d", http.StatusNoContent, w.Code)
	}
	hdr := w.Header()
	if got := hdr.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("unexpected Allow-Origin: %q", got)
	}
	if got := hdr.Get("Access-Control-Allow-Methods"); got == "" {
		t.Errorf("missing Allow-Methods header")
	}
	if got := hdr.Get("Access-Control-Allow-Headers"); got == "" {
		t.Errorf("missing Allow-Headers header")
	}
	if got := hdr.Get("Access-Control-Expose-Headers"); got != "ETag" {
		t.Errorf("expected Expose-Headers ETag got %q", got)
	}
}

func TestApplyCORS_Get(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handled := ApplyCORS(w, req)
	if handled {
		t.Fatalf("expected ApplyCORS to NOT treat GET as handled")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("unexpected Allow-Origin for GET: %q", got)
	}
}
