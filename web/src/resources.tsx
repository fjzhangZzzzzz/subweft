import { HeaderFields } from "./device-updates";
import { ruleTypes, ruleLabel } from "./rules";
import { SecretControl } from "./schema-form";
import { useEffect, useState } from "react";
import { Button, Field, CheckField } from "./ui";
import type { T } from "./ui";
import type { Meta } from "./types";
export type Resource = Meta & {
  headers?: Record<string, string>;
  url?: string;
  name: string;
  kind: string;
  type: string;
  behavior: string;
  format: string;
  enabled: boolean;
  interval: number;
  digest: string;
  count: number;
  lastError: string;
  lastSuccess: string;
};
type API = <T>(path: string, method?: string, body?: unknown) => Promise<T>;
type Run = (
  work: () => Promise<unknown>,
  success?: string,
  close?: boolean,
) => Promise<void>;
export function ResourcesPanel({
  values,
  t,
  request,
  onRun,
  busy,
  onDirtyChange,
}: {
  values: Resource[];
  t: T;
  request: API;
  onRun: Run;
  busy: boolean;
  onDirtyChange: (v: boolean) => void;
}) {
  const [edit, setEdit] = useState<Resource>();
  const [entries, setEntries] = useState<string[]>();
  const [query, setQuery] = useState("");
  const open = (v?: Resource) =>
    void onRun(async () => {
      setEntries(
        v?.type === "ruleset" && v.kind === "local" && v.digest
          ? await request<string[]>("resources/" + v.id + "/entries")
          : [],
      );
      setEdit(
        (v
          ? await request<Resource>("resources/" + v.id + "/edit")
          : undefined) || {
          id: "",
          revision: 0,
          name: "",
          kind: "local",
          type: "ruleset",
          behavior: "domain",
          format: "yaml",
          enabled: true,
          interval: 86400,
          digest: "",
          count: 0,
          lastError: "",
          lastSuccess: "",
        },
      );
    });
  if (edit)
    return (
      <ResourceForm
        key={edit.id}
        value={edit}
        initialEntries={entries || []}
        onDirtyChange={onDirtyChange}
        t={t}
        busy={busy}
        onCancel={() => {
          setEdit(undefined);
          onDirtyChange(false);
        }}
        onSave={(v) =>
          void onRun(
            async () => {
              await request<Resource>(
                edit.id ? "resources/" + edit.id : "resources",
                edit.id ? "PUT" : "POST",
                v,
              );
              setEdit(undefined);
              onDirtyChange(false);
            },
            t(
              "已保存；应用配置后客户端生效",
              "Resource saved; publish to apply",
            ),
          )
        }
      />
    );
  return (
    <section>
      <div className="page-heading">
        <div>
          <span className="eyebrow">RULES & RESOURCES</span>
          <h1>{t("规则集与地理数据", "Rulesets & geodata")}</h1>
          <p>
            {t(
              "本地规则在发布时固定；远程规则和地理数据由设备下载更新。",
              "Publishing freezes local rules. Devices download and update remote rules and geodata.",
            )}
          </p>
        </div>
        <Button primary onClick={() => open()}>
          {t("添加规则集或地理数据", "Add resource")}
        </Button>
      </div>
      <Field label={t("搜索名称", "Search resources")}>
        <input value={query} onChange={(e) => setQuery(e.target.value)} />
      </Field>
      <div className="resource-grid">
        {values
          .filter((v) => v.name.toLowerCase().includes(query.toLowerCase()))
          .map((v) => (
            <article className="panel" key={v.id}>
              <h3>{v.name}</h3>
              <p>
                {v.type === "ruleset"
                  ? t("规则集", "Ruleset")
                  : v.type.toUpperCase()}{" "}
                ·{" "}
                {v.kind === "http"
                  ? t("由设备更新", "Updated by devices")
                  : `${v.count} ${t("条目", "entries")}`}
              </p>
              <p className="muted">
                {v.kind === "http"
                  ? t("远程定义", "Remote definition")
                  : v.digest
                    ? t("内容已就绪", "Ready")
                    : t("尚未更新", "Not fetched")}{" "}
                · {v.enabled ? t("启用", "Enabled") : t("停用", "Disabled")}
              </p>
              {v.kind !== "http" && v.lastError && (
                <p role="status" className="alert error">
                  {v.lastError}
                </p>
              )}
              <div className="actions">
                <Button onClick={() => open(v)}>{t("编辑", "Edit")}</Button>
                <Button
                  danger
                  onClick={() => {
                    if (
                      confirm(
                        t(
                          "确认删除资源？被引用时会拒绝。",
                          "Delete resource? Referenced resources will be protected.",
                        ),
                      )
                    )
                      void onRun(() =>
                        request("resources/" + v.id, "DELETE", {
                          revision: v.revision,
                        }),
                      );
                  }}
                >
                  {t("删除", "Delete")}
                </Button>
              </div>
            </article>
          ))}
      </div>
    </section>
  );
}
function ResourceForm({
  value,
  initialEntries,
  onDirtyChange,
  t,
  busy,
  onSave,
  onCancel,
}: {
  value: Resource;
  initialEntries: string[];
  onDirtyChange: (dirty: boolean) => void;
  t: T;
  busy: boolean;
  onSave: (v: unknown) => void;
  onCancel: () => void;
}) {
  const [draft, setDraft] = useState(value);
  const [url, setURL] = useState(value.url || "");
  const [entries, setEntries] = useState(initialEntries);
  const [upload, setUpload] = useState<string>();
  const [batch, setBatch] = useState("");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  const [fileError, setFileError] = useState("");
  const dirty =
    JSON.stringify(draft) !== JSON.stringify(value) ||
    url !== (value.url || "") ||
    JSON.stringify(entries) !== JSON.stringify(initialEntries) ||
    !!upload ||
    !!batch.trim();
  useEffect(() => {
    onDirtyChange(dirty);
    return () => onDirtyChange(false);
  }, [dirty, onDirtyChange]);
  const cancel = () => {
    if (
      !dirty ||
      confirm(t("放弃未保存的表单修改？", "Discard unsaved form changes?"))
    )
      onCancel();
  };
  const set = (k: string, v: unknown) => setDraft({ ...draft, [k]: v });
  const filtered = entries
    .map((v, i) => ({ v, i }))
    .filter(({ v }) => v.toLowerCase().includes(query.toLowerCase()));
  return (
    <form
      className="panel settings-form"
      onSubmit={(e) => {
        e.preventDefault();
        const isLocalRuleset =
          draft.kind === "local" && draft.type === "ruleset";
        if (
          (batch.trim() ||
            JSON.stringify(entries) !== JSON.stringify(initialEntries)) &&
          !isLocalRuleset &&
          !confirm(
            t(
              "修改的条目不会保存到当前类型，仍要继续？",
              "Edited entries cannot be saved for this type. Continue?",
            ),
          )
        )
          return;
        const { digest, count, lastError, lastSuccess, ...definition } = draft;
        onSave({
          ...definition,
          url,
          entries: isLocalRuleset
            ? [
                ...entries,
                ...batch
                  .split(/\r?\n/)
                  .map((v) => v.trim())
                  .filter(Boolean),
              ]
            : null,
          ...(upload ? { data: upload } : {}),
        });
      }}
    >
      <h2>
        {value.id
          ? t("编辑规则集或地理数据", "Edit resource")
          : t("添加规则集或地理数据", "Add resource")}
      </h2>
      <Field name="name" label={t("名称", "Name")}>
        <input
          required
          value={draft.name}
          onChange={(e) => set("name", e.target.value)}
        />
      </Field>
      <div className="form-grid">
        <Field label={t("内容类型", "Content type")}>
          <select
            value={draft.type}
            onChange={(e) => {
              set("type", e.target.value);
              setUpload(undefined);
            }}
          >
            {["ruleset", "mmdb", "geoip", "geosite", "asn"].map((k) => (
              <option key={k} value={k}>
                {(
                  {
                    ruleset: t("规则集", "Ruleset"),
                    mmdb: "GeoIP (MMDB)",
                    geoip: "GeoIP (DAT)",
                    geosite: "GeoSite",
                    asn: "ASN",
                  } as Record<string, string>
                )[k] || k}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t("来源方式", "Acquisition")}>
          <select
            value={draft.kind}
            onChange={(e) => set("kind", e.target.value)}
          >
            <option value="http">{t("订阅远程规则", "Remote URL")}</option>
            <option value="local">{t("手动维护", "Edit locally")}</option>
          </select>
        </Field>
      </div>
      {draft.type === "ruleset" && (
        <div className="form-grid">
          <Field label={t("匹配类型", "Behavior")}>
            <select
              value={draft.behavior}
              onChange={(e) => set("behavior", e.target.value)}
            >
              {["domain", "ipcidr", "classical"].map((k) => (
                <option key={k} value={k}>
                  {
                    (
                      {
                        domain: t("域名", "Domains"),
                        ipcidr: t("IP 地址段", "IP ranges"),
                        classical: t("混合规则", "Mixed rules"),
                      } as Record<string, string>
                    )[k]
                  }
                </option>
              ))}
            </select>
          </Field>
          <Field label={t("格式", "Format")}>
            <select
              value={draft.format}
              onChange={(e) => set("format", e.target.value)}
            >
              <option value="yaml">YAML</option>
              <option value="text">{t("纯文本", "Plain text")}</option>
            </select>
          </Field>
        </div>
      )}
      {draft.kind === "http" ? (
        <>
          <Field
            label={t("下载地址（留空保留）", "Resource URL (blank retains)")}
          >
            <SecretControl
              t={t}
              name={t("下载地址", "Resource URL")}
              value={url}
              required={!draft.id}
              onChange={setURL}
            />
          </Field>
          {draft.type === "ruleset" && (
            <HeaderFields
              t={t}
              value={draft.headers || {}}
              onChange={(headers) => set("headers", headers)}
            />
          )}
          {draft.type === "ruleset" ? (
            <Field label={t("更新间隔（秒）", "Refresh interval (seconds)")}>
              <input
                type="number"
                min={60}
                max={604800}
                value={draft.interval}
                onChange={(e) => set("interval", +e.target.value)}
              />
            </Field>
          ) : (
            <p className="muted">
              {t(
                "地理数据更新周期在配置的连接设置中统一维护；首次下载需要设备可直连该地址。",
                "Manage geodata update intervals in profile connection settings. Initial downloads require direct device reachability.",
              )}
            </p>
          )}
        </>
      ) : draft.type === "ruleset" ? (
        <>
          <h3>
            {t("规则条目", "Rule entries")} · {entries.length}
          </h3>
          <p className="muted">
            {draft.behavior === "classical"
              ? t(
                  "选择匹配方式并填写内容；去向由使用此规则集的配置决定。",
                  "Choose a match type and value. The profile selects the destination.",
                )
              : t(
                  "逐项维护域名或 CIDR。支持批量添加。",
                  "Edit domains or CIDRs individually, or add a batch.",
                )}
          </p>
          <Field label={t("搜索条目", "Search entries")}>
            <input
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setPage(0);
              }}
            />
          </Field>
          {filtered.slice(page * 50, page * 50 + 50).map(({ v, i }) => (
            <div className="schema-row" key={i}>
              {draft.behavior === "classical" && (
                <Field label={t("匹配方式", "Match type")}>
                  <select
                    value={v.split(",")[0]}
                    onChange={(e) =>
                      setEntries(
                        entries.map((x, j) =>
                          i === j
                            ? e.target.value + "," + v.slice(v.indexOf(",") + 1)
                            : x,
                        ),
                      )
                    }
                  >
                    {[...new Set([...ruleTypes, v.split(",")[0]])].map((k) => (
                      <option key={k} value={k}>
                        {ruleLabel(k, t)}
                      </option>
                    ))}
                  </select>
                </Field>
              )}
              <Field
                label={
                  draft.behavior === "classical"
                    ? t("匹配内容", "Match value")
                    : String(i + 1)
                }
              >
                <input
                  value={
                    draft.behavior === "classical"
                      ? v.slice(v.indexOf(",") + 1)
                      : v
                  }
                  onChange={(e) =>
                    setEntries(
                      entries.map((v, j) =>
                        i === j
                          ? draft.behavior === "classical"
                            ? v.split(",")[0] + "," + e.target.value
                            : e.target.value
                          : v,
                      ),
                    )
                  }
                />
              </Field>
              <Button
                aria-label={t("删除条目", "Delete entry")}
                onClick={() => setEntries(entries.filter((_, j) => j !== i))}
              >
                ×
              </Button>
            </div>
          ))}
          <Button
            onClick={() =>
              setEntries([
                ...entries,
                draft.behavior === "classical" ? "DOMAIN-SUFFIX," : "",
              ])
            }
          >
            {t("添加条目", "Add entry")}
          </Button>
          <div className="actions">
            <Button disabled={page === 0} onClick={() => setPage(page - 1)}>
              ←
            </Button>
            <span>
              {page + 1} / {Math.max(1, Math.ceil(filtered.length / 50))}
            </span>
            <Button
              disabled={(page + 1) * 50 >= filtered.length}
              onClick={() => setPage(page + 1)}
            >
              →
            </Button>
          </div>
          <Field
            label={t("批量添加（每行一项）", "Add batch (one entry per line)")}
          >
            <textarea
              rows={5}
              value={batch}
              onChange={(e) => setBatch(e.target.value)}
            />
          </Field>
          <Button
            onClick={() => {
              setEntries([
                ...entries,
                ...batch
                  .split(/\r?\n/)
                  .map((v) => v.trim())
                  .filter(Boolean),
              ]);
              setBatch("");
            }}
            disabled={!batch.trim()}
          >
            {t("添加这些条目", "Add these entries")}
          </Button>
        </>
      ) : (
        <Field
          label={t(
            "上传数据文件（最多 64 MiB）",
            "Upload data file (up to 64 MiB)",
          )}
        >
          <input
            type="file"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (!f) return;
              if (f.size > 64 * 1024 * 1024) {
                setFileError(t("文件超出大小限制", "File exceeds size limit"));
                return;
              }
              const reader = new FileReader();
              reader.onload = () => {
                setUpload(String(reader.result).split(",")[1]);
                setFileError("");
              };
              reader.readAsDataURL(f);
            }}
          />
        </Field>
      )}
      {fileError && <p role="alert">{fileError}</p>}
      <CheckField
        label={t("启用", "Enable resource")}
        checked={draft.enabled}
        onChange={(v) => set("enabled", v)}
      />
      <div className="form-footer">
        <Button onClick={cancel}>{t("取消", "Cancel")}</Button>
        <Button primary type="submit" disabled={busy}>
          {t("保存", "Save resource")}
        </Button>
      </div>
    </form>
  );
}
