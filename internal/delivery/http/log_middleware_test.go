package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

func TestTruncateIP(t *testing.T) {
	tests := []struct{ in, want string }{
		{"203.0.113.42:5555", "203.0.113.0"},
		{"203.0.113.42", "203.0.113.0"},
		{"2001:db8:1:2:3:4:5:6", "2001:db8:1::"},
		{"not-an-ip", ""},
	}
	for _, tc := range tests {
		if got := truncateIP(tc.in); got != tc.want {
			t.Errorf("truncateIP(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseTraceparent(t *testing.T) {
	traceID, spanID := parseTraceparent("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	if traceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace_id = %q", traceID)
	}
	if spanID != "b7ad6b7169203331" {
		t.Errorf("span_id = %q", spanID)
	}

	if tr, sp := parseTraceparent(""); tr != "" || sp != "" {
		t.Errorf("empty header should yield empty ids, got %q/%q", tr, sp)
	}
}

func TestDeviceType(t *testing.T) {
	tests := []struct{ ua, want string }{
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0)", "mobile"},
		{"Mozilla/5.0 (Linux; Android 14)", "mobile"},
		{"Mozilla/5.0 (iPad; CPU OS 17_0)", "tablet"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "desktop"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := deviceType(tc.ua); got != tc.want {
			t.Errorf("deviceType(%q) = %q, want %q", tc.ua, got, tc.want)
		}
	}
}

func TestWithLogContext_PopulatesRequestContext(t *testing.T) {
	var captured domain.LogContext
	var found bool

	handler := WithLogContext(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured, found = domain.LogContextFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "http://x/lessons", nil)
	req.RemoteAddr = "203.0.113.42:5555"
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone)")
	req.Header.Set("traceparent", "00-traceabc-spanxyz-01")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("X-Consent-Analytics", "false")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !found {
		t.Fatal("WithLogContext did not store a LogContext")
	}
	if captured.Context.IPTrunc != "203.0.113.0" {
		t.Errorf("ip_trunc = %q, want 203.0.113.0", captured.Context.IPTrunc)
	}
	if captured.Context.DeviceType != "mobile" {
		t.Errorf("device_type = %q, want mobile", captured.Context.DeviceType)
	}
	if captured.Context.Locale != "en-US" {
		t.Errorf("locale = %q, want en-US", captured.Context.Locale)
	}
	if captured.TraceID != "traceabc" {
		t.Errorf("trace_id = %q, want traceabc", captured.TraceID)
	}
	if captured.Consent.Analytics {
		t.Error("consent.analytics should be false when caller opts out")
	}
	if captured.CorrelationID == "" {
		t.Error("correlation_id should be generated when absent")
	}
}
