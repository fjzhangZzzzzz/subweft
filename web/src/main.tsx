import React, { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { createRoot } from "react-dom/client";
import {
  Activity,
  ArrowDown,
  ArrowUp,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleHelp,
  Clipboard,
  Copy,
  Download,
  ExternalLink,
  FileCheck2,
  Globe2,
  Layers3,
  Link2,
  LoaderCircle,
  LogOut,
  Monitor,
  Moon,
  Network,
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Server,
  Settings2,
  ShieldCheck,
  Smartphone,
  Sun,
  Trash2,
  TriangleAlert,
  Unplug,
  X,
} from "lucide-react";
import type {
  Device,
  Group,
  ManualNode,
  Meta,
  Node,
  Profile,
  Release,
  Rule,
  Source,
  State,
} from "./types";
import { emptyGroup, emptyProfile } from "./types";
import "./style.css";

type Page = "overview" | "sources" | "profiles" | "devices" | "system";
let csrf = "";
class APIError extends Error {
  field: string;
  constructor(code: string, field = "") {
    super(code);
    this.field = field;
  }
}
async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch("/api/v1/" + path, {
    method,
    headers:
      method === "GET"
        ? {}
        : { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 204) return undefined as T;
  const data = await response.json();
  if (!response.ok)
    throw new APIError(
      data.error?.code || "request_failed",
      data.error?.field || "",
    );
  return data;
}
const split = (v: string) =>
  v
    .split(/[,，\n]/)
    .map((x) => x.trim())
    .filter(Boolean);
const toggle = (values: string[], value: string) =>
  values.includes(value)
    ? values.filter((v) => v !== value)
    : [...values, value];
const copy = (v: unknown) => structuredClone(v);

function App() {
  const [lang, setLang] = useState(
    localStorage.getItem("subweft.lang") || "zh",
  );
  const t = (zh: string, en: string) => (lang === "zh" ? zh : en);
  const [theme, setTheme] = useState(
    localStorage.getItem("subweft.theme") || "system",
  );
  const [page, setPage] = useState<Page>(
    (location.hash.slice(1) as Page) || "overview",
  );
  const [state, setState] = useState<State | null>(null);
  const [loading, setLoading] = useState(true);
  const [login, setLogin] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [unsaved, setUnsaved] = useState(false);
  const [toast, setToast] = useState("");
  const [modal, setModal] = useState<ReactNode>(null);
  const [sourceTab, setSourceTab] = useState("subscriptions");
  const [query, setQuery] = useState("");
  const [selectedProfile, setSelectedProfile] = useState("");
  const modalRef = useRef<HTMLDialogElement>(null);
  const msg = (e: unknown) => {
    if (!(e instanceof APIError))
      return t("请求失败，请重试。", "Request failed. Please retry.");
    const messages: Record<string, [string, string]> = {
      conflict: [
        "内容已变化，请刷新后重试。",
        "The revision changed. Reload and retry.",
      ],
      invalid_json: ["输入格式不正确。", "Invalid request data."],
      fetch_failed: [
        "抓取失败；请检查地址、网络与抓取允许列表。旧缓存已保留。",
        "Fetch failed. Check the address, network, and allowlist. The previous cache is preserved.",
      ],
      unsupported_or_invalid_field: [
        "存在不支持或无效的节点字段。",
        "Unsupported or invalid node field.",
      ],
      unsupported_protocol: [
        "此闭环版本仅支持 Shadowsocks。",
        "This initial slice supports Shadowsocks only.",
      ],
      unsupported_root_field: [
        "此版本接受仅含 proxies 的 provider YAML，或 ss:// 列表。",
        "Use provider YAML with only proxies, or an ss:// list.",
      ],
      empty_group: [
        "策略组没有匹配成员，请选择来源或明确的内置出口。",
        "The group is empty. Select sources or an explicit built-in outbound.",
      ],
      group_cycle: ["策略组存在循环引用。", "Policy groups contain a cycle."],
      rule_target_missing: [
        "规则目标不存在，请检查组名称。",
        "A rule target does not exist.",
      ],
      match_required: [
        "需要一条启用的 MATCH 兜底规则。",
        "An enabled MATCH fallback rule is required.",
      ],
      match_must_be_last: [
        "MATCH 必须是最后一条启用的规则。",
        "MATCH must be the last enabled rule.",
      ],
      source_unavailable: [
        "选中的订阅尚无有效节点或已停用。",
        "A selected source is disabled or has no valid nodes.",
      ],
      stale_cache_requires_ack: [
        "有订阅更新失败。确认使用旧缓存后再预览或发布。",
        "A source failed to refresh. Acknowledge the cached data before publishing.",
      ],
      native_validation_failed: [
        "Mihomo 校验未通过，当前发布保持不变。",
        "Mihomo rejected the candidate. The current release is unchanged.",
      ],
      resource_in_use: [
        "资源仍被方案引用，请先解除引用。",
        "This resource is referenced by a profile.",
      ],
      published_profile_cannot_delete: [
        "已发布方案暂不支持删除。",
        "Published profiles cannot be deleted in this slice.",
      ],
      profile_not_published: [
        "先发布方案，再绑定设备。",
        "Publish the profile before binding a device.",
      ],
      csrf_rejected: [
        "会话校验失败，请重新登录。",
        "Session validation failed. Sign in again.",
      ],
    };
    const text =
      messages[e.message]?.[lang === "zh" ? 0 : 1] ||
      `${t("操作未完成", "Operation rejected")}: ${e.message}`;
    return e.field ? `${text} (${e.field})` : text;
  };
  const reload = async () => {
    const value = await api<State>("state");
    csrf = value.csrf;
    setState(value);
    setLogin(false);
    return value;
  };
  useEffect(() => {
    reload()
      .catch((e) => {
        if (e instanceof APIError && e.message === "login_required")
          setLogin(true);
        else setError(msg(e));
      })
      .finally(() => setLoading(false));
  }, []);
  useEffect(() => {
    localStorage.setItem("subweft.lang", lang);
    document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";
  }, [lang]);
  useEffect(() => {
    localStorage.setItem("subweft.theme", theme);
    document.documentElement.dataset.theme = theme;
  }, [theme]);
  useEffect(() => {
    const onHash = () =>
      setPage((location.hash.slice(1) as Page) || "overview");
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  useEffect(() => {
    if (toast) {
      const timer = setTimeout(() => setToast(""), 4500);
      return () => clearTimeout(timer);
    }
  }, [toast]);
  useEffect(() => {
    if (modal && !modalRef.current?.open) {
      modalRef.current?.showModal();
      modalRef.current
        ?.querySelector<HTMLInputElement>("input,select,textarea")
        ?.focus();
    }
    if (!modal) modalRef.current?.close();
  }, [modal]);
  useEffect(() => {
    if (error) {
      const alert =
        document.querySelector<HTMLElement>("dialog[open] .alert") ||
        document.querySelector<HTMLElement>("#main .alert");
      alert?.setAttribute("tabindex", "-1");
      alert?.focus();
    }
  }, [error]);
  const navigate = (next: Page) => {
    if (
      unsaved &&
      !confirm(
        t(
          "离开将丢弃未保存的表单修改，是否继续？",
          "Leave and discard unsaved form changes?",
        ),
      )
    )
      return;
    setUnsaved(false);
    location.hash = next;
    setPage(next);
    setError("");
    setQuery("");
  };
  const run = async (
    work: () => Promise<unknown>,
    success?: string,
    close = false,
  ) => {
    setBusy(true);
    setError("");
    try {
      await work();
      await reload();
      if (success) setToast(success);
      if (close) setModal(null);
    } catch (e) {
      setError(msg(e));
      if (!(e instanceof APIError) || e.message !== "conflict") {
        try {
          await reload();
        } catch {}
      }
    } finally {
      setBusy(false);
    }
  };
  const showSource = (value?: Source) =>
    setModal(
      <SourceForm
        key={value?.id || "new"}
        value={value}
        t={t}
        onSave={(inValue) =>
          run(
            () =>
              api(
                value ? `sources/${value.id}` : "sources",
                value ? "PUT" : "POST",
                inValue,
              ),
            t("来源已保存", "Source saved"),
            true,
          )
        }
      />,
    );
  const showNode = (value?: ManualNode) =>
    setModal(
      <NodeForm
        value={value}
        t={t}
        onSave={(inValue) =>
          run(
            () =>
              api(
                value ? `nodes/${value.id}` : "nodes",
                value ? "PUT" : "POST",
                inValue,
              ),
            t("节点已保存", "Node saved"),
            true,
          )
        }
      />,
    );
  const showProfile = () =>
    setModal(
      <NameForm
        title={t("新建配置方案", "New configuration profile")}
        t={t}
        onSave={(name) =>
          run(
            async () => {
              const profile = await api<Profile>(
                "profiles",
                "POST",
                emptyProfile(name),
              );
              setSelectedProfile(profile.id);
              navigate("profiles");
            },
            t("方案草稿已创建", "Profile draft created"),
            true,
          )
        }
      />,
    );
  const showDevice = (value?: Device) =>
    state &&
    setModal(
      <DeviceForm
        value={value}
        profiles={state.profiles.filter((p) => state.active[p.id])}
        t={t}
        onSave={(inValue) =>
          run(
            () =>
              api(
                value ? `devices/${value.id}` : "devices",
                value ? "PUT" : "POST",
                inValue,
              ),
            t("设备已保存", "Device saved"),
            true,
          )
        }
      />,
    );
  const remove = (kind: string, v: Meta) => {
    if (
      confirm(
        t(
          "确认删除？仍被引用的资源将拒绝删除。",
          "Delete this item? Referenced resources cannot be deleted.",
        ),
      )
    )
      void run(
        () => api(`${kind}/${v.id}`, "DELETE", { revision: v.revision }),
        t("已删除", "Deleted"),
      );
  };
  const showNodes = async (source: Source) => {
    setBusy(true);
    try {
      const nodes = await api<
        Array<Pick<Node, "name" | "type" | "server" | "port">>
      >(`sources/${source.id}/nodes`);
      setModal(
        <SourceNodeList
          source={source}
          nodes={nodes}
          t={t}
          onCopy={(node) =>
            setModal(
              <NameForm
                title={t("复制为手工节点", "Copy to manual node")}
                initial={node.name + " copy"}
                t={t}
                onSave={(name) =>
                  run(
                    () =>
                      api(`sources/${source.id}/copy-node`, "POST", {
                        name: node.name,
                        newName: name,
                      }),
                    t("手工节点已创建", "Manual node created"),
                    true,
                  )
                }
              />,
            )
          }
        />,
      );
    } catch (e) {
      setError(msg(e));
    } finally {
      setBusy(false);
    }
  };
  const showLink = async (device: Device) => {
    setBusy(true);
    try {
      const { url } = await api<{ url: string }>(`devices/${device.id}/link`);
      setModal(
        <div>
          <ModalTitle title={t("设备订阅链接", "Device subscription link")} />
          <p className="muted">
            {t(
              "此链接包含设备凭据，请仅提供给这台设备。普通发布和切换方案不会改变链接。",
              "This link contains a device credential. Publishing and profile switching preserve it.",
            )}
          </p>
          <Field label={device.name}>
            <input readOnly value={url} onFocus={(e) => e.target.select()} />
          </Field>
          <div className="actions">
            <Button
              primary
              onClick={() => {
                void navigator.clipboard
                  .writeText(url)
                  .then(() => setToast(t("链接已复制", "Link copied")));
              }}
            >
              <Clipboard size={15} />
              {t("复制链接", "Copy link")}
            </Button>
            <a
              className="button"
              href={`clash://install-config?url=${encodeURIComponent(url)}`}
            >
              <ExternalLink size={15} />
              {t("导入 Clash Verge", "Import into Clash Verge")}
            </a>
          </div>
          <p className="hint">
            {t(
              "导入可能新增配置，不能保证覆盖已有配置。请在客户端确认。",
              "Import may create a new profile. Check the result in your client.",
            )}
          </p>
        </div>,
      );
    } catch (e) {
      setError(msg(e));
    } finally {
      setBusy(false);
    }
  };
  const time = (v?: string) =>
    !v || v.startsWith("0001-")
      ? t("尚未更新", "Not yet")
      : new Date(v).toLocaleString(lang === "zh" ? "zh-CN" : "en", {
          month: "short",
          day: "numeric",
          hour: "2-digit",
          minute: "2-digit",
        });
  const nav: Array<[Page, ReactNode, string]> = [
    ["overview", <Activity size={18} />, t("概览", "Overview")],
    ["sources", <Network size={18} />, t("来源", "Sources")],
    ["profiles", <Layers3 size={18} />, t("配置方案", "Profiles")],
    ["devices", <Monitor size={18} />, t("设备", "Devices")],
    ["system", <Settings2 size={18} />, t("系统", "System")],
  ];
  const activeProfile =
    state?.profiles.find((p) => p.id === selectedProfile) || state?.profiles[0];
  const filter = (name: string) =>
    name.toLocaleLowerCase().includes(query.toLocaleLowerCase());

  if (loading)
    return (
      <div className="center">
        <LoaderCircle className="spin" />
        <p>{t("正在连接 Subweft…", "Connecting to Subweft…")}</p>
      </div>
    );
  if (login || !state)
    return (
      <main className="login">
        <div className="login-card">
          <Brand />
          <span className="eyebrow">SUBSCRIPTIONS, WOVEN TOGETHER</span>
          <h1>{t("让每份配置，井然有序。", "A place for every policy.")}</h1>
          <p>
            {t(
              "在一个清晰的工作台中维护订阅、策略与设备。使用统一身份登录，开始管理。",
              "Manage your sources, policies, and devices in one workspace. Sign in with your identity provider.",
            )}
          </p>
          {error && <div className="alert error">{error}</div>}
          <a className="button primary wide" href="/auth/login">
            <ShieldCheck size={18} />
            {t("使用统一身份登录", "Sign in with OIDC")}
            <ChevronRight size={17} />
          </a>
          <small>
            {t(
              "开发预览 · 此闭环版本仅支持 Shadowsocks",
              "Development preview · Shadowsocks initial slice",
            )}
          </small>
        </div>
        <div className="login-art" aria-hidden="true">
          <div className="weave">
            <span />
            <span />
            <span />
            <span />
            <span />
            <span />
          </div>
          <p>Sources. Policies. Your devices.</p>
        </div>
      </main>
    );
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Brand />
        <div className="workspace-label">
          {t("个人工作台", "PERSONAL WORKSPACE")}
        </div>
        <nav aria-label={t("主导航", "Main navigation")}>
          {nav.map(([key, icon, label]) => (
            <a
              href={`#${key}`}
              key={key}
              className={page === key ? "nav-item active" : "nav-item"}
              onClick={(e) => {
                e.preventDefault();
                navigate(key);
              }}
              aria-label={label}
              aria-current={page === key ? "page" : undefined}
            >
              {icon}
              <span>{label}</span>
              {key === "sources" && <small>{state.sources.length}</small>}
            </a>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="identity">
            <div className="avatar">{state.user.slice(0, 1)}</div>
            <div>
              <strong>{state.user}</strong>
              <small>{t("管理员", "Administrator")}</small>
            </div>
            <button
              className="icon-button"
              aria-label={t("退出登录", "Sign out")}
              onClick={() =>
                void api("logout", "POST", {}).then(() => location.reload())
              }
            >
              <LogOut size={16} />
            </button>
          </div>
          <span className="version">
            <span className="dot" />
            Development preview · 0.1.0
          </span>
        </div>
      </aside>
      <div className="workspace">
        <header className="topbar">
          <div className="crumb">
            Subweft <ChevronRight size={13} />
            {nav.find((n) => n[0] === page)?.[2]}
          </div>
          <div className="top-actions">
            <button
              className="text-button"
              onClick={() => setLang(lang === "zh" ? "en" : "zh")}
            >
              <Globe2 size={15} />
              {lang === "zh" ? "EN" : "中文"}
            </button>
            <button
              className="icon-button"
              aria-label={t("切换主题", "Switch theme")}
              title={theme}
              onClick={() =>
                setTheme(
                  theme === "system"
                    ? "light"
                    : theme === "light"
                      ? "dark"
                      : "system",
                )
              }
            >
              {theme === "dark" ? (
                <Moon size={17} />
              ) : theme === "light" ? (
                <Sun size={17} />
              ) : (
                <Monitor size={17} />
              )}
            </button>
            <span className="pill subtle">
              {t("首个业务闭环", "Initial vertical slice")}
            </span>
          </div>
        </header>
        <main id="main" className="content">
          {error && (
            <div role="alert" className="alert error">
              <TriangleAlert size={18} />
              <div>{error}</div>
              <button
                className="icon-button"
                onClick={() => setError("")}
                aria-label={t("关闭错误", "Dismiss error")}
              >
                <X size={16} />
              </button>
            </div>
          )}
          {page === "overview" && (
            <>
              <PageHeader
                eyebrow="YOUR CONFIGURATION WORKSPACE"
                title={t(
                  "配置有条理，使用更轻松。",
                  "Your policies, in good order.",
                )}
                description={t(
                  "从来源到设备，每次修改都有清晰的去向。",
                  "From your sources to your devices, every change has a clear path.",
                )}
                action={
                  <Button primary onClick={() => showSource()}>
                    <Plus size={16} />
                    {t("添加来源", "Add source")}
                  </Button>
                }
              />
              <div className="stats">
                {[
                  [
                    t("订阅来源", "Sources"),
                    state.sources.length,
                    <Network size={19} />,
                  ],
                  [
                    t("可用节点", "Available nodes"),
                    state.sources.reduce((n, s) => n + s.nodeCount, 0) +
                      state.nodes.length,
                    <Server size={19} />,
                  ],
                  [
                    t("已发布方案", "Published profiles"),
                    Object.keys(state.active).length,
                    <FileCheck2 size={19} />,
                  ],
                  [
                    t("已登记设备", "Devices"),
                    state.devices.length,
                    <Monitor size={19} />,
                  ],
                ].map(([label, value, icon], i) => (
                  <div className="stat" key={i}>
                    <div className="stat-top">
                      <span>{label}</span>
                      {icon}
                    </div>
                    <strong>{value}</strong>
                    <small>{t("当前工作台", "In this workspace")}</small>
                  </div>
                ))}
              </div>
              <div className="overview-grid">
                <section className="panel">
                  <div className="panel-header">
                    <div>
                      <h2>{t("开始编排", "Build your configuration")}</h2>
                      <p>
                        {t(
                          "一个完整流程，四个清楚的步骤。",
                          "One complete workflow, four clear steps.",
                        )}
                      </p>
                    </div>
                    <span className="pill">{t("工作流", "Workflow")}</span>
                  </div>
                  {[
                    [
                      t("添加订阅或手工节点", "Add sources or manual nodes"),
                      t(
                        "来源凭据由服务保管，节点按需复用。",
                        "Keep credentials on the server and reuse sources.",
                      ),
                      "sources",
                      state.sources.length + state.nodes.length > 0,
                    ],
                    [
                      t("编排策略组与规则", "Arrange groups and rules"),
                      t(
                        "选择来源、设置策略，明确流量去向。",
                        "Choose members, policies, and routing destinations.",
                      ),
                      "profiles",
                      state.profiles.length > 0,
                    ],
                    [
                      t("校验并发布", "Validate and publish"),
                      t(
                        "校验通过后切换，失败保留当前版本。",
                        "Switch only after validation; preserve the current release on failure.",
                      ),
                      "profiles",
                      Object.keys(state.active).length > 0,
                    ],
                    [
                      t("绑定设备并导入", "Enroll and import"),
                      t(
                        "每台设备拥有独立、可撤销的链接。",
                        "Give every device its own revocable link.",
                      ),
                      "devices",
                      state.devices.length > 0,
                    ],
                  ].map(([title, description, route, done], i) => (
                    <button
                      className="workflow-step"
                      key={i}
                      onClick={() => navigate(route as Page)}
                    >
                      <span className={done ? "step done" : "step"}>
                        {done ? <Check size={15} /> : i + 1}
                      </span>
                      <span>
                        <strong>{title}</strong>
                        <small>{description}</small>
                      </span>
                      <ChevronRight size={17} />
                    </button>
                  ))}
                </section>
                <section className="panel status-panel">
                  <ShieldCheck size={28} className="accent" />
                  <h2>
                    {t("发布之前，先确定。", "Confidence before publishing.")}
                  </h2>
                  <p>
                    {t(
                      "草稿与发布版本分开维护。订阅更新失败时保留旧缓存，设备下载只读取已发布内容。",
                      "Drafts and releases stay separate. Failed refreshes preserve cached sources; devices receive published content only.",
                    )}
                  </p>
                  <div className="status-line">
                    <span className="dot" />
                    {t("独立设备授权", "Independent device authorization")}
                  </div>
                  <div className="status-line">
                    <span className="dot" />
                    {t("加密持久化", "Encrypted persistence")}
                  </div>
                  <div className="status-line">
                    <span className="dot" />
                    {t("原生 Mihomo 校验", "Native Mihomo validation")}
                  </div>
                  <div className="notice">
                    {t(
                      "当前仅实现首个 SS 闭环，完整首版能力仍在扩展。",
                      "The SS vertical slice is implemented. Full initial scope remains in progress.",
                    )}
                  </div>
                </section>
              </div>
            </>
          )}
          {page === "sources" && (
            <>
              <PageHeader
                eyebrow="SOURCES"
                title={t("来源", "Sources")}
                description={t(
                  "集中维护上游与手工节点，让方案专注于如何使用。",
                  "Maintain subscriptions and manual nodes independently of profiles.",
                )}
                action={
                  <Button
                    primary
                    onClick={() =>
                      sourceTab === "subscriptions" ? showSource() : showNode()
                    }
                  >
                    <Plus size={16} />
                    {sourceTab === "subscriptions"
                      ? t("添加订阅", "Add subscription")
                      : t("添加节点", "Add node")}
                  </Button>
                }
              />
              <div className="toolbar">
                <div className="tabs">
                  <button
                    className={sourceTab === "subscriptions" ? "selected" : ""}
                    onClick={() => setSourceTab("subscriptions")}
                  >
                    {t("订阅来源", "Subscriptions")}{" "}
                    <span>{state.sources.length}</span>
                  </button>
                  <button
                    className={sourceTab === "nodes" ? "selected" : ""}
                    onClick={() => setSourceTab("nodes")}
                  >
                    {t("手工节点", "Manual nodes")}{" "}
                    <span>{state.nodes.length}</span>
                  </button>
                </div>
                <div className="search">
                  <Search size={15} />
                  <input
                    aria-label={t("搜索来源", "Search sources")}
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder={t("搜索名称…", "Search names…")}
                  />
                </div>
              </div>
              <section className="panel table-panel">
                {sourceTab === "subscriptions" ? (
                  state.sources.length === 0 ? (
                    <Empty
                      icon={<Network />}
                      title={t("从第一个来源开始", "Start with a source")}
                      text={t(
                        "添加订阅地址，或导入 provider YAML / ss:// 列表。",
                        "Add a subscription URL or import provider YAML / ss:// links.",
                      )}
                      action={
                        <Button onClick={() => showSource()}>
                          <Plus size={15} />
                          {t("添加订阅", "Add subscription")}
                        </Button>
                      }
                    />
                  ) : (
                    <table>
                      <thead>
                        <tr>
                          <th>{t("名称与标签", "Name & tags")}</th>
                          <th>{t("节点", "Nodes")}</th>
                          <th>{t("来源状态", "Source status")}</th>
                          <th>{t("最近成功", "Last success")}</th>
                          <th>{t("操作", "Actions")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {state.sources
                          .filter((v) => filter(v.name))
                          .map((v) => (
                            <tr key={v.id}>
                              <td>
                                <div className="table-name">
                                  <span className="item-icon">
                                    <Network size={17} />
                                  </span>
                                  <div>
                                    <strong>{v.name}</strong>
                                    <small>
                                      {v.kind === "http"
                                        ? "HTTP"
                                        : t("本地导入", "Local import")}
                                      {(v.tags || []).map((tag) => (
                                        <span className="tag" key={tag}>
                                          {tag}
                                        </span>
                                      ))}
                                    </small>
                                  </div>
                                </div>
                              </td>
                              <td>
                                <button
                                  className="link-button"
                                  onClick={() => void showNodes(v)}
                                >
                                  {v.nodeCount} {t("个节点", "nodes")}
                                </button>
                              </td>
                              <td>
                                <span
                                  className={`pill ${!v.enabled ? "subtle" : v.lastError ? "warning" : v.nodeCount ? "success" : "subtle"}`}
                                >
                                  {!v.enabled
                                    ? t("已停用", "Disabled")
                                    : v.lastError
                                      ? t(
                                          "使用旧缓存 / 待更新",
                                          "Cached / refresh needed",
                                        )
                                      : v.nodeCount
                                        ? t("已有有效快照", "Valid snapshot")
                                        : t("等待更新", "Awaiting refresh")}
                                </span>
                              </td>
                              <td className="muted">{time(v.lastSuccess)}</td>
                              <td>
                                <div className="row-actions">
                                  {v.kind === "http" && (
                                    <Button
                                      disabled={busy || !v.enabled}
                                      onClick={() =>
                                        void run(
                                          () =>
                                            api(
                                              `sources/${v.id}/refresh`,
                                              "POST",
                                              { revision: v.revision },
                                            ),
                                          t(
                                            "来源已更新；重新发布方案后设备刷新可获取新配置",
                                            "Source refreshed. Publish the profile, then refresh the client.",
                                          ),
                                        )
                                      }
                                    >
                                      <RefreshCw size={14} />
                                      {t("更新", "Refresh")}
                                    </Button>
                                  )}
                                  <IconButton
                                    label={t("编辑来源", "Edit source")}
                                    onClick={() => showSource(v)}
                                  >
                                    <Pencil size={15} />
                                  </IconButton>
                                  <IconButton
                                    label={t("删除来源", "Delete source")}
                                    onClick={() => remove("sources", v)}
                                  >
                                    <Trash2 size={15} />
                                  </IconButton>
                                </div>
                              </td>
                            </tr>
                          ))}
                      </tbody>
                    </table>
                  )
                ) : state.nodes.length === 0 ? (
                  <Empty
                    icon={<Server />}
                    title={t("独立维护的手工节点", "Your manual nodes")}
                    text={t(
                      "通过表单创建 SS 节点，或从订阅节点复制。",
                      "Create SS nodes with forms, or copy from a subscription.",
                    )}
                    action={
                      <Button onClick={() => showNode()}>
                        <Plus size={15} />
                        {t("添加节点", "Add node")}
                      </Button>
                    }
                  />
                ) : (
                  <table>
                    <thead>
                      <tr>
                        <th>{t("节点", "Node")}</th>
                        <th>{t("协议", "Protocol")}</th>
                        <th>{t("状态", "Status")}</th>
                        <th>{t("操作", "Actions")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {state.nodes
                        .filter((v) => filter(v.node.name))
                        .map((v) => (
                          <tr key={v.id}>
                            <td>
                              <strong>{v.node.name}</strong>
                              <small>
                                {v.node.server}:{v.node.port}
                              </small>
                            </td>
                            <td>
                              <span className="pill">
                                {v.node.type.toUpperCase()}
                              </span>
                            </td>
                            <td>
                              {v.enabled
                                ? t("启用", "Enabled")
                                : t("停用", "Disabled")}
                            </td>
                            <td>
                              <div className="row-actions">
                                <Button onClick={() => showNode(v)}>
                                  <Pencil size={14} />
                                  {t("编辑", "Edit")}
                                </Button>
                                <IconButton
                                  label={t("删除节点", "Delete node")}
                                  onClick={() => remove("nodes", v)}
                                >
                                  <Trash2 size={15} />
                                </IconButton>
                              </div>
                            </td>
                          </tr>
                        ))}
                    </tbody>
                  </table>
                )}
              </section>
            </>
          )}
          {page === "profiles" && (
            <>
              <PageHeader
                eyebrow="CONFIGURATION PROFILES"
                title={t("配置方案", "Profiles")}
                description={t(
                  "组、规则与客户端设置，一份方案独立维护。",
                  "Maintain groups, routing rules, and client settings in one profile.",
                )}
                action={
                  <Button primary disabled={unsaved} onClick={showProfile}>
                    <Plus size={16} />
                    {t("新建方案", "New profile")}
                  </Button>
                }
              />
              {state.profiles.length === 0 ? (
                <section className="panel">
                  <Empty
                    icon={<Layers3 />}
                    title={t("把来源组织成方案", "Turn sources into a profile")}
                    text={t(
                      "每份方案拥有独立的策略组、规则和设置。",
                      "Each profile has independent groups, rules, and settings.",
                    )}
                    action={
                      <Button onClick={showProfile}>
                        <Plus size={15} />
                        {t("新建方案", "New profile")}
                      </Button>
                    }
                  />
                </section>
              ) : (
                <>
                  <div className="profile-picker">
                    {state.profiles.map((p) => (
                      <button
                        className={
                          activeProfile?.id === p.id
                            ? "profile-chip selected"
                            : "profile-chip"
                        }
                        key={p.id}
                        onClick={() => {
                          if (
                            !unsaved ||
                            confirm(
                              t(
                                "丢弃未保存修改并切换方案？",
                                "Discard unsaved changes and switch profiles?",
                              ),
                            )
                          ) {
                            setUnsaved(false);
                            setSelectedProfile(p.id);
                          }
                        }}
                      >
                        <Layers3 size={16} />
                        <strong>{p.name}</strong>
                        <span
                          className="dot"
                          data-active={!!state.active[p.id]}
                        />
                      </button>
                    ))}
                  </div>
                  {activeProfile && (
                    <ProfileEditor
                      key={activeProfile.id}
                      profile={activeProfile}
                      state={state}
                      busy={busy}
                      t={t}
                      time={time}
                      onRun={run}
                      onDelete={() => remove("profiles", activeProfile)}
                      onDirtyChange={setUnsaved}
                    />
                  )}
                </>
              )}
            </>
          )}
          {page === "devices" && (
            <>
              <PageHeader
                eyebrow="YOUR DEVICES"
                title={t("设备", "Devices")}
                description={t(
                  "独立链接，明确归属。切换方案无需更换订阅地址。",
                  "Independent links and clear ownership. Switch profiles without changing the URL.",
                )}
                action={
                  <Button
                    primary
                    disabled={!Object.keys(state.active).length}
                    onClick={() => showDevice()}
                  >
                    <Plus size={16} />
                    {t("添加设备", "Add device")}
                  </Button>
                }
              />
              <div className="notice inline">
                <CircleHelp size={17} />
                {t(
                  "最近下载不代表客户端已应用。吊销可阻止后续下载，不能收回已下载的代理凭据。",
                  "A download does not prove client application. Revocation blocks future downloads, not previously downloaded credentials.",
                )}
              </div>
              <section className="panel table-panel">
                {state.devices.length === 0 ? (
                  <Empty
                    icon={<Monitor />}
                    title={t(
                      "为每台设备生成专属链接",
                      "A dedicated link for each device",
                    )}
                    text={
                      Object.keys(state.active).length
                        ? t(
                            "绑定已发布方案，即可复制链接或导入 Clash Verge。",
                            "Bind a published profile, then copy the link or import into Clash Verge.",
                          )
                        : t(
                            "请先创建并发布一份配置方案。",
                            "Create and publish a profile first.",
                          )
                    }
                  />
                ) : (
                  <table>
                    <thead>
                      <tr>
                        <th>{t("设备", "Device")}</th>
                        <th>{t("绑定方案", "Profile")}</th>
                        <th>{t("授权状态", "Authorization")}</th>
                        <th>{t("最近下载", "Last download")}</th>
                        <th>{t("操作", "Actions")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {state.devices.map((d) => (
                        <tr key={d.id}>
                          <td>
                            <div className="table-name">
                              <span className="item-icon">
                                <Monitor size={17} />
                              </span>
                              <div>
                                <strong>{d.name}</strong>
                                <small>
                                  {d.expiresAt
                                    ? `${t("到期", "Expires")}: ${time(d.expiresAt)}`
                                    : t("长期有效", "No expiry")}
                                </small>
                              </div>
                            </div>
                          </td>
                          <td>
                            {
                              state.profiles.find((p) => p.id === d.profileId)
                                ?.name
                            }
                          </td>
                          <td>
                            <span
                              className={`pill ${d.state === "active" ? (d.expiresAt && Date.parse(d.expiresAt) <= Date.now() ? "warning" : "success") : d.state === "paused" ? "warning" : "subtle"}`}
                            >
                              {d.state === "active"
                                ? d.expiresAt &&
                                  Date.parse(d.expiresAt) <= Date.now()
                                  ? t("已到期", "Expired")
                                  : t("已授权", "Authorized")
                                : d.state === "paused"
                                  ? t("已暂停", "Paused")
                                  : t("已吊销", "Revoked")}
                            </span>
                          </td>
                          <td className="muted">{time(state.seen[d.id])}</td>
                          <td>
                            <div className="row-actions">
                              <Button
                                disabled={d.state === "revoked"}
                                onClick={() => void showLink(d)}
                              >
                                <Link2 size={14} />
                                {t("订阅链接", "Link")}
                              </Button>
                              <details className="action-menu">
                                <summary
                                  aria-label={t("设备操作", "Device actions")}
                                >
                                  ···
                                </summary>
                                <div>
                                  <button
                                    disabled={d.state === "revoked"}
                                    onClick={() => showDevice(d)}
                                  >
                                    {t(
                                      "编辑 / 切换方案",
                                      "Edit / switch profile",
                                    )}
                                  </button>
                                  <button
                                    disabled={d.state === "revoked"}
                                    onClick={() =>
                                      void run(
                                        () =>
                                          api(`devices/${d.id}`, "PUT", {
                                            id: d.id,
                                            revision: d.revision,
                                            name: d.name,
                                            profileId: d.profileId,
                                            state:
                                              d.state === "paused"
                                                ? "active"
                                                : "paused",
                                            expiresAt: d.expiresAt || null,
                                          }),
                                        t(
                                          "设备状态已更新",
                                          "Device state updated",
                                        ),
                                      )
                                    }
                                  >
                                    {d.state === "paused"
                                      ? t("恢复", "Resume")
                                      : t("暂停", "Pause")}
                                  </button>
                                  <button
                                    disabled={d.state === "revoked"}
                                    onClick={() => {
                                      if (
                                        confirm(
                                          t(
                                            "轮换后旧链接立即失效，需要重新导入新链接。",
                                            "The old link will stop working immediately. Import the new link again.",
                                          ),
                                        )
                                      )
                                        void run(
                                          () =>
                                            api(
                                              `devices/${d.id}/rotate`,
                                              "POST",
                                              { revision: d.revision },
                                            ),
                                          t(
                                            "已轮换，请重新获取链接",
                                            "Rotated. Retrieve the new link.",
                                          ),
                                        );
                                    }}
                                  >
                                    {t("轮换凭据", "Rotate credential")}
                                  </button>
                                  <button
                                    className="danger-text"
                                    disabled={d.state === "revoked"}
                                    onClick={() => {
                                      if (
                                        confirm(
                                          t(
                                            "确认永久吊销此设备链接？",
                                            "Permanently revoke this device link?",
                                          ),
                                        )
                                      )
                                        void run(
                                          () =>
                                            api(`devices/${d.id}`, "PUT", {
                                              id: d.id,
                                              revision: d.revision,
                                              name: d.name,
                                              profileId: d.profileId,
                                              state: "revoked",
                                              expiresAt: d.expiresAt || null,
                                            }),
                                          t("设备已吊销", "Device revoked"),
                                        );
                                    }}
                                  >
                                    {t("吊销", "Revoke")}
                                  </button>
                                  <button
                                    className="danger-text"
                                    onClick={() => remove("devices", d)}
                                  >
                                    {t("删除记录", "Delete record")}
                                  </button>
                                </div>
                              </details>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </section>
            </>
          )}
          {page === "system" && (
            <>
              <PageHeader
                eyebrow="SYSTEM"
                title={t("系统", "System")}
                description={t(
                  "了解当前实例的边界与验证状态。",
                  "Understand this instance’s scope and validation status.",
                )}
              />
              <div className="settings-grid">
                <section className="panel padded">
                  <ShieldCheck className="accent" size={26} />
                  <h2>{t("身份与存储", "Identity and storage")}</h2>
                  <dl>
                    <dt>{t("认证", "Authentication")}</dt>
                    <dd>OIDC · PKCE · nonce</dd>
                    <dt>{t("当前角色", "Current role")}</dt>
                    <dd>
                      {t(
                        "管理员（明确 sub 准入）",
                        "Administrator (explicit sub allowlist)",
                      )}
                    </dd>
                    <dt>{t("业务状态", "Business state")}</dt>
                    <dd>
                      {t(
                        "SQLite · 加密对象与发布快照",
                        "SQLite · encrypted records and snapshots",
                      )}
                    </dd>
                  </dl>
                </section>
                <section className="panel padded">
                  <FileCheck2 className="accent" size={26} />
                  <h2>{t("校验与能力", "Validation and scope")}</h2>
                  <dl>
                    <dt>{t("校验器", "Validator")}</dt>
                    <dd className="mono wrap">{state.validator}</dd>
                    <dt>{t("节点协议", "Node protocols")}</dt>
                    <dd>Shadowsocks · AEAD</dd>
                    <dt>{t("自动发布", "Automatic publishing")}</dt>
                    <dd>
                      {t(
                        "此闭环版本手动发布；来源更新不自动发布草稿",
                        "Manual publishing in this slice; source refreshes never publish drafts",
                      )}
                    </dd>
                  </dl>
                </section>
              </div>
              <div className="notice inline">
                <TriangleAlert size={18} />
                {t(
                  "这是开发预览：其余协议、完整 DNS/规则集、普通使用者授权、生产部署和真实 Pocket ID/Clash Verge 验收尚未完成。",
                  "Development preview: other protocols, full DNS/rulesets, member access, production deployment, and real Pocket ID/Clash Verge acceptance remain pending.",
                )}
              </div>
            </>
          )}
        </main>
        <footer className="footer">
          Subweft <span>·</span>
          {t("来源有序，策略清晰。", "Sources organized. Policies clear.")}
          <span className="footer-right">
            {t(
              "开发预览，尚未生产验收",
              "Development preview, not production accepted",
            )}
          </span>
        </footer>
      </div>
      {busy && (
        <div className="busy" role="status">
          <LoaderCircle size={16} className="spin" />
          {t("正在处理…", "Working…")}
        </div>
      )}
      {toast && (
        <div className="toast" role="status">
          <CheckCircle2 size={18} />
          {toast}
        </div>
      )}
      <dialog
        aria-labelledby="subweft-dialog-title"
        ref={modalRef}
        onCancel={() => setModal(null)}
        onClose={() => setModal(null)}
      >
        <button
          className="dialog-close icon-button"
          aria-label={t("关闭对话框", "Close dialog")}
          onClick={() => setModal(null)}
        >
          <X size={19} />
        </button>
        {error && (
          <div role="alert" className="alert error modal-error">
            {error}
          </div>
        )}
        <fieldset disabled={busy} className="dialog-content">
          {modal}
        </fieldset>
      </dialog>
    </div>
  );
}

function Brand() {
  return (
    <div className="brand">
      <span className="brand-mark">
        <span />
        <span />
        <span />
      </span>
      <span>
        Subweft<span className="brand-dot">.</span>
      </span>
    </div>
  );
}
function Button({
  children,
  primary = false,
  danger = false,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  primary?: boolean;
  danger?: boolean;
}) {
  return (
    <button
      type="button"
      className={`button ${primary ? "primary" : ""} ${danger ? "danger-text" : ""}`}
      {...props}
    >
      {children}
    </button>
  );
}
function IconButton({
  label,
  children,
  onClick,
}: {
  label: string;
  children: ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className="icon-button"
      aria-label={label}
      title={label}
      onClick={onClick}
    >
      {children}
    </button>
  );
}
function PageHeader({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow: string;
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="page-header">
      <div>
        <span className="eyebrow">{eyebrow}</span>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {action}
    </div>
  );
}
function Empty({
  icon,
  title,
  text,
  action,
}: {
  icon: ReactNode;
  title: string;
  text: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">{icon}</div>
      <h2>{title}</h2>
      <p>{text}</p>
      {action}
    </div>
  );
}
function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  const id = React.useId();
  const control = React.isValidElement(children)
    ? React.cloneElement(
        children as React.ReactElement<Record<string, unknown>>,
        { id, "aria-describedby": hint ? id + "-hint" : undefined },
      )
    : children;
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {control}
      {hint && <small id={id + "-hint"}>{hint}</small>}
    </div>
  );
}
function CheckField({
  label,
  checked,
  onChange,
  disabled = false,
}: {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <label className="check-field">
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span>{label}</span>
    </label>
  );
}
function ModalTitle({ title }: { title: string }) {
  return (
    <div className="modal-title">
      <span className="eyebrow">SUBWEFT WORKSPACE</span>
      <h2 id="subweft-dialog-title">{title}</h2>
    </div>
  );
}
type T = (zh: string, en: string) => string;
function NameForm({
  title,
  initial = "",
  t,
  onSave,
}: {
  title: string;
  initial?: string;
  t: T;
  onSave: (name: string) => unknown;
}) {
  const [name, setName] = useState(initial);
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        onSave(name.trim());
      }}
    >
      <ModalTitle title={title} />
      <Field label={t("名称", "Name")}>
        <input
          autoFocus
          required
          maxLength={160}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      <div className="form-footer">
        <Button type="submit" primary>
          <Plus size={15} />
          {t("创建", "Create")}
        </Button>
      </div>
    </form>
  );
}
function SourceForm({
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
  const [url, setURL] = useState("");
  const [content, setContent] = useState("");
  const [tags, setTags] = useState(value?.tags?.join(", ") || "");
  const [interval, setInterval] = useState(value?.interval || 3600);
  const [enabled, setEnabled] = useState(value?.enabled ?? true);
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
          tags: split(tags),
          interval,
          enabled,
        });
      }}
    >
      <ModalTitle
        title={
          value
            ? t("编辑来源", "Edit source")
            : t("添加订阅来源", "Add subscription source")
        }
      />
      <p className="muted">
        {t(
          "此版本支持仅含 proxies 的 SS provider YAML，或 ss:// 列表。",
          "This slice supports SS provider YAML containing only proxies, or ss:// lists.",
        )}
      </p>
      <Field
        label={t("来源名称", "Source name")}
        hint={t(
          "节点输出会带有此来源前缀。",
          "Output node names include this source prefix.",
        )}
      >
        <input
          autoFocus
          required
          value={name}
          maxLength={80}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      <Field label={t("来源方式", "Source type")}>
        <select
          value={kind}
          onChange={(e) => setKind(e.target.value as "http" | "local")}
        >
          <option value="http">HTTP {t("订阅", "subscription")}</option>
          <option value="local">
            {t("本地内容导入", "Local content import")}
          </option>
        </select>
      </Field>
      {kind === "http" ? (
        <Field
          label={t("订阅地址", "Subscription URL")}
          hint={
            value
              ? t(
                  "凭据默认隐藏；留空保留原地址。",
                  "Hidden by default. Leave blank to keep the current URL.",
                )
              : t(
                  "凭据由服务保存，不下发原地址。",
                  "Credentials stay server-side; the upstream URL is not distributed.",
                )
          }
        >
          <input
            type="url"
            required={!value || value.kind !== kind}
            value={url}
            placeholder="https://example.com/subscription"
            onChange={(e) => setURL(e.target.value)}
          />
        </Field>
      ) : (
        <>
          <Field
            label={t("导入内容", "Import content")}
            hint={
              value
                ? t(
                    "留空保留原内容。这里用于导入来源，不用于编辑生成配置。",
                    "Leave blank to keep existing input. This imports a source; it does not edit generated configuration.",
                  )
                : t(
                    "最大 2 MiB；未知字段会明确拒绝。",
                    "Maximum 2 MiB. Unknown fields are rejected.",
                  )
            }
          >
            <textarea
              rows={6}
              required={!value || value.kind !== kind}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              spellCheck={false}
            />
          </Field>
          <Field label={t("或选择文件", "Or choose a file")}>
            <input
              type="file"
              accept=".yaml,.yml,.txt"
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file && file.size <= 2 * 1024 * 1024)
                  void file.text().then(setContent);
              }}
            />
          </Field>
        </>
      )}
      <div className="form-grid">
        <Field label={t("标签，以逗号分隔", "Tags, comma separated")}>
          <input value={tags} onChange={(e) => setTags(e.target.value)} />
        </Field>
        <Field label={t("抓取间隔（秒）", "Refresh interval (seconds)")}>
          <input
            type="number"
            min={60}
            max={604800}
            required
            value={interval}
            onChange={(e) => setInterval(Number(e.target.value))}
          />
        </Field>
      </div>
      <CheckField
        label={t("启用来源", "Enable source")}
        checked={enabled}
        onChange={setEnabled}
      />
      <div className="form-footer">
        <Button primary type="submit">
          {t("保存来源", "Save source")}
        </Button>
      </div>
    </form>
  );
}
function NodeForm({
  value,
  t,
  onSave,
}: {
  value?: ManualNode;
  t: T;
  onSave: (v: unknown) => unknown;
}) {
  const [node, setNode] = useState<Node>(
    value?.node || {
      name: "",
      type: "ss",
      server: "",
      port: 443,
      cipher: "aes-128-gcm",
      password: "",
      udp: true,
    },
  );
  const [enabled, setEnabled] = useState(value?.enabled ?? true);
  const change = <K extends keyof Node>(key: K, v: Node[K]) =>
    setNode({ ...node, [key]: v });
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        onSave({
          id: value?.id || "",
          revision: value?.revision || 0,
          node,
          enabled,
        });
      }}
    >
      <ModalTitle
        title={t("Shadowsocks 手工节点", "Manual Shadowsocks node")}
      />
      <Field label={t("名称", "Name")}>
        <input
          autoFocus
          required
          value={node.name}
          onChange={(e) => change("name", e.target.value)}
        />
      </Field>
      <div className="form-grid">
        <Field label={t("服务器", "Server")}>
          <input
            required
            value={node.server}
            onChange={(e) => change("server", e.target.value)}
          />
        </Field>
        <Field label={t("端口", "Port")}>
          <input
            type="number"
            min={1}
            max={65535}
            required
            value={node.port}
            onChange={(e) => change("port", Number(e.target.value))}
          />
        </Field>
      </div>
      <Field label={t("加密方式", "Cipher")}>
        <select
          value={node.cipher}
          onChange={(e) => change("cipher", e.target.value)}
        >
          {["aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305"].map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
      </Field>
      <Field
        label={t("密码", "Password")}
        hint={
          value
            ? t(
                "留空保留现有密码。",
                "Leave blank to retain the existing password.",
              )
            : undefined
        }
      >
        <input
          type="password"
          autoComplete="new-password"
          required={!value}
          value={node.password}
          onChange={(e) => change("password", e.target.value)}
        />
      </Field>
      <CheckField
        label="UDP"
        checked={node.udp}
        onChange={(v) => change("udp", v)}
      />
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
function DeviceForm({
  value,
  profiles,
  t,
  onSave,
}: {
  value?: Device;
  profiles: Profile[];
  t: T;
  onSave: (v: unknown) => unknown;
}) {
  const [name, setName] = useState(value?.name || "");
  const [profileId, setProfile] = useState(
    value?.profileId || profiles[0]?.id || "",
  );
  const [expires, setExpires] = useState(
    value?.expiresAt
      ? new Date(
          new Date(value.expiresAt).getTime() -
            new Date().getTimezoneOffset() * 60000,
        )
          .toISOString()
          .slice(0, 16)
      : "",
  );
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        onSave({
          id: value?.id || "",
          revision: value?.revision || 0,
          name: name.trim(),
          profileId,
          state: value?.state || "active",
          expiresAt: expires ? new Date(expires).toISOString() : null,
        });
      }}
    >
      <ModalTitle
        title={
          value ? t("编辑设备", "Edit device") : t("添加设备", "Add device")
        }
      />
      <Field label={t("设备名称", "Device name")}>
        <input
          autoFocus
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      <Field
        label={t("绑定方案", "Profile")}
        hint={t(
          "切换方案保持订阅 URL 不变，客户端刷新后生效。",
          "Switching preserves the URL; the client applies it after refreshing.",
        )}
      >
        <select
          required
          value={profileId}
          onChange={(e) => setProfile(e.target.value)}
        >
          {profiles.map((p) => (
            <option value={p.id} key={p.id}>
              {p.name}
            </option>
          ))}
        </select>
      </Field>
      <Field
        label={t("到期时间（可选）", "Expiration (optional)")}
        hint={t("留空表示长期有效。", "Leave empty for no expiration.")}
      >
        <input
          type="datetime-local"
          value={expires}
          onChange={(e) => setExpires(e.target.value)}
        />
      </Field>
      <div className="form-footer">
        <Button primary type="submit">
          {t("保存设备", "Save device")}
        </Button>
      </div>
    </form>
  );
}

type Run = (
  work: () => Promise<unknown>,
  success?: string,
  close?: boolean,
) => Promise<void>;
function ProfileEditor({
  profile,
  state,
  busy,
  t,
  time,
  onRun,
  onDelete,
  onDirtyChange,
}: {
  profile: Profile;
  state: State;
  busy: boolean;
  t: T;
  time: (v?: string) => string;
  onRun: Run;
  onDelete: () => void;
  onDirtyChange: (v: boolean) => void;
}) {
  const [draft, setDraft] = useState<Profile>(copy(profile) as Profile);
  const [tab, setTab] = useState("groups");
  const [allowStale, setAllowStale] = useState(false);
  const [preview, setPreview] = useState<{
    config: string;
    members: Record<string, string[]>;
    stale: string[];
  } | null>(null);
  useEffect(() => {
    setDraft(copy(profile) as Profile);
    setPreview(null);
  }, [profile.id, profile.revision]);
  const dirty = JSON.stringify(draft) !== JSON.stringify(profile);
  const releases = state.releases
    .filter((r) => r.profileId === profile.id)
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  const active = releases.find((r) => r.id === state.active[profile.id]);
  const updateGroup = (i: number, value: Group) => {
    const oldName = draft.groups[i].name;
    const rename = (name: string) => (name === oldName ? value.name : name);
    setDraft({
      ...draft,
      groups: draft.groups.map((g, index) => ({
        ...(i === index ? value : g),
        groups: (i === index ? value : g).groups.map(rename),
      })),
      rules: draft.rules.map((rule) => ({
        ...rule,
        target: rename(rule.target),
      })),
    });
  };
  const setRule = (i: number, value: Rule) =>
    setDraft({
      ...draft,
      rules: draft.rules.map((r, index) => (i === index ? value : r)),
    });
  const moveRule = (i: number, offset: number) => {
    const rules = [...draft.rules];
    [rules[i], rules[i + offset]] = [rules[i + offset], rules[i]];
    setDraft({ ...draft, rules });
  };
  useEffect(() => {
    onDirtyChange(dirty);
    return () => onDirtyChange(false);
  }, [dirty, onDirtyChange]);
  useEffect(() => {
    const guard = (event: BeforeUnloadEvent) => {
      if (dirty) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [dirty]);
  const options = [...draft.groups.map((g) => g.name), "DIRECT", "REJECT"];
  return (
    <section className="panel profile-editor">
      <div className="editor-header">
        <div>
          <span className="pill subtle">
            {active?.profileRevision === profile.revision
              ? t("草稿与发布一致", "Draft matches release")
              : t("草稿待发布", "Unpublished draft")}
          </span>
          <p className="muted">
            {t("草稿修订", "Draft revision")} {profile.revision} ·{" "}
            {active
              ? `${t("发布修订", "Published revision")} ${active.profileRevision}`
              : t("尚未发布", "Not published")}
          </p>
        </div>
        <div className="actions">
          <Button
            disabled={busy || !dirty}
            onClick={() =>
              void onRun(
                () => api(`profiles/${profile.id}`, "PUT", draft),
                t("草稿已保存", "Draft saved"),
              )
            }
          >
            <Check size={15} />
            {t("保存草稿", "Save draft")}
          </Button>
          <Button
            disabled={busy || dirty}
            onClick={() =>
              void onRun(async () => {
                const value = await api<{
                  config: string;
                  members: Record<string, string[]>;
                  stale: string[];
                }>(`profiles/${profile.id}/preview`, "POST", {
                  revision: profile.revision,
                  allowStale,
                });
                setPreview(value);
                setTab("preview");
              })
            }
          >
            <Search size={15} />
            {t("预览", "Preview")}
          </Button>
          <Button
            primary
            disabled={busy || dirty}
            onClick={() =>
              void onRun(
                () =>
                  api(`profiles/${profile.id}/publish`, "POST", {
                    revision: profile.revision,
                    allowStale,
                  }),
                t("校验通过，方案已发布", "Validated and published"),
              )
            }
          >
            <FileCheck2 size={16} />
            {t("校验并发布", "Validate & publish")}
          </Button>
        </div>
      </div>
      <div className="editor-name">
        <Field label={t("方案名称", "Profile name")}>
          <input
            value={draft.name}
            maxLength={160}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
          />
        </Field>
        <div className="actions">
          <Button
            disabled={busy || dirty}
            onClick={() =>
              void onRun(
                () =>
                  api(`profiles/${profile.id}/copy`, "POST", {
                    revision: profile.revision,
                  }),
                t("方案已复制", "Profile copied"),
              )
            }
          >
            <Copy size={14} />
            {t("复制方案", "Copy")}
          </Button>
          <IconButton
            label={t("删除方案", "Delete profile")}
            onClick={onDelete}
          >
            <Trash2 size={15} />
          </IconButton>
        </div>
      </div>
      {dirty && (
        <div className="notice inline">
          {t(
            "有未保存的表单修改，请先保存草稿再预览或发布。",
            "Save your form changes before previewing or publishing.",
          )}
        </div>
      )}
      <div className="editor-tabs tabs">
        {[
          ["groups", t("策略组", "Policy groups")],
          ["rules", t("规则顺序", "Ordered rules")],
          ["settings", t("客户端设置", "Client settings")],
          ["preview", t("预览与版本", "Preview & revisions")],
        ].map(([key, label]) => (
          <button
            className={tab === key ? "selected" : ""}
            key={key}
            onClick={() => setTab(key)}
          >
            {label}
          </button>
        ))}
      </div>
      <div className="editor-body">
        {tab === "groups" && (
          <>
            <div className="section-intro">
              <div>
                <h3>
                  {t("明确每组使用哪些来源", "Choose what each group uses")}
                </h3>
                <p>
                  {t(
                    "来源、标签、节点与其他组可以组合；发布时会检查循环和空结果。",
                    "Combine sources, tags, nodes, and groups. Publishing checks for cycles and empty results.",
                  )}
                </p>
              </div>
              <Button
                onClick={() =>
                  setDraft({
                    ...draft,
                    groups: [
                      ...draft.groups,
                      emptyGroup(`Group${draft.groups.length + 1}`),
                    ],
                  })
                }
              >
                <Plus size={15} />
                {t("添加策略组", "Add group")}
              </Button>
            </div>
            {draft.groups.map((g, i) => (
              <GroupForm
                key={i}
                value={g}
                index={i}
                groups={draft.groups}
                state={state}
                t={t}
                onChange={(v) => updateGroup(i, v)}
                onDelete={() =>
                  setDraft({
                    ...draft,
                    groups: draft.groups.filter((_, index) => index !== i),
                  })
                }
              />
            ))}
          </>
        )}
        {tab === "rules" && (
          <>
            <div className="section-intro">
              <div>
                <h3>
                  {t(
                    "从上到下匹配，兜底放在最后",
                    "Match top to bottom; fallback last",
                  )}
                </h3>
                <p>
                  {t(
                    "支持域名与 IP 规则。当前闭环暂不包含规则集和 GEOIP。",
                    "Domain and IP rules are available. Rulesets and GEOIP are outside this slice.",
                  )}
                </p>
              </div>
              <Button
                onClick={() => {
                  const rules = [...draft.rules];
                  const at = rules.findIndex((r) => r.type === "MATCH");
                  rules.splice(at < 0 ? rules.length : at, 0, {
                    type: "DOMAIN-SUFFIX",
                    value: "",
                    target: options[0],
                    enabled: true,
                    noResolve: false,
                  });
                  setDraft({ ...draft, rules });
                }}
              >
                <Plus size={15} />
                {t("添加规则", "Add rule")}
              </Button>
            </div>
            <div className="rule-list">
              {draft.rules.map((r, i) => (
                <div className="rule-card" key={i}>
                  <div className="rule-number">
                    {String(i + 1).padStart(2, "0")}
                  </div>
                  <Field label={t("类型", "Type")}>
                    <select
                      value={r.type}
                      onChange={(e) =>
                        setRule(i, {
                          ...r,
                          type: e.target.value,
                          value: e.target.value === "MATCH" ? "" : r.value,
                          noResolve: false,
                        })
                      }
                    >
                      {[
                        "DOMAIN",
                        "DOMAIN-SUFFIX",
                        "DOMAIN-KEYWORD",
                        "IP-CIDR",
                        "IP-CIDR6",
                        "MATCH",
                      ].map((type) => (
                        <option key={type}>{type}</option>
                      ))}
                    </select>
                  </Field>
                  <Field label={t("匹配内容", "Match value")}>
                    <input
                      aria-label={`${t("匹配内容", "Match value")} ${i + 1}`}
                      disabled={r.type === "MATCH"}
                      value={r.value}
                      placeholder={
                        r.type === "MATCH"
                          ? t("所有剩余流量", "All remaining traffic")
                          : "example.com"
                      }
                      onChange={(e) =>
                        setRule(i, { ...r, value: e.target.value })
                      }
                    />
                  </Field>
                  <Field label={t("目标", "Target")}>
                    <select
                      value={r.target}
                      onChange={(e) =>
                        setRule(i, { ...r, target: e.target.value })
                      }
                    >
                      {options.map((o, index) => (
                        <option key={`${o}-${index}`}>{o}</option>
                      ))}
                    </select>
                  </Field>
                  <div className="rule-options">
                    <CheckField
                      label={t("启用", "Enabled")}
                      checked={r.enabled}
                      onChange={(enabled) => setRule(i, { ...r, enabled })}
                    />
                    {r.type.startsWith("IP-") && (
                      <CheckField
                        label="no-resolve"
                        checked={r.noResolve}
                        onChange={(noResolve) =>
                          setRule(i, { ...r, noResolve })
                        }
                      />
                    )}
                  </div>
                  <div className="rule-actions">
                    <button
                      className="icon-button"
                      disabled={i === 0}
                      aria-label={`${t("上移规则", "Move rule up")} ${i + 1}`}
                      onClick={() => moveRule(i, -1)}
                    >
                      <ArrowUp size={15} />
                    </button>
                    <button
                      className="icon-button"
                      disabled={i === draft.rules.length - 1}
                      aria-label={`${t("下移规则", "Move rule down")} ${i + 1}`}
                      onClick={() => moveRule(i, 1)}
                    >
                      <ArrowDown size={15} />
                    </button>
                    <IconButton
                      label={`${t("删除规则", "Delete rule")} ${i + 1}`}
                      onClick={() =>
                        setDraft({
                          ...draft,
                          rules: draft.rules.filter((_, index) => index !== i),
                        })
                      }
                    >
                      <Trash2 size={15} />
                    </IconButton>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
        {tab === "settings" && (
          <div className="settings-form">
            <h3>{t("客户端参数", "Client parameters")}</h3>
            <Field label={t("混合端口", "Mixed port")}>
              <input
                type="number"
                min={1}
                max={65535}
                value={draft.settings.mixedPort}
                onChange={(e) =>
                  setDraft({
                    ...draft,
                    settings: {
                      ...draft.settings,
                      mixedPort: Number(e.target.value),
                    },
                  })
                }
              />
            </Field>
            <CheckField
              label={t("允许局域网访问", "Allow LAN access")}
              checked={draft.settings.allowLan}
              onChange={(allowLan) =>
                setDraft({
                  ...draft,
                  settings: { ...draft.settings, allowLan },
                })
              }
            />
            <CheckField
              label="IPv6"
              checked={draft.settings.ipv6}
              onChange={(ipv6) =>
                setDraft({ ...draft, settings: { ...draft.settings, ipv6 } })
              }
            />
            <hr />
            <CheckField
              label={t("启用 DNS", "Enable DNS")}
              checked={draft.settings.dnsEnabled}
              onChange={(dnsEnabled) =>
                setDraft({
                  ...draft,
                  settings: { ...draft.settings, dnsEnabled },
                })
              }
            />
            {draft.settings.dnsEnabled && (
              <>
                <Field label={t("DNS 模式", "DNS mode")}>
                  <select
                    value={draft.settings.dnsMode}
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        settings: {
                          ...draft.settings,
                          dnsMode: e.target.value,
                        },
                      })
                    }
                  >
                    <option>fake-ip</option>
                    <option>redir-host</option>
                  </select>
                </Field>
                <Field
                  label={t("解析器（每行一个）", "Nameservers (one per line)")}
                >
                  <textarea
                    rows={4}
                    value={draft.settings.nameservers.join("\n")}
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        settings: {
                          ...draft.settings,
                          nameservers: e.target.value.split("\n"),
                        },
                      })
                    }
                  />
                </Field>
              </>
            )}
            <div className="notice inline">
              {t(
                "当前仅实现这组明确字段；完整 DNS 和其他高级字段后续按支持矩阵扩展。",
                "Only the listed fields are implemented. Full DNS and advanced fields follow the support matrix.",
              )}
            </div>
          </div>
        )}
        {tab === "preview" && (
          <>
            <div className="section-intro">
              <div>
                <h3>{t("只读预览", "Read-only preview")}</h3>
                <p>
                  {t(
                    "密码已遮蔽。实际发布使用完整配置并执行内核校验。",
                    "Passwords are redacted. Publishing validates the complete configuration.",
                  )}
                </p>
              </div>
            </div>
            {preview ? (
              <>
                <div className="member-preview">
                  {Object.entries(preview.members).map(([name, members]) => (
                    <div key={name}>
                      <strong>{name}</strong>
                      <span>
                        {members.length} {t("个成员", "members")}
                      </span>
                      <small>{members.join(" · ")}</small>
                    </div>
                  ))}
                </div>
                <pre className="config-preview">{preview.config}</pre>
              </>
            ) : (
              <div className="notice inline">
                {t(
                  "保存草稿后，点击顶部“预览”。",
                  "Save the draft, then use Preview above.",
                )}
              </div>
            )}
            <h3 className="history-title">
              {t("发布历史", "Release history")}
            </h3>
            {releases.length === 0 ? (
              <p className="muted">{t("尚无发布版本。", "No releases yet.")}</p>
            ) : (
              <div className="history-list">
                {releases.map((r) => (
                  <div key={r.id}>
                    <div>
                      <strong className="mono">{r.id.slice(0, 8)}</strong>
                      {state.active[profile.id] === r.id && (
                        <span className="pill success">
                          {t("当前", "Current")}
                        </span>
                      )}
                      <small>
                        {time(r.createdAt)} · {t("草稿修订", "Draft revision")}{" "}
                        {r.profileRevision} · SHA256 {r.digest.slice(0, 12)}
                      </small>
                    </div>
                    <Button
                      disabled={busy || state.active[profile.id] === r.id}
                      onClick={() => {
                        if (
                          confirm(
                            t(
                              "恢复此发布版本？当前草稿内容不变。",
                              "Restore this release? Draft content will remain unchanged.",
                            ),
                          )
                        )
                          void onRun(
                            () =>
                              api(`profiles/${profile.id}/rollback`, "POST", {
                                revision: profile.revision,
                                releaseId: r.id,
                              }),
                            t("发布版本已恢复", "Release restored"),
                          );
                      }}
                    >
                      <RotateCcw size={14} />
                      {t("恢复版本", "Restore")}
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>
      <div className="editor-footer">
        <CheckField
          label={t(
            "若有来源更新失败，确认使用其旧缓存",
            "Acknowledge stale cached sources if a refresh failed",
          )}
          checked={allowStale}
          onChange={setAllowStale}
        />
        <span>
          {t(
            "来源更新后需重新发布，设备刷新获取新版本",
            "Publish after source refreshes; devices update on client refresh",
          )}
        </span>
      </div>
    </section>
  );
}
function SourceNodeList({
  source,
  nodes,
  t,
  onCopy,
}: {
  source: Source;
  nodes: Array<Pick<Node, "name" | "type" | "server" | "port">>;
  t: T;
  onCopy: (node: Pick<Node, "name" | "type" | "server" | "port">) => void;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  const filtered = nodes.filter((node) =>
    node.name.toLowerCase().includes(query.toLowerCase()),
  );
  const pages = Math.max(1, Math.ceil(filtered.length / 50));
  return (
    <div>
      <ModalTitle title={source.name + " · " + t("节点", "Nodes")} />
      <p className="muted">
        {t(
          "仅展示非机密字段。需要修改协议参数时，可复制为手工节点。",
          "Only non-secret fields are shown. Copy a node to edit its protocol settings.",
        )}
      </p>
      <Field label={t("搜索节点", "Search nodes")}>
        <input
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
      </Field>
      <div className="node-list">
        {filtered.slice(page * 50, (page + 1) * 50).map((node) => (
          <div className="node-row" key={node.name}>
            <div>
              <strong>{node.name}</strong>
              <small>
                {node.type} · {node.server}:{node.port}
              </small>
            </div>
            <Button onClick={() => onCopy(node)}>
              <Copy size={14} />
              {t("复制", "Copy")}
            </Button>
          </div>
        ))}
      </div>
      <div className="node-pager">
        <Button disabled={page === 0} onClick={() => setPage(page - 1)}>
          {t("上一页", "Previous")}
        </Button>
        <span>
          {page + 1} / {pages} · {filtered.length}
        </span>
        <Button disabled={page + 1 >= pages} onClick={() => setPage(page + 1)}>
          {t("下一页", "Next")}
        </Button>
      </div>
    </div>
  );
}

function GroupForm({
  value: g,
  index,
  groups,
  state,
  t,
  onChange,
  onDelete,
}: {
  value: Group;
  index: number;
  groups: Group[];
  state: State;
  t: T;
  onChange: (g: Group) => void;
  onDelete: () => void;
}) {
  const set = <K extends keyof Group>(key: K, value: Group[K]) =>
    onChange({ ...g, [key]: value });
  const tags = [...new Set(state.sources.flatMap((s) => s.tags || []))];
  return (
    <div className="group-card">
      <div className="group-heading">
        <span className="group-index">
          {String(index + 1).padStart(2, "0")}
        </span>
        <strong>{g.name || t("未命名组", "Unnamed group")}</strong>
        <span className="pill">{g.type}</span>
        <IconButton
          label={`${t("删除策略组", "Delete group")} ${index + 1}`}
          onClick={onDelete}
        >
          <Trash2 size={15} />
        </IconButton>
      </div>
      <div className="form-grid">
        <Field label={t("组名称", "Group name")}>
          <input value={g.name} onChange={(e) => set("name", e.target.value)} />
        </Field>
        <Field label={t("选择策略", "Policy type")}>
          <select value={g.type} onChange={(e) => set("type", e.target.value)}>
            <option value="select">
              {t("手动选择", "Manual select")} · select
            </option>
            <option value="url-test">
              {t("自动测速", "URL test")} · url-test
            </option>
            <option value="fallback">
              {t("自动回退", "Fallback")} · fallback
            </option>
            <option value="load-balance">
              {t("负载均衡", "Load balance")} · load-balance
            </option>
          </select>
        </Field>
      </div>
      <div className="source-selector">
        <div className="selector-heading">
          <strong>{t("订阅来源", "Subscription sources")}</strong>
          <CheckField
            label={t("所有启用来源", "All enabled sources")}
            checked={g.allSources}
            onChange={(v) => set("allSources", v)}
          />
        </div>
        <div className="checkbox-grid">
          {state.sources.map((s) => (
            <CheckField
              key={s.id}
              label={`${s.name} (${s.nodeCount})${s.enabled ? "" : " · disabled"}`}
              checked={g.sourceIds.includes(s.id)}
              onChange={() => set("sourceIds", toggle(g.sourceIds, s.id))}
            />
          ))}
          {state.sources.length === 0 && (
            <span className="muted">
              {t("尚无来源，请先添加。", "Add a source first.")}
            </span>
          )}
        </div>
        {tags.length > 0 && (
          <div className="tag-selector">
            <span>{t("或动态匹配标签", "Or match tags dynamically")}</span>
            {tags.map((tag) => (
              <button
                key={tag}
                className={`tag-button ${g.tags.includes(tag) ? "selected" : ""}`}
                onClick={() => set("tags", toggle(g.tags, tag))}
              >
                {g.tags.includes(tag) && <Check size={12} />}
                {tag}
              </button>
            ))}
          </div>
        )}
      </div>
      {state.nodes.length > 0 && (
        <div className="source-selector">
          <strong>{t("手工节点", "Manual nodes")}</strong>
          <div className="checkbox-grid">
            {state.nodes.map((n) => (
              <CheckField
                key={n.id}
                label={n.node.name}
                checked={g.nodeIds.includes(n.id)}
                onChange={() => set("nodeIds", toggle(g.nodeIds, n.id))}
              />
            ))}
          </div>
        </div>
      )}
      <details className="advanced">
        <summary>
          {t("高级选择与筛选", "Advanced selection & filtering")}
        </summary>
        <div className="checkbox-grid">
          {groups
            .filter((_, i) => i !== index)
            .map((other, i) => (
              <CheckField
                key={`${other.name}-${i}`}
                label={other.name}
                checked={g.groups.includes(other.name)}
                onChange={() => set("groups", toggle(g.groups, other.name))}
              />
            ))}
          {["DIRECT", "REJECT"].map((name) => (
            <CheckField
              key={name}
              label={name}
              checked={g.builtins.includes(name)}
              onChange={() => set("builtins", toggle(g.builtins, name))}
            />
          ))}
        </div>
        <div className="form-grid">
          <Field
            label={t(
              "包含节点（正则，可选）",
              "Include nodes (regex, optional)",
            )}
          >
            <input
              value={g.filter}
              onChange={(e) => set("filter", e.target.value)}
            />
          </Field>
          <Field
            label={t(
              "排除节点（正则，可选）",
              "Exclude nodes (regex, optional)",
            )}
          >
            <input
              value={g.exclude}
              onChange={(e) => set("exclude", e.target.value)}
            />
          </Field>
        </div>
      </details>
      {g.type !== "select" && (
        <div className="test-settings">
          <Field label={t("测速地址", "Health-check URL")}>
            <input
              type="url"
              value={g.url}
              onChange={(e) => set("url", e.target.value)}
            />
          </Field>
          <div className="form-grid">
            <Field label={t("检查间隔（秒）", "Check interval (seconds)")}>
              <input
                type="number"
                min={30}
                max={86400}
                value={g.interval}
                onChange={(e) => set("interval", Number(e.target.value))}
              />
            </Field>
            {g.type === "url-test" && (
              <Field label={t("容差（毫秒）", "Tolerance (ms)")}>
                <input
                  type="number"
                  min={0}
                  value={g.tolerance}
                  onChange={(e) => set("tolerance", Number(e.target.value))}
                />
              </Field>
            )}
            {g.type === "load-balance" && (
              <Field label={t("分配策略", "Distribution strategy")}>
                <select
                  value={g.strategy}
                  onChange={(e) => set("strategy", e.target.value)}
                >
                  {["consistent-hashing", "round-robin", "sticky-sessions"].map(
                    (v) => (
                      <option key={v}>{v}</option>
                    ),
                  )}
                </select>
              </Field>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
