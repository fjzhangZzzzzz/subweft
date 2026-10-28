import { useContext, useEffect, useRef, useState } from "react";
import type { InputHTMLAttributes } from "react";
import { Eye, EyeOff, Plus, Trash2 } from "lucide-react";
import type { ManualNode, Node } from "./types";
import { Button, CheckField, Field, ModalTitle, FormErrorContext } from "./ui";
import type { T } from "./ui";

function initialNode(type = "ss"): Node {
  return {
    name: "",
    type,
    server: "",
    port: 443,
    ...(type !== "http" ? { udp: true } : {}),
    ...(type === "ss" ? { cipher: "aes-128-gcm" } : {}),
    ...(type === "vmess" ? { cipher: "auto", alterId: 0 } : {}),
  };
}
function SecretInput({
  t,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { t: T }) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="secret-control">
      <input
        {...props}
        type={visible ? "text" : "password"}
        autoComplete="new-password"
        spellCheck={false}
      />
      <button
        type="button"
        className="icon-button"
        aria-label={`${visible ? t("隐藏", "Hide") : t("显示", "Show")} ${props.name || ""}`}
        aria-pressed={visible}
        onClick={() => setVisible(!visible)}
      >
        {visible ? <EyeOff size={16} /> : <Eye size={16} />}
      </button>
    </div>
  );
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
  const formError = useContext(FormErrorContext);
  const [node, setNode] = useState<Node>(
    value ? structuredClone(value.node) : initialNode(),
  );
  const [enabled, setEnabled] = useState(value?.enabled ?? true);
  const [auth, setAuth] = useState(!!value?.node.username);
  const [headers, setHeaders] = useState(
    Object.entries(value?.node.wsOpts?.headers || {}).map(([key, value]) => ({
      key,
      value,
    })),
  );
  const [error, setError] = useState("");
  const errorRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (error) errorRef.current?.focus();
  }, [error]);
  const transport = ["vmess", "vless", "trojan"].includes(node.type);
  const account = ["http", "socks5"].includes(node.type);
  const tls = node.type === "trojan" || !!node.tls;
  const network = node.network || "tcp";
  const retained = !!value && value.node.type === node.type;
  const change = <K extends keyof Node>(key: K, v: Node[K]) =>
    setNode({ ...node, [key]: v });
  const changeProtocol = (type: string) => {
    formError.clear();
    setNode({
      ...initialNode(type),
      name: node.name,
      server: node.server,
      port: node.port,
    });
    setHeaders([]);
    setAuth(false);
    setError("");
  };
  const submit = () => {
    setError("");
    const data: Node = {
      name: node.name.trim(),
      type: node.type,
      server: node.server.trim(),
      port: node.port,
    };
    if (node.type !== "http") data.udp = !!node.udp;
    if (["ss", "vmess"].includes(node.type)) data.cipher = node.cipher;
    if (["ss", "trojan"].includes(node.type))
      data.password = node.password || "";
    if (["vmess", "vless"].includes(node.type)) data.uuid = node.uuid || "";
    if (node.type === "vmess") data.alterId = node.alterId || 0;
    if (account && auth) {
      data.username = node.username || "";
      data.password = node.password || "";
    }
    if (account || ["vmess", "vless"].includes(node.type))
      data.tls = !!node.tls;
    if (tls) {
      data.skipCertVerify = !!node.skipCertVerify;
      if (node.type === "http" || node.type === "trojan")
        data.sni = node.sni || "";
      if (["vmess", "vless"].includes(node.type))
        data.serverName = node.serverName || "";
      if (transport) {
        if (node.alpn !== undefined) data.alpn = node.alpn;
        data.clientFingerprint = node.clientFingerprint || "";
      }
    }
    if (transport) {
      data.network = network;
      if (network === "ws") {
        const entries: Record<string, string> = {};
        const keys = new Set<string>();
        for (const row of headers) {
          if (!row.key && !row.value) continue;
          const key = row.key.trim();
          if (!key || keys.has(key.toLowerCase())) {
            setError(
              t(
                "WebSocket 标头名称不能为空或重复（不区分大小写）。",
                "WebSocket header names must be nonempty and unique, ignoring case.",
              ),
            );
            return;
          }
          keys.add(key.toLowerCase());
          entries[key] = row.value;
        }
        data.wsOpts = {
          path: node.wsOpts?.path || "",
          headers: entries,
          maxEarlyData: node.wsOpts?.maxEarlyData || 0,
          earlyDataHeaderName: node.wsOpts?.earlyDataHeaderName || "",
        };
      }
      if (network === "grpc")
        data.grpcOpts = { serviceName: node.grpcOpts?.serviceName || "" };
    }
    if (node.type === "vless" && tls) {
      data.flow = node.flow || "";
      if (node.realityOpts) data.realityOpts = node.realityOpts;
    }
    onSave({
      id: value?.id || "",
      revision: value?.revision || 0,
      node: data,
      enabled,
    });
  };
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        submit();
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
          "协议字段按所选类型展示。更换协议会清除原协议的凭据和高级参数。",
          "Fields follow the selected protocol. Changing it clears previous credentials and advanced parameters.",
        )}
      </p>
      {error && (
        <div ref={errorRef} role="alert" tabIndex={-1} className="alert error">
          {error}
        </div>
      )}
      <div className="form-grid">
        <Field name="name" label={t("名称", "Name")}>
          <input
            autoFocus
            required
            maxLength={160}
            value={node.name}
            onChange={(e) => change("name", e.target.value)}
          />
        </Field>
        <Field name="type" label={t("协议", "Protocol")}>
          <select
            value={node.type}
            onChange={(e) => changeProtocol(e.target.value)}
          >
            {protocols.map((type) => (
              <option key={type}>{type}</option>
            ))}
          </select>
        </Field>
      </div>
      <div className="form-grid">
        <Field name="server" label={t("服务器", "Server")}>
          <input
            required
            value={node.server}
            maxLength={253}
            onChange={(e) => change("server", e.target.value)}
          />
        </Field>
        <Field name="port" label={t("端口", "Port")}>
          <input
            type="number"
            required
            min={1}
            max={65535}
            value={node.port}
            onChange={(e) => change("port", Number(e.target.value))}
          />
        </Field>
      </div>
      {["ss", "vmess"].includes(node.type) && (
        <Field name="cipher" label={t("加密方式", "Cipher")}>
          <select
            value={node.cipher}
            onChange={(e) => change("cipher", e.target.value)}
          >
            {(node.type === "ss"
              ? ["aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305"]
              : ["auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero"]
            ).map((cipher) => (
              <option key={cipher}>{cipher}</option>
            ))}
          </select>
        </Field>
      )}
      {["vmess", "vless"].includes(node.type) && (
        <Field
          name="uuid"
          label="UUID"
          hint={
            retained
              ? t(
                  "留空保留现有 UUID。",
                  "Leave blank to retain the existing UUID.",
                )
              : t("填写标准 UUID。", "Enter a standard UUID.")
          }
        >
          <SecretInput
            t={t}
            name="UUID"
            required={!retained}
            value={node.uuid || ""}
            onChange={(e) => change("uuid", e.target.value)}
          />
        </Field>
      )}
      {node.type === "vmess" && (
        <Field name="alterId" label="alterId">
          <input
            type="number"
            min={0}
            max={65535}
            value={node.alterId || 0}
            onChange={(e) => change("alterId", Number(e.target.value))}
          />
        </Field>
      )}
      {account && (
        <CheckField
          label={t("使用账号密码", "Use account credentials")}
          checked={auth}
          onChange={setAuth}
        />
      )}
      {account && auth && (
        <Field name="username" label={t("用户名", "Username")}>
          <SecretInput
            t={t}
            name={t("用户名", "Username")}
            required
            value={node.username || ""}
            onChange={(e) =>
              setNode({
                ...node,
                username: e.target.value,
                ...(retained ? { password: "" } : {}),
              })
            }
          />
        </Field>
      )}
      {(["ss", "trojan"].includes(node.type) || (account && auth)) && (
        <Field
          name="password"
          label={t("密码", "Password")}
          hint={
            retained
              ? t(
                  "留空保留同协议的现有密码；关闭账号密码可清除 HTTP/SOCKS5 身份。",
                  "Blank retains a password for the same protocol. Disable account credentials to clear HTTP/SOCKS5 authentication.",
                )
              : undefined
          }
        >
          <SecretInput
            t={t}
            name={t("密码", "Password")}
            required={
              !retained || (account && node.username !== value?.node.username)
            }
            value={node.password || ""}
            onChange={(e) => change("password", e.target.value)}
          />
        </Field>
      )}
      {node.type !== "ss" && (
        <section className="node-options">
          <h3>{t("传输与 TLS", "Transport & TLS")}</h3>
          {transport && (
            <Field name="network" label={t("传输方式", "Transport")}>
              <select
                value={network}
                onChange={(e) => {
                  change("network", e.target.value);
                }}
              >
                <option value="tcp">TCP</option>
                <option value="ws">WebSocket</option>
                <option value="grpc">gRPC</option>
              </select>
            </Field>
          )}
          {node.type === "trojan" ? (
            <p className="muted">
              {t("Trojan 始终使用 TLS。", "Trojan always uses TLS.")}
            </p>
          ) : (
            <CheckField
              label={t("启用 TLS", "Enable TLS")}
              checked={!!node.tls}
              onChange={(v) =>
                setNode({
                  ...node,
                  tls: v,
                  ...(v
                    ? {}
                    : {
                        skipCertVerify: false,
                        serverName: "",
                        sni: "",
                        alpn: undefined,
                        clientFingerprint: "",
                        realityOpts: undefined,
                        flow: "",
                      }),
                })
              }
            />
          )}
          {tls && (
            <>
              {node.type !== "socks5" && (
                <Field
                  name={
                    node.type === "trojan" || node.type === "http"
                      ? "sni"
                      : "serverName"
                  }
                  label={t("TLS 服务器名称（SNI）", "TLS server name (SNI)")}
                >
                  <input
                    value={
                      node.type === "trojan" || node.type === "http"
                        ? node.sni || ""
                        : node.serverName || ""
                    }
                    onChange={(e) =>
                      change(
                        node.type === "trojan" || node.type === "http"
                          ? "sni"
                          : "serverName",
                        e.target.value,
                      )
                    }
                  />
                </Field>
              )}
              <CheckField
                label={t(
                  "跳过证书验证（不安全）",
                  "Skip certificate verification (insecure)",
                )}
                checked={!!node.skipCertVerify}
                onChange={(v) => change("skipCertVerify", v)}
              />
              {transport && (
                <>
                  <Field name="alpn" label={t("ALPN 模式", "ALPN mode")}>
                    <select
                      value={
                        node.alpn === undefined
                          ? "default"
                          : node.alpn.length
                            ? "custom"
                            : "empty"
                      }
                      onChange={(e) =>
                        change(
                          "alpn",
                          e.target.value === "default"
                            ? undefined
                            : e.target.value === "empty"
                              ? []
                              : ["h2"],
                        )
                      }
                    >
                      <option value="default">
                        {t("内核默认", "Core default")}
                      </option>
                      <option value="custom">
                        {t("自定义协议列表", "Custom protocol list")}
                      </option>
                      <option value="empty">
                        {t("显式空列表", "Explicit empty list")}
                      </option>
                    </select>
                  </Field>
                  {node.alpn !== undefined && node.alpn.length > 0 && (
                    <Field
                      label={t("ALPN 协议列表", "ALPN protocol list")}
                      hint={t(
                        "逗号分隔，按协商优先级排列。",
                        "Comma-separated, in negotiation order.",
                      )}
                    >
                      <input
                        required
                        value={node.alpn.join(",")}
                        onChange={(e) =>
                          change(
                            "alpn",
                            e.target.value.split(",").map((v) => v.trim()),
                          )
                        }
                      />
                    </Field>
                  )}
                  <Field
                    name="clientFingerprint"
                    label={t("客户端指纹", "Client fingerprint")}
                  >
                    <select
                      value={node.clientFingerprint || ""}
                      onChange={(e) =>
                        change("clientFingerprint", e.target.value)
                      }
                    >
                      <option value="">{t("内核默认", "Core default")}</option>
                      {[
                        "chrome",
                        "firefox",
                        "safari",
                        "ios",
                        "android",
                        "edge",
                        "random",
                        "randomized",
                      ].map((v) => (
                        <option key={v}>{v}</option>
                      ))}
                    </select>
                  </Field>
                </>
              )}
            </>
          )}
          {node.type === "vless" && tls && (
            <>
              <Field name="flow" label="Flow">
                <select
                  value={node.flow || ""}
                  onChange={(e) => change("flow", e.target.value)}
                >
                  <option value="">{t("默认", "Default")}</option>
                  <option>xtls-rprx-vision</option>
                </select>
              </Field>
              <CheckField
                label="Reality"
                checked={!!node.realityOpts}
                onChange={(v) =>
                  setNode({
                    ...node,
                    realityOpts: v ? { publicKey: "", shortId: "" } : undefined,
                    ...(v
                      ? {
                          clientFingerprint: node.clientFingerprint || "chrome",
                          skipCertVerify: false,
                        }
                      : {}),
                  })
                }
              />
              {node.realityOpts && (
                <div className="form-grid">
                  <Field
                    name="realityOpts"
                    label={t("Reality 公钥", "Reality public key")}
                  >
                    <input
                      required
                      value={node.realityOpts.publicKey}
                      onChange={(e) =>
                        change("realityOpts", {
                          ...node.realityOpts!,
                          publicKey: e.target.value,
                        })
                      }
                    />
                  </Field>
                  <Field
                    label="Short ID"
                    hint={t(
                      "0 至 16 个十六进制字符，长度须为偶数。",
                      "0–16 hexadecimal characters, with an even length.",
                    )}
                  >
                    <SecretInput
                      t={t}
                      name="Short ID"
                      maxLength={16}
                      pattern="([0-9a-fA-F]{2}){0,8}"
                      value={node.realityOpts.shortId}
                      onChange={(e) =>
                        change("realityOpts", {
                          ...node.realityOpts!,
                          shortId: e.target.value,
                        })
                      }
                    />
                  </Field>
                </div>
              )}
            </>
          )}
          {transport && network === "ws" && (
            <div className="node-options">
              <Field
                name="wsOpts"
                label={t("WebSocket 路径", "WebSocket path")}
              >
                <input
                  value={node.wsOpts?.path || ""}
                  placeholder="/"
                  onChange={(e) =>
                    change("wsOpts", {
                      path: e.target.value,
                      headers: {},
                      maxEarlyData: node.wsOpts?.maxEarlyData || 0,
                      earlyDataHeaderName:
                        node.wsOpts?.earlyDataHeaderName || "",
                    })
                  }
                />
              </Field>
              <div className="section-intro">
                <h3>{t("WebSocket 标头", "WebSocket headers")}</h3>
                <Button
                  disabled={headers.length >= 32}
                  onClick={() =>
                    setHeaders([...headers, { key: "", value: "" }])
                  }
                >
                  <Plus size={14} />
                  {t("添加标头", "Add header")}
                </Button>
              </div>
              {headers.map((row, i) => (
                <div className="header-row" key={i}>
                  <Field label={`${t("标头名称", "Header name")} ${i + 1}`}>
                    <input
                      value={row.key}
                      onChange={(e) =>
                        setHeaders(
                          headers.map((r, index) =>
                            index === i ? { ...r, key: e.target.value } : r,
                          ),
                        )
                      }
                    />
                  </Field>
                  <Field label={`${t("标头值", "Header value")} ${i + 1}`}>
                    <SecretInput
                      t={t}
                      name={`${t("标头值", "Header value")} ${i + 1}`}
                      value={row.value}
                      onChange={(e) =>
                        setHeaders(
                          headers.map((r, index) =>
                            index === i ? { ...r, value: e.target.value } : r,
                          ),
                        )
                      }
                    />
                  </Field>
                  <button
                    type="button"
                    className="icon-button"
                    aria-label={`${t("删除标头", "Delete header")} ${i + 1}`}
                    onClick={() =>
                      setHeaders(headers.filter((_, index) => index !== i))
                    }
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              ))}
              <details className="advanced">
                <summary>
                  {t("Early Data 高级设置", "Early Data settings")}
                </summary>
                <Field
                  label={t("最大 Early Data 字节数", "Max Early Data bytes")}
                >
                  <input
                    type="number"
                    min={0}
                    max={65536}
                    value={node.wsOpts?.maxEarlyData || 0}
                    onChange={(e) =>
                      change("wsOpts", {
                        path: node.wsOpts?.path || "",
                        headers: {},
                        earlyDataHeaderName:
                          node.wsOpts?.earlyDataHeaderName || "",
                        maxEarlyData: Number(e.target.value),
                      })
                    }
                  />
                </Field>
                <Field
                  label={t("Early Data 标头名称", "Early Data header name")}
                >
                  <input
                    value={node.wsOpts?.earlyDataHeaderName || ""}
                    onChange={(e) =>
                      change("wsOpts", {
                        path: node.wsOpts?.path || "",
                        headers: {},
                        maxEarlyData: node.wsOpts?.maxEarlyData || 0,
                        earlyDataHeaderName: e.target.value,
                      })
                    }
                  />
                </Field>
              </details>
            </div>
          )}
          {transport && network === "grpc" && (
            <Field
              name="grpcOpts.serviceName"
              label={t("gRPC 服务名称", "gRPC service name")}
            >
              <input
                value={node.grpcOpts?.serviceName || ""}
                onChange={(e) =>
                  change("grpcOpts", { serviceName: e.target.value })
                }
              />
            </Field>
          )}
        </section>
      )}
      {node.type !== "http" && (
        <CheckField
          label="UDP"
          checked={!!node.udp}
          onChange={(v) => change("udp", v)}
        />
      )}
      <CheckField
        label={t("启用节点", "Enable node")}
        checked={enabled}
        onChange={setEnabled}
      />
      <div className="form-footer">
        <Button primary type="submit">
          {t("保存节点", "Save node")}
        </Button>
      </div>
    </form>
  );
}
