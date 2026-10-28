import { useEffect, useState } from "react";
import type { InputHTMLAttributes } from "react";
import { Button, Field, CheckField } from "./ui";
import type { T } from "./ui";
export type Value =
  string | number | boolean | Value[] | { [key: string]: Value };
export type ObjectValue = { [key: string]: Value };
export type Schema = {
  kind: string;
  choices?: string[];
  separator?: string;
  required?: boolean;
  secret?: boolean;
  enum?: string[];
  min?: number;
  max?: number;
  properties?: Record<string, Schema>;
  items?: Schema;
  values?: Schema;
  alternatives?: Schema[];
};
export type Catalog = {
  version: string;
  protocols: Record<string, Schema>;
  plugins: Record<string, Schema>;
  groups: Record<string, Schema>;
  dns: Schema;
  config: Schema;
};
let cached: Promise<Catalog> | undefined;
export function useCatalog() {
  const [catalog, setCatalog] = useState<Catalog>();
  const [error, setError] = useState(false);
  useEffect(() => {
    cached ??= fetch("/api/v1/fields").then(async (r) => {
      if (!r.ok) throw Error("catalog_unavailable");
      return r.json();
    });
    cached.then(setCatalog).catch(() => {
      cached = undefined;
      setError(true);
    });
  }, []);
  return { catalog, error };
}
const labels: Record<string, [string, string]> = {
  name: ["名称", "Name"],
  timeout: ["超时", "Timeout"],
  retries: ["重试次数", "Retries"],
  headers: ["请求标头", "Headers"],
  proxy: ["请求代理", "Request proxy"],
  userAgent: ["用户代理", "User agent"],
  pattern: ["匹配模式", "Pattern"],
  nameservers: ["解析器", "Nameservers"],
  sni: ["TLS 服务器名称", "TLS server name"],
  interval: ["间隔", "Interval"],
  server: ["服务器", "Server"],
  port: ["端口", "Port"],
  password: ["密码", "Password"],
  username: ["用户名", "Username"],
  cipher: ["加密方式", "Cipher"],
  "private-key": ["私钥（内联内容）", "Private key (inline)"],
  "public-key": ["公钥", "Public key"],
  psk: ["预共享密钥", "PSK"],
  uuid: ["UUID", "UUID"],
  network: ["传输方式", "Transport"],
  "plugin-opts": ["插件参数", "Plugin options"],
  "dialer-proxy": ["前置代理或组", "Dialer proxy"],
  tls: ["启用 TLS", "Enable TLS"],
};
export const defaultValue = (s: Schema): Value =>
  s.kind === "object"
    ? Object.fromEntries(
        Object.entries(s.properties || {})
          .filter(([, v]) => v.required)
          .map(([k, v]) => [k, defaultValue(v)]),
      )
    : s.kind === "map"
      ? {}
      : s.kind === "array"
        ? []
        : s.kind === "boolean"
          ? false
          : s.kind === "integer" || s.kind === "number"
            ? s.min || 0
            : s.kind === "union"
              ? defaultValue(s.alternatives![0])
              : s.enum?.[0] || "";
export function SecretControl({
  value,
  onChange,
  name,
  id,
  t,
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "onChange" | "value"> & {
  value: string;
  onChange: (v: string) => void;
  name: string;
  t: T;
}) {
  const [show, setShow] = useState(false);
  const multiline =
    name.endsWith("private-key") || name.endsWith("certificate");
  return (
    <div>
      <div className="secret-control">
        {multiline && show ? (
          <textarea
            id={id}
            name={name}
            aria-label={name}
            aria-invalid={props["aria-invalid"]}
            aria-describedby={props["aria-describedby"]}
            rows={5}
            autoComplete="off"
            spellCheck={false}
            value={value}
            onChange={(e) => onChange(e.target.value)}
          />
        ) : (
          <input
            {...props}
            id={id}
            name={name}
            aria-label={name}
            type={show ? "text" : "password"}
            autoComplete="new-password"
            spellCheck={false}
            value={value}
            onChange={(e) => onChange(e.target.value)}
          />
        )}
        <button
          type="button"
          className="icon-button"
          aria-label={(show ? t("隐藏 ", "Hide ") : t("显示 ", "Show ")) + name}
          aria-pressed={show}
          onClick={() => setShow(!show)}
        >
          {show ? "◉" : "○"}
        </button>
      </div>
      {multiline && (
        <input
          type="file"
          accept=".pem,.crt,.key,.txt"
          aria-label={name + " PEM"}
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file && file.size <= 65536) void file.text().then(onChange);
          }}
        />
      )}
    </div>
  );
}
export function ObjectForm({
  schema,
  value,
  onChange,
  t,
  path = "",
  exclude = [],
}: {
  schema: Schema;
  value: ObjectValue;
  onChange: (v: ObjectValue) => void;
  t: T;
  path?: string;
  exclude?: string[];
}) {
  const [search, setSearch] = useState("");
  const properties = schema.properties || {};
  const keys = Object.keys(properties).filter((k) => !exclude.includes(k));
  const shown = keys.filter((k) => properties[k].required || k in value);
  const add = keys.filter(
    (k) =>
      !shown.includes(k) &&
      (k + " " + (labels[k]?.join(" ") || ""))
        .toLowerCase()
        .includes(search.toLowerCase()),
  );
  return (
    <div className="schema-object" data-field={path}>
      {shown.map((key) => {
        const s = properties[key];
        const p = path ? path + "." + key : key;
        const label = labels[key] ? t(...labels[key]) : key;
        return (
          <div
            className="schema-field"
            key={key}
            data-complex={["object", "array", "map", "union", "any"].includes(
              s.kind,
            )}
            data-field={p}
          >
            <div className="schema-field-heading">
              <strong>
                {label}{" "}
                <small>
                  {labels[key] ? key : ""}
                  {s.required ? " *" : ""}
                </small>
              </strong>
              {!s.required && (
                <Button
                  onClick={() => {
                    const v = { ...value };
                    delete v[key];
                    onChange(v);
                  }}
                >
                  {t("移除", "Remove")}
                </Button>
              )}
            </div>
            <ValueForm
              schema={s}
              value={value[key] ?? defaultValue(s)}
              path={p}
              t={t}
              onChange={(v) => onChange({ ...value, [key]: v })}
            />
          </div>
        );
      })}
      {keys.some((k) => !shown.includes(k)) && (
        <div className="form-grid">
          <Field label={t("查找高级字段", "Find advanced field")}>
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("输入字段名筛选", "Filter by field name")}
            />
          </Field>
          <Field label={t("添加字段", "Add field")}>
            <select
              value=""
              onChange={(e) => {
                const k = e.target.value;
                if (k) onChange({ ...value, [k]: defaultValue(properties[k]) });
              }}
            >
              <option value="">{t("选择参数…", "Select a parameter…")}</option>
              {add.map((k) => (
                <option key={k} value={k}>
                  {labels[k] ? t(...labels[k]) + " · " : ""}
                  {k}
                </option>
              ))}
            </select>
          </Field>
        </div>
      )}
    </div>
  );
}
export function ValueForm({
  schema: s,
  value,
  path,
  t,
  onChange,
}: {
  schema: Schema;
  value: Value;
  path: string;
  t: T;
  onChange: (v: Value) => void;
}) {
  if (s.choices && s.separator) {
    const selected = String(value).split(s.separator).filter(Boolean);
    return (
      <div className="checkbox-grid">
        {s.choices.map((choice) => (
          <CheckField
            key={choice}
            label={choice}
            checked={selected.some(
              (v) => v.toLowerCase() === choice.toLowerCase(),
            )}
            onChange={(yes) =>
              onChange(
                [
                  ...selected.filter(
                    (v) => v.toLowerCase() !== choice.toLowerCase(),
                  ),
                  ...(yes ? [choice] : []),
                ].join(s.separator),
              )
            }
          />
        ))}
      </div>
    );
  }
  if (s.kind === "object")
    return (
      <ObjectForm
        schema={s}
        value={value as ObjectValue}
        onChange={onChange}
        t={t}
        path={path}
      />
    );
  if (s.kind === "union") {
    const kind = Array.isArray(value) ? "array" : typeof value;
    const index = Math.max(
      0,
      s.alternatives!.findIndex((x) => x.kind === kind),
    );
    return (
      <>
        <Field label={t("值类型", "Value type")}>
          <select
            value={index}
            onChange={(e) =>
              onChange(defaultValue(s.alternatives![+e.target.value]))
            }
          >
            {s.alternatives!.map((a, i) => (
              <option value={i} key={i}>
                {a.kind}
              </option>
            ))}
          </select>
        </Field>
        <ValueForm
          schema={{
            ...s.alternatives![index],
            secret: s.secret || s.alternatives![index].secret,
          }}
          value={value}
          onChange={onChange}
          path={path}
          t={t}
        />
      </>
    );
  }
  if (s.kind === "any") {
    const kind = Array.isArray(value)
      ? "array"
      : typeof value === "object"
        ? "map"
        : typeof value === "number"
          ? "number"
          : typeof value === "boolean"
            ? "boolean"
            : "string";
    return (
      <>
        <Field label={t("值类型", "Value type")}>
          <select
            value={kind}
            onChange={(e) => onChange(defaultValue({ kind: e.target.value }))}
          >
            {["string", "number", "boolean", "array", "map"].map((k) => (
              <option key={k}>{k}</option>
            ))}
          </select>
        </Field>
        <ValueForm
          schema={{
            kind,
            secret: s.secret,
            items: { kind: "any" },
            values: { kind: "any" },
          }}
          value={value}
          onChange={onChange}
          path={path}
          t={t}
        />
      </>
    );
  }
  if (s.kind === "array") {
    const a = value as Value[];
    return (
      <div className="schema-rows">
        {a.map((v, i) => (
          <div key={i} className="schema-row">
            <ValueForm
              schema={s.secret ? { ...s.items!, secret: true } : s.items!}
              value={v}
              path={path + "." + i}
              t={t}
              onChange={(v) => onChange(a.map((x, j) => (i === j ? v : x)))}
            />
            <div className="actions">
              <Button
                aria-label={t("上移", "Move up")}
                disabled={i === 0}
                onClick={() => {
                  const n = [...a];
                  [n[i - 1], n[i]] = [n[i], n[i - 1]];
                  onChange(n);
                }}
              >
                ↑
              </Button>
              <Button
                aria-label={t("删除条目", "Delete item")}
                onClick={() => onChange(a.filter((_, j) => j !== i))}
              >
                ×
              </Button>
            </div>
          </div>
        ))}
        <Button onClick={() => onChange([...a, defaultValue(s.items!)])}>
          {t("添加条目", "Add item")}
        </Button>
      </div>
    );
  }
  if (s.kind === "map")
    return (
      <MapForm
        schema={s}
        value={value as ObjectValue}
        onChange={onChange}
        path={path}
        t={t}
      />
    );
  return (
    <Field name={path} label={path}>
      {s.kind === "boolean" ? (
        <select
          value={String(value)}
          onChange={(e) => onChange(e.target.value === "true")}
        >
          <option value="true">true</option>
          <option value="false">false</option>
        </select>
      ) : s.enum ? (
        <select
          value={String(value)}
          onChange={(e) => onChange(e.target.value)}
        >
          {s.enum.map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
      ) : s.kind === "integer" || s.kind === "number" ? (
        <input
          type="number"
          step={s.kind === "integer" ? 1 : "any"}
          min={s.min}
          max={s.max}
          value={Number(value)}
          onChange={(e) => onChange(e.target.valueAsNumber || 0)}
        />
      ) : s.secret || path.endsWith("certificate") ? (
        <SecretControl
          t={t}
          name={path}
          value={String(value)}
          onChange={onChange}
        />
      ) : (
        <input
          value={String(value)}
          onChange={(e) => onChange(e.target.value)}
          spellCheck={false}
        />
      )}
    </Field>
  );
}
function MapForm({
  schema: s,
  value,
  onChange,
  path,
  t,
}: {
  schema: Schema;
  value: ObjectValue;
  onChange: (v: Value) => void;
  path: string;
  t: T;
}) {
  const [key, setKey] = useState("");
  return (
    <div className="schema-rows">
      {Object.entries(value).map(([k, v]) => (
        <div key={k} className="schema-row">
          <div>
            <strong>{k}</strong>
            <ValueForm
              schema={s.secret ? { ...s.values!, secret: true } : s.values!}
              value={v}
              path={path + "." + k}
              t={t}
              onChange={(v) => onChange({ ...value, [k]: v })}
            />
          </div>
          <Button
            aria-label={t("删除条目", "Delete item")}
            onClick={() => {
              const n = { ...value };
              delete n[k];
              onChange(n);
            }}
          >
            ×
          </Button>
        </div>
      ))}
      <div className="form-grid">
        <Field label={t("新条目名称", "New entry name")}>
          <input value={key} onChange={(e) => setKey(e.target.value)} />
        </Field>
        <Button
          disabled={!key || key in value}
          onClick={() => {
            onChange({ ...value, [key]: defaultValue(s.values!) });
            setKey("");
          }}
        >
          {t("添加条目", "Add item")}
        </Button>
      </div>
    </div>
  );
}
