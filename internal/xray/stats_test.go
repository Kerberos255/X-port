package xray

import (
	"encoding/json"
	"testing"
)

func TestParseInboundStats(t *testing.T) {
	got, err := ParseInboundStats([]byte(`{"stat":[{"name":"inbound>>>xport-20001-a>>>traffic>>>uplink","value":"10"},{"name":"inbound>>>xport-20001-a>>>traffic>>>downlink","value":20},{"name":"outbound>>>direct>>>traffic>>>downlink","value":"99"}]}`))
	if err != nil { t.Fatal(err) }
	v := got["xport-20001-a"]
	if v.Up != 10 || v.Down != 20 { t.Fatalf("unexpected traffic: %+v", v) }
}
// Xray statsquery omits "value" for zero-valued counters. Those entries must
// not prevent the nonzero counters in the same batch from being persisted.
func TestParseInboundStatsOmittedZeroCounters(t *testing.T) {
	got, err := ParseInboundStats([]byte(`{"stat":[
		{"name":"inbound>>>inbound-10001>>>traffic>>>uplink"},
		{"name":"inbound>>>xport-20001-alpha>>>traffic>>>uplink"},
		{"name":"inbound>>>xport-20002-beta>>>traffic>>>downlink","value":141},
		{"name":"inbound>>>xport-20002-beta>>>traffic>>>uplink","value":279},
		{"name":"inbound>>>xport-20003-gamma>>>traffic>>>downlink","value":"201"},
		{"name":"inbound>>>xport-20003-gamma>>>traffic>>>uplink","value":"282"},
		{"name":"inbound>>>xport-20004-delta>>>traffic>>>downlink","value":null},
		{"name":"inbound>>>xport-20004-delta>>>traffic>>>uplink","value":4},
		{"name":"outbound>>>direct>>>traffic>>>downlink","value":9999},
		{"name":"inbound>>>xport-20005-unused>>>traffic>>>unknown","value":"not-a-number"}
	]}`))
	if err!=nil {t.Fatal(err)}
	want := map[string]Traffic{
		"inbound-10001": {},
		"xport-20001-alpha": {},
		"xport-20002-beta": {Up:279, Down:141},
		"xport-20003-gamma": {Up:282, Down:201},
		"xport-20004-delta": {Up:4},
	}
	if len(got)!=len(want) {t.Fatalf("unexpected inbound count: got=%d want=%d",len(got),len(want))}
	for tag, expected := range want {
		if got[tag]!=expected {t.Errorf("%s: got=%+v want=%+v",tag,got[tag],expected)}
	}
}

func TestParseStatValueZeroRepresentationsAndErrors(t *testing.T) {
	for _,raw := range []json.RawMessage{nil, []byte("null"), []byte("0"), []byte(`"0"`), []byte(`""`)} {
		got,err := parseStatValue(raw)
		if err!=nil||got!=0 {t.Errorf("%q: got=%d err=%v",raw,got,err)}
	}
	for _,raw := range []json.RawMessage{[]byte("{}"), []byte(`"not-a-number"`)} {
		if _,err:=parseStatValue(raw);err==nil {t.Errorf("expected malformed counter %q to fail",raw)}
	}
}
