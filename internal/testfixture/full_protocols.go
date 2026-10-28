package testfixture

// All values are synthetic documentation fixtures, never live credentials.
const RemainingProtocolProviderYAML = `proxies:
  - {name: SSR, type: ssr, server: 192.0.2.1, port: 443, cipher: aes-128-cfb, password: fixture, obfs: plain, protocol: origin}
  - {name: Hysteria, type: hysteria, server: 192.0.2.1, port: 443, up: 10 Mbps, down: 50 Mbps, auth-str: fixture, sni: example.invalid}
  - {name: Hysteria2, type: hysteria2, server: 192.0.2.1, port: 443, password: fixture, sni: example.invalid, obfs: salamander, obfs-password: fixture-obfs}
  - {name: TUIC, type: tuic, server: 192.0.2.1, port: 443, uuid: 00000000-0000-4000-8000-000000000003, password: fixture, sni: example.invalid, alpn: [h3], congestion-controller: bbr, udp-relay-mode: native}
  - {name: WireGuard, type: wireguard, server: 192.0.2.1, port: 51820, ip: 10.0.0.2, private-key: AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=, public-key: XkNgOvP3x6fQGmXWjGa6Rc6xeKPzwdv2KC83ZV7U3zE=, allowed-ips: [0.0.0.0/0], reserved: [1, 2, 3], mtu: 1420, udp: true}
  - {name: Snell, type: snell, server: 192.0.2.1, port: 443, psk: fixture-snell-secret, version: 3, udp: true, obfs-opts: {mode: tls, host: example.invalid}}
  - {name: AnyTLS, type: anytls, server: 192.0.2.1, port: 443, password: fixture, sni: example.invalid, alpn: [h2, http/1.1], udp: true}
`
