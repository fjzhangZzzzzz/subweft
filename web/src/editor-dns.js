import preset from "./dns-preset.json";

const esc = (v) =>
  String(v ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
const bools = {
  enable: "启用 DNS",
  ipv6: "IPv6 解析",
  "respect-rules": "DNS 请求遵守路由",
  "use-hosts": "使用配置中的 hosts",
  "fallback-filter.geoip": "按 GeoIP 筛选",
};
const lists = {
  "default-nameserver": "引导解析服务器",
  "proxy-server-nameserver": "节点域名解析服务器",
  fallback: "后备解析服务器",
  "fake-ip-filter": "Fake-IP 例外",
  "fallback-filter.ipcidr": "需使用后备结果的 IP 网段",
  "fallback-filter.domain": "直接使用后备解析的域名",
};
const get = (dns, key) => key.split(".").reduce((v, k) => v?.[k], dns);
const lines = (v) => (Array.isArray(v) ? v.join("\n") : (v ?? ""));
const split = (v) =>
  v
    .split(/\r?\n/)
    .map((x) => x.trim())
    .filter(Boolean);
const id = (key) => `dns-${key.replaceAll(".", "-")}`;
function field(key, title, control, hint = "") {
  return `<div class="field"><label for="f-${id(key)}">${title}</label>${control}${hint ? `<small class="hint">${hint}</small>` : ""}</div>`;
}
function choice(dns, key, title, choices, hint = "") {
  const value = get(dns, key);
  return field(
    key,
    title,
    `<select id="f-${id(key)}" name="${id(key)}" data-dns-key="${key}"><option value="" ${value === undefined ? "selected" : ""}>未设置（使用 Mihomo 默认）</option>${choices.map(([v, label]) => `<option value="${v}" ${String(value) === v ? "selected" : ""}>${label}</option>`).join("")}</select>`,
    hint,
  );
}
function boolean(dns, key) {
  return choice(
    dns,
    key,
    bools[key],
    [
      ["true", "开启"],
      ["false", "关闭"],
    ],
    key === "respect-rules"
      ? "开启后需配置节点域名解析服务器，避免解析依赖循环。"
      : "",
  );
}
function list(dns, key) {
  return field(
    key,
    lists[key],
    `<textarea id="f-${id(key)}" name="${id(key)}" data-dns-key="${key}" rows="3" spellcheck="false">${esc(lines(get(dns, key)))}</textarea>`,
    "每行一项；清空后不指定此字段。",
  );
}
function text(dns, key, title, hint) {
  return field(
    key,
    title,
    `<input id="f-${id(key)}" name="${id(key)}" data-dns-key="${key}" value="${esc(get(dns, key))}" spellcheck="false">`,
    hint,
  );
}
let sequence = 0;
function serverRow(value = "") {
  const n = sequence++;
  return `<div class="dns-server-row"><input aria-label="默认解析服务器地址" name="dns-server-${n}" data-dns-server value="${esc(value)}" spellcheck="false"><button type="button" data-action="dns-remove-row" aria-label="移除此解析服务器">−</button></div>`;
}
function policyRow(key = "", value = "") {
  const n = sequence++;
  return `<div class="dns-policy-row"><label>匹配条件<input name="dns-policy-key-${n}" data-dns-policy-key value="${esc(key)}" placeholder="geosite:gfw" spellcheck="false"></label><label>解析服务器<textarea name="dns-policy-value-${n}" data-dns-policy-value rows="2" spellcheck="false" placeholder="每行一个地址">${esc(lines(value))}</textarea></label><button type="button" data-action="dns-remove-row" aria-label="移除此解析策略">移除</button></div>`;
}
export function dnsEditor(dns = {}) {
  return `<section class="dns-settings"><div class="row between wrap"><h3>DNS 设置</h3><button type="button" data-action="dns-preset">应用 clash-config DNS 预设</button></div><p class="small muted">仅填充当前表单的 DNS 设置，保存到草稿后再审阅发布。未编辑字段保持原值。</p>
 ${boolean(dns, "enable")}${boolean(dns, "ipv6")}${choice(
   dns,
   "enhanced-mode",
   "解析模式",
   [
     ["fake-ip", "fake-ip"],
     ["redir-host", "redir-host"],
   ],
 )}
 <fieldset class="dns-list"><legend>默认解析服务器</legend><div data-dns-servers>${(dns.nameserver || []).map(serverRow).join("")}</div><button type="button" data-action="dns-add-server" class="text-button">＋ 添加服务器</button></fieldset>
 <details class="advanced"><summary>高级设置</summary>${boolean(dns, "respect-rules")}${list(dns, "default-nameserver")}${list(dns, "proxy-server-nameserver")}
 <fieldset class="dns-list"><legend>域名解析策略</legend><p class="small muted">匹配域名或 geosite 分类，每行一个解析服务器；优先于默认及后备解析。</p><div data-dns-policies>${Object.entries(
   dns["nameserver-policy"] || {},
 )
   .map(([k, v]) => policyRow(k, v))
   .join(
     "",
   )}</div><button type="button" data-action="dns-add-policy" class="text-button">＋ 添加策略</button></fieldset>
 ${list(dns, "fallback")}<details class="advanced"><summary>后备筛选条件</summary>${boolean(dns, "fallback-filter.geoip")}${text(dns, "fallback-filter.geoip-code", "采用默认解析结果的国家代码", "例如 CN；留空保持核心默认。")}${list(dns, "fallback-filter.ipcidr")}${list(dns, "fallback-filter.domain")}</details>
 <details class="advanced"><summary>Fake-IP 范围与例外</summary>${text(dns, "fake-ip-range", "Fake-IP IPv4 范围", "例如 198.18.0.1/16；例外沿用当前配置的过滤模式。")}${list(dns, "fake-ip-filter")}${boolean(dns, "use-hosts")}</details>
 <p class="notice">GeoSite 策略和 GeoIP 筛选依赖客户端对应数据。服务端缺库时，检查报告会注明未验证项；不代表实际解析效果已验证。</p></details></section>`;
}
export function readDNS(root, original = {}) {
  const dns = structuredClone(original);
  const set = (key, value) => {
    const parts = key.split(".");
    let node = dns;
    if (parts.length === 2) node = dns[parts[0]] ??= {};
    const prop = parts.at(-1);
    if (value === undefined) delete node[prop];
    else node[prop] = value;
    if (parts.length === 2 && !Object.keys(node).length) delete dns[parts[0]];
  };
  for (const input of root.querySelectorAll("[data-dns-key]")) {
    const key = input.dataset.dnsKey,
      old = get(original, key),
      value = input.value;
    const initial = key in lists ? lines(old) : String(old ?? "");
    if (value === initial) continue;
    set(
      key,
      value.trim() === ""
        ? undefined
        : key in bools
          ? value === "true"
          : key in lists
            ? split(value)
            : value.trim(),
    );
  }
  const servers = [...root.querySelectorAll("[data-dns-server]")]
    .map((x) => x.value.trim())
    .filter(Boolean);
  if (JSON.stringify(servers) !== JSON.stringify(original.nameserver || []))
    set("nameserver", servers.length ? servers : undefined);
  const policy = {};
  for (const row of root.querySelectorAll(".dns-policy-row")) {
    const key = row.querySelector("[data-dns-policy-key]").value.trim();
    const value = row.querySelector("[data-dns-policy-value]").value;
    if (!key && !value.trim()) continue;
    if (!key || !value.trim())
      throw Error("请填写解析策略的匹配条件和服务器地址。");
    if (Object.hasOwn(policy, key)) throw Error("解析策略的匹配条件不能重复。");
    const old = original["nameserver-policy"]?.[key];
    Object.defineProperty(policy, key, {
      value: value === lines(old) ? old : split(value),
      enumerable: true,
      writable: true,
      configurable: true,
    });
  }
  if (
    JSON.stringify(policy) !==
    JSON.stringify(original["nameserver-policy"] || {})
  )
    set("nameserver-policy", Object.keys(policy).length ? policy : undefined);
  return dns;
}
export function applyDNSPreset(original = {}) {
  return {
    ...structuredClone(original),
    ...structuredClone(preset),
    "fallback-filter": {
      ...structuredClone(original["fallback-filter"] || {}),
      ...structuredClone(preset["fallback-filter"]),
    },
  };
}
export function dnsRowAction(action, el, root) {
  if (action === "dns-remove-row") {
    const row = el.closest(".dns-server-row,.dns-policy-row"),
      container = row.parentElement;
    const next = row.nextElementSibling || row.previousElementSibling;
    row.remove();
    (
      next?.querySelector("input") ||
      container.parentElement.querySelector("button")
    )?.focus();
  } else {
    const servers = action === "dns-add-server",
      container = root.querySelector(
        servers ? "[data-dns-servers]" : "[data-dns-policies]",
      );
    container.insertAdjacentHTML(
      "beforeend",
      servers ? serverRow() : policyRow(),
    );
    container.lastElementChild.querySelector("input").focus();
  }
}
