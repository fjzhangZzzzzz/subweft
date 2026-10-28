import { useState } from "react";
import type { ManualNode, Node } from "./types";
import { Button, Field, CheckField, ModalTitle } from "./ui";
import type { T } from "./ui";
import { useCatalog, ObjectForm, defaultValue } from "./schema-form";
import type { ObjectValue } from "./schema-form";
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
    setNode(n);
  };
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        const { name, type, server, port, ...options } = node;
        onSave({
          id: value?.id || "",
          revision: value?.revision || 0,
          node: {
            name,
            type,
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
      {error ? (
        <p role="alert">
          {t(
            "字段目录载入失败，请重新打开。",
            "Field catalog failed to load. Reopen this form.",
          )}
        </p>
      ) : schema ? (
        <ObjectForm
          schema={schema}
          value={node}
          exclude={["type"]}
          t={t}
          onChange={setNode}
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
