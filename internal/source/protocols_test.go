package source_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
)

func TestProtocolYAMLAndBase64PreserveTypedOptions(t *testing.T) {
	for _, input := range []string{testfixture.ProtocolProviderYAML, base64.StdEncoding.EncodeToString([]byte(testfixture.ProtocolProviderYAML))} {
		nodes, e := source.Parse([]byte(input))
		if e != nil || len(nodes) != 8 {
			t.Fatal("protocol provider rejected", e)
		}
		if nodes[0].WS.Headers["Authorization"] != "fixture-private-header" || nodes[0].WS.MaxEarlyData != 1024 || nodes[1].Reality.ShortID != "0123456789abcdef" || nodes[2].GRPC.ServiceName != "fixture-private-service" || !nodes[7].TLS {
			t.Fatal("typed options changed")
		}
	}
	for name, input := range map[string]string{
		"coerced uuid":            strings.Replace(testfixture.ProtocolProviderYAML, "uuid: 00000000-0000-4000-8000-000000000001", "uuid: 123", 1),
		"coerced nested header":   strings.Replace(testfixture.ProtocolProviderYAML, "Authorization: fixture-private-header", "Authorization: 123", 1),
		"unknown nested field":    strings.Replace(testfixture.ProtocolProviderYAML, "max-early-data: 1024", "unknown: 1024", 1),
		"wrong transport options": strings.Replace(testfixture.ProtocolProviderYAML, "network: ws", "network: grpc", 1),
		"reality without tls":     strings.Replace(testfixture.ProtocolProviderYAML, "    flow: xtls-rprx-vision", "    tls: false\n    flow: xtls-rprx-vision", 1),
		"wrong protocol field":    strings.Replace(testfixture.ProviderYAML, "udp: true", "tls: false", 1),
		"duplicate field":         strings.Replace(testfixture.ProtocolProviderYAML, "    alterId: 0", "    alterId: 0\n    alterId: 1", 1),
		"case duplicate header":   strings.Replace(testfixture.ProtocolProviderYAML, "Authorization: fixture-private-header", "Authorization: fixture-private-header\n        host: duplicate.invalid", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := source.Parse([]byte(input)); e == nil {
				t.Fatal("unsupported or mistyped options accepted")
			}
		})
	}
}

func TestMixedURIFormatsAndUnknownParameters(t *testing.T) {
	vmess := `{"v":"2","ps":"VMess","add":"192.0.2.1","port":"443","id":"00000000-0000-4000-8000-000000000001","aid":"0","scy":"auto","net":"ws","type":"none","host":"proxy.example.invalid","path":"/ws","tls":"tls","sni":"proxy.example.invalid"}`
	lines := []string{
		"ss://YWVzLTEyOC1nY206Zml4dHVyZQ@192.0.2.1:443#SS",
		"vmess://" + base64.StdEncoding.EncodeToString([]byte(vmess)),
		"vless://00000000-0000-4000-8000-000000000002@192.0.2.2:443?security=tls&type=grpc&serviceName=fixture&sni=proxy.example.invalid#VLESS",
		"trojan://fixture-password@192.0.2.3:443?security=tls&sni=proxy.example.invalid#Trojan",
		"https://fixture-user:fixture-password@192.0.2.4:443#HTTP",
		"socks5://fixture-user:fixture-password@[2001:db8::1]:1080#SOCKS5",
	}
	input := strings.Join(lines, "\n")
	for _, encoded := range []string{input, base64.RawURLEncoding.EncodeToString([]byte(input))} {
		nodes, e := source.Parse([]byte(encoded))
		if e != nil || len(nodes) != 6 {
			t.Fatal("mixed URI list rejected", e)
		}
		if nodes[1].UUID == "" || nodes[1].WS.Path != "/ws" || nodes[2].GRPC.ServiceName != "fixture" || nodes[4].Type != "http" || !nodes[4].TLS || nodes[5].Server != "2001:db8::1" {
			t.Fatal("URI semantics changed")
		}
	}
	for _, invalid := range []string{strings.Replace(lines[2], "#VLESS", "&mystery=yes#VLESS", 1), strings.Replace(lines[2], "type=grpc", "type=xhttp", 1), strings.Replace(lines[2], "security=tls", "security=unknown", 1), strings.Replace(lines[2], "type=grpc", "type=grpc&type=ws", 1), "vmess://" + base64.StdEncoding.EncodeToString([]byte(strings.Replace(vmess, `"aid":"0"`, `"aid":null`, 1))), strings.Replace(lines[3], "security=tls", "security=tls&plugin=unknown", 1)} {
		if _, e := source.Parse([]byte(invalid)); e == nil {
			t.Fatal("invalid URI silently accepted")
		}
	}
}
