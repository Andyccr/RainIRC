// Package netutil lists local network addresses without extra dependencies.
package netutil

import (
	"net"
	"strconv"
	"strings"
)

// PrivateIPv4 returns non-loopback IPv4 addresses on up interfaces.
func PrivateIPv4() []net.IP {
	ifaces, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, a := range ifaces {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.IsLoopback() {
			continue
		}
		ip := ipn.IP.To4()
		if ip == nil {
			continue
		}
		out = append(out, ip)
	}
	return out
}

func Join(ip net.IP, port int) string {
	if ip == nil {
		return ""
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(port))
}

// LocalCandidates is loopback plus private IPv4s on the listen port.
func LocalCandidates(port int) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	for _, ip := range PrivateIPv4() {
		add(Join(ip, port))
	}
	return out
}

func Unique(addrs []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, a := range addrs {
		if a == "" {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	return out
}

// AdvertiseCandidates is private IPv4 listen addresses (no loopback).
// These are the unsigned TCP hints put on hello/welcome.
func AdvertiseCandidates(port int) []string {
	var out []string
	for _, ip := range PrivateIPv4() {
		out = append(out, Join(ip, port))
	}
	return Unique(out)
}

// SanitizeAddrs keeps well-formed host:port strings, capped at max.
func SanitizeAddrs(in []string, max int) []string {
	if max <= 0 {
		max = 8
	}
	seen := map[string]struct{}{}
	var out []string
	for _, a := range in {
		a = strings.TrimSpace(a)
		if a == "" || len(a) > 128 {
			continue
		}
		host, port, err := net.SplitHostPort(a)
		if err != nil || host == "" {
			continue
		}
		p, err := strconv.Atoi(port)
		if err != nil || p <= 0 || p > 65535 {
			continue
		}
		canon := net.JoinHostPort(host, strconv.Itoa(p))
		if _, ok := seen[canon]; ok {
			continue
		}
		seen[canon] = struct{}{}
		out = append(out, canon)
		if len(out) >= max {
			break
		}
	}
	return out
}
