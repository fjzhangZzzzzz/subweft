// Development fixture only. This command is not included in product builds.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:18881", "loopback listener")
	base := flag.String("base-url", "http://127.0.0.1:18880", "local application base URL")
	flag.Parse()
	host, _, e := net.SplitHostPort(*listen)
	u, err := url.Parse(*base)
	if e != nil || err != nil || !loopback(host) || !loopback(u.Hostname()) || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" {
		fmt.Fprintln(os.Stderr, "fixture_requires_loopback_http")
		os.Exit(1)
	}
	issuer, e := testfixture.New("http://"+*listen, *base+"/auth/callback", testfixture.Subject)
	if e != nil {
		os.Exit(1)
	}
	fmt.Println("Loopback-only development OIDC and source fixture ready. No real identity or proxy traffic.")
	server := http.Server{Addr: *listen, Handler: issuer.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	if server.ListenAndServe() != nil {
		os.Exit(1)
	}
}
func loopback(s string) bool { ip := net.ParseIP(s); return ip != nil && ip.IsLoopback() }
