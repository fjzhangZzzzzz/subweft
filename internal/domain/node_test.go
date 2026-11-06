package domain_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
)

func TestManagementJSONRejectsProtocolFieldsAndNestedCoercion(t *testing.T) {
	for _, data := range []string{
		`{"name":"n","type":"http","server":"192.0.2.1","port":443,"uuid":""}`,
		`{"name":"n","type":"ss","server":"192.0.2.1","port":443,"tls":false}`,
		`{"name":"n","type":"vmess","server":"192.0.2.1","port":443,"wsOpts":{"headers":{"Host":null}}}`,
		`{"name":"n","type":"vless","server":"192.0.2.1","port":443,"grpcOpts":{"serviceName":123}}`,
		`{"name":"n","type":"vless","server":"192.0.2.1","port":443,"realityOpts":{"publicKey":"key","shortId":"","unknown":true}}`,
		`{"name":"n","type":"ss","server":"192.0.2.1","port":"443"}`,
	} {
		var value domain.Node
		if json.Unmarshal([]byte(data), &value) == nil {
			t.Fatal("invalid node JSON accepted")
		}
	}
}

func TestRequiredZeroAndExplicitEmptyALPNSurviveStorageAndRendering(t *testing.T) {
	empty := []string{}
	node := domain.Node{Name: "fixture", Type: "vmess", Server: "192.0.2.1", Port: 443, UUID: "00000000-0000-4000-8000-000000000001", Cipher: "auto", TLS: true, ALPN: &empty}
	encoded, e := json.Marshal(node)
	if e != nil {
		t.Fatal(e)
	}
	var restored domain.Node
	if e = json.Unmarshal(encoded, &restored); e != nil {
		t.Fatal(e)
	}
	if restored.ALPN == nil || len(*restored.ALPN) != 0 {
		t.Fatal("explicit empty ALPN was lost")
	}
	rendered, e := yaml.Marshal(restored)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(rendered, []byte("alterId: 0")) || !bytes.Contains(rendered, []byte("alpn: []")) {
		t.Fatal("required zero or empty-list semantics changed")
	}
	restored.ALPN = nil
	rendered, e = yaml.Marshal(restored)
	if e != nil || bytes.Contains(rendered, []byte("alpn:")) {
		t.Fatal("omitted ALPN became an explicit empty list")
	}
}
func TestProtocolConstraints(t *testing.T) {
	base := domain.Node{Name: "fixture", Type: "vless", Server: "192.0.2.1", Port: 443, UUID: "00000000-0000-4000-8000-000000000001"}
	cases := map[string]func(*domain.Node){
		"uuid":              func(n *domain.Node) { n.UUID = "bad" },
		"TLS required":      func(n *domain.Node) { n.SkipCertVerify = true },
		"transport":         func(n *domain.Node) { n.Network = "tcp"; n.WS = &domain.WSOptions{Path: "/ws"} },
		"flow requires TCP": func(n *domain.Node) { n.TLS = true; n.Flow = "xtls-rprx-vision"; n.Network = "ws" },
		"header injection": func(n *domain.Node) {
			n.Network = "ws"
			n.WS = &domain.WSOptions{Headers: map[string]string{"Host": "a\r\nb"}}
		},
		"invalid public key": func(n *domain.Node) {
			n.TLS = true
			n.ServerName = "example.invalid"
			n.ClientFingerprint = "chrome"
			n.Reality = &domain.RealityOptions{PublicKey: "bad", ShortID: "abc"}
		},
		"credentials pair": func(n *domain.Node) {
			*n = domain.Node{Name: "fixture", Type: "socks5", Server: "192.0.2.1", Port: 443, Username: "user"}
		},
	}
	if e := domain.ValidateNode(base); e != nil {
		t.Fatal("valid baseline rejected", e)
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			value := base
			modify(&value)
			if domain.ValidateNode(value) == nil {
				t.Fatal("invalid protocol combination accepted")
			}
		})
	}
}
