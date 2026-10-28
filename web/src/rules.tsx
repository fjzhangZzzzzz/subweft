import { useState, useEffect } from "react";
import { Button, Field, CheckField, EditorDialog } from "./ui";
import type { T } from "./ui";
import type { Rule, Condition } from "./types";
import type { Resource } from "./resources";
export const ruleTypes = [
  "DOMAIN",
  "DOMAIN-SUFFIX",
  "DOMAIN-KEYWORD",
  "DOMAIN-REGEX",
  "IP-CIDR",
  "IP-CIDR6",
  "SRC-IP-CIDR",
  "SRC-IP-CIDR6",
  "GEOIP",
  "SRC-GEOIP",
  "IP-ASN",
  "SRC-IP-ASN",
  "GEOSITE",
  "DST-PORT",
  "SRC-PORT",
  "IN-PORT",
  "PROCESS-NAME",
  "PROCESS-PATH",
  "PROCESS-NAME-WILDCARD",
  "PROCESS-PATH-WILDCARD",
  "PROCESS-NAME-REGEX",
  "PROCESS-PATH-REGEX",
  "NETWORK",
  "IN-NAME",
  "IN-TYPE",
  "IN-USER",
  "DSCP",
  "AND",
  "OR",
  "NOT",
];
const logical = (type: string) => ["AND", "OR", "NOT"].includes(type);
const initial = (): Condition => ({
  type: "DOMAIN-SUFFIX",
  value: "",
  noResolve: false,
});
const labels: Record<string, string> = {
  DOMAIN: "完整域名",
  "DOMAIN-SUFFIX": "域名后缀",
  "DOMAIN-KEYWORD": "域名关键词",
  "DOMAIN-REGEX": "域名正则",
  "IP-CIDR": "IPv4 地址段",
  "IP-CIDR6": "IPv6 地址段",
  "SRC-IP-CIDR": "来源 IPv4 地址段",
  "SRC-IP-CIDR6": "来源 IPv6 地址段",
  GEOIP: "IP 地区",
  GEOSITE: "网站分类",
  "IP-ASN": "目标自治系统",
  "SRC-IP-ASN": "来源自治系统",
  "SRC-GEOIP": "来源 IP 地区",
  "DST-PORT": "目标端口",
  "SRC-PORT": "来源端口",
  "IN-PORT": "入站端口",
  "PROCESS-NAME": "进程名称",
  "PROCESS-PATH": "进程路径",
  NETWORK: "网络协议",
  "RULE-SET": "规则集",
  MATCH: "其余流量",
  AND: "同时满足",
  OR: "任一满足",
  NOT: "不满足",
};
export const ruleLabel = (type: string, t: T) =>
  labels[type] ? t(labels[type], type) : type;
export function ConditionForm({
  value,
  onChange,
  t,
  depth = 0,
}: {
  value: Condition;
  onChange: (v: Condition) => void;
  t: T;
  depth?: number;
}) {
  return (
    <div className="schema-field">
      <div className="form-grid">
        <Field label={t("条件类型", "Condition type")}>
          <select
            value={value.type}
            onChange={(e) =>
              onChange({
                ...initial(),
                type: e.target.value,
                ...(logical(e.target.value) ? { children: [initial()] } : {}),
              })
            }
          >
            {ruleTypes
              .filter((k) => depth < 7 || !logical(k))
              .map((k) => (
                <option key={k} value={k}>
                  {ruleLabel(k, t)}
                </option>
              ))}
          </select>
        </Field>
        {!logical(value.type) && (
          <Field label={t("匹配内容", "Match value")}>
            <input
              value={value.value}
              onChange={(e) => onChange({ ...value, value: e.target.value })}
            />
          </Field>
        )}
      </div>
      {logical(value.type) ? (
        <>
          {(value.children || []).map((c, i) => (
            <div className="schema-row" key={i}>
              <ConditionForm
                value={c}
                onChange={(v) =>
                  onChange({
                    ...value,
                    children: value.children!.map((x, j) => (i === j ? v : x)),
                  })
                }
                t={t}
                depth={depth + 1}
              />
              <Button
                disabled={value.children!.length === 1}
                aria-label={t("删除条件", "Delete condition")}
                onClick={() =>
                  onChange({
                    ...value,
                    children: value.children!.filter((_, j) => j !== i),
                  })
                }
              >
                ×
              </Button>
            </div>
          ))}
          {value.type !== "NOT" && (
            <Button
              onClick={() =>
                onChange({
                  ...value,
                  children: [...(value.children || []), initial()],
                })
              }
            >
              {t("添加条件", "Add condition")}
            </Button>
          )}
        </>
      ) : (
        <CheckField
          label={t(
            "不解析域名（IP 匹配）",
            "Skip DNS resolution for IP matching",
          )}
          checked={value.noResolve}
          onChange={(noResolve) => onChange({ ...value, noResolve })}
        />
      )}
    </div>
  );
}

export function RulesEditor({
  rules,
  onChange,
  targets,
  resources,
  t,
  onManageResources,
  onDirtyChange,
}: {
  rules: Rule[];
  onChange: (v: Rule[]) => void;
  targets: string[];
  resources: Resource[];
  t: T;
  onManageResources?: () => void;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<number[]>([]);
  const [edit, setEdit] = useState<{
    index: number;
    value: Rule;
    original: Rule;
  }>();
  const [batchOpen, setBatchOpen] = useState(false);
  const [batch, setBatch] = useState("");
  const [batchType, setBatchType] = useState("DOMAIN-SUFFIX");
  const [batchTarget, setBatchTarget] = useState(targets[0] || "DIRECT");
  const dialogDirty =
    (!!edit && JSON.stringify(edit.value) !== JSON.stringify(edit.original)) ||
    (batchOpen && !!batch.trim());
  useEffect(() => {
    onDirtyChange(dialogDirty);
    return () => onDirtyChange(false);
  }, [dialogDirty, onDirtyChange]);
  const update = (i: number, r: Rule) =>
    onChange(rules.map((v, j) => (i === j ? r : v)));
  const add = (items: Rule[]) => {
    const next = [...rules];
    const at = next.findIndex((r) => r.type === "MATCH");
    next.splice(at < 0 ? next.length : at, 0, ...items);
    onChange(next);
    setSelected([]);
  };
  const open = (index: number, value: Rule) =>
    setEdit({
      index,
      value: structuredClone(value),
      original: structuredClone(value),
    });
  const change = (v: Partial<Rule>) =>
    setEdit(edit && { ...edit, value: { ...edit.value, ...v } });
  const filtered = rules
    .map((r, i) => ({ r, i }))
    .filter(({ r }) => r.type !== "MATCH")
    .filter(({ r }) =>
      (
        ruleLabel(r.type, t) +
        " " +
        r.value +
        " " +
        r.target +
        " " +
        (resources.find((x) => x.id === r.value)?.name || "")
      )
        .toLowerCase()
        .includes(query.toLowerCase()),
    );
  const pages = Math.max(1, Math.ceil(filtered.length / 50));
  const current = Math.min(page, pages - 1);
  const move = (i: number, offset: number) => {
    const next = [...rules];
    [next[i], next[i + offset]] = [next[i + offset], next[i]];
    onChange(next);
    setSelected([]);
  };
  const fallbackIndex = rules.findIndex((r) => r.type === "MATCH");
  const fallback = rules[fallbackIndex];
  return (
    <>
      <div className="section-intro">
        <div>
          <h3>{t("分流规则", "Routing rules")}</h3>
          <p>
            {t(
              "从上到下匹配，第一条命中即生效。",
              "The first matching rule takes effect.",
            )}
          </p>
        </div>
        <div className="actions">
          <Button onClick={() => setBatchOpen(true)}>
            {t("批量添加", "Add in batch")}
          </Button>
          <Button
            primary
            onClick={() =>
              open(-1, {
                ...initial(),
                target: targets[0] || "DIRECT",
                enabled: true,
              })
            }
          >
            {t("添加规则", "Add rule")}
          </Button>
        </div>
      </div>
      <Field label={t("搜索规则", "Search rules")}>
        <input
          type="search"
          value={query}
          placeholder={t(
            "搜索匹配内容、类型或去向",
            "Match value, type or destination",
          )}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
      </Field>
      <div className="actions selection-bar">
        <Button
          onClick={() =>
            setSelected(
              filtered.slice(current * 50, current * 50 + 50).map((v) => v.i),
            )
          }
        >
          {t("选择本页", "Select page")}
        </Button>
        <span className="muted">
          {filtered.length} {t("条规则", "rules")}
        </span>
        {selected.length > 0 && (
          <>
            <span>
              {t("已选", "Selected")} {selected.length}
            </span>
            <Button onClick={() => setSelected([])}>
              {t("取消选择", "Clear")}
            </Button>
            {[true, false].map((enabled) => (
              <Button
                key={String(enabled)}
                onClick={() =>
                  onChange(
                    rules.map((r, i) =>
                      selected.includes(i) ? { ...r, enabled } : r,
                    ),
                  )
                }
              >
                {enabled
                  ? t("启用所选", "Enable selected")
                  : t("停用所选", "Disable selected")}
              </Button>
            ))}
            <Button
              danger
              onClick={() => {
                if (confirm(t("删除所选规则？", "Delete selected rules?"))) {
                  onChange(rules.filter((_, i) => !selected.includes(i)));
                  setSelected([]);
                }
              }}
            >
              {t("删除所选", "Delete selected")}
            </Button>
          </>
        )}
      </div>
      <div className="table-scroll">
        <table className="compact-table responsive-cards">
          <thead>
            <tr>
              <th>{t("顺序", "Order")}</th>
              <th>{t("匹配条件", "Match")}</th>
              <th>{t("去向", "Destination")}</th>
              <th>{t("启用", "Enabled")}</th>
              <th>{t("操作", "Actions")}</th>
            </tr>
          </thead>
          <tbody>
            {filtered.slice(current * 50, current * 50 + 50).map(({ r, i }) => (
              <tr key={i} className={r.enabled ? "" : "muted"}>
                <td data-label={t("顺序", "Order")}>
                  <CheckField
                    label={String(i + 1)}
                    checked={selected.includes(i)}
                    onChange={(v) =>
                      setSelected(
                        v ? [...selected, i] : selected.filter((x) => x !== i),
                      )
                    }
                  />
                </td>
                <td data-label={t("匹配条件", "Match")}>
                  <strong>{ruleLabel(r.type, t)}</strong>
                  <small className="cell-detail">
                    {r.type === "RULE-SET"
                      ? resources.find((x) => x.id === r.value)?.name ||
                        t("规则集不存在", "Missing ruleset")
                      : logical(r.type)
                        ? t("组合条件", "Combined conditions") +
                          " · " +
                          (r.conditions?.length || 0)
                        : r.value}
                  </small>
                </td>
                <td data-label={t("去向", "Destination")}>
                  {r.target === "DIRECT"
                    ? t("直连", "DIRECT")
                    : r.target === "REJECT"
                      ? t("拒绝", "REJECT")
                      : r.target}
                </td>
                <td data-label={t("启用", "Enabled")}>
                  <CheckField
                    label={t("启用规则", "Enable rule") + " " + (i + 1)}
                    checked={r.enabled}
                    onChange={(enabled) => update(i, { ...r, enabled })}
                  />
                </td>
                <td data-label={t("操作", "Actions")}>
                  <div className="actions">
                    <Button onClick={() => open(i, r)}>
                      {t("编辑", "Edit")}
                    </Button>
                    <Button
                      aria-label={t("上移规则", "Move rule up") + " " + (i + 1)}
                      disabled={i === 0}
                      onClick={() => move(i, -1)}
                    >
                      ↑
                    </Button>
                    <Button
                      aria-label={
                        t("下移规则", "Move rule down") + " " + (i + 1)
                      }
                      disabled={
                        i === rules.length - 1 || rules[i + 1]?.type === "MATCH"
                      }
                      onClick={() => move(i, 1)}
                    >
                      ↓
                    </Button>
                    <details className="row-more">
                      <summary
                        aria-label={
                          t("更多操作", "More actions") + " " + (i + 1)
                        }
                      >
                        ···
                      </summary>
                      <Button onClick={() => open(-1, r)}>
                        {t("复制", "Copy")}
                      </Button>
                      <Button
                        danger
                        onClick={() => {
                          if (
                            confirm(t("删除这条规则？", "Delete this rule?"))
                          ) {
                            onChange(rules.filter((_, j) => i !== j));
                            setSelected([]);
                          }
                        }}
                      >
                        {t("删除", "Delete")}
                      </Button>
                    </details>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!filtered.length && (
          <p className="empty-list">
            {query
              ? t("没有匹配的规则", "No matching rules")
              : t(
                  "尚无分流规则，所有流量使用下方默认去向。",
                  "No rules yet. All traffic uses the fallback below.",
                )}
          </p>
        )}
      </div>
      {pages > 1 && (
        <div className="actions pagination">
          <Button disabled={current === 0} onClick={() => setPage(current - 1)}>
            ←
          </Button>
          <span>
            {current + 1} / {pages}
          </span>
          <Button
            disabled={current + 1 >= pages}
            onClick={() => setPage(current + 1)}
          >
            →
          </Button>
        </div>
      )}
      <div className="fallback-row">
        <div>
          <strong>{t("其余流量", "All other traffic")}</strong>
          <p className="muted">
            {t("没有命中上方规则时使用", "Used when no rule above matches")}
          </p>
        </div>
        <Field label={t("默认去向", "Fallback destination")}>
          <select
            value={fallback?.target || ""}
            onChange={(e) => {
              const r: Rule = {
                type: "MATCH",
                value: "",
                target: e.target.value,
                enabled: true,
                noResolve: false,
              };
              fallback ? update(fallbackIndex, r) : onChange([...rules, r]);
            }}
          >
            <option value="" disabled>
              {t("请选择去向", "Select destination")}
            </option>
            {targets.map((v) => (
              <option key={v} value={v}>
                {v === "DIRECT"
                  ? t("直连", "DIRECT")
                  : v === "REJECT"
                    ? t("拒绝", "REJECT")
                    : v}
              </option>
            ))}
          </select>
        </Field>
        {fallback && (
          <CheckField
            label={t("启用", "Enabled")}
            checked={fallback.enabled}
            onChange={(enabled) =>
              update(fallbackIndex, { ...fallback, enabled })
            }
          />
        )}
      </div>
      {edit && (
        <EditorDialog
          title={
            edit.index < 0
              ? t("添加规则", "Add rule")
              : t("编辑规则", "Edit rule")
          }
          t={t}
          dirty={JSON.stringify(edit.value) !== JSON.stringify(edit.original)}
          onClose={() => setEdit(undefined)}
          footer={
            <Button
              primary
              disabled={
                !edit.value.target ||
                (!logical(edit.value.type) && !edit.value.value.trim())
              }
              onClick={() => {
                edit.index < 0
                  ? add([edit.value])
                  : update(edit.index, edit.value);
                setEdit(undefined);
              }}
            >
              {t("保存规则", "Save rule")}
            </Button>
          }
        >
          <div className="form-grid">
            <Field label={t("匹配方式", "Match type")}>
              <select
                value={edit.value.type}
                onChange={(e) =>
                  change({
                    type: e.target.value,
                    value: "",
                    noResolve: false,
                    conditions: logical(e.target.value) ? [initial()] : [],
                  })
                }
              >
                {[...ruleTypes, "RULE-SET"].map((k) => (
                  <option key={k} value={k}>
                    {ruleLabel(k, t)}
                  </option>
                ))}
              </select>
            </Field>
            <Field label={t("去向", "Destination")}>
              <select
                value={edit.value.target}
                onChange={(e) => change({ target: e.target.value })}
              >
                {targets.map((v) => (
                  <option key={v} value={v}>
                    {v === "DIRECT"
                      ? t("直连", "DIRECT")
                      : v === "REJECT"
                        ? t("拒绝", "REJECT")
                        : v}
                  </option>
                ))}
              </select>
            </Field>
          </div>
          {edit.value.type === "RULE-SET" ? (
            <>
              <Field label={t("规则集", "Ruleset")}>
                <select
                  value={edit.value.value}
                  onChange={(e) => change({ value: e.target.value })}
                >
                  <option value="">{t("选择规则集", "Select ruleset")}</option>
                  {resources
                    .filter((v) => v.type === "ruleset")
                    .map((v) => (
                      <option key={v.id} value={v.id}>
                        {v.name}
                      </option>
                    ))}
                </select>
              </Field>
              {onManageResources && (
                <Button onClick={onManageResources}>
                  {t("添加或管理规则集", "Add or manage rulesets")}
                </Button>
              )}
            </>
          ) : logical(edit.value.type) ? (
            <ConditionForm
              t={t}
              value={{ ...edit.value, children: edit.value.conditions }}
              onChange={(v) => change({ ...v, conditions: v.children })}
            />
          ) : (
            <Field label={t("匹配内容", "Match value")}>
              <input
                value={edit.value.value}
                placeholder={
                  edit.value.type.startsWith("DOMAIN") ? "example.com" : ""
                }
                onChange={(e) => change({ value: e.target.value })}
              />
            </Field>
          )}
          {!logical(edit.value.type) && (
            <details>
              <summary>{t("高级选项", "Advanced options")}</summary>
              <CheckField
                label={t(
                  "不解析域名（IP 匹配）",
                  "Skip DNS resolution for IP matching",
                )}
                checked={edit.value.noResolve}
                onChange={(noResolve) => change({ noResolve })}
              />
            </details>
          )}
          <CheckField
            label={t("启用规则", "Enable rule")}
            checked={edit.value.enabled}
            onChange={(enabled) => change({ enabled })}
          />
        </EditorDialog>
      )}
      {batchOpen && (
        <EditorDialog
          title={t("批量添加规则", "Add rules in batch")}
          t={t}
          dirty={!!batch.trim()}
          onClose={() => {
            setBatchOpen(false);
            setBatch("");
          }}
          footer={
            <Button
              primary
              disabled={!batch.trim()}
              onClick={() => {
                add(
                  batch
                    .split(/\r?\n/)
                    .map((v) => v.trim())
                    .filter(Boolean)
                    .map((value) => ({
                      type: batchType,
                      value,
                      target: batchTarget,
                      enabled: true,
                      noResolve: false,
                    })),
                );
                setBatch("");
                setBatchOpen(false);
              }}
            >
              {t("添加规则", "Add rules")}
            </Button>
          }
        >
          <div className="form-grid">
            <Field label={t("匹配方式", "Match type")}>
              <select
                value={batchType}
                onChange={(e) => setBatchType(e.target.value)}
              >
                {ruleTypes
                  .filter((k) => !logical(k))
                  .map((k) => (
                    <option key={k} value={k}>
                      {ruleLabel(k, t)}
                    </option>
                  ))}
              </select>
            </Field>
            <Field label={t("去向", "Destination")}>
              <select
                value={batchTarget}
                onChange={(e) => setBatchTarget(e.target.value)}
              >
                {targets.map((v) => (
                  <option key={v}>{v}</option>
                ))}
              </select>
            </Field>
          </div>
          <Field
            label={t("匹配内容（每行一条）", "Match values (one per line)")}
          >
            <textarea
              rows={8}
              value={batch}
              onChange={(e) => setBatch(e.target.value)}
            />
          </Field>
        </EditorDialog>
      )}
    </>
  );
}
