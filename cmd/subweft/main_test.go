package main

import "testing"

func TestLocalHTTPIsStrictlyLoopback(t *testing.T) {
	if validateURLs("http://127.0.0.1:8080", "http://127.0.0.1:8081", "127.0.0.1:8080", true) != nil {
		t.Fatal("loopback fixture rejected")
	}
	for _, test := range []struct {
		base, issuer, listen string
		local                bool
	}{{"http://127.0.0.1:8080", "http://127.0.0.1:8081", "0.0.0.0:8080", true}, {"http://example.com", "http://127.0.0.1:8081", "127.0.0.1:8080", true}, {"https://example.com", "http://127.0.0.1:8081", "127.0.0.1:8080", false}, {"https://example.com/path", "https://id.example.com", "127.0.0.1:8080", false}} {
		if validateURLs(test.base, test.issuer, test.listen, test.local) == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
}
