import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { Button, Field, CheckField, EditorDialog } from "./ui";
import type { T } from "./ui";
import type { Group, Profile, Source, SourcePool, State } from "./types";
import { emptyGroup } from "./types";
import { RulesEditor } from "./rules";

// A purpose is an editing view over a group and the rules targeting it.
// There is deliberately no second purpose entity or reordered rule collection.
export function RoutingWorkspace({
  profile,
  state,
  t,
  onChange,
  onDirtyChange,
  onManageResources,
  renderGroup,
}: {
  profile: Profile;
  state: State;
  t: T;
  onChange: (p: Profile) => void;
  onDirtyChange: (dirty: boolean) => void;
  onManageResources: () => void;
  renderGroup: (
    g: Group,
    index: number,
    change: (g: Group) => void,
    rules: ReactNode,
  ) => ReactNode;
}) {
  const [edit, setEdit] = useState<{
    index: number;
    target: string;
    draft: Profile;
    group: Group;
    original: string;
  }>();
  const [nestedDirty, setNestedDirty] = useState(false);
  const [error, setError] = useState("");
  const [allRules, setAllRules] = useState(false);
  const [remove, setRemove] = useState<string>();
  const pending =
    !!edit &&
    (JSON.stringify({ group: edit.group, rules: edit.draft.rules }) !==
      edit.original ||
      nestedDirty);
  useEffect(() => {
    onDirtyChange(pending || nestedDirty);
    return () => onDirtyChange(false);
  }, [pending, nestedDirty, onDirtyChange]);
  const open = (index: number) => {
    const group = structuredClone(
      index < 0 ? emptyGroup("") : profile.groups[index],
    );
    const draft = structuredClone(profile);
    setError("");
    setNestedDirty(false);
    setEdit({
      index,
      target: index < 0 ? "\0new-purpose" : group.name,
      group,
      draft,
      original: JSON.stringify({ group, rules: draft.rules }),
    });
  };
  const changeGroup = (group: Group) => {
    if (edit) setEdit({ ...edit, group });
  };
  const save = () => {
    if (!edit) return;
    const g = edit.group;
    if (
      !g.name ||
      g.name !== g.name.trim() ||
      /[,\r\n\0]/.test(g.name) ||
      ["DIRECT", "REJECT"].includes(g.name) ||
      profile.groups.some((v, i) => i !== edit.index && v.name === g.name)
    ) {
      setError(
        t(
          "请填写不重复的用途名称，不含逗号或首尾空格。",
          "Enter a unique name without commas or surrounding spaces.",
        ),
      );
      return;
    }
    if (
      !g.sourceIds.length &&
      !(g.pools || []).length &&
      !g.nodeIds.length &&
      !g.groups.length &&
      !g.builtins.length &&
      !g.allSources &&
      !g.tags.length
    ) {
      setError(
        t(
          "请为此用途选择订阅、节点或其他去向。",
          "Choose subscriptions, nodes or another destination for this purpose.",
        ),
      );
      return;
    }
    const oldName = profile.groups[edit.index]?.name;
    const groups =
      edit.index < 0
        ? [...profile.groups, g]
        : profile.groups.map((v, i) => (i === edit.index ? g : v));
    onChange({
      ...profile,
      groups: groups.map((v) => ({
        ...v,
        groups: v.groups.map((n) => (n === oldName ? g.name : n)),
      })),
      rules: edit.draft.rules.map((r) =>
        r.target === edit.target ? { ...r, target: g.name } : r,
      ),
    });
    setEdit(undefined);
  };
  const targets = [...profile.groups.map((g) => g.name), "DIRECT", "REJECT"];
  const fallback = profile.rules.find((r) => r.type === "MATCH");
  return (
    <>
      <div className="section-intro">
        <div>
          <h3>{t("按用途分流", "Route by purpose")}</h3>
          <p>
            {t(
              "在同一处设置匹配内容、节点来源和选择方式。",
              "Edit matching traffic, sources and selection together.",
            )}
          </p>
        </div>
        <Button primary onClick={() => open(-1)}>
          {t("新增用途", "Add purpose")}
        </Button>
      </div>
      <div className="purpose-list">
        {profile.groups.map((g, i) => {
          const rules = profile.rules.filter(
            (r) => r.target === g.name && r.type !== "MATCH",
          );
          const sources = state.sources
            .filter((s) => g.sourceIds.includes(s.id))
            .map((s) => s.name);
          const strategy = (
            {
              select: t("手动选择", "Manual"),
              "url-test": t("自动测速", "Automatic"),
              fallback: t("故障转移", "Fallback"),
              "load-balance": t("负载均衡", "Load balance"),
            } as Record<string, string>
          )[g.type];
          return (
            <article className="purpose-row" key={g.name}>
              <div>
                <strong>{g.name}</strong>
                <p className="muted">
                  {rules.length} {t("条匹配规则", "matching rules")}
                  {fallback?.target === g.name
                    ? t(" · 默认去向", " · Default destination")
                    : ""}
                </p>
                <small>
                  {rules
                    .slice(0, 3)
                    .map((r) =>
                      r.type === "RULE-SET"
                        ? state.resources.find((v) => v.id === r.value)?.name ||
                          t("规则集不存在", "Missing ruleset")
                        : r.value || r.type,
                    )
                    .join(" · ")}
                </small>
              </div>
              <div>
                <span className="pill subtle">{strategy}</span>
                <p className="muted">
                  {[
                    ...sources,
                    ...(g.pools || []).map((n) => t("订阅池：", "Pool: ") + n),
                    ...state.nodes
                      .filter((n) => g.nodeIds.includes(n.id))
                      .map((n) => n.node.name),
                    ...g.groups,
                    ...g.builtins,
                    ...(g.allSources
                      ? [t("全部订阅（动态）", "All subscriptions (dynamic)")]
                      : []),
                    ...g.tags.map((n) => t("标签：", "Tag: ") + n),
                  ].join("、") || t("尚未选择来源", "No sources selected")}
                </p>
              </div>
              <div className="actions">
                <Button onClick={() => open(i)}>
                  {t("编辑用途", "Edit purpose")}
                </Button>
                <Button danger onClick={() => setRemove(g.name)}>
                  {t("删除", "Delete")}
                </Button>
              </div>
            </article>
          );
        })}
      </div>
      <div className="builtin-routing">
        {["DIRECT", "REJECT"].map((target) => (
          <div key={target}>
            <strong>
              {target === "DIRECT" ? t("直连", "Direct") : t("拒绝", "Reject")}
            </strong>
            <span className="muted">
              {
                profile.rules.filter(
                  (r) => r.target === target && r.type !== "MATCH",
                ).length
              }{" "}
              {t("条规则", "rules")}
            </span>
          </div>
        ))}
        <Button onClick={() => setAllRules(!allRules)}>
          {allRules
            ? t("收起全部规则", "Hide all rules")
            : t("编辑全部规则与顺序", "Edit all rules & order")}
        </Button>
      </div>
      {allRules && (
        <RulesEditor
          rules={profile.rules}
          onChange={(rules) => onChange({ ...profile, rules })}
          targets={targets}
          resources={state.resources || []}
          t={t}
          onDirtyChange={setNestedDirty}
          onManageResources={onManageResources}
        />
      )}
      {!allRules && (
        <div className="fallback-row">
          <div>
            <strong>{t("其他流量", "Other traffic")}</strong>
            <p className="muted">
              {t("未匹配规则时的去向", "Destination when no rule matches")}
            </p>
          </div>
          <Field label={t("默认去向", "Default destination")}>
            <select
              value={fallback?.target || ""}
              onChange={(e) => {
                const rule = {
                  type: "MATCH",
                  value: "",
                  target: e.target.value,
                  enabled: true,
                  noResolve: false,
                };
                onChange({
                  ...profile,
                  rules: fallback
                    ? profile.rules.map((r) => (r === fallback ? rule : r))
                    : [...profile.rules, rule],
                });
              }}
            >
              <option value="" disabled>
                {t("请选择", "Select")}
              </option>
              {targets.map((n) => (
                <option key={n}>{n}</option>
              ))}
            </select>
          </Field>
          {fallback && !fallback.enabled && (
            <span role="alert">
              {t(
                "当前兜底规则已停用，请重新选择去向以启用。",
                "Fallback is disabled. Select a destination to enable it.",
              )}
            </span>
          )}
        </div>
      )}
      <div className="actions routing-tools">
        <Button onClick={onManageResources}>
          {t("管理规则集", "Manage rulesets")}
        </Button>
      </div>
      {edit && (
        <EditorDialog
          title={
            edit.index < 0
              ? t("新增用途", "Add purpose")
              : t("编辑用途", "Edit purpose")
          }
          t={t}
          dirty={pending}
          onClose={() => setEdit(undefined)}
          footer={
            <Button primary disabled={nestedDirty} onClick={save}>
              {t("完成编辑", "Done")}
            </Button>
          }
        >
          {error && (
            <p role="alert" className="alert error">
              {error}
            </p>
          )}
          {renderGroup(
            edit.group,
            edit.index,
            changeGroup,
            <section className="purpose-rules">
              <h3>{t("哪些流量使用这个用途", "Traffic using this purpose")}</h3>
              <p className="muted">
                {t(
                  "原有规则保持顺序；新增规则放在兜底之前，可在全部规则中调整优先级。",
                  "Existing order is preserved. New rules go before the fallback; adjust priority in all rules.",
                )}
              </p>
              {edit.group.name &&
              !profile.groups.some(
                (g, i) => i !== edit.index && g.name === edit.group.name,
              ) &&
              !["DIRECT", "REJECT"].includes(edit.group.name) ? (
                <RulesEditor
                  rules={edit.draft.rules}
                  focusTarget={edit.target}
                  targetLabel={edit.group.name}
                  targets={[edit.target]}
                  resources={state.resources || []}
                  t={t}
                  onChange={(rules) =>
                    setEdit({ ...edit, draft: { ...edit.draft, rules } })
                  }
                  onDirtyChange={setNestedDirty}
                  onManageResources={onManageResources}
                />
              ) : (
                <p>
                  {t(
                    "先填写用途名称，再添加域名、IP 或规则集。",
                    "Name this purpose before adding domains, IPs or rulesets.",
                  )}
                </p>
              )}
            </section>,
          )}
        </EditorDialog>
      )}
      {remove && (
        <EditorDialog
          title={t("删除用途", "Delete purpose")}
          t={t}
          dirty={false}
          onClose={() => setRemove(undefined)}
          footer={
            <Button
              danger
              disabled={
                profile.rules.some((r) => r.target === remove) ||
                profile.groups.some((g) => g.groups.includes(remove))
              }
              onClick={() => {
                onChange({
                  ...profile,
                  groups: profile.groups.filter((g) => g.name !== remove),
                });
                setRemove(undefined);
              }}
            >
              {t("确认删除", "Delete")}
            </Button>
          }
        >
          <p>
            {profile.rules.some((r) => r.target === remove) ||
            profile.groups.some((g) => g.groups.includes(remove))
              ? t(
                  "仍有规则或其他用途引用它。请先调整去向或移除引用，再删除。",
                  "Rules or other purposes still reference it. Change their destinations first.",
                )
              : t(
                  "删除这个没有引用的用途？",
                  "Delete this unreferenced purpose?",
                )}
          </p>
        </EditorDialog>
      )}
    </>
  );
}

export function PoolEditor({
  pools,
  groups,
  sources,
  t,
  onChange,
  onDirtyChange,
}: {
  pools: SourcePool[];
  groups: Group[];
  sources: Source[];
  t: T;
  onChange: (pools: SourcePool[], groups: Group[]) => void;
  onDirtyChange: (v: boolean) => void;
}) {
  const [edit, setEdit] = useState<{
    index: number;
    value: SourcePool;
    original: string;
  }>();
  const [error, setError] = useState("");
  const dirty = !!edit && JSON.stringify(edit.value) !== edit.original;
  useEffect(() => {
    onDirtyChange(dirty);
    return () => onDirtyChange(false);
  }, [dirty, onDirtyChange]);
  return (
    <section className="pool-section">
      <div className="section-intro">
        <div>
          <h3>{t("订阅池（可选）", "Subscription pools (optional)")}</h3>
          <p>
            {t(
              "给一组订阅起名，供多个用途复用。",
              "Name a list of subscriptions to reuse across purposes.",
            )}
          </p>
        </div>
        <Button
          onClick={() => {
            const value = { name: "", sourceIds: [] };
            setError("");
            setEdit({ index: -1, value, original: JSON.stringify(value) });
          }}
        >
          {t("新增订阅池", "Add pool")}
        </Button>
      </div>
      {pools.map((p, i) => (
        <div className="purpose-row" key={p.name}>
          <div>
            <strong>{p.name}</strong>
            <p className="muted">
              {p.sourceIds
                .map(
                  (id) =>
                    sources.find((s) => s.id === id)?.name ||
                    t("订阅不存在", "Missing source"),
                )
                .join("、")}
            </p>
          </div>
          <div>
            {t("影响用途：", "Used by: ")}
            {groups
              .filter((g) => g.pools?.includes(p.name))
              .map((g) => g.name)
              .join("、") || t("尚未使用", "Unused")}
          </div>
          <div className="actions">
            <Button
              onClick={() => {
                setError("");
                setEdit({
                  index: i,
                  value: structuredClone(p),
                  original: JSON.stringify(p),
                });
              }}
            >
              {t("编辑", "Edit")}
            </Button>
            <Button
              danger
              disabled={groups.some((g) => g.pools?.includes(p.name))}
              onClick={() => {
                if (
                  confirm(
                    t("删除这个未使用的订阅池？", "Delete this unused pool?"),
                  )
                )
                  onChange(
                    pools.filter((_, j) => j !== i),
                    groups,
                  );
              }}
            >
              {t("删除", "Delete")}
            </Button>
          </div>
        </div>
      ))}
      {edit && (
        <EditorDialog
          title={t("编辑订阅池", "Edit pool")}
          t={t}
          dirty={dirty}
          onClose={() => setEdit(undefined)}
          footer={
            <Button
              primary
              onClick={() => {
                const value = { ...edit.value, name: edit.value.name.trim() };
                if (
                  !value.name ||
                  /[,\r\n\0]/.test(value.name) ||
                  !value.sourceIds.length ||
                  pools.some(
                    (p, i) => i !== edit.index && p.name === value.name,
                  )
                ) {
                  setError(
                    t(
                      "请填写唯一名称，并选择至少一个订阅。",
                      "Enter a unique name and select at least one subscription.",
                    ),
                  );
                  return;
                }
                const oldName = pools[edit.index]?.name;
                onChange(
                  edit.index < 0
                    ? [...pools, value]
                    : pools.map((p, i) => (i === edit.index ? value : p)),
                  groups.map((g) => ({
                    ...g,
                    pools: g.pools?.map((n) =>
                      n === oldName ? value.name : n,
                    ),
                  })),
                );
                setEdit(undefined);
              }}
            >
              {t("完成编辑", "Done")}
            </Button>
          }
        >
          {error && (
            <p role="alert" className="alert error">
              {error}
            </p>
          )}
          <Field label={t("订阅池名称", "Pool name")}>
            <input
              maxLength={160}
              value={edit.value.name}
              onChange={(e) =>
                setEdit({
                  ...edit,
                  value: { ...edit.value, name: e.target.value },
                })
              }
            />
          </Field>
          <div className="checkbox-grid">
            {sources.map((s) => (
              <CheckField
                key={s.id}
                label={
                  s.name + (s.enabled ? "" : t(" · 已停用", " · Disabled"))
                }
                checked={edit.value.sourceIds.includes(s.id)}
                onChange={(checked) =>
                  setEdit({
                    ...edit,
                    value: {
                      ...edit.value,
                      sourceIds: checked
                        ? [...edit.value.sourceIds, s.id]
                        : edit.value.sourceIds.filter((id) => id !== s.id),
                    },
                  })
                }
              />
            ))}
          </div>
          <p className="muted">
            {t("影响用途：", "Affected purposes: ")}
            {groups
              .filter((g) => g.pools?.includes(pools[edit.index]?.name))
              .map((g) => g.name)
              .join("、") || t("尚未使用", "Unused")}
          </p>
        </EditorDialog>
      )}
    </section>
  );
}
