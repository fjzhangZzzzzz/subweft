export type Meta = { id: string; revision: number };
export type Node = {
  name: string;
  type: string;
  server: string;
  port: number;
  cipher: string;
  password: string;
  udp: boolean;
};
export type ManualNode = Meta & { node: Node; enabled: boolean };
export type Source = Meta & {
  name: string;
  kind: "http" | "local";
  enabled: boolean;
  tags: string[];
  interval: number;
  nodeCount: number;
  lastSuccess: string;
  lastAttempt: string;
  lastError: string;
};
export type Group = {
  name: string;
  type: string;
  sourceIds: string[];
  tags: string[];
  allSources: boolean;
  nodeIds: string[];
  groups: string[];
  builtins: string[];
  filter: string;
  exclude: string;
  url: string;
  interval: number;
  tolerance: number;
  strategy: string;
};
export type Rule = {
  type: string;
  value: string;
  target: string;
  noResolve: boolean;
  enabled: boolean;
};
export type Settings = {
  mixedPort: number;
  allowLan: boolean;
  ipv6: boolean;
  dnsEnabled: boolean;
  dnsMode: string;
  nameservers: string[];
};
export type Profile = Meta & {
  name: string;
  groups: Group[];
  rules: Rule[];
  settings: Settings;
};
export type Device = Meta & {
  name: string;
  profileId: string;
  state: string;
  owner?: string;
  expiresAt?: string | null;
};
export type Release = Meta & {
  profileId: string;
  profileRevision: number;
  createdAt: string;
  digest: string;
  validator: string;
};
export type State = {
  user: string;
  csrf: string;
  sources: Source[];
  nodes: ManualNode[];
  profiles: Profile[];
  devices: Device[];
  releases: Release[];
  active: Record<string, string>;
  seen: Record<string, string>;
  validator: string;
  capabilities: {
    protocols: string[];
    groups: string[];
    automaticPublish: boolean;
  };
};
export const emptyGroup = (name = "Proxy"): Group => ({
  name,
  type: "select",
  sourceIds: [],
  tags: [],
  allSources: true,
  nodeIds: [],
  groups: [],
  builtins: [],
  filter: "",
  exclude: "",
  url: "https://www.gstatic.com/generate_204",
  interval: 300,
  tolerance: 50,
  strategy: "consistent-hashing",
});
export const emptyProfile = (name: string): Profile => ({
  id: "",
  revision: 0,
  name,
  groups: [emptyGroup()],
  rules: [
    {
      type: "MATCH",
      value: "",
      target: "Proxy",
      enabled: true,
      noResolve: false,
    },
  ],
  settings: {
    mixedPort: 7890,
    allowLan: false,
    ipv6: false,
    dnsEnabled: false,
    dnsMode: "fake-ip",
    nameservers: ["https://dns.google/dns-query"],
  },
});
