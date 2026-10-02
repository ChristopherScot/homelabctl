package main

// Tests the client floor backed by MIN_VERSION. Delete this file when a
// service drops the floor; the minVersion package variable goes with it.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The client floor, off unless MIN_VERSION is set. Every FAIL OPEN case
// matters more than the refusal - the kubelet, Alloy and curl all send
// no version.
func TestClientFloor(t *testing.T) {
	for _, tc := range []struct {
		name, floor, sent string
		want              int
	}{
		{"no floor set", "", "0.1.0", http.StatusOK},
		{"above the floor", "0.3.0", "0.4.0", http.StatusOK},
		{"exactly at the floor", "0.3.0", "0.3.0", http.StatusOK},
		{"below the floor", "0.3.0", "0.2.9", http.StatusGone},
		{"no version sent fails open", "0.3.0", "", http.StatusOK},
		{"unparseable fails open", "0.3.0", "banana", http.StatusOK},
		{"a v-prefixed floor still compares", "v0.3.0", "0.2.0", http.StatusGone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := minVersion
			minVersion = tc.floor
			defer func() { minVersion = old }()

			h, err := handler()
			if err != nil {
				t.Fatalf("handler() = %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.sent != "" {
				req.Header.Set(ClientVersionHeader, tc.sent)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Errorf("floor=%q sent=%q -> %d, want %d", tc.floor, tc.sent, w.Code, tc.want)
			}
		})
	}
}

// A raised floor must not crash-loop probes.
func TestProbesSurviveAnImpossibleFloor(t *testing.T) {
	old := minVersion
	minVersion = "99.99.99"
	defer func() { minVersion = old }()

	h, err := handler()
	if err != nil {
		t.Fatalf("handler() = %v", err)
	}
	for _, path := range []string{"/health", "/metrics", "/"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code == http.StatusGone {
				t.Errorf("%s was refused; probes and raw curl must never be blocked", path)
			}
		})
	}
}

// A version is sent on purpose: without one, fail-open would pass
// whether or not /metrics is excluded.
func TestMetricsIsOutsideTheFloor(t *testing.T) {
	old := minVersion
	minVersion = "9.9.9"
	defer func() { minVersion = old }()

	h, err := handler()
	if err != nil {
		t.Fatalf("handler() = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set(ClientVersionHeader, "0.0.1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/metrics = %d, want 200", w.Code)
	}
}
