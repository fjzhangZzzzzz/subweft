package testfixture

// Documentation addresses and public test values only; no live proxy endpoints.
const ProtocolProviderYAML = `proxies:
  - name: VMess-WS
    type: vmess
    server: 192.0.2.10
    port: 443
    uuid: 00000000-0000-4000-8000-000000000001
    alterId: 0
    cipher: auto
    udp: true
    tls: true
    servername: proxy.example.invalid
    network: ws
    alpn: [http/1.1]
    ws-opts:
      path: /fixture-private-path
      headers:
        Host: proxy.example.invalid
        Authorization: fixture-private-header
      max-early-data: 1024
      early-data-header-name: Sec-WebSocket-Protocol
  - name: VLESS-Reality
    type: vless
    server: 192.0.2.11
    port: 443
    uuid: 00000000-0000-4000-8000-000000000002
    tls: true
    servername: proxy.example.invalid
    client-fingerprint: chrome
    flow: xtls-rprx-vision
    reality-opts:
      public-key: hSDwCYkwp1R0i33ctD73Wg2_Og0mOBr066SpjqqbTmo
      short-id: "0123456789abcdef"
  - name: VLESS-gRPC
    type: vless
    server: 192.0.2.12
    port: 443
    uuid: 00000000-0000-4000-8000-000000000003
    tls: true
    servername: proxy.example.invalid
    network: grpc
    grpc-opts:
      grpc-service-name: fixture-private-service
  - name: Trojan-TCP
    type: trojan
    server: 192.0.2.13
    port: 443
    password: fixture-trojan-password
    sni: proxy.example.invalid
  - name: Trojan-WS
    type: trojan
    server: 192.0.2.14
    port: 443
    password: fixture-trojan-ws-password
    sni: proxy.example.invalid
    network: ws
    ws-opts:
      path: /fixture-private-path
  - name: Trojan-gRPC
    type: trojan
    server: 192.0.2.15
    port: 443
    password: fixture-trojan-grpc-password
    sni: proxy.example.invalid
    network: grpc
    grpc-opts:
      grpc-service-name: fixture-private-service
  - name: HTTP-TLS
    type: http
    server: 192.0.2.16
    port: 443
    username: fixture-http-user
    password: fixture-http-password
    tls: true
    sni: proxy.example.invalid
  - name: SOCKS5-TLS
    type: socks5
    server: 192.0.2.17
    port: 443
    username: fixture-socks-user
    password: fixture-socks-password
    tls: true
    udp: true
`
