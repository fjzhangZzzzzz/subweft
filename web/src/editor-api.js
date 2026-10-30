import {
  setState,
  state,
  render,
  openForm,
  toast,
  form,
  formError,
} from "./editor.js";

let current = null;
export const yamlCache = new Map();
export const getServerState = () => current;
const errors = {
  unsupported_import_field: "导入包含首版不支持的字段，请根据支持矩阵调整。",
  invalid_import_type: "导入字段类型错误，请检查布尔值、数值和字符串。",
  multiple_yaml_documents: "仅支持单个 YAML 配置文档。",
  pool_mapping_required: "订阅的组引用无法归为两个池，请先简化分组。",
  initial_import_only: "首次导入仅适用于尚未编辑或发布的空配置。",
  conflict: "草稿或设备已被其他操作修改。输入已保留，请关闭表单后刷新并核对。",
  login_required: "登录已失效，请重新登录。",
  access_denied: "当前身份没有管理权限。",
  csrf_rejected: "会话校验失败，请刷新后重试。",
  check_required: "请重新检查当前草稿后再发布。",
  check_busy: "已有检查正在执行，请稍后重试。",
  geodb_required: "部署尚未提供 GeoDB，无法检查 GEOIP 规则。",
  native_validation_failed: "Mihomo 配置检查未通过，请检查配置字段和引用。",
  base_readonly: "基础配置只读，请通过首次导入设置。",
  no_changes: "当前草稿与发布版本一致。",
  device_revoked: "此设备已撤销，不能恢复原链接。",
  validator_unavailable: "原生校验器不可用。",
  invalid_or_duplicate_entry: "规则条目格式错误或重复。",
  invalid_or_duplicate_name: "名称格式错误或重复。",
  invalid_url: "请输入有效的 HTTP(S) 地址。",
  configuration_incomplete: "配置存在未完成的引用或空策略组，请先修复。",
  invalid_remote_ruleset: "远程规则集字段不完整或含不支持的内容。",
};
export async function api(path, method = "GET", body) {
  let response;
  try {
    response = await fetch("/api/v1/" + path, {
      method,
      credentials: "same-origin",
      headers:
        method === "GET"
          ? {}
          : {
              "Content-Type": "application/json",
              "X-CSRF-Token": current?.csrf || "",
            },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new Error("连接服务失败，输入已保留，请检查服务后重试。");
  }
  if (response.status === 204) return null;
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    const e = new Error(
      errors[data.error?.code] || "操作失败，请检查输入或稍后重试。",
    );
    e.status = response.status;
    e.field = data.error?.field;
    throw e;
  }
  return data;
}
function baseView(base = {}) {
  return {
    mixedPort: base["mixed-port"],
    allowLan: base["allow-lan"],
    dnsEnabled: base.dns?.enable,
    dnsMode: base.dns?.["enhanced-mode"],
    nameservers: base.dns?.nameserver || [],
  };
}
function view(c) {
  return {
    sources: (c.subscriptions || []).map((s) => ({
      ...s,
      health: s.healthCheck,
    })),
    groups: (c.groups || []).map((g) => ({ ...g, mode: g.type })),
    sets: (c.ruleSets || []).map((s) => ({ ...s, entries: s.entries || [] })),
    rules: c.rules || [],
    fallback: c.fallback,
    base: baseView(c.base),
  };
}
export function emptyConfig() {
  return {
    sources: [],
    groups: [],
    sets: [],
    rules: [],
    fallback: "DIRECT",
    base: {},
  };
}
export function toServer(c) {
  return {
    subscriptions: c.sources.map((s) => ({
      id: s.id,
      name: s.name,
      url: s.url,
      pool: s.pool,
      enabled: s.enabled,
      interval: s.interval,
      prefix: s.prefix || "",
      healthCheck: s.health,
      proxy: s.proxy,
    })),
    groups: c.groups.map((g) => ({
      id: g.id,
      name: g.name,
      type: g.mode,
      pools: g.pools,
      builtins: g.builtins,
      url: g.url,
      interval: g.interval,
      tolerance: g.tolerance,
    })),
    ruleSets: c.sets.map((s) =>
      s.kind === "http"
        ? {
            id: s.id,
            name: s.name,
            kind: s.kind,
            url: s.url,
            behavior: s.behavior,
            interval: s.interval,
            entries: [],
          }
        : { id: s.id, name: s.name, kind: s.kind, entries: s.entries },
    ),
    rules: c.rules,
    fallback: c.fallback,
    base: current.draft.config.base,
  };
}
function problem(p) {
  const [area, id] = p.field.split(".");
  return {
    text: p.message,
    hint: "保存修复后重新检查。",
    page:
      {
        subscriptions: "sources",
        groups: "groups",
        ruleSets: "sets",
        rules: "rules",
        fallback: "rules",
      }[area] || "overview",
    id,
  };
}
export async function loadState() {
  current = await api("state");
  yamlCache.clear();
  const draft = view(current.draft.config),
    versions = current.releases.map((r) => ({
      n: r.number,
      date: new Date(r.created).toLocaleString("zh-CN"),
      note: r.note,
      config: view(r.config),
    }));
  yamlCache.set(JSON.stringify(draft), current.draftYAML);
  const active = versions.find((v) => v.n === current.active);
  if (active) yamlCache.set(JSON.stringify(active.config), current.activeYAML);
  setState(
    {
      draft,
      rev: current.draft.revision,
      active: current.active,
      versions,
      checked: current.check
        ? {
            rev: current.check.revision,
            errors: current.check.problems.map(problem),
          }
        : null,
      note: state?.note || "",
      devices: current.devices.map((d) => ({
        ...d,
        expires: d.expiresAt ? localDate(d.expiresAt) : "",
        seen: current.seen[d.id]
          ? new Date(current.seen[d.id]).toLocaleString("zh-CN")
          : "尚未下载",
      })),
    },
    current,
  );
}
function localDate(raw) {
  const d = new Date(raw);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}
export function showError(e) {
  if (e.status === 401) {
    document.querySelector("#app").innerHTML =
      '<main class="main"><section class="panel padded"><h1>Subweft</h1><p class="description">登录后管理订阅、分流配置和设备授权。</p><p><a href="/auth/login">使用 OIDC 登录</a></p></section></main>';
    return;
  }
  if (form) {
    formError(e.field?.split(".").at(-1) || "name", e.message);
  } else if (state) {
    toast(e.message);
  } else {
    document.querySelector("#app").innerHTML =
      '<main class="main"><h1>无法加载配置</h1><p>请确认服务可用且当前身份已获管理员授权，然后刷新页面。</p></main>';
  }
}
export async function showDeviceLink(id) {
  const data = await api("devices/" + encodeURIComponent(id) + "/link");
  openForm(
    "link",
    { ...state.devices.find((d) => d.id === id), link: data.url },
    { readonly: true },
  );
}
export async function deviceAction(d, action) {
  await api("devices", "POST", {
    action,
    device: {
      id: d.id,
      revision: d.revision,
      name: d.name,
      state: d.state,
      expiresAt: d.expires ? new Date(d.expires).toISOString() : null,
    },
  });
  await loadState();
  render();
  toast("设备授权已更新，立即生效。");
}
export async function fetchVersionYAML(v) {
  const data = await api("releases/" + v.n);
  yamlCache.set(JSON.stringify(v.config), data.yaml);
}
