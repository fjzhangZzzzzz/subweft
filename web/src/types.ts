export type Meta = { id: string; revision: number };
export type Node = {
  options?: import("./schema-form").ObjectValue;
  name: string;
  type: string;
  server: string;
  port: number;
  udp?: boolean;
  cipher?: string;
  password?: string;
  username?: string;
  uuid?: string;
  alterId?: number;
  tls?: boolean;
  skipCertVerify?: boolean;
  serverName?: string;
  sni?: string;
  alpn?: string[];
  clientFingerprint?: string;
  network?: string;
  flow?: string;
  realityOpts?: { publicKey: string; shortId: string };
  wsOpts?: {
    path: string;
    headers: Record<string, string>;
    maxEarlyData: number;
    earlyDataHeaderName: string;
  };
  grpcOpts?: { serviceName: string };
};
export type ManualNode = Meta & { node: Node; enabled: boolean };
export type Source = Meta & {
  update?: import("./device-updates").DeviceUpdate;
  legacyProxy?: string;
  migrationRequired?: boolean;
  url?: string;
  clientProvider?: {
    enabled: boolean;
    interval: number;
    proxy?: string;
    healthCheck: {
      enable: boolean;
      url: string;
      interval: number;
      timeout: number;
      lazy: boolean;
      expectedStatus?: string;
    };
  };
  extractNodes?: boolean;
  overrides?: {
    identity: string;
    name: string;
    excluded: boolean;
    notes: string;
  }[];
  fetch?: {
    timeout: number;
    retries: number;
    proxy?: string;
    userAgent?: string;
    headers?: Record<string, string>;
  };
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
  options?: import("./schema-form").ObjectValue;
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
export type Condition = {
  type: string;
  value: string;
  noResolve: boolean;
  children?: Condition[];
};
export type Rule = {
  conditions?: Condition[];
  type: string;
  value: string;
  target: string;
  noResolve: boolean;
  enabled: boolean;
};
export type Settings = {
  options?: import("./schema-form").ObjectValue;
  dns?: import("./schema-form").ObjectValue;
  nameserverPolicy?: { pattern: string; nameservers: string[] }[];
  proxyNameserverPolicy?: { pattern: string; nameservers: string[] }[];
  mixedPort: number;
  allowLan: boolean;
  ipv6: boolean;
  dnsEnabled: boolean;
  dnsMode: string;
  nameservers: string[];
};
export type Profile = Meta & {
  geoDataMode?: boolean;
  geoAutoUpdate?: boolean;
  geoUpdateInterval?: number;
  sourceUpdates?: Record<string, string>;
  ruleUpdates?: Record<string, string>;
  autoPublish?: boolean;
  geoIpId?: string;
  geoAsnId?: string;
  geoSiteId?: string;
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
  build?: { version: string; commit: string; dirty: boolean; platform: string };
  publishStatus?: { id: string; attempt: string; error?: string }[];
  role: "admin" | "user";
  issuer: string;
  users: import("./users").User[];
  resources: import("./resources").Resource[];
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
  allSources: false,
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
    nameservers: [],
  },
});
