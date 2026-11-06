package testfixture

import (
	"fmt"
	"net/http"
	"strings"
)

// Synthetic documentation-address providers and a 300,000-entry public-style ruleset.
func benchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /bench/provider/{index}", func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		b.WriteString("proxies:\n")
		for i := 0; i < 200; i++ {
			fmt.Fprintf(&b, "  - {name: Node-%04d, type: ss, server: 192.0.2.10, port: 443, cipher: aes-128-gcm, password: fixture-benchmark-password, udp: true}\n", i)
		}
		_, _ = w.Write([]byte(b.String()))
	})
	mux.HandleFunc("GET /bench/rules.txt", func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		for i := 0; i < 300000; i++ {
			fmt.Fprintf(&b, "+.r%06d.example.invalid\n", i)
		}
		_, _ = w.Write([]byte(b.String()))
	})
}
