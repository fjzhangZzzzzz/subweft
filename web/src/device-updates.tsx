import { useState } from "react";
import { Button, Field, CheckField, ModalTitle } from "./ui";
import type { T } from "./ui";
import { SecretControl } from "./schema-form";
import type { Source, Profile, State } from "./types";

export type DeviceUpdate = {
  userAgent?: string;
  headers?: Record<string, string>;
  prefix?: string;
  suffix?: string;
  filter?: string;
  exclude?: string;
  renames?: { pattern: string; replacement: string }[];
  healthCheck: {
    enable: boolean;
    url: string;
    interval: number;
    timeout: number;
    lazy: boolean;
    expectedStatus?: string;
  };
};
export function HeaderFields({
  value,
  onChange,
  t,
}: {
  value: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  t: T;
}) {
  const [rows, setRows] = useState<string[][]>(() => Object.entries(value));
  const update = (next: string[][]) => {
    setRows(next);
    onChange(Object.fromEntries(next));
  };
  return (
    <fieldset>
      <legend>{t("请求头", "Request headers")}</legend>
      {rows.map(([name, content], i) => (
        <div className="form-grid" key={i}>
          <Field label={t("名称", "Name")}>
            <input
              required
              ref={(el) =>
                el?.setCustomValidity(
                  rows.some(
                    (r, j) =>
                      j !== i && r[0].toLowerCase() === name.toLowerCase(),
                  )
                    ? t("请求头名称不能重复", "Header names must be unique")
                    : "",
                )
              }
              value={name}
              onChange={(e) =>
                update(
                  rows.map((r, j) => (j === i ? [e.target.value, r[1]] : r)),
                )
              }
            />
          </Field>
          <Field label={t("值", "Value")}>
            <SecretControl
              t={t}
              name={name || t("请求头值", "Header value")}
              value={content}
              onChange={(v) =>
                update(rows.map((r, j) => (j === i ? [r[0], v] : r)))
              }
            />
          </Field>
          <Button onClick={() => update(rows.filter((_, j) => j !== i))}>
            {t("移除请求头", "Remove header")}
          </Button>
        </div>
      ))}
      <Button onClick={() => update([...rows, ["", ""]])}>
        {t("添加请求头", "Add header")}
      </Button>
    </fieldset>
  );
}
export function SourceForm({
  value,
  t,
  onSave,
}: {
  value?: Source;
  t: T;
  onSave: (v: unknown) => unknown;
}) {
  const [name, setName] = useState(value?.name || "");
  const [kind, setKind] = useState(value?.kind || "http");
  const [url, setURL] = useState(value?.url || "");
  const [content, setContent] = useState("");
  const [tags, setTags] = useState(value?.tags?.join(", ") || "");
  const [interval, setInterval] = useState(value?.interval || 3600);
  const [enabled, setEnabled] = useState(value?.enabled ?? true);
  const [extractNodes, setExtract] = useState(value?.extractNodes ?? true);
  const [resolveLegacyProxy, setResolve] = useState(false);
  const [convertCached, setConvertCached] = useState(false);
  const [update, setUpdate] = useState<DeviceUpdate>(
    value?.update || {
      healthCheck: {
        enable: false,
        url: "https://www.gstatic.com/generate_204",
        interval: 300,
        timeout: 5000,
        lazy: true,
      },
    },
  );
  const health = (v: Partial<DeviceUpdate["healthCheck"]>) =>
    setUpdate({ ...update, healthCheck: { ...update.healthCheck, ...v } });
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        onSave({
          id: value?.id || "",
          revision: value?.revision || 0,
          name: name.trim(),
          kind,
          url,
          content,
          tags: tags
            .split(",")
            .map((v) => v.trim())
            .filter(Boolean),
          interval,
          enabled,
          extractNodes,
          update,
          resolveLegacyProxy,
          convertCached,
        });
      }}
    >
      <ModalTitle
        title={
          value
            ? t("编辑订阅", "Edit subscription")
            : t("添加订阅", "Add subscription")
        }
      />
      {value?.overrides?.length ? (
        <div className="alert warning">
          <p>
            {t(
              "此订阅有旧版逐节点修改。可转换为本地节点集合以保留所有改名和排除；转换后不再更新该远程订阅。没有唯一匹配的旧修改需要先在节点页处理。",
              "Legacy per-node edits require migration. Convert the cached nodes to a local collection to preserve renames and exclusions; this stops remote updates. Resolve unmatched edits in the nodes page first.",
            )}
          </p>
          <CheckField
            label={t(
              "将已有节点和修改转换为本地集合（保存后生效）",
              "Convert existing nodes and edits to a local collection on save",
            )}
            checked={convertCached}
            onChange={setConvertCached}
          />
        </div>
      ) : null}
      <Field
        label={t("订阅名称", "Name")}
        hint={t(
          "节点名称会加上订阅前缀，方便区分。",
          "Node names include a subscription prefix.",
        )}
      >
        <input
          autoFocus
          required
          maxLength={80}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      <Field label={t("订阅方式", "Source type")}>
        <select
          value={kind}
          onChange={(e) => setKind(e.target.value as "http" | "local")}
        >
          <option value="http">{t("远程订阅", "Remote subscription")}</option>
          <option value="local">{t("本地内容", "Local content")}</option>
        </select>
      </Field>
      {kind === "http" ? (
        <>
          <Field
            label={t("订阅地址", "Subscription URL")}
            hint={t(
              "地址和必要请求头会下发给授权设备，由设备直接更新。",
              "Authorized devices receive this URL and required headers, and update directly.",
            )}
          >
            <SecretControl
              t={t}
              name={t("订阅地址", "Subscription URL")}
              required={!value || value.kind !== kind}
              value={url}
              onChange={setURL}
            />
          </Field>
          <p className="muted">
            {t(
              "订阅只提供节点；规则与 DNS 由配置表单管理。更新使用直连或策略组，在配置中选择。",
              "Subscriptions provide nodes. Profile forms manage rules, DNS and the update route.",
            )}
          </p>
        </>
      ) : (
        <>
          <Field
            label={t("导入内容（留空保留）", "Import content (blank retains)")}
          >
            <textarea
              rows={6}
              required={!value || value.kind !== kind}
              value={content}
              onChange={(e) => setContent(e.target.value)}
            />
          </Field>
          <Field label={t("或选择文件", "Or choose file")}>
            <input
              type="file"
              accept=".yaml,.yml,.txt"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f && f.size <= 2 * 1024 * 1024)
                  void f.text().then(setContent);
              }}
            />
          </Field>
          <CheckField
            label={t(
              "从完整配置中提取节点",
              "Extract nodes from full configuration",
            )}
            checked={extractNodes}
            onChange={setExtract}
          />
        </>
      )}
      <div className="form-grid">
        <Field label={t("标签，以逗号分隔", "Tags, comma separated")}>
          <input value={tags} onChange={(e) => setTags(e.target.value)} />
        </Field>
        {kind === "http" && (
          <Field
            label={t("设备更新间隔（秒）", "Device update interval (seconds)")}
          >
            <input
              type="number"
              min={60}
              max={604800}
              required
              value={interval}
              onChange={(e) => setInterval(+e.target.value)}
            />
          </Field>
        )}
      </div>
      {kind === "http" && (
        <>
          {value?.legacyProxy && (
            <div className="alert warning">
              <p>
                {t(
                  "旧版设置了服务端下载代理。请在各配置中选择设备的更新出站后确认。",
                  "A legacy server proxy needs an explicit device route in each profile.",
                )}
              </p>
              <CheckField
                label={t(
                  "已检查配置中的更新出站，移除旧服务端代理设置",
                  "I reviewed profile update routes; remove the legacy server proxy",
                )}
                checked={resolveLegacyProxy}
                onChange={setResolve}
              />
            </div>
          )}
          <details>
            <summary>{t("请求设置", "Request settings")}</summary>
            <Field
              label="User-Agent"
              hint={t(
                "默认 mihomo/1.19.33，可按订阅要求修改。",
                "Defaults to mihomo/1.19.33; override if required.",
              )}
            >
              <input
                value={update.userAgent || ""}
                placeholder="mihomo/1.19.33"
                onChange={(e) =>
                  setUpdate({ ...update, userAgent: e.target.value })
                }
              />
            </Field>
            <HeaderFields
              t={t}
              value={update.headers || {}}
              onChange={(headers) => setUpdate({ ...update, headers })}
            />
          </details>
          <details>
            <summary>{t("节点筛选与命名", "Node filtering & names")}</summary>
            <div className="form-grid">
              {(["prefix", "suffix", "filter", "exclude"] as const).map(
                (key) => (
                  <Field
                    key={key}
                    label={
                      {
                        prefix: t("附加前缀", "Extra prefix"),
                        suffix: t("附加后缀", "Extra suffix"),
                        filter: t("保留名称匹配（正则）", "Include name regex"),
                        exclude: t(
                          "排除名称匹配（正则）",
                          "Exclude name regex",
                        ),
                      }[key]
                    }
                  >
                    <input
                      value={update[key] || ""}
                      onChange={(e) =>
                        setUpdate({ ...update, [key]: e.target.value })
                      }
                    />
                  </Field>
                ),
              )}
            </div>
            {(update.renames || []).map((r, i) => (
              <div className="form-grid" key={i}>
                <Field label={t("名称匹配（正则）", "Name pattern")}>
                  <input
                    value={r.pattern}
                    required
                    onChange={(e) =>
                      setUpdate({
                        ...update,
                        renames: update.renames!.map((v, j) =>
                          i === j ? { ...v, pattern: e.target.value } : v,
                        ),
                      })
                    }
                  />
                </Field>
                <Field label={t("替换为", "Replace with")}>
                  <input
                    value={r.replacement}
                    onChange={(e) =>
                      setUpdate({
                        ...update,
                        renames: update.renames!.map((v, j) =>
                          i === j ? { ...v, replacement: e.target.value } : v,
                        ),
                      })
                    }
                  />
                </Field>
                <Button
                  onClick={() =>
                    setUpdate({
                      ...update,
                      renames: update.renames!.filter((_, j) => j !== i),
                    })
                  }
                >
                  {t("移除替换", "Remove replacement")}
                </Button>
              </div>
            ))}
            <Button
              onClick={() =>
                setUpdate({
                  ...update,
                  renames: [
                    ...(update.renames || []),
                    { pattern: "", replacement: "" },
                  ],
                })
              }
            >
              {t("添加名称替换", "Add name replacement")}
            </Button>
          </details>
          <details>
            <summary>{t("健康检查", "Health checks")}</summary>
            <CheckField
              label={t(
                "定期检查节点可用性",
                "Check node availability periodically",
              )}
              checked={update.healthCheck.enable}
              onChange={(enable) => health({ enable })}
            />
            {update.healthCheck.enable && (
              <>
                <Field label={t("检查地址", "Check URL")}>
                  <input
                    type="url"
                    required
                    value={update.healthCheck.url}
                    onChange={(e) => health({ url: e.target.value })}
                  />
                </Field>
                <div className="form-grid">
                  {(["interval", "timeout"] as const).map((key) => (
                    <Field
                      key={key}
                      label={
                        key === "interval"
                          ? t("检查间隔（秒）", "Interval (seconds)")
                          : t("超时（毫秒）", "Timeout (milliseconds)")
                      }
                    >
                      <input
                        type="number"
                        min={key === "interval" ? 30 : 100}
                        max={key === "interval" ? 86400 : 30000}
                        value={update.healthCheck[key]}
                        onChange={(e) => health({ [key]: +e.target.value })}
                      />
                    </Field>
                  ))}
                </div>
                <CheckField
                  label={t("仅检查使用中的节点", "Check only nodes in use")}
                  checked={update.healthCheck.lazy}
                  onChange={(lazy) => health({ lazy })}
                />
                <Field label={t("期望 HTTP 状态码", "Expected HTTP status")}>
                  <input
                    value={update.healthCheck.expectedStatus || ""}
                    onChange={(e) => health({ expectedStatus: e.target.value })}
                  />
                </Field>
              </>
            )}
          </details>
        </>
      )}
      <CheckField
        label={t("启用订阅", "Enable subscription")}
        checked={enabled}
        onChange={setEnabled}
      />
      <div className="form-footer">
        <Button primary type="submit">
          {t("保存订阅", "Save subscription")}
        </Button>
      </div>
    </form>
  );
}

export function UpdateRoutes({
  profile,
  state,
  t,
  onChange,
}: {
  profile: Profile;
  state: State;
  t: T;
  onChange: (v: Profile) => void;
}) {
  const sources = state.sources.filter(
    (s) =>
      s.kind === "http" &&
      profile.groups.some(
        (g) =>
          g.sourceIds.includes(s.id) ||
          (s.enabled &&
            (g.allSources || g.tags.some((tag) => s.tags.includes(tag)))),
      ),
  );
  const rules = state.resources.filter(
    (r) =>
      r.kind === "http" &&
      r.type === "ruleset" &&
      profile.rules.some(
        (rule) =>
          rule.enabled && rule.type === "RULE-SET" && rule.value === r.id,
      ),
  );
  const options = [
    "DIRECT",
    ...profile.groups.map((g) => g.name),
    ...state.nodes.filter((n) => n.enabled).map((n) => n.node.name),
  ];
  if (!sources.length && !rules.length) return null;
  return (
    <section className="update-routes">
      <h3>{t("设备更新出站", "Device update routes")}</h3>
      <p className="muted">
        {t(
          "默认直连。需要代理时，选择本配置中的独立节点或策略组；不能通过订阅自身下载该订阅。",
          "Direct by default. Choose an independent node or group when a proxy is required; a subscription cannot download through itself.",
        )}
      </p>
      <div className="form-grid">
        {[
          ...sources.map((s) => ({ ...s, field: "sourceUpdates" as const })),
          ...rules.map((r) => ({ ...r, field: "ruleUpdates" as const })),
        ].map((v) => (
          <Field key={v.field + v.id} label={v.name}>
            <select
              value={profile[v.field]?.[v.id] || "DIRECT"}
              onChange={(e) =>
                onChange({
                  ...profile,
                  [v.field]: { ...profile[v.field], [v.id]: e.target.value },
                })
              }
            >
              {profile[v.field]?.[v.id] &&
                !options.includes(profile[v.field]![v.id]) && (
                  <option value={profile[v.field]![v.id]}>
                    {t(
                      "出站已不存在，请重新选择",
                      "Missing route; select again",
                    )}
                  </option>
                )}
              {options.map((o) => (
                <option key={o} value={o}>
                  {o === "DIRECT" ? t("直连", "Direct") : o}
                </option>
              ))}
            </select>
          </Field>
        ))}
      </div>
    </section>
  );
}
