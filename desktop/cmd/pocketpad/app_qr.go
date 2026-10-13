package main

import (
	"fmt"
	"net/url"
)

// pairingURI is what the QR code on screen contains: this receiver's address
// and pairing code, so the phone fills its form in with one scan.
//
// Format: pocketpad://pair?h=<host>&p=<port>&c=<code>
func pairingURI(host string, port int, code string) string {
	v := url.Values{}
	v.Set("h", host)
	v.Set("p", fmt.Sprintf("%d", port))
	v.Set("c", code)
	return "pocketpad://pair?" + v.Encode()
}
