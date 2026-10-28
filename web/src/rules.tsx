import { useState } from "react";
import { Button, Field, CheckField } from "./ui";
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
                <option key={k}>{k}</option>
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
          label="no-resolve"
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
}: {
  rules: Rule[];
  onChange: (v: Rule[]) => void;
  targets: string[];
  resources: Resource[];
  t: T;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<number[]>([]);
  const [batch, setBatch] = useState("");
  const [batchType, setBatchType] = useState("DOMAIN-SUFFIX");
  const [batchTarget, setBatchTarget] = useState(targets[0]);
  const update = (i: number, r: Rule) =>
    onChange(rules.map((v, j) => (i === j ? r : v)));
  const add = (newRules: Rule[]) => {
    const n = [...rules];
    const index = n.findIndex((r) => r.type === "MATCH");
    n.splice(index < 0 ? n.length : index, 0, ...newRules);
    onChange(n);
  };
  const filtered = rules
    .map((r, i) => ({ r, i }))
    .filter(({ r }) =>
      (r.type + " " + r.value + " " + r.target)
        .toLowerCase()
        .includes(query.toLowerCase()),
    );
  const move = (i: number, offset: number) => {
    const n = [...rules];
    [n[i], n[i + offset]] = [n[i + offset], n[i]];
    onChange(n);
    setSelected([]);
  };
  return (
    <>
      <div className="section-intro">
        <div>
          <h3>{t("规则顺序", "Ordered rules")}</h3>
          <p>
            {t(
              "自上而下匹配；规则集和逻辑条件均可通过表单编辑。",
              "Match top to bottom; edit rulesets and logical conditions with forms.",
            )}
          </p>
        </div>
        <Button
          onClick={() =>
            add([{ ...initial(), target: targets[0], enabled: true }])
          }
        >
          {t("添加规则", "Add rule")}
        </Button>
      </div>
      <Field label={t("搜索规则", "Search rules")}>
        <input
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
      </Field>
      <div className="actions">
        <Button
          onClick={() =>
            setSelected(
              filtered.slice(page * 50, page * 50 + 50).map((v) => v.i),
            )
          }
        >
          {t("选择本页", "Select page")}
        </Button>
        <Button
          disabled={!selected.length}
          onClick={() =>
            onChange(
              rules.map((v, i) =>
                selected.includes(i) ? { ...v, enabled: true } : v,
              ),
            )
          }
        >
          {t("启用所选", "Enable selected")}
        </Button>
        <Button
          disabled={!selected.length}
          onClick={() =>
            onChange(
              rules.map((v, i) =>
                selected.includes(i) ? { ...v, enabled: false } : v,
              ),
            )
          }
        >
          {t("停用所选", "Disable selected")}
        </Button>
        <Button
          disabled={!selected.length}
          danger
          onClick={() => {
            onChange(rules.filter((_, i) => !selected.includes(i)));
            setSelected([]);
          }}
        >
          {t("删除所选", "Delete selected")}
        </Button>
      </div>
      <div className="rule-list">
        {filtered.slice(page * 50, page * 50 + 50).map(({ r, i }) => (
          <article className="panel" key={i}>
            <div className="actions">
              <CheckField
                label={String(i + 1)}
                checked={selected.includes(i)}
                onChange={(v) =>
                  setSelected(
                    v ? [...selected, i] : selected.filter((x) => x !== i),
                  )
                }
              />
              <CheckField
                label={t("启用", "Enabled")}
                checked={r.enabled}
                onChange={(enabled) => update(i, { ...r, enabled })}
              />
              <Button
                disabled={i === 0}
                aria-label={t("上移规则", "Move rule up") + " " + (i + 1)}
                onClick={() => move(i, -1)}
              >
                ↑
              </Button>
              <Button
                disabled={i === rules.length - 1}
                aria-label={t("下移规则", "Move rule down") + " " + (i + 1)}
                onClick={() => move(i, 1)}
              >
                ↓
              </Button>
              <Button
                onClick={() => {
                  const n = [...rules];
                  n.splice(i + 1, 0, structuredClone(r));
                  onChange(n);
                }}
              >
                {t("复制", "Copy")}
              </Button>
              <Button
                danger
                onClick={() => onChange(rules.filter((_, j) => i !== j))}
              >
                {t("删除", "Delete")}
              </Button>
            </div>
            <div className="form-grid">
              <Field label={t("规则类型", "Rule type")}>
                <select
                  value={r.type}
                  onChange={(e) =>
                    update(i, {
                      ...r,
                      type: e.target.value,
                      value: "",
                      noResolve: false,
                      conditions: logical(e.target.value) ? [initial()] : [],
                    })
                  }
                >
                  {[...ruleTypes, "RULE-SET", "MATCH"].map((k) => (
                    <option key={k}>{k}</option>
                  ))}
                </select>
              </Field>
              <Field label={t("目标", "Target")}>
                <select
                  value={r.target}
                  onChange={(e) => update(i, { ...r, target: e.target.value })}
                >
                  {targets.map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              </Field>
            </div>
            {r.type === "RULE-SET" ? (
              <Field label={t("规则集", "Ruleset")}>
                <select
                  value={r.value}
                  onChange={(e) => update(i, { ...r, value: e.target.value })}
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
            ) : logical(r.type) ? (
              <ConditionForm
                value={{
                  type: r.type,
                  value: r.value,
                  noResolve: r.noResolve,
                  children: r.conditions,
                }}
                t={t}
                onChange={(v) =>
                  update(i, {
                    ...r,
                    type: v.type,
                    value: v.value,
                    noResolve: v.noResolve,
                    conditions: v.children,
                  })
                }
              />
            ) : (
              r.type !== "MATCH" && (
                <Field label={t("匹配内容", "Match value")}>
                  <input
                    value={r.value}
                    onChange={(e) => update(i, { ...r, value: e.target.value })}
                  />
                </Field>
              )
            )}
            {!logical(r.type) && r.type !== "MATCH" && (
              <CheckField
                label="no-resolve"
                checked={r.noResolve}
                onChange={(noResolve) => update(i, { ...r, noResolve })}
              />
            )}
          </article>
        ))}
      </div>
      <div className="actions">
        <Button disabled={page === 0} onClick={() => setPage(page - 1)}>
          ←
        </Button>
        <span>
          {page + 1} / {Math.max(1, Math.ceil(filtered.length / 50))} ·{" "}
          {filtered.length}
        </span>
        <Button
          disabled={(page + 1) * 50 >= filtered.length}
          onClick={() => setPage(page + 1)}
        >
          →
        </Button>
      </div>
      <details>
        <summary>{t("批量添加规则", "Add rules in batch")}</summary>
        <div className="form-grid">
          <Field label={t("规则类型", "Rule type")}>
            <select
              value={batchType}
              onChange={(e) => setBatchType(e.target.value)}
            >
              {ruleTypes
                .filter((k) => !logical(k))
                .map((k) => (
                  <option key={k}>{k}</option>
                ))}
            </select>
          </Field>
          <Field label={t("目标", "Target")}>
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
        <Field label={t("匹配内容（每行一条）", "Match values (one per line)")}>
          <textarea
            rows={5}
            value={batch}
            onChange={(e) => setBatch(e.target.value)}
          />
        </Field>
        <Button
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
          }}
        >
          {t("添加到兜底规则之前", "Insert before fallback")}
        </Button>
      </details>
    </>
  );
}
