package testfixture

const MigrationConfig = `mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
external-controller: 127.0.0.1:9090
dns:
  enable: true
  ipv6: false
  default-nameserver: [1.1.1.1]
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  use-hosts: true
  respect-rules: true
  proxy-server-nameserver: [1.1.1.1]
  nameserver: [https://dns.google/dns-query]
  nameserver-policy:
    '+.example.invalid': [1.1.1.1, 8.8.8.8]
    '*.invalid': 1.1.1.1
  fallback: [https://dns.google/dns-query]
  fallback-filter: {geoip: true, geoip-code: CN, ipcidr: [240.0.0.0/4], domain: ['+.example.invalid']}
proxy-providers:
  primary:
    type: http
    url: https://subscriptions.example.invalid/primary?credential=synthetic-primary-secret
    interval: 3600
    path: ./old/primary.yaml
    override: {additional-prefix: '[primary] '}
    health-check: {enable: true, url: https://www.gstatic.com/generate_204, interval: 300}
  quota:
    type: http
    url: https://subscriptions.example.invalid/quota?credential=synthetic-quota-secret
    interval: 3600
    path: ./old/quota.yaml
    proxy: DIRECT
    override: {additional-prefix: '[quota] '}
    health-check: {enable: true, url: https://www.gstatic.com/generate_204, interval: 300}
proxies:
  - {name: landing, type: ss, server: 192.0.2.1, port: 443, cipher: aes-128-gcm, password: synthetic-landing-password, udp: true}
proxy-groups:
  - {name: Choose, type: select, use: [primary, quota]}
  - {name: Fast, type: url-test, use: [quota], url: https://www.gstatic.com/generate_204, interval: 300}
  - {name: Manual, type: select, use: [quota]}
rule-providers:
  self:
    type: inline
    behavior: domain
    payload: ['+.example.invalid']
  public:
    type: http
    behavior: domain
    url: https://rules.example.invalid/public.yaml
    interval: 86400
    path: ./old/public.yaml
rules:
  - RULE-SET,self,Choose
  - RULE-SET,public,Fast
  - GEOIP,CN,DIRECT
  - MATCH,Fast
`
