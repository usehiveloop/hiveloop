package proxy

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
)

var (
	// Disallowed networks (IPv4)
	_, loopback4, _    = net.ParseCIDR("127.0.0.0/8")
	_, linkLocal4, _   = net.ParseCIDR("169.254.0.0/16")
	_, privateA, _     = net.ParseCIDR("10.0.0.0/8")
	_, privateB, _     = net.ParseCIDR("172.16.0.0/12")
	_, privateC, _     = net.ParseCIDR("192.168.0.0/16")
	_, cgNAT, _        = net.ParseCIDR("100.64.0.0/10")
	_, multicast4, _   = net.ParseCIDR("224.0.0.0/4")
	_, reserved4, _    = net.ParseCIDR("240.0.0.0/4")
	_, unspecified4, _ = net.ParseCIDR("0.0.0.0/8")
	// Disallowed networks (IPv6)
	_, loopback6, _    = net.ParseCIDR("::1/128")
	_, linkLocal6, _   = net.ParseCIDR("fe80::/10")
	_, uniqueLocal6, _ = net.ParseCIDR("fc00::/7")
	_, multicast6, _   = net.ParseCIDR("ff00::/8")
	_, unspecified6, _ = net.ParseCIDR("::/128")

	// Explicitly blocked hostnames commonly used for metadata/internal resolution
	blockedHostnames = map[string]struct{}{
		"localhost":                {},
		"localhost.localdomain":    {},
		"metadata.google.internal": {},
		"metadata":                 {}, // Azure often uses "metadata"
	}

	// AllowLoopback can be set to true in tests to allow loopback/private addresses.
	AllowLoopback = false

	// allowedBaseURLHosts is the process-wide allowlist of hostnames that may
	// be used as credential base_urls. When empty, no host-level allowlist is
	// enforced and only the SSRF/IP checks below gate the URL (this preserves
	// local/dev behaviour where operators have not supplied an allowlist).
	allowedBaseURLHosts   = map[string]struct{}{}
	allowedBaseURLHostsMu sync.RWMutex
)

// SetAllowedBaseURLHosts configures the process-wide allowlist of hostnames
// (case-insensitive) that ValidateBaseURL will accept. Passing an empty slice
// disables host-level allowlisting and falls back to SSRF-only validation.
func SetAllowedBaseURLHosts(hosts []string) {
	normalised := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		h = strings.TrimSpace(strings.ToLower(h))
		if h == "" {
			continue
		}
		normalised[h] = struct{}{}
	}
	allowedBaseURLHostsMu.Lock()
	allowedBaseURLHosts = normalised
	allowedBaseURLHostsMu.Unlock()
}

// baseURLHostAllowed reports whether the given host passes the allowlist.
// When the allowlist is empty the function always returns true — callers that
// require an explicit allowlist must check for that separately or configure
// one at startup.
func baseURLHostAllowed(host string) bool {
	host = strings.ToLower(host)
	allowedBaseURLHostsMu.RLock()
	defer allowedBaseURLHostsMu.RUnlock()
	if len(allowedBaseURLHosts) == 0 {
		return true
	}
	_, ok := allowedBaseURLHosts[host]
	return ok
}

func ipInNets(ip net.IP, nets ...*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func isDisallowedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.To4() != nil {
		// IPv4 checks
		return ipInNets(ip, loopback4, linkLocal4, privateA, privateB, privateC, cgNAT, multicast4, reserved4, unspecified4)
	}
	// IPv6 checks
	return ipInNets(ip, loopback6, linkLocal6, uniqueLocal6, multicast6, unspecified6)
}

// ValidateBaseURL verifies that the provided base URL is http(s), has a hostname,
// and does not resolve to loopback, link-local, private, or otherwise disallowed ranges.
// Set AllowLoopback = true in tests to bypass IP checks.
func ValidateBaseURL(raw string) error {
	if AllowLoopback {
		// In test mode, only validate scheme and host presence
		u, err := url.Parse(raw)
		if err != nil {
			return errors.New("invalid base_url: parse failed")
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return errors.New("invalid base_url: scheme must be http or https")
		}
		if u.Hostname() == "" {
			return errors.New("invalid base_url: missing host")
		}
		if !baseURLHostAllowed(u.Hostname()) {
			return errors.New("invalid base_url: host not in provider allowlist")
		}
		return nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid base_url: parse failed")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("invalid base_url: scheme must be http or https")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("invalid base_url: missing host")
	}
	hLower := strings.ToLower(host)
	if _, blocked := blockedHostnames[hLower]; blocked {
		return errors.New("invalid base_url: host not allowed")
	}
	if !baseURLHostAllowed(hLower) {
		return errors.New("invalid base_url: host not in provider allowlist")
	}

	// If host is an IP literal, validate directly; otherwise resolve and validate each address
	if ip := net.ParseIP(host); ip != nil {
		if isDisallowedIP(ip) {
			return errors.New("invalid base_url: destination address not allowed")
		}
		return nil
	}

	// Attempt DNS resolution and validate all returned IPs
	ips, err := net.LookupIP(host)
	if err == nil {
		for _, ip := range ips {
			if isDisallowedIP(ip) {
				return errors.New("invalid base_url: destination resolves to a disallowed address")
			}
		}
	}

	return nil
}
