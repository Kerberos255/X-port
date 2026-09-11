package xray

import "testing"

func TestParseInboundStats(t *testing.T) {
	got, err := ParseInboundStats([]byte(`{"stat":[{"name":"inbound>>>xport-20001-a>>>traffic>>>uplink","value":"10"},{"name":"inbound>>>xport-20001-a>>>traffic>>>downlink","value":20},{"name":"outbound>>>direct>>>traffic>>>downlink","value":"99"}]}`))
	if err != nil { t.Fatal(err) }
	v := got["xport-20001-a"]
	if v.Up != 10 || v.Down != 20 { t.Fatalf("unexpected traffic: %+v", v) }
}
