// Package upnp maps a TCP listen port through an IGD (Internet Gateway Device).
// It is best-effort: many networks have no UPnP, and failure must not stop the node.
package upnp

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const ssdpAddr = "239.255.255.250:1900"

type Mapping struct {
	External string
	LocalIP  string
	Port     int
	control  string
	service  string
}

// MapTCP discovers an IGD and adds a TCP port mapping. The returned function
// attempts to delete the mapping.
func MapTCP(ctx context.Context, localIP string, port int) (*Mapping, func(), error) {
	if port <= 0 {
		return nil, nil, fmt.Errorf("invalid port")
	}
	loc, err := discover(ctx)
	if err != nil {
		return nil, nil, err
	}
	control, service, err := controlURL(ctx, loc)
	if err != nil {
		return nil, nil, err
	}
	extIP, _ := getExternalIP(ctx, control, service)
	if err := soap(ctx, control, service, "AddPortMapping", map[string]string{
		"NewRemoteHost":             "",
		"NewExternalPort":           fmt.Sprintf("%d", port),
		"NewProtocol":               "TCP",
		"NewInternalPort":           fmt.Sprintf("%d", port),
		"NewInternalClient":         localIP,
		"NewEnabled":                "1",
		"NewPortMappingDescription": "p2pirc",
		"NewLeaseDuration":          "3600",
	}); err != nil {
		return nil, nil, err
	}
	host := extIP
	if host == "" {
		host = "0.0.0.0"
	}
	m := &Mapping{
		External: net.JoinHostPort(host, fmt.Sprintf("%d", port)),
		LocalIP:  localIP,
		Port:     port,
		control:  control,
		service:  service,
	}
	cleanup := func() {
		cctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = soap(cctx, control, service, "DeletePortMapping", map[string]string{
			"NewRemoteHost":   "",
			"NewExternalPort": fmt.Sprintf("%d", port),
			"NewProtocol":     "TCP",
		})
	}
	return m, cleanup, nil
}

func (m *Mapping) Renew(ctx context.Context) error {
	if m == nil {
		return fmt.Errorf("nil mapping")
	}
	return soap(ctx, m.control, m.service, "AddPortMapping", map[string]string{
		"NewRemoteHost":             "",
		"NewExternalPort":           fmt.Sprintf("%d", m.Port),
		"NewProtocol":               "TCP",
		"NewInternalPort":           fmt.Sprintf("%d", m.Port),
		"NewInternalClient":         m.LocalIP,
		"NewEnabled":                "1",
		"NewPortMappingDescription": "p2pirc",
		"NewLeaseDuration":          "3600",
	})
}

func discover(ctx context.Context) (string, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	dst, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return "", err
	}
	req := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 2\r\n" +
		"ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n"
	if _, err := conn.WriteTo([]byte(req), dst); err != nil {
		return "", err
	}
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetReadDeadline(deadline)
	buf := make([]byte, 4096)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		return "", fmt.Errorf("UPnP discover: %w", err)
	}
	loc := parseLocation(string(buf[:n]))
	if loc == "" {
		return "", fmt.Errorf("UPnP discover: no LOCATION")
	}
	return loc, nil
}

var locationRe = regexp.MustCompile(`(?i)LOCATION:\s*(\S+)`)

func parseLocation(resp string) string {
	m := locationRe.FindStringSubmatch(resp)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func controlURL(ctx context.Context, loc string) (control, service string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
	if err != nil {
		return "", "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	control, service = findWANIPControl(loc, body)
	if control == "" {
		return "", "", fmt.Errorf("no WANIPConnection control URL")
	}
	return control, service, nil
}

func findWANIPControl(base string, xmlBody []byte) (control, service string) {
	raw := string(xmlBody)
	for _, svc := range []string{
		"urn:schemas-upnp-org:service:WANIPConnection:1",
		"urn:schemas-upnp-org:service:WANIPConnection:2",
		"urn:schemas-upnp-org:service:WANPPPConnection:1",
	} {
		idx := strings.Index(raw, svc)
		if idx < 0 {
			continue
		}
		chunk := raw[idx:]
		if end := strings.Index(chunk, "</service>"); end > 0 {
			chunk = chunk[:end]
		}
		cu := extractTag(chunk, "controlURL")
		if cu == "" {
			continue
		}
		return resolveURL(base, cu), svc
	}
	return "", ""
}

func extractTag(s, tag string) string {
	re := regexp.MustCompile(`(?s)<` + tag + `[^>]*>\s*([^<]+)\s*</` + tag + `>`)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func resolveURL(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	u := base
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			u = u[:i+3+j]
		}
	}
	if !strings.HasPrefix(ref, "/") {
		ref = "/" + ref
	}
	return u + ref
}

func getExternalIP(ctx context.Context, control, service string) (string, error) {
	body, err := soapRaw(ctx, control, service, "GetExternalIPAddress", nil)
	if err != nil {
		return "", err
	}
	return extractTag(body, "NewExternalIPAddress"), nil
}

func soap(ctx context.Context, control, service, action string, args map[string]string) error {
	_, err := soapRaw(ctx, control, service, action, args)
	return err
}

func soapRaw(ctx context.Context, control, service, action string, args map[string]string) (string, error) {
	var inner bytes.Buffer
	for k, v := range args {
		fmt.Fprintf(&inner, "<%s>%s</%s>", k, xmlEscape(v), k)
	}
	envelope := `<?xml version="1.0"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
		`<s:Body><u:` + action + ` xmlns:u="` + service + `">` + inner.String() +
		`</u:` + action + `></s:Body></s:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, control, strings.NewReader(envelope))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+service+"#"+action+`"`)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return string(b), fmt.Errorf("UPnP %s: HTTP %d", action, res.StatusCode)
	}
	return string(b), nil
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
