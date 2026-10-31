import { diffLines } from "diff";
import "./editor.css";
import {
  api,
  loadState,
  toServer,
  emptyConfig,
  showError,
  showDeviceLink,
  deviceAction,
  fetchVersionYAML,
  yamlCache,
  getServerState,
} from "./editor-api.js";
let serverState;

// API-backed configuration editor. Business state and rendering are owned by the server.
// The pure model below demonstrates draft / checked revision / active version.
const Model = (() => {
  const clone = (x) => structuredClone(x);
  const active = (s) =>
    s.versions.find((v) => v.n === s.active) || {
      n: 0,
      date: "尚未发布",
      note: "新增订阅和分流后检查并发布",
      config: emptyConfig(),
    };
  const targetName = (c, id) => c.groups.find((g) => g.id === id)?.name || id;
  const members = (c, g) =>
    c.sources.filter((x) => x.enabled && g.pools.includes(x.pool));
  const poolName = (x) => (x === "quality" ? "高质量池" : "高流量池");
  const groupSources = (g) =>
    [
      g.pools.length === 2 ? "全部订阅" : g.pools.map(poolName).join("、"),
      ...g.builtins,
    ]
      .filter(Boolean)
      .join("、") || "尚未选择来源";
  const diffs = (s) => {
    const before = active(s).config,
      after = s.draft,
      out = [];
    for (const [key, label, page] of [
      ["sources", "订阅", "sources"],
      ["groups", "策略组", "groups"],
      ["sets", "规则集", "sets"],
      ["rules", "分流规则", "rules"],
    ]) {
      for (const item of after[key]) {
        const old = before[key].find((x) => x.id === item.id);
        if (!old || JSON.stringify(old) !== JSON.stringify(item)) {
          const name =
            item.name ||
            (item.type === "RULE-SET"
              ? after.sets.find((x) => x.id === item.value)?.name
              : item.value) ||
            item.type;
          let detail = "";
          if (key === "sources")
            detail = `${poolName(item.pool)} · ${item.enabled ? "已启用" : "已禁用"}`;
          if (key === "groups") detail = groupSources(item);
          if (key === "sets")
            detail =
              item.kind === "local"
                ? `${item.entries.length} 条条目`
                : "远程规则集";
          if (key === "rules") detail = `→ ${targetName(after, item.target)}`;
          out.push({
            kind: old ? "edit" : "add",
            title: `${old ? "修改" : "新增"}${label}：${name}`,
            detail,
            page,
            id: item.id,
          });
        }
      }
      for (const item of before[key])
        if (!after[key].some((x) => x.id === item.id))
          out.push({
            kind: "delete",
            title: `删除${label}：${item.name || (item.type === "RULE-SET" ? before.sets.find((x) => x.id === item.value)?.name : item.value) || "规则"}`,
            detail: "从草稿移除，当前发布仍保留",
            page,
            id: item.id,
          });
    }
    const order = (c) =>
      c.rules
        .filter(
          (r) =>
            before.rules.some((x) => x.id === r.id) &&
            after.rules.some((x) => x.id === r.id),
        )
        .map((r) => r.id)
        .join(",");
    if (order(before) !== order(after))
      out.push({
        kind: "edit",
        title: "调整分流规则顺序",
        detail: "按界面中的顺序匹配",
        page: "rules",
      });
    if (before.fallback !== after.fallback)
      out.push({
        kind: "edit",
        title: "修改兜底去向",
        detail: `${targetName(before, before.fallback)} → ${targetName(after, after.fallback)}`,
        page: "rules",
      });
    for (const message of (s === state ? serverState?.sensitiveChanges : []) ||
      [])
      if (!out.some((x) => message.includes(x.title.split("：")[1])))
        out.push({
          kind: "edit",
          title: message,
          detail: "敏感内容已隐藏",
          page: "sources",
        });
    return out;
  };
  const validate = (c) => {
    const errors = [],
      targets = new Set([...c.groups.map((g) => g.id), "DIRECT", "REJECT"]);
    for (const s of c.sources.filter((x) => x.enabled))
      if (s.proxy && !targets.has(s.proxy))
        errors.push({
          text: `${s.name} 的拉取代理不存在`,
          hint: "在订阅的高级设置中选择拉取代理。",
          page: "sources",
          id: s.id,
        });
    for (const g of c.groups)
      if (!members(c, g).length && !g.builtins.length)
        errors.push({
          text: `${g.name} 没有启用的来源或成员`,
          hint: `启用${g.pools.map(poolName).join("或") || "一个订阅池"}中的订阅，或调整策略组来源。`,
          page: "groups",
          id: g.id,
        });
    const generated = new Set();
    for (const set of c.sets) {
      const names =
        set.kind === "http"
          ? [set.name]
          : [
              set.entries.some((e) => e.type !== "ip")
                ? set.name + "-domain"
                : null,
              set.entries.some((e) => e.type === "ip")
                ? set.name + "-ipcidr"
                : null,
            ].filter(Boolean);
      for (const n of names) {
        if (generated.has(n))
          errors.push({
            text: `生成的规则集名称重复：${n}`,
            hint: "请调整规则集名称。",
            page: "sets",
            id: set.id,
          });
        generated.add(n);
      }
    }
    for (const r of c.rules) {
      if (!targets.has(r.target))
        errors.push({
          text: "分流规则的目标不存在",
          hint: "请选择现有策略组或内置动作。",
          page: "rules",
          id: r.id,
        });
      if (r.type === "RULE-SET") {
        const set = c.sets.find((x) => x.id === r.value);
        if (!set || (set.kind === "local" && !set.entries.length))
          errors.push({
            text: `${set?.name || "引用的规则集"}没有可用条目`,
            hint: "添加条目或删除对应分流规则。",
            page: "sets",
            id: set?.id,
          });
      }
    }
    if (!targets.has(c.fallback))
      errors.push({
        text: "兜底目标不存在",
        hint: "设置其他流量的去向。",
        page: "rules",
      });
    return errors;
  };
  const entryError = (type, value) => {
    if (!value.trim()) return "请填写匹配内容";
    if (type === "ip") {
      const parts = value.split("/");
      if (parts.length !== 2 || !/^\d+$/.test(parts[1]))
        return "请使用 CIDR 格式，例如 192.0.2.0/24";
      if (parts[0].includes(":")) {
        try {
          new URL(`http://[${parts[0]}]/`);
        } catch {
          return "IPv6 地址格式不正确";
        }
        if (+parts[1] > 128) return "IPv6 前缀长度应为 0–128";
      } else if (
        !/^(\d{1,3}\.){3}\d{1,3}$/.test(parts[0]) ||
        parts[0].split(".").some((x) => +x > 255) ||
        +parts[1] > 32
      )
        return "IPv4 地址或前缀长度不正确";
      return "";
    }
    return value.length > 253 ||
      !value
        .split(".")
        .every((p) => /^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$/.test(p))
      ? "请输入域名，不含协议、路径、端口或通配符"
      : "";
  };
  const deviceStatus = (d) =>
    d.state === "revoked"
      ? "已撤销"
      : d.state === "paused"
        ? "已暂停"
        : d.expires && Date.parse(d.expires) <= Date.now()
          ? "已到期"
          : "有效";
  const yaml = (c) =>
    yamlCache.get(JSON.stringify(c)) || "当前配置不完整，请先修复检查问题。";
  return {
    clone,
    active,
    targetName,
    members,
    poolName,
    groupSources,
    diffs,
    validate,
    entryError,
    deviceStatus,
    yaml,
  };
})();
// Locked jsdiff produces the line changes; presentation adds context and inline highlights.
// Redact before comparing: changing an address must remain visible without exposing it.
function configurationDiff() {
  const lines = [];
  let old = 0,
    next = 0;
  for (const part of diffLines(
    serverState.activeYAML || "",
    serverState.draftYAML || "",
  )) {
    const texts = part.value.replace(/\n$/, "").split("\n");
    for (const text of texts) {
      const line = {
        kind: part.added ? "add" : part.removed ? "del" : "same",
        text,
      };
      if (!part.added) line.old = ++old;
      if (!part.removed) line.new = ++next;
      lines.push(line);
    }
  }
  return {
    lines,
    sensitive: serverState.sensitiveChanges || [],
    added: lines.filter((l) => l.kind === "add").length,
    removed: lines.filter((l) => l.kind === "del").length,
  };
}
// UI shell follows. All data stays in this tab's memory.
let diffMode = null,
  diffExpanded = new Set(),
  diffKey = "";
let state = null,
  page = "overview",
  selectedSet = null,
  query = "",
  poolFilter = "",
  sourceStatus = "",
  reviewTab = "business",
  busy = false,
  publishError = "",
  form = null,
  formBaseline = "",
  confirmResolve = null,
  lastFocus = null;
const $ = (s) => document.querySelector(s),
  esc = (x) =>
    String(x ?? "").replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    ),
  clone = Model.clone;
const paths = {
  overview:
    '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
  sources:
    '<path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-2 2M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l2-2"/>',
  groups:
    '<path d="M3 6h18M3 12h18M3 18h18"/><circle cx="8" cy="6" r="2" fill="white"/><circle cx="16" cy="12" r="2" fill="white"/><circle cx="10" cy="18" r="2" fill="white"/>',
  rules: '<path d="M7 3v13a4 4 0 0 0 4 4h7M7 8h11m-3-3 3 3-3 3m0 6 3 3-3 3"/>',
  sets: '<path d="m12 3 9 5-9 5-9-5 9-5Zm-9 9 9 5 9-5m-18 5 9 5 9-5"/>',
  history: '<path d="M3 11a9 9 0 1 1 2 7M3 4v7h7m2-5v6l4 2"/>',
  devices:
    '<rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8m-4-4v4"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  close: '<path d="m6 6 12 12M18 6 6 18"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  alert: '<path d="m12 3 10 18H2L12 3Z"/><path d="M12 9v5m0 3h.01"/>',
  arrow: '<path d="M5 12h14m-5-5 5 5-5 5"/>',
  back: '<path d="M19 12H5m5-5-5 5 5 5"/>',
  menu: '<path d="M4 6h16M4 12h16M4 18h16"/>',
  up: '<path d="m6 14 6-6 6 6"/>',
  down: '<path d="m6 10 6 6 6-6"/>',
  dots: '<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>',
  grip: '<path d="M8 5h.01M8 12h.01M8 19h.01M15 5h.01M15 12h.01M15 19h.01"/>',
};
const icon = (n) =>
  `<svg viewBox="0 0 24 24" aria-hidden="true">${paths[n] || paths.sets}</svg>`;
const btn = (label, action, attrs = "", cls = "") =>
  `<button type="button" class="${cls}" data-action="${action}" ${attrs}>${label}</button>`;
const badge = (s, c = "") => `<span class="badge ${c}">${esc(s)}</span>`;
const navs = [
  ["overview", "概览"],
  ["sources", "订阅"],
  ["groups", "策略组"],
  ["sets", "规则集"],
  ["rules", "分流规则"],
  ["history", "发布记录"],
  ["devices", "设备"],
];
const ruleLabels = {
  "RULE-SET": "规则集",
  DOMAIN: "完整域名",
  "DOMAIN-SUFFIX": "域名及子域名",
  "DOMAIN-KEYWORD": "域名关键词",
  "IP-CIDR": "IP 网段",
  GEOIP: "GEOIP",
};
const entryLabels = {
  suffix: "域名及子域名",
  domain: "完整域名",
  ip: "IP 网段",
};
const uid = (prefix) => prefix + crypto.randomUUID().replaceAll("-", "");
function toast(text) {
  $("#toast").textContent = text;
  $("#toast").classList.add("visible");
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => $("#toast").classList.remove("visible"), 4200);
}
function ask(title, text, label = "确认", danger = false) {
  return new Promise((resolve) => {
    confirmResolve = resolve;
    const d = $("#confirm-dialog");
    d.innerHTML = `<h2 id="confirm-title">${esc(title)}</h2><p>${esc(text)}</p><div class="row">${btn("取消", "confirm-no")}${btn(esc(label), "confirm-yes", "", danger ? "primary danger" : "primary")}</div>`;
    d.showModal();
    d.querySelector("button").focus();
  });
}
function go(next) {
  page = next;
  query = "";
  poolFilter = "";
  sourceStatus = "";
  selectedSet = null;
  reviewTab = "business";
  render();
  window.scrollTo(0, 0);
}
async function commit(config, message = "已保存到草稿，尚未发布") {
  await api("draft", "PUT", { revision: state.rev, config: toServer(config) });
  await loadState();
  publishError = "";
  render();
  toast(message);
}
function rowsTable(headers, rows, empty = "暂无内容") {
  return `<div class="table-wrap"><table><thead><tr>${headers.map((h) => `<th>${h}</th>`).join("")}</tr></thead><tbody>${rows || `<tr><td colspan="${headers.length}" class="empty">${empty}</td></tr>`}</tbody></table></div>`;
}
function heading(title, desc, actions = "") {
  return `<div class="page-heading"><div><h1>${title}</h1><p class="description">${desc}</p></div>${actions}</div>`;
}
function menu(id, items) {
  return `<details class="row-menu"><summary aria-label="更多操作 ${esc(id)}">${icon("dots")}</summary><div>${items.join("")}</div></details>`;
}
function changesList(changes) {
  return changes.length
    ? `<ul class="change-list">${changes.map((x) => `<li><span class="change-mark ${x.kind}">${x.kind === "add" ? "+" : x.kind === "delete" ? "−" : "~"}</span><div class="change-title">${esc(x.title)}<small>${esc(x.detail)}</small></div>${btn("查看", "change-go", `data-page="${x.page}" data-id="${x.id || ""}"`, "text-button")}</li>`).join("")}</ul>`
    : `<div class="empty">与当前发布版本一致。可以从左侧开始编辑。</div>`;
}
// PAGES
function render() {
  const changes = Model.diffs(state),
    active = Model.active(state),
    current = navs.find((x) => x[0] === page)?.[1] || "检查与发布";
  $("#app").innerHTML = `
    <div class="prototype-strip"><strong>Subweft</strong><span>配置编辑</span><span class="spacer"></span>${btn("刷新", "refresh")}${btn("退出", "logout")}</div>
    <div class="layout"><aside class="sidebar" id="sidebar"><div class="brand"><img class="brand-symbol" src="/favicon.svg" alt="" width="32" height="32" />Subweft ${btn(icon("close"), "nav-close", 'aria-label="收起导航" data-layout="layout-style-0"', "icon-button menu-close")}</div><div class="nav-label">配置管理</div><nav class="nav" aria-label="主导航">${navs.map(([key, label]) => `<a href="#${key}" data-action="navigate" data-page="${key}" class="${page === key ? "active" : ""}" ${page === key ? 'aria-current="page"' : ""}>${icon(key)}${label}</a>`).join("")}</nav><div class="identity"><div class="row"><span class="avatar">管</span><div>单管理员<small data-layout="layout-style-1">已通过 OIDC 登录</small></div></div></div></aside>
    <div class="work"><header class="topbar">${btn(icon("menu"), "nav-open", 'aria-label="展开导航"', "icon-button mobile-nav-toggle")}<div class="draft-state"><strong><span class="draft-dot ${changes.length ? "changed" : ""}"></span>${!state.active ? "尚未发布" : changes.length ? "正在编辑草稿" : "与当前发布版本一致"}</strong><span class="revision">r${state.rev}</span><div class="small muted">${!state.active ? "首次检查并发布后，设备才能下载" : changes.length ? `${changes.length} 项待发布变更 · 当前仍提供 v${active.n}` : `当前发布 v${active.n} · 设备更新订阅后获取`}</div></div><span class="spacer"></span>${btn("查看变更并发布", "review", busy ? "disabled" : "", "primary")}</header><main id="main" class="main" tabindex="-1" aria-label="${current}">${pageHTML()}</main></div></div>`;
}
function pageHTML() {
  const c = state.draft,
    active = Model.active(state),
    diff = Model.diffs(state);
  if (page === "overview") {
    const errors = Model.validate(c);
    return (
      heading(
        "概览",
        "编辑保存到草稿，审阅后再发布给设备。",
        !state.active &&
          !c.sources.length &&
          !c.groups.length &&
          !c.sets.length &&
          !c.rules.length
          ? btn("首次导入配置", "import-open")
          : "",
      ) +
      `<div class="overview-grid"><section class="panel padded"><div class="row between"><h3>当前发布</h3>${state.active ? badge("正在提供下载", "green") : badge("尚未发布", "amber")}</div><div class="version-number">${state.active ? `v${active.n}` : "—"}</div><p class="muted">${esc(active.date)} · ${esc(active.note)}</p><div class="row wrap" data-layout="layout-style-2">${btn("查看配置", "preview-active", state.active ? "" : "disabled")}${btn("发布记录", "navigate", 'data-page="history"', "text-button")}</div></section><section class="panel padded"><div class="row between"><h3>草稿状态</h3>${badge(diff.length ? "尚未发布" : "无变更", diff.length ? "amber" : "")}</div><div class="version-number">${diff.length}<span data-layout="layout-style-3">项变更</span></div><p class="muted">${errors.length ? `${errors.length} 项问题需要处理` : !state.active ? "首次发布前请检查配置" : diff.length ? "保存的修改不会影响当前发布版本" : "与当前发布版本一致"}</p><div class="row" data-layout="layout-style-2">${btn("查看变更并发布", "review", !diff.length && state.active ? "disabled" : "", "primary")}${diff.length ? btn("放弃草稿", "discard", "", "text-button") : ""}</div></section></div>
      ${errors.length ? `<section class="panel" data-layout="layout-style-4"><div class="panel-heading"><h2>需要处理<span class="count">${errors.length}</span></h2>${badge("发布前修复", "amber")}</div><div class="padded check-list">${errors.map((e) => errorHTML(e)).join("")}</div></section>` : ""}
      <section class="panel"><div class="panel-heading"><h2>待发布变更</h2>${badge(`草稿 r${state.rev}`)}</div>${changesList(diff)}</section><div class="row between wrap" data-layout="layout-style-5"><p class="muted small">配置包含 ${c.sources.filter((s) => s.enabled).length} 个启用订阅、${c.groups.length} 个策略组、${c.rules.length} 条分流规则及 1 条兜底。</p>${btn("查看基础配置", "base", "", "text-button")}</div><p class="page-note">本页不展示节点健康或设备在线状态。上游内容与实际连接由客户端加载后确认。</p>`
    );
  }
  if (page === "sources") {
    const visible = c.sources.filter(
      (s) =>
        (!query || s.name.toLowerCase().includes(query.toLowerCase())) &&
        (!poolFilter || s.pool === poolFilter) &&
        (!sourceStatus || (sourceStatus === "on") === s.enabled),
    );
    return (
      heading(
        "订阅",
        "按订阅池组织来源。设备自行加载上游订阅内容。",
        btn(icon("plus") + "新增订阅", "new-source", "", "primary"),
      ) +
      `<div class="toolbar"><input type="search" aria-label="搜索订阅" placeholder="搜索订阅名称…" value="${esc(query)}" data-search><label>订阅池 <select data-filter="pool" aria-label="筛选订阅池"><option value="">全部</option><option value="quality" ${poolFilter === "quality" ? "selected" : ""}>高质量池</option><option value="quota" ${poolFilter === "quota" ? "selected" : ""}>高流量池</option></select></label><label>状态 <select data-filter="status" aria-label="筛选订阅状态"><option value="">全部</option><option value="on" ${sourceStatus === "on" ? "selected" : ""}>已启用</option><option value="off" ${sourceStatus === "off" ? "selected" : ""}>已禁用</option></select></label></div>` +
      rowsTable(
        ["名称", "订阅池", "草稿状态", "关联策略组", "操作"],
        visible
          .map(
            (s) =>
              `<tr><td><strong>${esc(s.name)}</strong><span class="sub">客户端每 ${s.interval / 60} 分钟更新</span></td><td data-label="订阅池">${badge(Model.poolName(s.pool), "blue")}</td><td data-label="草稿状态">${badge(s.enabled ? "已启用" : "已禁用", s.enabled ? "green" : "")}</td><td data-label="关联策略组">${
                c.groups
                  .filter((g) => g.pools.includes(s.pool))
                  .map((g) => esc(g.name))
                  .join("、") || '<span class="muted">暂无</span>'
              }</td><td class="actions">${btn("编辑", "edit-source", `data-id="${s.id}"`)}${menu(s.name, [btn(s.enabled ? "禁用" : "启用", "toggle-source", `data-id="${s.id}"`), btn("调整订阅池", "edit-source", `data-id="${s.id}"`), btn("删除", "delete-source", `data-id="${s.id}"`, "danger")])}</td></tr>`,
          )
          .join(""),
        "没有匹配的订阅。",
      ) +
      `<p class="page-note">“已启用”是草稿中的配置状态，不代表上游可用。新增、禁用、移动和删除订阅均在发布后影响设备配置。</p>`
    );
  }
  if (page === "groups")
    return (
      heading(
        "策略组",
        "决定使用哪些订阅池，以及如何选择节点。",
        btn(icon("plus") + "新增策略组", "new-group", "", "primary"),
      ) +
      rowsTable(
        ["名称", "选择方式", "来源", "被引用", "操作"],
        c.groups
          .map((g) => {
            const refs =
                c.rules.filter((r) => r.target === g.id).length +
                (c.fallback === g.id ? 1 : 0),
              empty = !Model.members(c, g).length && !g.builtins.length;
            return `<tr><td><strong>${esc(g.name)}</strong>${empty ? '<span class="sub warning-text">没有启用的来源或成员</span>' : ""}</td><td data-label="选择方式">${g.mode === "select" ? "手动选择" : "自动测速"}</td><td data-label="来源">${esc(Model.groupSources(g))}<span class="sub">${Model.members(c, g).length} 个启用订阅${g.builtins.length ? ` · ${g.builtins.length} 个内置成员` : ""}</span></td><td data-label="被引用">${refs} 条规则${c.fallback === g.id ? "（含兜底）" : ""}</td><td class="actions">${btn("编辑", "edit-group", `data-id="${g.id}"`)}${menu(g.name, [btn("删除", "delete-group", `data-id="${g.id}"`, "danger")])}</td></tr>`;
          })
          .join(""),
      ) +
      `<p class="page-note">同时选择两个池表示使用全部订阅。策略组不嵌套，不需要逐一选择订阅内的节点。</p>`
    );
  if (page === "rules") {
    const visible = c.rules
      .map((r, i) => ({ r, i }))
      .filter(
        ({ r }) =>
          !query ||
          `${ruleLabels[r.type]} ${r.type === "RULE-SET" ? c.sets.find((s) => s.id === r.value)?.name : r.value} ${Model.targetName(c, r.target)}`
            .toLowerCase()
            .includes(query.toLowerCase()),
      );
    return (
      heading(
        "分流规则",
        "从上到下匹配，命中后使用对应目标。",
        btn(icon("plus") + "新增规则", "new-rule", "", "primary"),
      ) +
      `<div class="toolbar"><input data-search type="search" aria-label="搜索规则" placeholder="搜索规则内容或目标…" value="${esc(query)}"><span class="small muted">${query ? "筛选时暂停排序；清空搜索可调整顺序。" : "可拖动、上下移动，或在编辑时指定位置。"}</span></div>` +
      rowsTable(
        ["顺序", "匹配类型", "匹配内容", "目标策略", "操作"],
        visible
          .map(({ r, i }) => {
            const set = c.sets.find((s) => s.id === r.value);
            return `<tr data-drop="${r.id}"><td class="order-cell"><button class="drag-handle" ${query ? "disabled" : 'draggable="true"'} data-drag="${r.id}" aria-label="拖动第 ${i + 1} 条规则">${icon("grip")}</button>${i + 1}</td><td data-label="类型">${ruleLabels[r.type]}</td><td data-label="匹配内容">${r.type === "RULE-SET" ? btn(esc(set?.name || "缺失的规则集"), "open-set", `data-id="${r.value}"`, "name-button") : `<span class="mono">${esc(r.value)}</span>`}${r.type === "RULE-SET" && set?.kind === "local" ? '<span class="sub">整个集合 · 域名优先于 IP</span>' : ""}${r.noResolve ? '<span class="sub">IP 匹配不主动解析 DNS</span>' : ""}</td><td data-label="目标">${badge(Model.targetName(c, r.target), ["DIRECT", "REJECT"].includes(r.target) ? "" : "blue")}</td><td class="actions"><div class="rule-actions">${btn("编辑", "edit-rule", `data-id="${r.id}"`)}${btn(icon("up"), "move-rule", `data-id="${r.id}" data-offset="-1" aria-label="上移规则 ${i + 1}" ${i === 0 || query ? "disabled" : ""}`, "icon-button")}${btn(icon("down"), "move-rule", `data-id="${r.id}" data-offset="1" aria-label="下移规则 ${i + 1}" ${i === c.rules.length - 1 || query ? "disabled" : ""}`, "icon-button")}${menu(`规则 ${i + 1}`, [btn("删除", "delete-rule", `data-id="${r.id}"`, "danger")])}</div></td></tr>`;
          })
          .join(""),
        "没有匹配的规则。",
      ).replace(
        "</tbody></table></div>",
        `</tbody></table><div class="fallback"><div><strong>兜底 · 其余流量</strong><p class="small muted">未命中上方任何规则时使用，固定在最后。</p></div>${badge(Model.targetName(c, c.fallback), "blue")}${btn("编辑", "edit-fallback")}</div></div>`,
      ) +
      `<p class="page-note">自维护规则集作为整体排序，生成时在当前位置先展开域名，再展开 IP。单条例外可以放在集合之前。</p>`
    );
  }
  if (page === "sets") {
    if (selectedSet) return setDetailHTML();
    const visible = c.sets.filter(
      (s) => !query || s.name.toLowerCase().includes(query.toLowerCase()),
    );
    return (
      heading(
        "规则集",
        "按用途维护匹配内容，再由分流规则指定去向。",
        btn(icon("plus") + "新增规则集", "new-set", "", "primary"),
      ) +
      `<div class="toolbar"><input type="search" data-search aria-label="搜索规则集" placeholder="搜索规则集名称…" value="${esc(query)}"></div>` +
      rowsTable(
        ["名称", "来源", "内容", "被引用", "操作"],
        visible
          .map(
            (s) =>
              `<tr><td>${btn(esc(s.name), "open-set", `data-id="${s.id}"`, "name-button")}</td><td data-label="来源">${badge(s.kind === "local" ? "自维护规则集" : "远程规则集", s.kind === "local" ? "blue" : "")}</td><td data-label="内容">${s.kind === "local" ? `${s.entries.filter((e) => e.type !== "ip").length} 条域名 · ${s.entries.filter((e) => e.type === "ip").length} 条 IP` : `${s.behavior === "domain" ? "域名" : "IP 网段"} · YAML`}</td><td data-label="被引用">${c.rules.filter((r) => r.type === "RULE-SET" && r.value === s.id).length} 条规则</td><td class="actions">${btn(s.kind === "local" ? "管理条目" : "编辑来源", s.kind === "local" ? "open-set" : "edit-set", `data-id="${s.id}"`)}${menu(s.name, [btn("编辑名称与来源", "edit-set", `data-id="${s.id}"`), btn("删除", "delete-set", `data-id="${s.id}"`, "danger")])}</td></tr>`,
          )
          .join(""),
      ) +
      `<p class="page-note">同一个自维护规则集可以包含域名和 IP，无需手动拆分。远程规则由客户端加载，不能在这里直接修改远端条目。</p>`
    );
  }
  if (page === "review") return reviewHTML();
  if (page === "history")
    return (
      heading(
        "发布记录",
        "恢复历史版本会生成草稿，检查并发布后才更新设备配置。",
      ) +
      rowsTable(
        ["版本", "发布时间", "备注", "状态", "操作"],
        state.versions
          .map(
            (v) =>
              `<tr><td><strong>v${v.n}</strong></td><td data-label="发布时间">${esc(v.date)}</td><td data-label="备注">${esc(v.note)}</td><td data-label="状态">${v.n === state.active ? badge("当前发布", "green") : badge("历史版本")}</td><td class="actions">${btn("查看", "view-version", `data-version="${v.n}"`)}${menu(`版本 ${v.n}`, [btn("与当前版本比较", "compare-version", `data-version="${v.n}"`), btn("恢复到草稿", "restore-version", `data-version="${v.n}"`), btn("导出示例配置", "export-version", `data-version="${v.n}"`)])}</td></tr>`,
          )
          .join(""),
      ) +
      `<p class="page-note">历史保留的是配置定义和自维护规则。上游订阅节点与远程规则内容由客户端重新获取，不承诺回到当时的远端内容。</p>`
    );
  if (page === "devices")
    return (
      heading(
        "设备",
        "每个设备独立授权，全部使用当前发布配置。",
        btn(icon("plus") + "新增设备", "new-device", "", "primary"),
      ) +
      `<div class="notice" data-layout="layout-style-6">设备授权操作立即生效，不需要发布配置。${state.active ? `当前下载版本为 <strong>v${state.active}</strong>。` : "尚无发布版本，设备暂时不能下载。"}</div>` +
      rowsTable(
        ["设备名称", "授权状态", "有效期", "最近下载", "操作"],
        state.devices
          .map((d) => {
            const st = Model.deviceStatus(d);
            return `<tr><td><strong>${esc(d.name)}</strong></td><td data-label="授权状态">${badge(st, st === "有效" ? "green" : st === "已撤销" ? "" : "amber")}</td><td data-label="有效期">${d.expires ? esc(d.expires.replace("T", " ")) : "长期有效"}</td><td data-label="最近下载">${esc(d.seen)}</td><td class="actions">${btn("查看链接", "device-link", `data-id="${d.id}" ${st !== "有效" ? "disabled" : ""}`)}${menu(d.name, [btn("编辑", "edit-device", `data-id="${d.id}" ${d.state === "revoked" ? "disabled" : ""}`), btn(d.state === "paused" ? "恢复" : "暂停", "toggle-device", `data-id="${d.id}" ${d.state === "revoked" ? "disabled" : ""}`), btn("更换链接", "rotate-device", `data-id="${d.id}" ${d.state === "revoked" ? "disabled" : ""}`), btn("撤销", "revoke-device", `data-id="${d.id}" ${d.state === "revoked" ? "disabled" : ""}`, "danger")])}</td></tr>`;
          })
          .join(""),
        "还没有设备。创建一个独立链接，供设备更新配置。",
      ) +
      `<p class="page-note">撤销阻止后续下载，不能收回已下载的上游凭据。最近下载不代表客户端已经应用，也不代表设备在线。</p>`
    );
  return "";
}
function setDetailHTML() {
  const s = state.draft.sets.find((x) => x.id === selectedSet);
  if (!s) {
    selectedSet = null;
    return pageHTML();
  }
  const refs = state.draft.rules.filter(
    (r) => r.type === "RULE-SET" && r.value === s.id,
  );
  return (
    btn(icon("back") + "全部规则集", "sets-back", "", "text-button back") +
    heading(
      esc(s.name),
      s.kind === "local"
        ? "在同一处维护域名和 IP 条目。"
        : `${s.behavior === "domain" ? "域名" : "IP 网段"} · YAML · 客户端加载`,
      s.kind === "local"
        ? `<div class="row wrap">${btn("批量导入", "bulk", `data-id="${s.id}"`)}${btn(icon("plus") + "添加条目", "new-entry", `data-id="${s.id}"`, "primary")}</div>`
        : btn("编辑来源", "edit-set", `data-id="${s.id}"`, "primary"),
    ) +
    `<div class="row wrap" data-layout="layout-style-6">${badge(s.kind === "local" ? "自维护规则集" : "远程规则集", "blue")}<span class="small muted">引用：${refs.length ? refs.map((r) => `规则 #${state.draft.rules.indexOf(r) + 1} → ${esc(Model.targetName(state.draft, r.target))}`).join("；") : "尚未被分流规则使用"}</span>${btn("新增分流规则", "new-rule", `data-set="${s.id}"`, "text-button")}</div>` +
    (s.kind === "http"
      ? `<div class="panel padded stack"><h3>远程规则集</h3><p class="mono" data-layout="layout-style-7">${esc(s.url)}</p><p class="muted">客户端每 ${s.interval / 3600} 小时更新。远端内容未获取、未验证；此处不提供本地条目编辑。</p></div>`
      : rowsTable(
          ["匹配方式", "匹配内容", "操作"],
          s.entries
            .map(
              (e) =>
                `<tr><td>${entryLabels[e.type]}</td><td data-label="匹配内容"><span class="mono">${esc(e.value)}</span>${e.type === "suffix" ? '<span class="sub">包含该域名及其子域名</span>' : ""}</td><td class="actions">${btn("编辑", "edit-entry", `data-id="${e.id}" data-set="${s.id}"`)}${btn("删除", "delete-entry", `data-id="${e.id}" data-set="${s.id}"`, "text-button danger")}</td></tr>`,
            )
            .join(""),
          "还没有条目。可以先保存空集合；被引用的空集合会阻止发布。",
        ) +
        `<details class="advanced"><summary>查看生成方式</summary><p class="small muted">有域名时生成 ${esc(s.name)}-domain，有 IP 时生成 ${esc(s.name)}-ipcidr。名称和引用由服务自动维护，不需要分别编辑。</p></details>`)
  );
}
function errorHTML(e) {
  return `<div class="check-line bad">${icon("alert")}<div class="text"><strong>${esc(e.text)}</strong><small>${esc(e.hint)}</small></div>${btn("前往修复", "fix", `data-page="${e.page}" data-id="${e.id || ""}"`, "text-button")}</div>`;
}
function changedText(text, other) {
  if (other === undefined) return esc(text);
  let start = 0,
    end = 0;
  while (
    start < Math.min(text.length, other.length) &&
    text[start] === other[start]
  )
    start++;
  while (
    end < Math.min(text.length, other.length) - start &&
    text[text.length - 1 - end] === other[other.length - 1 - end]
  )
    end++;
  return (
    esc(text.slice(0, start)) +
    (start < text.length - end
      ? `<mark>${esc(text.slice(start, text.length - end))}</mark>`
      : "") +
    (end ? esc(text.slice(-end)) : "")
  );
}
function configDiffHTML(before, after) {
  if (!serverState.draftYAML)
    return `<div class="notice amber">当前草稿尚不能生成完整配置。请先检查并修复引用或字段问题，再查看配置差异。</div>`;
  const diff = configurationDiff(before, after),
    mode =
      diffMode ||
      (matchMedia("(max-width:760px)").matches ? "inline" : "split");
  const key = JSON.stringify(diff);
  if (key !== diffKey) {
    diffExpanded.clear();
    diffKey = key;
  }
  const cell = (line, other, inline = false) => {
    if (!line) return '<div class="diff-cell blank" aria-hidden="true"></div>';
    return `<div class="diff-cell ${line.kind} ${inline ? "inline-cell" : ""}" aria-label="${line.kind === "add" ? "新增" : line.kind === "del" ? "删除" : "未变化"}行"><span class="diff-number">${line.old ?? ""}</span>${inline ? `<span class="diff-number">${line.new ?? ""}</span>` : ""}<span class="diff-sign">${line.kind === "add" ? "+" : line.kind === "del" ? "−" : " "}</span><code class="diff-code">${changedText(line.text, other?.text)}</code></div>`;
  };
  const splitCell = (line, other, right = false) => {
    if (!line) return cell(null);
    return cell({ ...line, old: right ? line.new : line.old }, other);
  };
  const same = (line) =>
    `<div class="diff-row">${mode === "inline" ? cell(line, null, true) : splitCell(line) + splitCell(line, null, true)}</div>`;
  let body = "";
  for (let i = 0; i < diff.lines.length;) {
    const start = i;
    if (diff.lines[i].kind === "same") {
      while (i < diff.lines.length && diff.lines[i].kind === "same") i++;
      const run = diff.lines.slice(start, i),
        head = start === 0 ? 0 : 3,
        tail = i === diff.lines.length ? 0 : 3,
        hidden = run.length - head - tail;
      if (hidden > 1) {
        const expanded = diffExpanded.has(start);
        body += run.slice(0, head).map(same).join("");
        body += `<div class="diff-fold">${btn(`${expanded ? "收起" : "展开"} ${hidden} 行未变化内容`, "diff-expand", `data-start="${start}" aria-expanded="${expanded}"`)}</div>`;
        if (expanded)
          body += run
            .slice(head, run.length - tail)
            .map(same)
            .join("");
        if (tail) body += run.slice(-tail).map(same).join("");
      } else body += run.map(same).join("");
    } else {
      while (i < diff.lines.length && diff.lines[i].kind !== "same") i++;
      const group = diff.lines.slice(start, i),
        removed = group.filter((x) => x.kind === "del"),
        added = group.filter((x) => x.kind === "add");
      if (mode === "split")
        for (let n = 0; n < Math.max(removed.length, added.length); n++)
          body += `<div class="diff-row">${splitCell(removed[n], added[n])}${splitCell(added[n], removed[n], true)}</div>`;
      else
        body +=
          removed
            .map(
              (line, n) =>
                `<div class="diff-row">${cell(line, added[n], true)}</div>`,
            )
            .join("") +
          added
            .map(
              (line, n) =>
                `<div class="diff-row">${cell(line, removed[n], true)}</div>`,
            )
            .join("");
    }
  }
  return `<div class="diff-tools"><div class="diff-counts"><span class="added">+${diff.added} 行新增</span><span class="removed">−${diff.removed} 行删除</span></div><div class="diff-switch" role="group" aria-label="差异展示方式">${btn("左右对照", "diff-mode", `data-mode="split" aria-pressed="${mode === "split"}"`)}${btn("行内对比", "diff-mode", `data-mode="inline" aria-pressed="${mode === "inline"}"`)}</div></div>${diff.sensitive.length ? `<ul class="sensitive-changes">${diff.sensitive.map((x) => `<li>${esc(x)}（内容已隐藏）</li>`).join("")}</ul>` : ""}${!diff.added && !diff.removed ? '<div class="empty">生成配置没有变化。</div>' : `<div class="config-diff ${mode}"><div class="diff-scroll"><div class="diff-head">${mode === "split" ? `<span>当前发布 v${state.active}</span><span>待发布草稿 r${state.rev}</span>` : `<span>当前发布 v${state.active} → 待发布草稿 r${state.rev}</span>`}</div><div aria-label="配置差异内容">${body}</div></div></div>`}<p class="diff-note">− 删除 · + 新增；深色标记为修改文字。订阅和远程规则集地址已隐藏。内容由服务端编译生成。</p>`;
}
function reviewHTML() {
  const diff = Model.diffs(state),
    checked = state.checked && state.checked.rev === state.rev,
    errors = checked ? state.checked.errors : [],
    canPublish =
      checked && !errors.length && (diff.length || !state.active) && !busy;
  return (
    heading(
      "检查与发布",
      `${state.active ? `基于当前版本 v${state.active}` : "首次发布"} · 草稿版本 r${state.rev}`,
      btn("返回概览", "navigate", 'data-page="overview"'),
    ) +
    (publishError
      ? `<div role="alert" class="notice red" data-layout="layout-style-6">${esc(publishError)}</div>`
      : "") +
    `<section class="panel" data-layout="layout-style-4"><div class="panel-heading"><h2>待发布变更<span class="count">${diff.length} 项</span></h2>${btn("放弃全部修改", "discard", !state.active || !diff.length || busy ? "disabled" : "", "text-button danger")}</div><div class="padded"><div class="tabs">${btn("变更摘要", "review-tab", 'data-tab="business"', reviewTab === "business" ? "active" : "")}${btn("配置差异", "review-tab", 'data-tab="config"', reviewTab === "config" ? "active" : "")}</div>${reviewTab === "business" ? changesList(diff) : configDiffHTML(Model.active(state).config, state.draft)}</div></section>
    <section class="panel"><div class="panel-heading"><h2>检查结果 </h2>${btn(busy ? "处理中…" : checked ? "重新检查" : "检查草稿", "check", (!diff.length && state.active) || busy ? "disabled" : "", checked ? "" : "primary")}</div><div class="padded">${!diff.length && state.active ? '<p class="muted">没有待发布变更。</p>' : !checked ? '<p class="muted">草稿尚未检查，或检查后发生了修改。请检查当前草稿版本。</p>' : errors.length ? `<div class="check-list">${errors.map(errorHTML).join("")}</div>` : `<div class="check-list">${["配置结构与名称检查通过", "规则目标、引用和兜底完整", "策略组都有启用的来源或成员", "Mihomo 原生配置检查通过"].map((x, i) => `<div class="check-line">${icon("check")}<div class="text">${x}${i === 3 ? "<small>已执行固定版本 Mihomo 配置检查，远程订阅和规则内容未验证。</small>" : ""}</div></div>`).join("")}</div>`}<div class="notice amber" data-layout="layout-style-8">远端订阅、远程规则内容和实际连接未验证。发布成功也不等于节点可连接。</div></div></section>
    <div class="review-footer"><div class="field"><label for="publish-note">发布备注（可选）</label><input id="publish-note" placeholder="例如：调整 V2EX 来源并补充 AI 条目" value="${esc(state.note || "")}" maxlength="160"></div>${btn(busy ? "发布中…" : "发布此版本", "publish", canPublish ? "" : "disabled", "primary")}</div><p class="page-note">只发布刚检查通过的草稿版本；${state.active ? `失败仍提供 v${state.active}` : "失败时不提供下载"}，并保留草稿。发布备注不计入配置变更。</p>`
  );
}
// FORMS
const option = (value, label, current) =>
  `<option value="${esc(value)}" ${String(value) === String(current) ? "selected" : ""}>${esc(label)}</option>`;
const input = (name, value = "", type = "text", extra = "") =>
  `<input id="f-${name}" name="${name}" type="${type}" value="${esc(value)}" ${extra}>`;
const field = (name, label, control, hint = "") =>
  `<div class="field" data-field="${name}"><label for="f-${name}">${label}</label>${control}${hint ? `<small class="hint">${hint}</small>` : ""}<small class="field-error" id="err-${name}"></small></div>`;
const check = (name, label, checked, value = "on") =>
  `<label class="check"><input name="${name}" type="checkbox" value="${esc(value)}" ${checked ? "checked" : ""}>${label}</label>`;
const targets = (name, current, exclude = "", allowNew = false) =>
  `<select id="f-${name}" name="${name}">${option("", name === "proxy" ? "跟随客户端路由（不指定）" : "请选择目标", current)}<optgroup label="策略组">${state.draft.groups
    .filter((g) => g.id !== exclude)
    .map((g) => option(g.id, g.name, current))
    .join(
      "",
    )}</optgroup><optgroup label="内置动作">${["DIRECT", "REJECT"].map((x) => option(x, x, current)).join("")}</optgroup>${allowNew ? option("__new", "＋ 就地新增策略组", current) : ""}</select>`;
const pools = (name, selected = []) =>
  `<div class="choices">${["quality", "quota"].map((p) => `<label class="choice"><input type="checkbox" name="${name}" value="${p}" ${selected.includes(p) ? "checked" : ""}>${Model.poolName(p)}</label>`).join("")}</div>`;
function readForm() {
  const node = $("#drawer form");
  if (!node) return {};
  const fd = new FormData(node),
    out = Object.fromEntries(fd);
  for (const name of ["pools", "builtins", "newPools"])
    out[name] = fd.getAll(name);
  for (const name of ["enabled", "health", "noResolve"])
    out[name] = fd.has(name);
  return out;
}
function formDirty() {
  return form && !form.readonly && JSON.stringify(readForm()) !== formBaseline;
}
function openForm(type, value = {}, extra = {}) {
  lastFocus = document.activeElement;
  form = { type, value: clone(value), ...extra };
  renderDrawer();
  formBaseline = JSON.stringify(readForm());
  const dialog = $("#drawer");
  if (!dialog.open) dialog.showModal();
  (
    dialog.querySelector(
      "input:not([type=checkbox]):not([type=radio]),select,textarea",
    ) || dialog.querySelector("button")
  )?.focus();
}
async function closeDrawer(force = false) {
  if (
    !force &&
    formDirty() &&
    !(await ask(
      "放弃未保存的表单修改？",
      "这些输入尚未保存到草稿。放弃后不会改变当前草稿或发布版本。",
      "放弃修改",
    ))
  )
    return;
  $("#drawer").close();
  form = null;
  lastFocus?.focus();
}
function renderDrawer() {
  const v = form.value,
    c = state.draft;
  let title = "",
    subtitle = "保存后进入草稿，当前发布版本保持不变。",
    body = "",
    save = "保存到草稿";
  if (form.type === "import") {
    title = "首次导入配置";
    subtitle = "选择现有 Mihomo YAML，预览后导入草稿。";
    save = "导入到草稿";
    body =
      '<div class="notice">仅接受当前支持的配置字段。手工节点、文件型 provider、无法映射到两个池的组及未知字段会明确拒绝。不会请求上游地址。</div><div class="field"><label for="import-file">配置文件</label><input id="import-file" type="file" accept=".yaml,.yml"></div>' +
      btn("预览导入", "import-preview") +
      '<p id="import-result"></p>';
  }
  if (form.type === "source") {
    title = v.id ? "编辑订阅" : "新增订阅";
    body =
      field(
        "name",
        "订阅名称 *",
        input("name", v.name, "text", 'maxlength="80" autocomplete="off"'),
      ) +
      field(
        "url",
        "订阅地址 *",
        `<div class="password-wrap">${input("url", v.url, "password", 'autocomplete="off" spellcheck="false"')}${btn("显示", "reveal", 'data-input="f-url"')}</div>`,
        "客户端直接加载该地址；本服务不抓取、不转换订阅。",
      ) +
      field(
        "pool",
        "所属订阅池 *",
        `<div class="choices">${["quality", "quota"].map((p) => `<label class="choice"><input type="radio" name="pool" value="${p}" ${(v.pool || "quality") === p ? "checked" : ""}>${Model.poolName(p)}</label>`).join("")}</div>`,
        "每个订阅只属于一个池。",
      ) +
      check("enabled", "启用订阅", v.enabled !== false) +
      `<details class="advanced"><summary>高级设置</summary>${field("interval", "客户端更新间隔（秒）", input("interval", v.interval ?? 3600, "number", 'min="60" max="604800"'))}${field("prefix", "节点名前缀", input("prefix", v.prefix || "", "text", 'maxlength="80"'), "只影响客户端显示名称。")}${check("health", "启用客户端健康检查", v.health !== false)}${field("proxy", "拉取订阅使用的代理", targets("proxy", v.proxy ?? ""), "不指定时省略 proxy，由客户端路由决定；DIRECT 为强制直连。此设置不影响服务器。")}</details><div class="impact" id="source-impact"></div>`;
  }
  if (form.type === "group") {
    title = v.id ? "编辑策略组" : "新增策略组";
    body =
      field(
        "name",
        "策略组名称 *",
        input("name", v.name, "text", 'maxlength="80"'),
      ) +
      field(
        "mode",
        "选择方式",
        `<select id="f-mode" name="mode">${option("select", "手动选择", v.mode || "select")}${option("url-test", "自动测速", v.mode)}</select>`,
      ) +
      field(
        "pools",
        "订阅池",
        pools("pools", v.pools),
        "两池都选即使用全部订阅。",
      ) +
      `<div class="impact" id="group-preview"></div><details class="advanced"><summary>额外成员（可选）</summary>${["DIRECT", "REJECT"].map((x) => check("builtins", x, (v.builtins || []).includes(x), x)).join("")}</details>` +
      (v.mode === "url-test"
        ? `<div class="section-label">自动测速参数</div>${field("url", "测试地址 *", input("url", v.url || "https://www.gstatic.com/generate_204", "url"))}<div class="form-grid">${field("interval", "检查间隔（秒）", input("interval", v.interval ?? 300, "number", 'min="30"'))}${field("tolerance", "容差（毫秒）", input("tolerance", v.tolerance ?? 50, "number", 'min="0"'))}</div>`
        : "") +
      `<p class="small muted">被引用：${v.id ? `${c.rules.filter((r) => r.target === v.id).length} 条分流规则${c.fallback === v.id ? "及兜底规则" : ""}` : "尚未保存"}</p>`;
  }
  if (form.type === "set") {
    title = v.id ? "编辑规则集" : "新增规则集";
    body =
      field(
        "name",
        "用途名称 *",
        input("name", v.name, "text", 'maxlength="80"'),
        "例如 AI、V2EX、直连。名称不自动绑定同名策略组。",
      ) +
      (v.id
        ? `<div class="field"><span class="label">来源</span><div>${badge(v.kind === "local" ? "自维护规则集" : "远程规则集", "blue")}</div></div>`
        : field(
            "kind",
            "来源",
            `<select id="f-kind" name="kind">${option("local", "自维护规则集", v.kind || "local")}${option("http", "远程规则集", v.kind)}</select>`,
          )) +
      (v.kind === "http"
        ? field(
            "behavior",
            "内容类型",
            `<select id="f-behavior" name="behavior">${option("domain", "域名", v.behavior || "domain")}${option("ipcidr", "IP 网段", v.behavior)}</select>`,
          ) +
          field(
            "url",
            "远程规则集地址 *",
            input("url", v.url, "url", 'spellcheck="false"'),
          ) +
          field(
            "interval",
            "客户端更新间隔（秒）",
            input(
              "interval",
              v.interval ?? 86400,
              "number",
              'min="60" max="604800"',
            ),
          ) +
          '<p class="small muted">格式固定为 YAML。客户端加载远端内容，此处不编辑远端条目。</p>'
        : `<div class="notice">同一个集合可以包含域名和 IP。先保存用途名称，再添加条目；生成时自动拆分类型。</div>`);
  }
  if (form.type === "entry") {
    const parent = c.sets.find((s) => s.id === form.setId);
    title = v.id ? "编辑条目" : "添加条目";
    subtitle = `规则集：${parent.name} · 保存到草稿，尚未发布。`;
    const type = v.type || "suffix";
    body =
      field(
        "type",
        "匹配方式",
        `<select id="f-type" name="type">${Object.entries(entryLabels)
          .map(([k, l]) => option(k, l, type))
          .join("")}</select>`,
      ) +
      field(
        "value",
        type === "ip" ? "IP 网段 *" : "域名 *",
        input(
          "value",
          v.value,
          "text",
          `spellcheck="false" autocomplete="off" placeholder="${type === "ip" ? "192.0.2.0/24 或 2001:db8::/32" : "example.com"}"`,
        ),
        type === "ip"
          ? "支持 IPv4 / IPv6 CIDR，请包含前缀长度。"
          : type === "suffix"
            ? "匹配该域名及所有子域名。不填写通配符。"
            : "只匹配这个完整域名，不包含子域名。",
      ) +
      `<div class="impact" id="entry-preview"></div><p class="page-note">条目只描述匹配内容。流量去向在“分流规则”中设置。</p>`;
  }
  if (form.type === "bulk") {
    title = "批量添加条目";
    subtitle = `规则集：${c.sets.find((s) => s.id === form.setId).name} · 先检查，再明确添加。`;
    save = "添加有效条目到草稿";
    body =
      field(
        "entryType",
        "本批条目的匹配方式",
        `<select id="f-entryType" name="entryType">${Object.entries(entryLabels)
          .map(([k, l]) => option(k, l, v.entryType || "suffix"))
          .join("")}</select>`,
      ) +
      field(
        "text",
        "每行一个条目",
        `<textarea id="f-text" name="text" rows="9" spellcheck="false" placeholder="example.com\nexample.net">${esc(v.text)}</textarea>`,
        "逐行检查并去重；错误不会被静默忽略。",
      ) +
      btn("检查条目", "check-bulk") +
      `<div class="bulk-preview" id="bulk-preview">${bulkPreviewHTML()}</div>`;
  }
  if (form.type === "rule") {
    title = v.id ? "编辑分流规则" : "新增分流规则";
    const type = v.type || "RULE-SET";
    const valueControl =
      type === "RULE-SET"
        ? `<select id="f-value" name="value"><option value="">选择规则集</option>${c.sets.map((s) => option(s.id, s.name + (s.kind === "local" ? " · 自维护规则集" : " · 远程规则集"), v.value)).join("")}</select>`
        : input(
            "value",
            v.value,
            "text",
            `spellcheck="false" placeholder="${type === "IP-CIDR" ? "192.0.2.0/24" : type === "GEOIP" ? "CN" : "example.com"}"`,
          );
    body =
      field(
        "type",
        "匹配类型 *",
        `<select id="f-type" name="type">${Object.entries(ruleLabels)
          .map(([k, l]) => option(k, l, type))
          .join("")}</select>`,
      ) +
      field(
        "value",
        type === "RULE-SET" ? "规则集 *" : "匹配内容 *",
        valueControl,
        type === "RULE-SET"
          ? "引用整个集合，内部按域名 → IP 展开。"
          : type === "DOMAIN-SUFFIX"
            ? "包含该域名及所有子域名。"
            : "",
      ) +
      field(
        "target",
        "目标策略 *",
        targets("target", v.target || "", "", true),
      ) +
      (v.target === "__new"
        ? `<section class="nested-form"><h3>就地新增策略组</h3>${field("newName", "策略组名称 *", input("newName", v.newName, "text", 'maxlength="80"'))}${field("newMode", "选择方式", `<select id="f-newMode" name="newMode">${option("select", "手动选择", v.newMode || "select")}${option("url-test", "自动测速", v.newMode)}</select>`)}${field("newPools", "订阅池", pools("newPools", v.newPools || []))}<p class="small muted">与这条规则一起保存；取消此表单不会创建策略组。自动测速使用默认测试参数，可稍后在策略组中修改。</p></section>`
        : "") +
      field(
        "position",
        v.id ? "规则位置" : "插入位置",
        `<select id="f-position" name="position">${c.rules
          .filter((r) => r.id !== v.id)
          .map((r, i) =>
            option(
              i,
              `第 ${i + 1} 条之前：${r.type === "RULE-SET" ? c.sets.find((s) => s.id === r.value)?.name : r.value}`,
              v.position ?? c.rules.length,
            ),
          )
          .join(
            "",
          )}${option(c.rules.filter((r) => r.id !== v.id).length, "最后一条分流规则（兜底之前）", v.position ?? c.rules.length)}</select>`,
      ) +
      (["RULE-SET", "IP-CIDR", "GEOIP"].includes(type)
        ? `<details class="advanced"><summary>IP 匹配选项</summary>${check("noResolve", "不主动触发 DNS 解析", !!v.noResolve)}<p class="small muted">仅作用于 IP 匹配部分；域名部分不受影响。</p></details>`
        : "") +
      `<div class="impact" id="rule-preview"></div>`;
  }
  if (form.type === "fallback") {
    title = "编辑兜底去向";
    body =
      '<p class="muted" data-layout="layout-style-6">未匹配任何分流规则的流量使用此目标。兜底固定在最后，不能删除或移动。</p>' +
      field("target", "目标策略 *", targets("target", c.fallback));
  }
  if (form.type === "deleteGroup") {
    title = `删除策略组：${v.name}`;
    save = "替换引用并删除";
    body =
      `<div class="notice amber" data-layout="layout-style-4">此策略组仍被引用。选择替代目标后，引用更新和删除会作为同一次草稿修改保存。</div><ul>${c.rules
        .filter((r) => r.target === v.id)
        .map((r) => `<li>分流规则 #${c.rules.indexOf(r) + 1}</li>`)
        .join("")}${c.fallback === v.id ? "<li>兜底规则</li>" : ""}${c.sources
        .filter((s) => s.proxy === v.id)
        .map((s) => `<li>订阅拉取方式：${esc(s.name)}</li>`)
        .join("")}</ul>` +
      field("replacement", "替代目标 *", targets("replacement", "", v.id));
  }
  if (form.type === "device") {
    title = v.id ? "编辑设备" : "新增设备";
    subtitle = "授权保存后立即生效，不进入配置草稿。";
    save = v.id ? "保存设备" : "新增设备";
    body =
      field(
        "name",
        "设备名称 *",
        input(
          "name",
          v.name,
          "text",
          'maxlength="80" placeholder="例如：我的手机"',
        ),
      ) +
      `<div class="notice">此设备将下载当前发布 <strong>v${state.active}</strong>，后续发布会沿用同一个订阅链接。</div><details class="advanced" ${v.expires ? "open" : ""}><summary>到期时间（可选）</summary>${field("expires", "到期时间", input("expires", v.expires, "datetime-local"), "留空表示长期有效。")}</details>`;
  }
  if (form.type === "link") {
    title = "设备订阅链接";
    subtitle = v.name + " · 导入客户端后更新订阅";
    save = "";
    body =
      field(
        "link",
        "订阅链接",
        input("link", v.link, "text", 'readonly class="mono device-link"'),
      ) +
      btn("复制链接", "copy-link") +
      '<p class="page-note">链接包含设备凭据，请只保存在对应客户端中。暂停、撤销或更换链接会立即影响后续下载。</p>';
  }
  if (form.type === "preview") {
    title = v.title || "配置预览";
    subtitle =
      "服务端生成的配置预览，敏感地址已隐藏。是否通过检查以检查结果为准。";
    save = "";
    body = `<pre>${esc(Model.yaml(v.config))}</pre>`;
  }
  if (form.type === "compare") {
    title = `v${v.n} 与当前 v${state.active} 比较`;
    subtitle = "恢复先生成草稿，不会直接切换正在提供的版本。";
    save = "";
    const comparison = { ...state, draft: v.config };
    body =
      changesList(Model.diffs(comparison)) +
      `<details class="advanced"><summary>查看历史配置（脱敏）</summary><pre>${esc(Model.yaml(v.config))}</pre></details>`;
  }
  if (form.type === "base") {
    title = "基础配置";
    subtitle = "首次导入后保留，随草稿和发布版本保存。首版不提供编辑器。";
    save = "";
    const b = c.base;
    body = `<dl class="detail-list"><div><dt>混合端口</dt><dd>${esc(b.mixedPort ?? "未设置")}</dd></div><div><dt>允许局域网访问</dt><dd>${b.allowLan ? "开启" : "关闭"}</dd></div><div><dt>DNS</dt><dd>${b.dnsEnabled ? "开启" : "关闭"}</dd></div><div><dt>DNS 模式</dt><dd>${esc(b.dnsMode ?? "默认")}</dd></div></dl><div class="notice" data-layout="layout-style-9">基础配置由首次导入确定，随草稿和发布版本保存。</div>`;
  }
  if (form.type === "blockedSet") {
    title = "此规则集仍被引用";
    subtitle = "请先在分流规则中更换或删除引用。";
    save = "";
    body = `<p><strong>${esc(v.name)}</strong> 被以下规则使用：</p><ul>${c.rules
      .filter((r) => r.type === "RULE-SET" && r.value === v.id)
      .map(
        (r) =>
          `<li>规则 #${c.rules.indexOf(r) + 1} → ${esc(Model.targetName(c, r.target))}</li>`,
      )
      .join("")}</ul>${btn("前往分流规则", "drawer-go-rules", "", "primary")}`;
  }
  $("#drawer").innerHTML =
    `<header class="drawer-header"><div><h2 id="drawer-title">${esc(title)}</h2><p>${esc(subtitle)}</p></div>${btn(icon("close"), "drawer-close", 'aria-label="关闭编辑面板"', "icon-button")}</header><form class="drawer-form" novalidate><div class="drawer-body"><div id="form-error" role="alert"></div>${body}</div><footer class="drawer-footer">${save ? `<small>${form.type === "device" ? "立即生效" : "保存后尚未发布"}</small>` : ""}${btn(save ? "取消" : "关闭", "drawer-close")}${save ? `<button type="submit" class="primary" ${(form.type === "bulk" && !form.bulkPreview?.valid.length) || (form.type === "import" && !form.importContent) ? "disabled" : ""}>${save}</button>` : ""}</footer></form>`;
  updateFormPreview();
}
function updateFormPreview() {
  if (!form) return;
  const v = { ...form.value, ...readForm() },
    c = state.draft;
  if (form.type === "source") {
    const affected = c.groups.filter((g) => g.pools.includes(v.pool));
    const before = form.value.id
      ? c.sources.find((s) => s.id === form.value.id)
      : null;
    const leaving =
      before && before.pool !== v.pool
        ? c.groups.filter(
            (g) => g.pools.includes(before.pool) && !g.pools.includes(v.pool),
          )
        : [];
    $("#source-impact").innerHTML =
      `<strong>保存后预计影响</strong><p>${v.enabled ? "来源将供以下组使用：" : "来源将不再供以下组使用："}${affected.map((g) => esc(g.name)).join("、") || "暂无关联策略组"}。</p>${leaving.length ? `<p>移出原池后失去此来源：${leaving.map((g) => esc(g.name)).join("、")}。</p>` : ""}<p class="muted small">影响只进入草稿，发布后才更新设备配置。</p>`;
  }
  if (form.type === "group") {
    const ms = Model.members(c, v);
    $("#group-preview").innerHTML =
      `<strong>来源预览</strong><p>${ms.map((s) => esc(s.name)).join("、") || "没有启用的订阅"}${v.builtins.length ? " · " + v.builtins.join("、") : ""}</p><p class="small muted">实际节点由客户端加载订阅后获得。</p>${!ms.length && !v.builtins.length ? '<p class="warning-text">可以保存为空的草稿，但检查会阻止发布。</p>' : ""}`;
  }
  if (form.type === "entry")
    $("#entry-preview").innerHTML =
      `<strong>匹配预览</strong><p>${v.value ? esc(v.value) + (v.type === "suffix" ? " 及其子域名" : v.type === "ip" ? " 中的目标 IP" : "（仅此域名）") : "填写内容后可查看匹配含义。"}</p>`;
  if (form.type === "rule") {
    const content =
      v.type === "RULE-SET"
        ? c.sets.find((s) => s.id === v.value)?.name
        : v.value;
    $("#rule-preview").innerHTML =
      `<strong>规则预览</strong><p>${esc(content || "待填写匹配内容")} → ${esc(v.target === "__new" ? v.newName || "待创建策略组" : Model.targetName(c, v.target) || "待选择目标")}</p>`;
  }
}
function formError(name, message) {
  const el = $(`#err-${name}`);
  if (el) el.textContent = message;
  const ctrl = $(`#f-${name}`);
  if (ctrl) {
    const details = ctrl.closest("details");
    if (details) details.open = true;
    ctrl.setAttribute("aria-invalid", "true");
    ctrl.setAttribute("aria-describedby", `err-${name}`);
    ctrl.focus();
  }
  $("#form-error").innerHTML =
    `<p class="notice red dialog-error">${esc(message)}</p>`;
  return false;
}
function validName(name, collection, id) {
  if (
    !name ||
    name !== name.trim() ||
    /[\r\n,\0]/.test(name) ||
    name.length > 80 ||
    ["DIRECT", "REJECT"].includes(name)
  )
    return "名称不能为空，不能使用保留名、逗号或首尾空格。";
  if (collection.some((x) => x.id !== id && x.name === name))
    return "这个名称已存在，请使用不同名称。";
  return "";
}
function validURL(value) {
  try {
    const u = new URL(value);
    return ["https:", "http:"].includes(u.protocol) && !!u.hostname;
  } catch {
    return false;
  }
}
function bulkPreviewHTML() {
  const p = form.bulkPreview;
  if (!p) return "";
  return `<div class="notice ${p.errors.length ? "amber" : "green"}">识别到 ${p.total} 条：可添加 ${p.valid.length} 条 · 重复 ${p.duplicates} 条 · 格式错误 ${p.errors.length} 条</div>${p.errors.length ? `<ul>${p.errors.map((x) => `<li>第 ${x.line} 行：${esc(x.message)}</li>`).join("")}</ul><p class="small muted">可以返回修改，也可以明确选择只添加有效条目。</p>` : ""}`;
}
function checkBulk() {
  form.value = { ...form.value, ...readForm() };
  const { entryType: type, text } = form.value,
    set = state.draft.sets.find((x) => x.id === form.setId),
    seen = new Set(
      set.entries.map((e) => e.type + "|" + e.value.toLowerCase()),
    ),
    valid = [],
    errors = [];
  let duplicates = 0,
    total = 0;
  (text || "").split(/\r?\n/).forEach((raw, i) => {
    const value = raw.trim();
    if (!value) return;
    total++;
    const err = Model.entryError(type, value);
    if (err) errors.push({ line: i + 1, message: err });
    else if (seen.has(type + "|" + value.toLowerCase())) duplicates++;
    else {
      seen.add(type + "|" + value.toLowerCase());
      valid.push({ type, value });
    }
  });
  form.bulkPreview = { valid, errors, duplicates, total };
  renderDrawer();
  $("#drawer button[type=submit]").textContent =
    `添加有效的 ${valid.length} 条`;
}
async function saveForm() {
  if (busy) return;
  busy = true;
  const button = $("#drawer button[type=submit]");
  if (button) button.disabled = true;
  try {
    await saveFormContents();
  } finally {
    busy = false;
    if (button) button.disabled = false;
    render();
  }
}
async function saveFormContents() {
  if (form.type === "import") {
    if (!form.importContent) return;
    await api("import", "POST", {
      revision: state.rev,
      content: form.importContent,
    });
    await loadState();
    closeDrawer(true);
    page = "review";
    render();
    toast("配置已导入草稿，尚未发布。");
    return;
  }
  const v = { ...form.value, ...readForm() },
    c = clone(state.draft);
  let error = "";
  if (form.type === "source") {
    if ((error = validName(v.name, c.sources, v.id)))
      return formError("name", error);
    if (v.url !== "[地址已隐藏]" && !validURL(v.url))
      return formError("url", "请输入有效的 HTTP 或 HTTPS 订阅地址。");
    if (
      !Number.isInteger(+v.interval) ||
      +v.interval < 60 ||
      +v.interval > 604800
    )
      return formError("interval", "更新间隔应为 60–604800 秒。");
    const item = {
      id: v.id || uid("s"),
      name: v.name,
      url: v.url,
      pool: v.pool,
      enabled: v.enabled,
      interval: +v.interval,
      prefix: v.prefix,
      health: v.health,
      proxy: v.proxy,
    };
    if (v.id) c.sources = c.sources.map((x) => (x.id === v.id ? item : x));
    else c.sources.push(item);
  }
  if (form.type === "group") {
    if ((error = validName(v.name, c.groups, v.id)))
      return formError("name", error);
    if (v.mode === "url-test" && !validURL(v.url))
      return formError("url", "请输入有效的测试地址。");
    if (
      v.mode === "url-test" &&
      (!Number.isInteger(+v.interval) ||
        +v.interval < 30 ||
        +v.interval > 86400)
    )
      return formError("interval", "检查间隔应为 30–86400 秒。");
    if (
      v.mode === "url-test" &&
      (!Number.isInteger(+v.tolerance) || +v.tolerance < 0)
    )
      return formError("tolerance", "容差应为非负整数。");
    const item = {
      id: v.id || uid("g"),
      name: v.name,
      mode: v.mode,
      pools: v.pools,
      builtins: v.builtins,
      url: v.url || "https://www.gstatic.com/generate_204",
      interval: +v.interval || 300,
      tolerance: +v.tolerance || 0,
    };
    if (v.id) c.groups = c.groups.map((x) => (x.id === v.id ? item : x));
    else c.groups.push(item);
  }
  if (form.type === "set") {
    if ((error = validName(v.name, c.sets, v.id)))
      return formError("name", error);
    if (v.kind === "http" && v.url !== "[地址已隐藏]" && !validURL(v.url))
      return formError("url", "请输入有效的远程规则集地址。");
    if (
      v.kind === "http" &&
      (!Number.isInteger(+v.interval) ||
        +v.interval < 60 ||
        +v.interval > 604800)
    )
      return formError("interval", "更新间隔应为 60–604800 秒。");
    const item =
      v.kind === "http"
        ? {
            id: v.id || uid("c"),
            name: v.name,
            kind: "http",
            url: v.url,
            behavior: v.behavior,
            interval: +v.interval,
          }
        : {
            id: v.id || uid("c"),
            name: v.name,
            kind: "local",
            entries: v.entries || [],
          };
    if (v.id) c.sets = c.sets.map((x) => (x.id === v.id ? item : x));
    else {
      c.sets.push(item);
      page = "sets";
      selectedSet = item.id;
    }
  }
  if (form.type === "entry") {
    const value = v.value.trim(),
      set = c.sets.find((x) => x.id === form.setId);
    if ((error = Model.entryError(v.type, value)))
      return formError("value", error);
    if (
      set.entries.some(
        (x) =>
          x.id !== v.id &&
          x.type === v.type &&
          x.value.toLowerCase() === value.toLowerCase(),
      )
    )
      return formError("value", "该匹配方式与内容已经存在。");
    const item = { id: v.id || uid("e"), type: v.type, value };
    if (v.id) set.entries = set.entries.map((x) => (x.id === v.id ? item : x));
    else set.entries.push(item);
  }
  if (form.type === "bulk") {
    if (!form.bulkPreview?.valid.length) return;
    c.sets
      .find((s) => s.id === form.setId)
      .entries.push(
        ...form.bulkPreview.valid.map((x) => ({ ...x, id: uid("e") })),
      );
  }
  if (form.type === "rule") {
    if (v.type === "RULE-SET" && !c.sets.some((s) => s.id === v.value))
      return formError("value", "请选择规则集。");
    if (
      ["DOMAIN", "DOMAIN-SUFFIX", "IP-CIDR"].includes(v.type) &&
      (error = Model.entryError(
        v.type === "IP-CIDR" ? "ip" : "domain",
        v.value,
      ))
    )
      return formError("value", error);
    if (
      v.type === "DOMAIN-KEYWORD" &&
      (!v.value.trim() || /[\s,\r\n]/.test(v.value))
    )
      return formError("value", "请输入不含空格或逗号的域名关键词。");
    if (v.type === "GEOIP" && !/^[A-Za-z]{2}$/.test(v.value))
      return formError("value", "请输入两位地区代码，例如 CN。");
    if (!v.target) return formError("target", "请选择目标策略。");
    if (v.target === "__new") {
      if ((error = validName(v.newName, c.groups, "")))
        return formError("newName", error);
      const id = uid("g");
      c.groups.push({
        id,
        name: v.newName,
        mode: v.newMode,
        pools: v.newPools,
        builtins: [],
        url: "https://www.gstatic.com/generate_204",
        interval: 300,
        tolerance: 50,
      });
      v.target = id;
    }
    const item = {
      id: v.id || uid("r"),
      type: v.type,
      value: v.type === "GEOIP" ? v.value.toUpperCase() : v.value.trim(),
      target: v.target,
      noResolve:
        ["RULE-SET", "IP-CIDR", "GEOIP"].includes(v.type) && v.noResolve,
    };
    c.rules = c.rules.filter((x) => x.id !== v.id);
    c.rules.splice(+v.position, 0, item);
  }
  if (form.type === "fallback") {
    if (!v.target) return formError("target", "请选择兜底去向。");
    c.fallback = v.target;
  }
  if (form.type === "deleteGroup") {
    if (!v.replacement) return formError("replacement", "请选择替代目标。");
    c.rules = c.rules.map((r) =>
      r.target === v.id ? { ...r, target: v.replacement } : r,
    );
    if (c.fallback === v.id) c.fallback = v.replacement;
    c.groups = c.groups.filter((g) => g.id !== v.id);
    c.sources = c.sources.map((s) =>
      s.proxy === v.id ? { ...s, proxy: v.replacement } : s,
    );
  }
  if (form.type === "device") {
    if ((error = validName(v.name, state.devices, v.id)))
      return formError("name", error);
    const device = {
      id: v.id || "",
      revision: v.revision || 0,
      name: v.name,
      state: v.state || "active",
      expiresAt: v.expires ? new Date(v.expires).toISOString() : null,
    };
    const result = await api("devices", "POST", { device, action: "save" });
    await loadState();
    closeDrawer(true);
    render();
    toast("设备授权已保存，立即生效。");
    if (!v.id) await showDeviceLink(result.id);
    return;
  }
  await commit(c);
  closeDrawer(true);
}
// User actions call authenticated management endpoints.
const newSource = () =>
  openForm("source", {
    name: "",
    url: "",
    pool: "quality",
    enabled: true,
    health: true,
    interval: 3600,
    proxy: "",
  });
const newGroup = () =>
  openForm("group", { name: "", mode: "select", pools: [], builtins: [] });
const newRule = (set = "") =>
  openForm("rule", {
    type: "RULE-SET",
    value: set,
    target: "",
    position: state.draft.rules.length,
  });
async function checkDraft() {
  if (busy) return;
  busy = true;
  publishError = "";
  render();
  try {
    await api("check", "POST", { revision: state.rev });
    await loadState();
    toast(
      state.checked?.errors.length
        ? "请修复检查问题。"
        : "原生配置检查通过，远程内容未验证。",
    );
  } catch (e) {
    publishError = e.message;
  } finally {
    busy = false;
    render();
  }
}
async function publishDraft() {
  if (busy) return;
  busy = true;
  publishError = "";
  render();
  try {
    await api("publish", "POST", {
      revision: state.rev,
      note: state.note || "",
    });
    await loadState();
    page = "overview";
    toast("配置已发布，设备更新订阅后获取。");
  } catch (e) {
    publishError = e.message;
  } finally {
    busy = false;
    render();
  }
}
async function handleAction(el) {
  const a = el.dataset.action,
    id = el.dataset.id,
    c = clone(state.draft);
  if (a === "import-open") {
    openForm("import", {});
    return;
  }
  if (a === "import-preview") {
    const file = $("#import-file").files[0];
    if (!file) {
      toast("请选择配置文件。");
      return;
    }
    if (file.size > 4 * 1024 * 1024) {
      toast("文件超过 4 MiB。");
      return;
    }
    const content = await file.text();
    const result = await api("import/preview", "POST", { content });
    form.importContent = content;
    $("#import-result").textContent =
      result.summary +
      " 系统按现有组引用推导两个池，导入后请审阅池名称与成员。";
    $("#drawer button[type=submit]").disabled = false;
    return;
  }
  if (a === "refresh") {
    await loadState();
    render();
    return;
  }
  if (a === "logout") {
    await api("logout", "POST", {});
    location.reload();
    return;
  }
  if (a === "confirm-no" || a === "confirm-yes") {
    const resolve = confirmResolve;
    confirmResolve = null;
    $("#confirm-dialog").close();
    resolve?.(a === "confirm-yes");
    return;
  }
  if (a === "drawer-close") {
    await closeDrawer();
    return;
  }
  if (a === "nav-open") {
    $("#sidebar").classList.add("open");
    return;
  }
  if (a === "nav-close") {
    $("#sidebar").classList.remove("open");
    return;
  }
  if (a === "reveal") {
    const ctrl = document.getElementById(el.dataset.input);
    ctrl.type = ctrl.type === "password" ? "text" : "password";
    el.textContent = ctrl.type === "password" ? "显示" : "隐藏";
    return;
  }
  if (a === "check-bulk") {
    checkBulk();
    return;
  }
  if (a === "copy-link") {
    try {
      await navigator.clipboard.writeText($("#f-link").value);
      toast("已复制设备链接。");
    } catch {
      $("#f-link").select();
      toast("请使用系统复制快捷键复制选中的链接。");
    }
    return;
  }
  if (busy) return;
  if (a === "navigate" || a === "change-go" || a === "fix") {
    if ($("#drawer").open) await closeDrawer(true);
    go(el.dataset.page);
    if (a === "fix" && id) {
      if (page === "sources")
        openForm(
          "source",
          c.sources.find((x) => x.id === id),
        );
      else if (page === "groups")
        openForm(
          "group",
          c.groups.find((x) => x.id === id),
        );
      else if (page === "rules" && c.rules.some((x) => x.id === id)) {
        const r = c.rules.find((x) => x.id === id);
        openForm("rule", { ...r, position: c.rules.indexOf(r) });
      } else if (page === "sets") {
        selectedSet = id;
        render();
      }
    }
    return;
  }
  if (a === "review") {
    go("review");
    return;
  }
  if (a === "review-tab") {
    reviewTab = el.dataset.tab;
    render();
    return;
  }
  if (a === "diff-mode") {
    diffMode = el.dataset.mode;
    render();
    return;
  }
  if (a === "diff-expand") {
    const start = Number(el.dataset.start);
    if (diffExpanded.has(start)) diffExpanded.delete(start);
    else diffExpanded.add(start);
    render();
    return;
  }
  if (a === "check") {
    if (page !== "review") go("review");
    await checkDraft();
    return;
  }
  if (a === "publish") {
    await publishDraft();
    return;
  }
  if (a === "discard") {
    if (!state.active) {
      toast("尚无已发布版本可恢复。");
      return;
    }
    if (
      await ask(
        "放弃全部草稿修改？",
        `草稿将恢复为当前发布 v${state.active}。设备授权不受影响。`,
        "放弃修改",
        true,
      )
    ) {
      await api("restore", "POST", {
        revision: state.rev,
        number: state.active,
      });
      await loadState();
      render();
      toast("草稿已恢复为当前发布配置。");
    }
    return;
  }
  if (a === "new-source") {
    newSource();
    return;
  }
  if (a === "edit-source") {
    openForm(
      "source",
      c.sources.find((x) => x.id === id),
    );
    return;
  }
  if (a === "toggle-source") {
    const s = c.sources.find((x) => x.id === (id || "s2"));
    if (!s) {
      toast("样例订阅已删除，请重置后重试。");
      return;
    }
    s.enabled = !s.enabled;
    await commit(c);
    return;
  }
  if (a === "delete-source") {
    const s = c.sources.find((x) => x.id === id),
      refs = c.groups
        .filter((g) => g.pools.includes(s.pool))
        .map((g) => g.name)
        .join("、");
    if (
      await ask(
        `删除订阅“${s.name}”？`,
        `关联策略组：${refs || "无"}。删除仅保存到草稿；没有来源的策略组会阻止发布。`,
        "删除订阅",
        true,
      )
    ) {
      c.sources = c.sources.filter((x) => x.id !== id);
      await commit(c);
    }
    return;
  }
  if (a === "new-group") {
    newGroup();
    return;
  }
  if (a === "edit-group") {
    openForm(
      "group",
      c.groups.find((x) => x.id === id),
    );
    return;
  }
  if (a === "delete-group") {
    const g = c.groups.find((x) => x.id === id);
    if (
      c.rules.some((r) => r.target === id) ||
      c.fallback === id ||
      c.sources.some((s) => s.proxy === id)
    )
      openForm("deleteGroup", g);
    else if (
      await ask(
        `删除策略组“${g.name}”？`,
        "此组没有引用。删除只保存到草稿。",
        "删除",
        true,
      )
    ) {
      c.groups = c.groups.filter((x) => x.id !== id);
      await commit(c);
    }
    return;
  }
  if (a === "new-set") {
    openForm("set", { name: "", kind: "local", entries: [] });
    return;
  }
  if (a === "edit-set") {
    openForm(
      "set",
      c.sets.find((x) => x.id === id),
    );
    return;
  }
  if (a === "open-set") {
    go("sets");
    selectedSet = id;
    render();
    return;
  }
  if (a === "sets-back") {
    selectedSet = null;
    query = "";
    render();
    return;
  }
  if (a === "delete-set") {
    const s = c.sets.find((x) => x.id === id);
    if (c.rules.some((r) => r.type === "RULE-SET" && r.value === id))
      openForm("blockedSet", s, { readonly: true });
    else if (
      await ask(
        `删除规则集“${s.name}”？`,
        "其自维护条目会一同从草稿删除。",
        "删除",
        true,
      )
    ) {
      c.sets = c.sets.filter((x) => x.id !== id);
      await commit(c);
    }
    return;
  }
  if (a === "new-entry") {
    openForm("entry", { type: "suffix", value: "" }, { setId: id });
    return;
  }
  if (a === "edit-entry") {
    openForm(
      "entry",
      c.sets
        .find((x) => x.id === el.dataset.set)
        .entries.find((x) => x.id === id),
      { setId: el.dataset.set },
    );
    return;
  }
  if (a === "delete-entry") {
    const s = c.sets.find((x) => x.id === el.dataset.set);
    s.entries = s.entries.filter((x) => x.id !== id);
    await commit(c, "条目已从草稿删除，当前发布不变。");
    return;
  }
  if (a === "bulk") {
    openForm("bulk", { entryType: "suffix", text: "" }, { setId: id });
    return;
  }
  if (a === "new-rule") {
    newRule(el.dataset.set || "");
    return;
  }
  if (a === "edit-rule") {
    const r = c.rules.find((x) => x.id === id);
    openForm("rule", { ...r, position: c.rules.indexOf(r) });
    return;
  }
  if (a === "move-rule") {
    const i = c.rules.findIndex((r) => r.id === id),
      next = i + Number(el.dataset.offset);
    if (next < 0 || next >= c.rules.length) return;
    const [r] = c.rules.splice(i, 1);
    c.rules.splice(next, 0, r);
    await commit(c);
    return;
  }
  if (a === "delete-rule") {
    if (
      await ask(
        "删除这条分流规则？",
        "后续规则及兜底将接收原先匹配的流量，发布后生效。",
        "删除规则",
        true,
      )
    ) {
      c.rules = c.rules.filter((x) => x.id !== id);
      await commit(c);
    }
    return;
  }
  if (a === "edit-fallback") {
    openForm("fallback", { target: c.fallback });
    return;
  }
  if (a === "drawer-go-rules") {
    await closeDrawer(true);
    go("rules");
    return;
  }
  if (a === "base") {
    openForm("base", {}, { readonly: true });
    return;
  }
  if (a === "preview-active" || a === "preview-draft") {
    openForm(
      "preview",
      {
        title:
          a === "preview-active"
            ? `当前发布 v${state.active}`
            : `草稿 r${state.rev}`,
        config: a === "preview-active" ? Model.active(state).config : c,
      },
      { readonly: true },
    );
    return;
  }
  if (
    [
      "view-version",
      "compare-version",
      "restore-version",
      "export-version",
    ].includes(a)
  ) {
    const v = state.versions.find((x) => x.n === Number(el.dataset.version));
    if (a === "view-version") {
      await fetchVersionYAML(v);
      openForm(
        "preview",
        { title: `发布版本 v${v.n}`, config: v.config },
        { readonly: true },
      );
    }
    if (a === "compare-version") {
      await fetchVersionYAML(v);
      openForm("compare", v, { readonly: true });
    }
    if (
      a === "restore-version" &&
      (await ask(
        `将 v${v.n} 恢复到草稿？`,
        "会替换现有草稿。当前发布和设备授权不变，仍需检查并发布。",
        "恢复到草稿",
      ))
    ) {
      await api("restore", "POST", { revision: state.rev, number: v.n });
      await loadState();
      page = "review";
      render();
      toast("已恢复到草稿，尚未发布。");
    }
    if (a === "export-version") {
      await fetchVersionYAML(v);
      const url = URL.createObjectURL(
          new Blob([Model.yaml(v.config)], { type: "text/yaml" }),
        ),
        link = document.createElement("a");
      link.href = url;
      link.download = `redacted-v${v.n}.yaml`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      toast("已导出脱敏配置。真实客户端请使用设备订阅链接。");
    }
    return;
  }
  if (a === "new-device") {
    openForm("device", { name: "", expires: "" });
    return;
  }
  if (a === "edit-device") {
    openForm(
      "device",
      state.devices.find((x) => x.id === id),
    );
    return;
  }
  if (a === "device-link") {
    await showDeviceLink(id);
    return;
  }
  if (a === "toggle-device") {
    const d = state.devices.find((x) => x.id === id);
    await deviceAction(d, d.state === "paused" ? "resume" : "pause");
    return;
  }
  if (a === "revoke-device") {
    const d = state.devices.find((x) => x.id === id);
    if (
      await ask(
        "撤销设备授权？",
        "立即阻止后续下载，不能收回已经下载的配置。",
        "撤销授权",
        true,
      )
    )
      await deviceAction(d, "revoke");
    return;
  }
  if (a === "rotate-device") {
    const d = state.devices.find((x) => x.id === id);
    if (
      await ask(
        "更换设备链接？",
        "旧链接立即失效，需要在客户端重新导入。",
        "更换链接",
      )
    ) {
      await deviceAction(d, "rotate");
      await showDeviceLink(id);
    }
    return;
  }
}
document.addEventListener("click", (event) => {
  const el = event.target.closest("[data-action]");
  if (!el || el.disabled) return;
  event.preventDefault();
  handleAction(el).catch(showError);
});
document.addEventListener("submit", (event) => {
  if (event.target.matches("#drawer form")) {
    event.preventDefault();
    saveForm().catch(showError);
  }
});
document.addEventListener("input", (event) => {
  const el = event.target;
  if (el.matches("[data-search]")) {
    const pos = el.selectionStart;
    query = el.value;
    render();
    const search = $("[data-search]");
    search.focus();
    search.setSelectionRange(pos, pos);
    return;
  }
  if (el.id === "publish-note") {
    state.note = el.value;
    return;
  }
  if (el.closest("#drawer form") && form) {
    if (form.type === "bulk") {
      form.bulkPreview = null;
      $("#bulk-preview").innerHTML = "";
      $("#drawer button[type=submit]").disabled = true;
    }
    el.removeAttribute("aria-invalid");
    const error = $(`#err-${el.name}`);
    if (error) error.textContent = "";
    $("#form-error").innerHTML = "";
    updateFormPreview();
  }
});
document.addEventListener("change", (event) => {
  const el = event.target;
  if (el.dataset.filter) {
    if (el.dataset.filter === "pool") poolFilter = el.value;
    else sourceStatus = el.value;
    render();
    return;
  }
  if (!form || !el.closest("#drawer form")) return;
  if (
    (form.type === "group" && el.name === "mode") ||
    (form.type === "set" && el.name === "kind") ||
    (["entry", "rule"].includes(form.type) && el.name === "type") ||
    (form.type === "rule" && el.name === "target")
  ) {
    const values = readForm();
    form.value = { ...form.value, ...values };
    if (form.type === "rule" && el.name === "type") form.value.value = "";
    renderDrawer();
    $(`#f-${el.name}`)?.focus();
  } else updateFormPreview();
});
$("#drawer").addEventListener("cancel", (event) => {
  event.preventDefault();
  closeDrawer();
});
$("#confirm-dialog").addEventListener("cancel", (event) => {
  event.preventDefault();
  const resolve = confirmResolve;
  confirmResolve = null;
  $("#confirm-dialog").close();
  resolve?.(false);
});
$("#drawer").addEventListener("click", (event) => {
  if (event.target === $("#drawer")) {
    const r = $("#drawer").getBoundingClientRect();
    if (
      event.clientX < r.left ||
      event.clientX > r.right ||
      event.clientY < r.top ||
      event.clientY > r.bottom
    )
      closeDrawer();
  }
});
let draggedRule = null;
document.addEventListener("dragstart", (event) => {
  const handle = event.target.closest("[data-drag]");
  if (!handle || query) return;
  draggedRule = handle.dataset.drag;
  event.dataTransfer.effectAllowed = "move";
  event.dataTransfer.setData("text/plain", draggedRule);
});
document.addEventListener("dragover", (event) => {
  if (draggedRule && event.target.closest("[data-drop]"))
    event.preventDefault();
});
document.addEventListener("drop", async (event) => {
  const row = event.target.closest("[data-drop]");
  if (!draggedRule || !row || query) return;
  event.preventDefault();
  const c = clone(state.draft),
    from = c.rules.findIndex((r) => r.id === draggedRule),
    to = c.rules.findIndex((r) => r.id === row.dataset.drop);
  draggedRule = null;
  if (from === to || from < 0 || to < 0) return;
  const [r] = c.rules.splice(from, 1);
  c.rules.splice(to, 0, r);
  await commit(c).catch(showError);
});
document.addEventListener("dragend", () => {
  draggedRule = null;
});
// START
matchMedia("(max-width:760px)").addEventListener("change", () => {
  if (page === "review" && reviewTab === "config") render();
});
loadState().then(render).catch(showError);

export { state, render, openForm, toast, form, formError };
export function setState(value, server) {
  state = value;
  serverState = server;
}
