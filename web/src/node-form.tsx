import { useState, useEffect, useRef } from "react";
import type { ManualNode, Node } from "./types";
import { Button, Field, CheckField, ModalTitle } from "./ui";
import type { T } from "./ui";
import { useCatalog, ObjectForm, defaultValue } from "./schema-form";
import type { Schema, Value, ObjectValue } from "./schema-form";
const keys: Record<string, string> = {
  skipCertVerify: "skip-cert-verify",
  serverName: "servername",
  clientFingerprint: "client-fingerprint",
  realityOpts: "reality-opts",
  wsOpts: "ws-opts",
  grpcOpts: "grpc-opts",
  publicKey: "public-key",
  shortId: "short-id",
  maxEarlyData: "max-early-data",
  earlyDataHeaderName: "early-data-header-name",
  serviceName: "grpc-service-name",
};
function core(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(core);
  if (value && typeof value === "object")
    return Object.fromEntries(
      Object.entries(value).map(([k, v]) => [keys[k] || k, core(v)]),
    );
  return value;
}
function fromNode(node?: Node): ObjectValue {
  if (!node)
    return {
      name: "",
      type: "ss",
      server: "",
      port: 443,
      cipher: "aes-128-gcm",
      password: "",
    };
  const { options, ...rest } = node;
  return { ...(core(rest) as ObjectValue), ...options };
}
export function NodeForm({
  value,
  protocols,
  t,
  onSave,
}: {
  value?: ManualNode;
  protocols: string[];
  t: T;
  onSave: (v: unknown) => unknown;
}) {
  const { catalog, error } = useCatalog();
  const [node, setNode] = useState<ObjectValue>(() => fromNode(value?.node));
  const [issues, setIssues] = useState<string[]>([]);
  const summaryRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (issues.length) summaryRef.current?.focus();
  }, [issues]);
  const [enabled, setEnabled] = useState(value?.enabled ?? true);
  const type = String(node.type);
  let schema = catalog?.protocols[type];
  if (
    schema &&
    type === "ss" &&
    node.plugin &&
    catalog?.plugins[String(node.plugin)]
  )
    schema = {
      ...schema,
      properties: {
        ...schema.properties,
        "plugin-opts": catalog.plugins[String(node.plugin)],
      },
    };
  const account = ["http", "socks5"].includes(type);
  const accountEnabled =
    node.username !== undefined || node.password !== undefined;
  if (schema) {
    const properties = { ...schema.properties };
    const required = (key: string, yes = true) => {
      if (properties[key])
        properties[key] = { ...properties[key], required: yes };
    };
    if (account && accountEnabled) {
      required("username");
      required("password");
    }
    if (type === "tuic") {
      if (node.token !== undefined) required("token");
      else {
        required("uuid");
        required("password");
      }
    }
    if (type === "wireguard") {
      required("private-key");
      if (!node.ipv6) required("ip");
      if (!Array.isArray(node.peers) || node.peers.length === 0) {
        required("server");
        required("port");
        required("public-key");
      }
    }
    if (type === "hysteria") {
      required("up", !node["up-speed"]);
      required("down", !node["down-speed"]);
    }
    schema = { ...schema, properties };
  }
  const changeProtocol = (kind: string) => {
    const n: ObjectValue = { name: node.name, type: kind };
    const fields = catalog!.protocols[kind].properties!;
    for (const [k, s] of Object.entries(fields)) {
      if (s.required && k !== "type" && k !== "name")
        n[k] =
          k === "server"
            ? node.server || ""
            : k === "port"
              ? node.port || 443
              : defaultValue(s);
    }
    if (kind === "ss") n.cipher = "aes-128-gcm";
    if (kind === "ssr") n.cipher = "aes-128-cfb";
    if (kind === "snell") n.version = 3;
    if (kind === "tuic") {
      n.uuid = "";
      n.password = "";
    }
    if (kind === "hysteria") {
      n.up = "";
      n.down = "";
      n["auth-str"] = "";
    }
    if (kind === "hysteria2") n.password = "";
    if (kind === "vmess") {
      n.cipher = "auto";
      n.alterId = 0;
    }
    for (const [key, value] of Object.entries({
      server: node.server || "",
      port: kind === "wireguard" ? 51820 : node.port || 443,
      udp: true,
      ...(["vmess", "vless", "http", "socks5"].includes(kind)
        ? { tls: false }
        : {}),
      ...(["trojan", "hysteria", "hysteria2", "tuic", "anytls"].includes(kind)
        ? { sni: "" }
        : {}),
      ...(["vmess", "vless", "trojan"].includes(kind)
        ? { network: "tcp" }
        : {}),
      ...(kind === "wireguard"
        ? { ip: "", "public-key": "", "allowed-ips": [] }
        : {}),
      ...(kind === "ssr" ? { obfs: "plain", protocol: "origin" } : {}),
    })) {
      if (fields[key]) n[key] = value;
    }
    setIssues([]);
    setNode(n);
  };
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        const missing: string[] = [];
        const retained = value?.node.type === type;
        const walk = (s: Schema, v: Value | undefined, path: string) => {
          if (
            s.required &&
            (v === undefined || v === "") &&
            !(
              retained &&
              ((path === "password" &&
                type !== "tuic" &&
                !!value?.node.password &&
                (!["http", "socks5"].includes(type) ||
                  node.username === value?.node.username)) ||
                (path === "uuid" &&
                  ["vmess", "vless"].includes(type) &&
                  !!value?.node.uuid))
            )
          )
            missing.push(path);
          if (
            v &&
            s.kind === "object" &&
            typeof v === "object" &&
            !Array.isArray(v)
          ) {
            for (const [k, field] of Object.entries(s.properties || {}))
              walk(field, (v as ObjectValue)[k], path ? path + "." + k : k);
          }
        };
        if (schema) walk(schema, node, "");
        if (
          ["http", "socks5"].includes(type) &&
          node.username &&
          !node.password &&
          node.username !== value?.node.username
        )
          missing.push("password");
        if (missing.length) {
          setIssues(missing);
          return;
        }
        const cleaned = { ...node };
        if (type === "tuic") {
          if (node.token !== undefined) {
            delete cleaned.uuid;
            delete cleaned.password;
          } else delete cleaned.token;
        }
        const { name, type: nodeType, server, port, ...options } = cleaned;
        onSave({
          id: value?.id || "",
          revision: value?.revision || 0,
          node: {
            name,
            type: nodeType,
            ...(server !== undefined ? { server } : {}),
            ...(port !== undefined ? { port } : {}),
            options,
          },
          enabled,
        });
      }}
    >
      <ModalTitle
        title={
          value
            ? t("编辑手工节点", "Edit manual node")
            : t("添加手工节点", "Add manual node")
        }
      />
      <p className="muted">
        {t(
          "所有参数均为固定内核版本的结构化字段。更换协议会清除原凭据与高级参数。",
          "All parameters are structured fields for the pinned core version. Changing protocol clears credentials and advanced options.",
        )}
      </p>
      {issues.length > 0 && (
        <div
          role="alert"
          tabIndex={-1}
          ref={summaryRef}
          className="alert error"
        >
          <strong>{t("请补全必填字段", "Complete required fields")}</strong>
          <ul>
            {issues.map((path) => (
              <li key={path}>
                <button
                  type="button"
                  className="text-button"
                  onClick={() =>
                    document
                      .querySelector<HTMLElement>(
                        '[data-field="' +
                          CSS.escape(path) +
                          '"] :is(input,select,textarea)',
                      )
                      ?.focus()
                  }
                >
                  {path}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
      <Field name="type" label={t("协议", "Protocol")}>
        <select
          value={type}
          disabled={!catalog}
          onChange={(e) => changeProtocol(e.target.value)}
        >
          {protocols.map((p) => (
            <option key={p}>{p}</option>
          ))}
        </select>
      </Field>
      {account && (
        <CheckField
          label={t("使用账号密码", "Use account credentials")}
          checked={accountEnabled}
          onChange={(yes) => {
            const next = { ...node };
            if (yes) {
              next.username = "";
              next.password = "";
            } else {
              delete next.username;
              delete next.password;
            }
            setNode(next);
            setIssues([]);
          }}
        />
      )}
      {type === "tuic" && (
        <Field label={t("认证方式", "Authentication mode")}>
          <select
            value={node.token !== undefined ? "token" : "uuid"}
            onChange={(e) => {
              const next = { ...node };
              delete next.token;
              delete next.uuid;
              delete next.password;
              if (e.target.value === "token") next.token = "";
              else {
                next.uuid = "";
                next.password = "";
              }
              setNode(next);
              setIssues([]);
            }}
          >
            <option value="uuid">UUID + password (TUIC v5)</option>
            <option value="token">Token (TUIC v4)</option>
          </select>
        </Field>
      )}
      {error ? (
        <p role="alert">
          {t(
            "字段目录载入失败，请重新打开。",
            "Field catalog failed to load. Reopen this form.",
          )}
        </p>
      ) : schema ? (
        <ObjectForm
          key={type}
          schema={schema}
          value={node}
          exclude={[
            "type",
            ...(type === "tuic"
              ? node.token !== undefined
                ? ["uuid", "password"]
                : ["token"]
              : []),
            ...(account && !accountEnabled ? ["username", "password"] : []),
          ]}
          t={t}
          onChange={(next) => {
            setIssues([]);
            if (
              ["http", "socks5"].includes(type) &&
              next.username !== node.username
            )
              next.password = "";
            if (type === "ss" && next.plugin !== node.plugin) {
              if (next.plugin && catalog?.plugins[String(next.plugin)])
                next["plugin-opts"] = defaultValue(
                  catalog.plugins[String(next.plugin)],
                );
              else delete next["plugin-opts"];
            }
            if (
              next.network !== node.network &&
              [
                "tcp",
                "ws",
                "grpc",
                "h2",
                "http",
                "xhttp",
                "mkcp",
                "mekya",
                "tlsmirror",
              ].includes(String(next.network))
            ) {
              for (const key of [
                "ws-opts",
                "grpc-opts",
                "h2-opts",
                "http-opts",
                "xhttp-opts",
                "mkcp-opts",
                "mekya-opts",
                "tlsmirror-opts",
              ]) {
                if (key !== String(next.network) + "-opts") delete next[key];
              }
              const key = String(next.network) + "-opts";
              if (schema?.properties?.[key] && next[key] === undefined)
                next[key] =
                  next.network === "ws"
                    ? { path: "/" }
                    : next.network === "grpc"
                      ? { "grpc-service-name": "" }
                      : defaultValue(schema.properties[key]);
            }
            setNode(next);
          }}
        />
      ) : (
        <p>{t("正在载入字段…", "Loading fields…")}</p>
      )}
      <CheckField
        label={t("启用节点", "Enable node")}
        checked={enabled}
        onChange={setEnabled}
      />
      <div className="form-footer">
        <Button primary type="submit" disabled={!catalog}>
          {t("保存节点", "Save node")}
        </Button>
      </div>
    </form>
  );
}
