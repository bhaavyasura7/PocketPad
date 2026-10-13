package main

import (
	"net/url"
	"testing"
)

func TestPairingURI(t *testing.T) {
	uri := pairingURI("192.168.1.20", 47770, "AVBB-QGJC-GNVM-T9DT-G7EM-VKR7")
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "pocketpad" || u.Host != "pair" {
		t.Fatalf("scheme/host: %s://%s", u.Scheme, u.Host)
	}
	q := u.Query()
	if q.Get("h") != "192.168.1.20" {
		t.Fatalf("host %q", q.Get("h"))
	}
	if q.Get("p") != "47770" {
		t.Fatalf("port %q", q.Get("p"))
	}
	if q.Get("c") != "AVBB-QGJC-GNVM-T9DT-G7EM-VKR7" {
		t.Fatalf("code %q", q.Get("c"))
	}
}
