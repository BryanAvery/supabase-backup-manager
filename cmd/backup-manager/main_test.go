package main

import "testing"

func TestValidateListen(t *testing.T) {
	cases := []struct {
		name    string
		address string
		allowed bool
	}{
		{"IPv4 loopback", "127.0.0.1:8765", true},
		{"IPv6 loopback", "[::1]:8765", true},
		{"all IPv4 interfaces", "0.0.0.0:8765", false},
		{"all IPv6 interfaces", "[::]:8765", false},
		{"remote address", "192.0.2.5:8765", false},
		{"hostname rejected", "localhost:8765", false},
		{"missing host", ":8765", false},
		{"missing port", "127.0.0.1:", false},
		{"port zero", "127.0.0.1:0", false},
		{"high port", "127.0.0.1:65536", false},
		{"malformed", "not-an-address", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateListen(tc.address) == nil; got != tc.allowed {
				t.Errorf("validateListen(%q) allowed=%t, want %t", tc.address, got, tc.allowed)
			}
		})
	}
}
