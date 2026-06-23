package http

import (
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
)

const (
	// IP truncation masks for privacy (§4): keep the network prefix, zero the host.
	ipv4PrefixBits = 24
	ipv4TotalBits  = 32
	ipv6PrefixBits = 48
	ipv6TotalBits  = 128

	// A W3C traceparent has four hyphen-separated fields: version-trace-span-flags.
	traceparentFields = 4
)

// WithLogContext captures the request-scoped logging context (§2 envelope:
// correlation/session/tracing ids, the context block, and consent) once per
// request and stores it for the AnalyticsRecorder to read. It runs as the
// outermost middleware so every request — authenticated or not — carries a
// LogContext. The actor is resolved later by RequireAuth.
func WithLogContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID, spanID := parseTraceparent(r.Header.Get("traceparent"))

		lc := domain.LogContext{
			CorrelationID: headerOrUUID(r, "X-Correlation-ID"),
			SessionID:     r.Header.Get("X-Session-ID"),
			TraceID:       traceID,
			SpanID:        spanID,
			AnonymousID:   r.Header.Get("X-Anonymous-ID"),
			Consent:       parseConsent(r),
			Context: domain.EventContext{
				IPTrunc:    truncateIP(clientIP(r)),
				UserAgent:  r.UserAgent(),
				DeviceType: deviceType(r.UserAgent()),
				Locale:     firstLocale(r.Header.Get("Accept-Language")),
				Referrer:   r.Referer(),
				PageURL:    r.Header.Get("X-Page-URL"),
			},
		}

		ctx := domain.ContextWithLogContext(r.Context(), lc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// headerOrUUID returns the header value, or a fresh correlation id when the
// caller didn't supply one so every request is still traceable end-to-end.
func headerOrUUID(r *http.Request, header string) string {
	if v := r.Header.Get(header); v != "" {
		return v
	}
	return uuid.NewString()
}

// parseTraceparent extracts trace_id and span_id from a W3C traceparent header
// (version-traceid-spanid-flags), returning empty strings when it is absent or
// malformed.
func parseTraceparent(h string) (traceID, spanID string) {
	parts := strings.Split(h, "-")
	if len(parts) < traceparentFields {
		return "", ""
	}
	return parts[1], parts[2]
}

// parseConsent reads the analytics/marketing consent signals. Analytics defaults
// to true unless the caller explicitly opts out; marketing defaults to false.
func parseConsent(r *http.Request) domain.Consent {
	return domain.Consent{
		Analytics: !isOptOut(r.Header.Get("X-Consent-Analytics")),
		Marketing: isOptIn(r.Header.Get("X-Consent-Marketing")),
	}
}

func isOptOut(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "deny", "no":
		return true
	default:
		return false
	}
}

func isOptIn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "allow", "yes":
		return true
	default:
		return false
	}
}

// clientIP returns the originating IP, preferring the first hop in
// X-Forwarded-For (set by a trusted proxy/gateway) over the direct peer.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	return r.RemoteAddr
}

// truncateIP zeroes the host bits of the caller IP for privacy (§4): the last
// octet of an IPv4 address (/24) or the lower 80 bits of an IPv6 address (/48).
func truncateIP(remote string) string {
	host := remote
	if h, _, err := net.SplitHostPort(remote); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(ipv4PrefixBits, ipv4TotalBits)).String()
	}
	return ip.Mask(net.CIDRMask(ipv6PrefixBits, ipv6TotalBits)).String()
}

// deviceType is a coarse classification of the User-Agent for analytics rollups.
func deviceType(ua string) string {
	lower := strings.ToLower(ua)
	switch {
	case lower == "":
		return ""
	case strings.Contains(lower, "ipad"), strings.Contains(lower, "tablet"):
		return "tablet"
	case strings.Contains(lower, "mobi"), strings.Contains(lower, "android"), strings.Contains(lower, "iphone"):
		return "mobile"
	default:
		return "desktop"
	}
}

// firstLocale returns the highest-priority language tag from Accept-Language,
// dropping any quality weight (e.g. "en-US,en;q=0.9" -> "en-US").
func firstLocale(header string) string {
	if header == "" {
		return ""
	}
	first, _, _ := strings.Cut(header, ",")
	tag, _, _ := strings.Cut(first, ";")
	return strings.TrimSpace(tag)
}
