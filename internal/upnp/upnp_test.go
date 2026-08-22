package upnp

import "testing"

func TestParseLocation(t *testing.T) {
	resp := "HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=120\r\nLOCATION: http://192.168.1.1:5000/rootDesc.xml\r\nST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n"
	got := parseLocation(resp)
	if got != "http://192.168.1.1:5000/rootDesc.xml" {
		t.Fatalf("got %q", got)
	}
}

func TestFindWANIPControl(t *testing.T) {
	xml := `
<root>
  <device>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>
        <controlURL>/ctl/IPConn</controlURL>
      </service>
    </serviceList>
  </device>
</root>`
	c, s := findWANIPControl("http://192.168.1.1:5000/root.xml", []byte(xml))
	if s != "urn:schemas-upnp-org:service:WANIPConnection:1" {
		t.Fatalf("service %q", s)
	}
	if c != "http://192.168.1.1:5000/ctl/IPConn" {
		t.Fatalf("control %q", c)
	}
}

func TestResolveURL(t *testing.T) {
	if got := resolveURL("http://192.168.1.1:5000/root.xml", "/x"); got != "http://192.168.1.1:5000/x" {
		t.Fatal(got)
	}
}
