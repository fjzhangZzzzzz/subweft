import { ImportWizard } from "./import";
import { UsersPanel } from "./users";
import { RoutingWorkspace, PoolEditor } from "./routing";
import { ResourcesPanel } from "./resources";
import {
  Button,
  Field,
  CheckField,
  ModalTitle,
  FormErrorContext,
  ErrorFieldLink,
  EditorDialog,
} from "./ui";
import type { T } from "./ui";
import {
  useCatalog,
  ObjectForm,
  ValueForm,
  SecretControl,
} from "./schema-form";
import { NodeForm } from "./node-form";
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
import "./workspace.css";

type Page =
  | "overview"
  | "nodes"
  | "sources"
  | "profiles"
  | "devices"
  | "system"
  | "resources"
  | "users";
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
    (location.hash.slice(1) as Page) || "profiles",
  );
  const [state, setState] = useState<State | null>(null);
  const [loading, setLoading] = useState(true);
  const [login, setLogin] = useState(false);
  const [error, setError] = useState("");
  const [fieldError, setFieldError] = useState({ field: "", message: "" });
  const [busy, setBusy] = useState(false);
  const [unsaved, setUnsaved] = useState(false);
  const [toast, setToast] = useState("");
  const [modal, setModal] = useState<ReactNode>(null);
  const [modalDirty, setModalDirty] = useState(false);
  const closeModal = () => {
    if (
      modalDirty &&
      !confirm(t("放弃未保存的表单修改？", "Discard unsaved form changes?"))
    )
      return;
    setModal(null);
  };
  useEffect(() => setModalDirty(false), [modal]);
  useEffect(() => {
    const guard = (e: BeforeUnloadEvent) => {
      if (unsaved || modalDirty) {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [unsaved, modalDirty]);
  const [nodeSource, setNodeSource] = useState("");
  const sourceTab = page === "nodes" ? "nodes" : "subscriptions";
  const [query, setQuery] = useState("");
  const [selectedProfile, setSelectedProfile] = useState("");
  const modalRef = useRef<HTMLDialogElement>(null);
  const msg = (e: unknown) => {
    if (!(e instanceof APIError))
      return t("请求失败，请重试。", "Request failed. Please retry.");
    const messages: Record<string, [string, string]> = {
      invalid_name: [
        "名称不能为空、含逗号或首尾空白。",
        "Enter a name without commas or surrounding whitespace.",
      ],
      invalid_port: [
        "端口须为 1 至 65535 的整数。",
        "Use an integer port from 1 to 65535.",
      ],
      invalid_server: [
        "请填写有效的服务器名称或 IP 地址。",
        "Enter a server hostname or IP address.",
      ],
      duplicate_provider_name: [
        "订阅或规则集名称重复，请使用不同名称。",
        "Provider names must be unique.",
      ],
      pool_missing: [
        "订阅池不存在，请重新选择。",
        "Select an existing subscription pool.",
      ],
      invalid_pool_members: [
        "订阅池需要明确且不重复的订阅成员。",
        "Select unique subscription pool members.",
      ],
      invalid_field_type: [
        "字段类型不正确，请检查整数、布尔值和文本格式。",
        "Check field types: integers, booleans and strings must retain their types.",
      ],
      invalid_flow: [
        "Vision Flow 要求 TLS 和 TCP 传输。",
        "Vision Flow requires TLS and TCP transport.",
      ],
      invalid_websocket: [
        "WebSocket 路径须以 / 开头；请检查 Early Data 范围。",
        "Use a WebSocket path beginning with / and check Early Data bounds.",
      ],
      invalid_header: [
        "标头名称须为 HTTP token，值不能含换行。",
        "Use HTTP token header names and values without line breaks.",
      ],
      duplicate_header: [
        "标头名称不能重复，不区分大小写。",
        "Header names must be unique, ignoring case.",
      ],
      invalid_alpn: [
        "ALPN 最多 8 项，协议名称不能为空。",
        "Supply at most 8 nonempty ALPN protocol names.",
      ],
      invalid_grpc_service: [
        "gRPC 服务名过长或含有控制字符。",
        "The gRPC service name is too long or contains control characters.",
      ],
      unsupported_transport: [
        "请检查所选协议适用的传输方式。",
        "Check which transports apply to this protocol.",
      ],
      unsupported_cipher: [
        "所选加密方式不在当前协议的支持范围内。",
        "Select a cipher supported for this protocol.",
      ],
      unsupported_fingerprint: [
        "请选择当前支持的客户端指纹。",
        "Select a supported client fingerprint.",
      ],
      invalid_uuid: [
        "UUID 格式不正确，请填写标准 UUID。",
        "Enter a valid standard UUID.",
      ],
      credentials_pair_required: [
        "用户名和密码须同时填写或同时清除。",
        "Supply both username and password, or clear both.",
      ],
      tls_required: [
        "这些 TLS 参数要求先启用 TLS。",
        "Enable TLS before setting these options.",
      ],
      invalid_reality: [
        "检查 Reality 公钥、Short ID、SNI 和客户端指纹；须验证证书。",
        "Check the Reality key, Short ID, SNI and client fingerprint; certificate verification is required.",
      ],
      transport_options_mismatch: [
        "传输参数与所选方式不一致。",
        "Transport options do not match the selected transport.",
      ],
      unsupported_or_invalid_uri: [
        "代理 URI 含有不支持或无效的字段。请使用字段矩阵中的格式。",
        "The proxy URI contains unsupported or invalid fields. Check the field matrix.",
      ],
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
        "该协议尚未支持，请查看字段矩阵。",
        "This protocol is not supported yet. See the field matrix.",
      ],
      unsupported_root_field: [
        "请使用仅含 proxies 的节点 YAML，或支持的代理 URI 列表。",
        "Use 节点 YAML with only proxies, or a supported proxy URI list.",
      ],
      empty_group: [
        "策略组没有匹配成员，请选择订阅或明确的内置出口。",
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
        "资源仍被配置引用，请先解除引用。",
        "This resource is referenced by a profile.",
      ],
      published_profile_cannot_delete: [
        "已发布配置暂不支持删除。",
        "Published profiles cannot be deleted in this slice.",
      ],
      profile_not_published: [
        "先发布配置，再绑定设备。",
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
    const location = e.field
      .replace(
        /^groups\.(\d+)/,
        (_, n) => t("策略组 ", "Group ") + (Number(n) + 1),
      )
      .replace(
        /^rules\.(\d+)/,
        (_, n) => t("规则 ", "Rule ") + (Number(n) + 1),
      );
    return location ? `${text} (${location})` : text;
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
    if (state?.role === "user") {
      setPage("devices");
      location.hash = "devices";
    }
  }, [state?.role]);
  useEffect(() => {
    localStorage.setItem("subweft.lang", lang);
    document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";
  }, [lang]);
  useEffect(() => {
    localStorage.setItem("subweft.theme", theme);
    document.documentElement.dataset.theme = theme;
  }, [theme]);
  useEffect(() => {
    const onHash = () => {
      let next = (location.hash.slice(1) || "sources") as Page;
      if (
        ![
          "overview",
          "sources",
          "nodes",
          "profiles",
          "resources",
          "devices",
          "users",
          "system",
        ].includes(next)
      )
        next = "sources";
      if (state?.role === "user") next = "devices";
      if (next === page) return;
      if (
        (unsaved || modalDirty) &&
        !confirm(
          t("离开并放弃未保存修改？", "Leave and discard unsaved changes?"),
        )
      ) {
        history.replaceState(null, "", "#" + page);
        return;
      }
      setUnsaved(false);
      setPage(next);
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, [page, unsaved, modalDirty, state?.role]);
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
    if (next === page) return;
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
    setFieldError({ field: "", message: "" });
    try {
      await work();
      await reload();
      if (success) setToast(success);
      if (close) setModal(null);
    } catch (e) {
      setError(msg(e));
      setFieldError({
        field: e instanceof APIError ? e.field : "",
        message: msg(e),
      });
      if (!(e instanceof APIError) || e.message !== "conflict") {
        try {
          await reload();
        } catch {}
      }
    } finally {
      setBusy(false);
    }
  };
  const showSource = async (value?: Source) => {
    try {
      const editable = value
        ? await api<Source>(`sources/${value.id}/edit`)
        : undefined;
      setModal(
        <SourceForm
          key={value?.id || "new"}
          value={editable}
          t={t}
          onSave={(inValue) =>
            run(
              async () => {
                const saved = await api<Source>(
                  value ? `sources/${value.id}` : "sources",
                  value ? "PUT" : "POST",
                  inValue,
                );
                setModal(null);
                setModalDirty(false);
                if (saved.kind === "http" && saved.enabled)
                  await api(`sources/${saved.id}/refresh`, "POST", {
                    revision: saved.revision,
                  });
              },
              t("订阅已保存", "Source saved"),
              true,
            )
          }
        />,
      );
    } catch (e) {
      setError(msg(e));
    }
  };
  const showNode = async (value?: ManualNode) => {
    setBusy(true);
    setError("");
    setFieldError({ field: "", message: "" });
    try {
      const editable = value
        ? await api<ManualNode>(`nodes/${value.id}/edit`)
        : undefined;
      setModal(
        <NodeForm
          value={editable}
          protocols={state?.capabilities.protocols || []}
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
    } catch (e) {
      setError(msg(e));
    } finally {
      setBusy(false);
    }
  };
  const showProfile = () =>
    setModal(
      <NewProfileForm
        state={state!}
        t={t}
        onSave={(value) =>
          run(
            async () => {
              const profile = await api<Profile>("profiles", "POST", value);
              setSelectedProfile(profile.id);
              navigate("profiles");
            },
            t("配置草稿已创建", "Profile draft created"),
            true,
          )
        }
      />,
    );
  const showDevice = (value?: Device, selectedProfileId?: string) =>
    state &&
    setModal(
      <DeviceForm
        value={value}
        profiles={state.profiles.filter(
          (p) =>
            state.active[p.id] &&
            (!selectedProfileId || p.id === selectedProfileId),
        )}
        t={t}
        onSave={(inValue) =>
          run(
            async () => {
              const saved = await api<Device>(
                value ? `devices/${value.id}` : "devices",
                value ? "PUT" : "POST",
                inValue,
              );
              setModalDirty(false);
              if (value) setModal(null);
              else await showLink(saved);
            },
            t("设备已保存", "Device saved"),
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
        Array<
          Pick<Node, "name" | "type" | "server" | "port"> & {
            identity?: string;
            ambiguous?: boolean;
            override?: {
              identity: string;
              name: string;
              excluded: boolean;
              notes: string;
            };
          }
        >
      >(`sources/${source.id}/nodes`);
      setModal(
        <SourceNodeList
          onDirtyChange={setModalDirty}
          onChanged={() => void reload()}
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
        <DeviceLink
          device={device}
          url={url}
          t={t}
          onCopied={() => setToast(t("链接已复制", "Link copied"))}
        />,
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
    ["profiles", <Layers3 size={18} />, t("我的配置", "Configuration")],
    ...(state?.role === "user"
      ? ([
          ["devices", <Link2 size={18} />, t("订阅链接", "Subscription links")],
        ] as Array<[Page, ReactNode, string]>)
      : []),
    ["system", <Settings2 size={18} />, t("设置", "Settings")],
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
              "开发预览 · 结构化订阅与策略管理",
              "Development preview · structured subscriptions and policies",
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
          {nav
            .filter(([key]) => state.role === "admin" || key === "devices")
            .map(([key, icon, label]) => (
              <a
                href={`#${key}`}
                key={key}
                className={
                  page === key ||
                  (key === "system" &&
                    ["users", "resources", "overview"].includes(page))
                    ? "nav-item active"
                    : "nav-item"
                }
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
              <small>
                {state.role === "admin"
                  ? t("管理员", "Administrator")
                  : t("使用者", "User")}
              </small>
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
            {state.build?.version || "Subweft"}
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
              {t("开发预览", "Development preview")}
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
          {state.role === "admin" &&
            ["system", "users", "resources", "overview"].includes(page) && (
              <div className="tabs settings-nav">
                {(
                  [
                    ["system", t("通用", "General")],
                    ["resources", t("规则集与地理数据", "Rulesets & geodata")],
                    ["users", t("使用者", "Users")],
                    ["overview", t("运行概览", "Overview")],
                  ] as [Page, string][]
                ).map(([key, label]) => (
                  <button
                    key={key}
                    className={key === page ? "selected" : ""}
                    onClick={() => navigate(key)}
                  >
                    {label}
                  </button>
                ))}
              </div>
            )}
          {page === "overview" && state.role === "admin" && (
            <>
              <PageHeader
                eyebrow="YOUR CONFIGURATION WORKSPACE"
                title={t(
                  "配置有条理，使用更轻松。",
                  "Your policies, in good order.",
                )}
                description={t(
                  "从订阅到设备，每次修改都有清晰的去向。",
                  "From your sources to your devices, every change has a clear path.",
                )}
                action={
                  <Button primary onClick={() => showSource()}>
                    <Plus size={16} />
                    {t("添加订阅", "Add source")}
                  </Button>
                }
              />
              <div className="stats">
                {[
                  [
                    t("订阅", "Sources"),
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
                    t("已发布配置", "Published profiles"),
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
                        "订阅凭据由服务保管，节点按需复用。",
                        "Keep credentials on the server and reuse sources.",
                      ),
                      "sources",
                      state.sources.length + state.nodes.length > 0,
                    ],
                    [
                      t("编排策略组与规则", "Arrange groups and rules"),
                      t(
                        "选择订阅、设置策略，明确流量去向。",
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
                      "13 种协议、版本化资源与使用者授权；已提供完整人工验收流程。",
                      "13 protocols, versioned resources and member access; complete acceptance is in progress.",
                    )}
                  </div>
                </section>
              </div>
            </>
          )}
          {page === "users" && state.role === "admin" && (
            <UsersPanel
              values={state.users || []}
              issuer={state.issuer}
              profiles={state.profiles}
              t={t}
              request={api}
              onRun={run}
              busy={busy}
              onDirtyChange={setUnsaved}
            />
          )}
          {page === "resources" && state.role === "admin" && (
            <ResourcesPanel
              values={state.resources || []}
              t={t}
              request={api}
              onRun={run}
              busy={busy}
              onDirtyChange={setUnsaved}
            />
          )}
          {(page === "sources" || page === "nodes") &&
            state.role === "admin" && (
              <>
                <PageHeader
                  eyebrow="SOURCES"
                  title={
                    page === "nodes"
                      ? t("节点", "Nodes")
                      : t("订阅", "Subscriptions")
                  }
                  description={t(
                    "集中维护上游与手工节点，让配置专注于如何使用。",
                    "Maintain subscriptions and manual nodes independently of profiles.",
                  )}
                  action={
                    <Button
                      primary
                      onClick={() =>
                        sourceTab === "subscriptions"
                          ? showSource()
                          : showNode()
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
                  {page === "nodes" && (
                    <Field label={t("节点来源", "Node source")}>
                      <select
                        value={nodeSource}
                        onChange={(e) => {
                          if (
                            !unsaved ||
                            confirm(
                              t(
                                "放弃未保存的节点修改？",
                                "Discard unsaved node changes?",
                              ),
                            )
                          )
                            setNodeSource(e.target.value);
                        }}
                      >
                        <option value="">
                          {t("手工节点", "Manual nodes")}
                        </option>
                        {state.sources.map((s) => (
                          <option key={s.id} value={s.id}>
                            {s.name} · {s.nodeCount}
                          </option>
                        ))}
                      </select>
                    </Field>
                  )}
                  {!(page === "nodes" && nodeSource) && (
                    <div className="search">
                      <Search size={15} />
                      <input
                        aria-label={
                          page === "nodes"
                            ? t("搜索节点", "Search nodes")
                            : t("搜索订阅", "Search subscriptions")
                        }
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        placeholder={t("搜索名称…", "Search names…")}
                      />
                    </div>
                  )}
                </div>
                <section className="panel table-panel">
                  {page === "nodes" && nodeSource ? (
                    <SubscriptionNodes
                      onDirtyChange={setUnsaved}
                      key={nodeSource}
                      source={state.sources.find((s) => s.id === nodeSource)!}
                      t={t}
                      onChanged={() => void reload()}
                      onCopy={(name) =>
                        setModal(
                          <NameForm
                            title={t("复制为手工节点", "Copy to manual node")}
                            initial={name + t(" 副本", " copy")}
                            t={t}
                            onSave={(newName) =>
                              run(
                                () =>
                                  api(
                                    `sources/${nodeSource}/copy-node`,
                                    "POST",
                                    { name, newName },
                                  ),
                                t("手工节点已创建", "Manual node created"),
                                true,
                              )
                            }
                          />,
                        )
                      }
                    />
                  ) : sourceTab === "subscriptions" ? (
                    state.sources.length === 0 ? (
                      <Empty
                        icon={<Network />}
                        title={t("从第一个订阅开始", "Start with a source")}
                        text={t(
                          "添加订阅地址，或导入节点 YAML、代理 URI / Base64 列表。",
                          "Add a subscription URL or import node YAML, proxy URI / Base64 lists.",
                        )}
                        action={
                          <Button onClick={() => showSource()}>
                            <Plus size={15} />
                            {t("添加订阅", "Add subscription")}
                          </Button>
                        }
                      />
                    ) : (
                      <table className="responsive-cards">
                        <thead>
                          <tr>
                            <th>{t("名称与标签", "Name & tags")}</th>
                            <th>{t("节点", "Nodes")}</th>
                            <th>{t("订阅状态", "Source status")}</th>
                            <th>{t("最近成功", "Last success")}</th>
                            <th>{t("操作", "Actions")}</th>
                          </tr>
                        </thead>
                        <tbody>
                          {state.sources
                            .filter((v) => filter(v.name))
                            .map((v) => (
                              <tr key={v.id}>
                                <td data-label={t("订阅", "Subscription")}>
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
                                <td data-label={t("节点", "Nodes")}>
                                  <button
                                    className="link-button"
                                    onClick={() => void showNodes(v)}
                                  >
                                    {v.nodeCount} {t("个节点", "nodes")}
                                  </button>
                                </td>
                                <td data-label={t("状态", "Status")}>
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
                                <td
                                  data-label={t("最近成功", "Last success")}
                                  className="muted"
                                >
                                  {time(v.lastSuccess)}
                                </td>
                                <td data-label={t("操作", "Actions")}>
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
                                              "订阅已更新；重新发布配置后设备刷新可获取新配置",
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
                                      label={t("编辑订阅", "Edit source")}
                                      onClick={() => showSource(v)}
                                    >
                                      <Pencil size={15} />
                                    </IconButton>
                                    <IconButton
                                      label={t("删除订阅", "Delete source")}
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
                        "按协议表单创建节点，或从订阅节点复制。",
                        "Create nodes with protocol forms, or copy from a subscription.",
                      )}
                      action={
                        <Button onClick={() => showNode()}>
                          <Plus size={15} />
                          {t("添加节点", "Add node")}
                        </Button>
                      }
                    />
                  ) : (
                    <table className="responsive-cards">
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
                              <td data-label={t("节点", "Node")}>
                                <strong>{v.node.name}</strong>
                                <small>
                                  {v.node.server}:{v.node.port}
                                </small>
                              </td>
                              <td data-label={t("协议", "Protocol")}>
                                <span className="pill">
                                  {v.node.type.toUpperCase()}
                                </span>
                              </td>
                              <td data-label={t("状态", "Status")}>
                                {v.enabled
                                  ? t("启用", "Enabled")
                                  : t("停用", "Disabled")}
                              </td>
                              <td data-label={t("操作", "Actions")}>
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
          {page === "profiles" && state.role === "admin" && (
            <>
              <PageHeader
                eyebrow="CONFIGURATION PROFILES"
                title={t("我的配置", "Configuration")}
                description={t(
                  "选择节点，按用途分流，然后应用到客户端。",
                  "Maintain groups, routing rules, and client settings in one profile.",
                )}
                action={
                  <Button disabled={unsaved} onClick={showProfile}>
                    <Plus size={16} />
                    {t("新建配置", "New profile")}
                  </Button>
                }
              />
              {state.profiles.length === 0 ? (
                <section className="panel">
                  <Empty
                    icon={<Layers3 />}
                    title={t("把订阅组织成配置", "Turn sources into a profile")}
                    text={t(
                      "从添加订阅开始，设置用途后再应用。",
                      "Each profile has independent groups, rules, and settings.",
                    )}
                    action={
                      <Button
                        primary
                        disabled={busy}
                        onClick={() =>
                          void run(
                            async () => {
                              const created = await api<Profile>(
                                "profiles",
                                "POST",
                                emptyProfile(t("我的配置", "My configuration")),
                              );
                              setSelectedProfile(created.id);
                            },
                            t(
                              "先添加订阅，再设置分流",
                              "Add a subscription, then configure routing",
                            ),
                          )
                        }
                      >
                        {t("开始配置", "Start configuring")}
                      </Button>
                    }
                  />
                </section>
              ) : (
                <>
                  {state.profiles.length > 1 && (
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
                            if (activeProfile?.id === p.id) return;
                            if (
                              !unsaved ||
                              confirm(
                                t(
                                  "丢弃未保存修改并切换配置？",
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
                  )}
                  {activeProfile && (
                    <ProfileEditor
                      key={activeProfile.id}
                      profile={activeProfile}
                      state={state}
                      busy={busy}
                      t={t}
                      time={time}
                      onRun={run}
                      error={error}
                      onDelete={() => remove("profiles", activeProfile)}
                      onSource={showSource}
                      onNode={showNode}
                      onSourceNodes={showNodes}
                      onRemove={remove}
                      onLink={showLink}
                      onNewLink={() => showDevice(undefined, activeProfile.id)}
                      onEditLink={(d) => showDevice(d, activeProfile.id)}
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
                title={t("订阅链接", "Subscription links")}
                description={t(
                  "独立链接，明确归属。切换配置无需更换订阅地址。",
                  "Independent links and clear ownership. Switch profiles without changing the URL.",
                )}
                action={
                  <Button
                    primary
                    disabled={!Object.keys(state.active).length}
                    onClick={() => showDevice()}
                  >
                    <Plus size={16} />
                    {t("创建订阅链接", "Create subscription link")}
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
                            "绑定已发布配置，即可复制链接或导入 Clash Verge。",
                            "Bind a published profile, then copy the link or import into Clash Verge.",
                          )
                        : t(
                            "请先创建并发布一份配置。",
                            "Create and publish a profile first.",
                          )
                    }
                  />
                ) : (
                  <table className="responsive-cards">
                    <thead>
                      <tr>
                        <th>{t("设备", "Device")}</th>
                        <th>{t("绑定配置", "Profile")}</th>
                        <th>{t("授权状态", "Authorization")}</th>
                        <th>{t("最近下载", "Last download")}</th>
                        <th>{t("操作", "Actions")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {state.devices.map((d) => (
                        <tr key={d.id}>
                          <td data-label={t("设备", "Device")}>
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
                          <td data-label={t("绑定配置", "Profile")}>
                            {
                              state.profiles.find((p) => p.id === d.profileId)
                                ?.name
                            }
                          </td>
                          <td data-label={t("授权状态", "Authorization")}>
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
                          <td
                            data-label={t("最近下载", "Last download")}
                            className="muted"
                          >
                            {time(state.seen[d.id])}
                          </td>
                          <td data-label={t("操作", "Actions")}>
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
                                      "编辑 / 切换配置",
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
          {page === "system" && state.role === "admin" && (
            <>
              <PageHeader
                eyebrow="SYSTEM"
                title={t("设置", "Settings")}
                description={t(
                  "了解当前实例的边界与验证状态。",
                  "Understand this instance’s scope and validation status.",
                )}
              />
              <ImportWizard
                t={t}
                request={api}
                onRun={run}
                busy={busy}
                onDirtyChange={setUnsaved}
              />
              <div className="settings-grid">
                <section className="panel padded">
                  <ShieldCheck className="accent" size={26} />
                  <h2>{t("身份与存储", "Identity and storage")}</h2>
                  <dl>
                    <dt>{t("应用版本", "Application version")}</dt>
                    <dd>{state.build?.version || "unknown"}</dd>
                  </dl>
                  <details>
                    <summary>{t("技术详情", "Technical details")}</summary>
                    <dl>
                      <dt>{t("源码与平台", "Source and platform")}</dt>
                      <dd className="mono wrap">
                        {state.build?.commit.slice(0, 12)} ·{" "}
                        {state.build?.platform}
                        {state.build?.dirty
                          ? t(" · 未提交构建", " · uncommitted build")
                          : ""}
                      </dd>
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
                  </details>
                  <p className="muted">
                    {t(
                      "备份包含加密业务状态与发布资源，不含主密钥。恢复必须另外持有原主密钥。",
                      "Backups contain encrypted state and resources, without the master key. Restore also requires the original key.",
                    )}
                  </p>
                  <Button
                    disabled={busy}
                    onClick={() =>
                      void run(
                        async () => {
                          const response = await fetch("/api/v1/backup", {
                            method: "POST",
                            headers: {
                              "Content-Type": "application/json",
                              "X-CSRF-Token": csrf,
                            },
                            body: "{}",
                          });
                          if (!response.ok) throw new APIError("backup_failed");
                          const blob = await response.blob();
                          const url = URL.createObjectURL(blob);
                          const link = document.createElement("a");
                          link.href = url;
                          link.download = "subweft-encrypted.sqlite";
                          link.click();
                          setTimeout(() => URL.revokeObjectURL(url), 10000);
                        },
                        t("备份已生成", "Backup generated"),
                      )
                    }
                  >
                    {t("下载一致性备份", "Download consistent backup")}
                  </Button>{" "}
                </section>
                <section className="panel padded">
                  <FileCheck2 className="accent" size={26} />
                  <h2>{t("校验与能力", "Validation and scope")}</h2>
                  <dl></dl>
                  <details>
                    <summary>{t("技术详情", "Technical details")}</summary>
                    <dl>
                      <dt>{t("校验器", "Validator")}</dt>
                      <dd className="mono wrap">{state.validator}</dd>
                      <dt>{t("节点协议", "Node protocols")}</dt>
                      <dd>{state.capabilities.protocols.join(" · ")}</dd>
                      <dt>{t("自动发布", "Automatic publishing")}</dt>
                      <dd>
                        {t(
                          "自动更新最近已发布的定义；保存草稿不会激活",
                          "Updates use the last published definition; saving drafts never activates them",
                        )}
                      </dd>
                    </dl>
                  </details>
                </section>
              </div>
              <div className="notice inline">
                <TriangleAlert size={18} />
                {t(
                  "候选版本：完整表单、迁移和容器流程已提供。真实 Pocket ID、Clash Verge 与生产切换仍须分别验收。",
                  "Development build: migration, container and browser acceptance are in progress. Real Pocket ID, Clash Verge and production cutover require separate acceptance.",
                )}
              </div>
            </>
          )}
        </main>
        <footer className="footer">
          Subweft <span>·</span>
          {t("订阅有序，策略清晰。", "Sources organized. Policies clear.")}
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
        onCancel={(e) => {
          e.preventDefault();
          closeModal();
        }}
        onClick={(e) => {
          if (e.target === e.currentTarget) {
            const r = e.currentTarget.getBoundingClientRect();
            if (
              e.clientX < r.left ||
              e.clientX > r.right ||
              e.clientY < r.top ||
              e.clientY > r.bottom
            )
              closeModal();
          }
        }}
        onClose={() => setModal(null)}
      >
        <button
          className="dialog-close icon-button"
          aria-label={t("关闭对话框", "Close dialog")}
          onClick={closeModal}
        >
          <X size={19} />
        </button>
        {error && (
          <div role="alert" className="alert error modal-error">
            {error}
            <ErrorFieldLink
              field={fieldError.field}
              label={t("定位字段", "Go to field")}
            />
          </div>
        )}
        <FormErrorContext.Provider
          value={{
            ...fieldError,
            clear: () => {
              setFieldError({ field: "", message: "" });
              setError("");
            },
          }}
        >
          <fieldset
            disabled={busy}
            className="dialog-content"
            onChangeCapture={(e) => {
              if ((e.target as HTMLElement).closest("form"))
                setModalDirty(true);
            }}
          >
            {modal}
          </fieldset>
        </FormErrorContext.Provider>
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
  const [url, setURL] = useState(value?.url || "");
  const [content, setContent] = useState("");
  const [tags, setTags] = useState(value?.tags?.join(", ") || "");
  const [interval, setInterval] = useState(value?.interval || 3600);
  const [clientProvider, setClientProvider] = useState(
    value?.clientProvider || {
      enabled: !value,
      interval: 3600,
      healthCheck: {
        enable: !value,
        url: "https://www.gstatic.com/generate_204",
        interval: 300,
        timeout: 5000,
        lazy: true,
      },
    },
  );
  const [extractNodes, setExtract] = useState(value?.extractNodes || false);
  const [fetchOptions, setFetch] = useState(
    value?.fetch || { timeout: 20, retries: 1 },
  );
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
          extractNodes,
          fetch: fetchOptions,
          clientProvider,
        });
      }}
    >
      <ModalTitle
        title={
          value
            ? t("编辑订阅", "Edit source")
            : t("添加订阅", "Add subscription source")
        }
      />
      <p className="muted">
        {t(
          "支持 13 种协议的节点 YAML、常见代理 URI 列表及单层 Base64。未知节点字段会明确拒绝。",
          "Use supported fields in node YAML, proxy URI lists, or one Base64 envelope. Unknown fields are rejected.",
        )}
      </p>
      <Field
        label={t("订阅名称", "Source name")}
        hint={t(
          "节点输出会带有此订阅前缀。",
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
      <Field label={t("订阅方式", "Source type")}>
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
          <SecretControl
            t={t}
            name={t("订阅地址", "Subscription URL")}
            required={!value || value.kind !== kind}
            value={url}
            onChange={setURL}
          />
        </Field>
      ) : (
        <>
          <Field
            label={t("导入内容", "Import content")}
            hint={
              value
                ? t(
                    "留空保留原内容。这里用于导入订阅，不用于编辑生成配置。",
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
      <details>
        <summary>{t("更新设置", "Refresh settings")}</summary>
        <div className="form-grid">
          {tags && (
            <Field label={t("标签，以逗号分隔", "Tags, comma separated")}>
              <input value={tags} onChange={(e) => setTags(e.target.value)} />
            </Field>
          )}
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
      </details>
      <CheckField
        label={t(
          "从完整配置中只提取节点",
          "Extract only nodes from a complete configuration",
        )}
        checked={extractNodes}
        onChange={setExtract}
      />
      <p className="muted">
        {t(
          "启用提取时，订阅中的规则、DNS 和客户端设置不会被导入。配置通过独立表单维护。",
          "Extraction imports only nodes; profile forms manage rules, DNS and client settings independently.",
        )}
      </p>
      {kind === "http" && (
        <details>
          <summary>{t("抓取高级参数", "Advanced fetch settings")}</summary>
          <ObjectForm
            schema={{
              kind: "object",
              properties: {
                timeout: { kind: "integer", min: 3, max: 120 },
                retries: { kind: "integer", min: 0, max: 3 },
                proxy: { kind: "string", secret: true },
                userAgent: { kind: "string" },
                headers: {
                  kind: "map",
                  values: { kind: "string" },
                  secret: true,
                },
              },
            }}
            value={fetchOptions}
            t={t}
            path="fetch"
            onChange={(v) => setFetch(v as typeof fetchOptions)}
          />
          <p className="muted">
            {t(
              "代理仅支持 HTTP(S) CONNECT；目标与代理地址都会检查 SSRF 边界。",
              "Proxies use HTTP(S) CONNECT; both target and proxy destinations receive SSRF checks.",
            )}
          </p>
        </details>
      )}
      <details>
        <summary>
          {t("客户端更新与健康检查", "Client updates & health checks")}
        </summary>
        <CheckField
          label={t("使用具名 proxy provider", "Use a named proxy provider")}
          checked={clientProvider.enabled}
          onChange={(enabled) =>
            setClientProvider({ ...clientProvider, enabled })
          }
        />
        {clientProvider.enabled && (
          <>
            <div className="form-grid">
              <Field
                label={t(
                  "客户端更新间隔（秒）",
                  "Client refresh interval (seconds)",
                )}
              >
                <input
                  type="number"
                  min={60}
                  max={604800}
                  value={clientProvider.interval}
                  onChange={(e) =>
                    setClientProvider({
                      ...clientProvider,
                      interval: Number(e.target.value),
                    })
                  }
                />
              </Field>
              <Field
                label={t("更新使用的代理（可选）", "Update proxy (optional)")}
              >
                <input
                  value={clientProvider.proxy || ""}
                  onChange={(e) =>
                    setClientProvider({
                      ...clientProvider,
                      proxy: e.target.value,
                    })
                  }
                />
              </Field>
            </div>
            <CheckField
              label={t(
                "定期检查节点可用性",
                "Check node availability periodically",
              )}
              checked={clientProvider.healthCheck.enable}
              onChange={(enable) =>
                setClientProvider({
                  ...clientProvider,
                  healthCheck: { ...clientProvider.healthCheck, enable },
                })
              }
            />
            {clientProvider.healthCheck.enable && (
              <>
                <Field label={t("检查地址", "Check URL")}>
                  <input
                    type="url"
                    value={clientProvider.healthCheck.url}
                    onChange={(e) =>
                      setClientProvider({
                        ...clientProvider,
                        healthCheck: {
                          ...clientProvider.healthCheck,
                          url: e.target.value,
                        },
                      })
                    }
                  />
                </Field>
                <div className="form-grid">
                  {(["interval", "timeout"] as const).map((key) => (
                    <Field
                      key={key}
                      label={
                        key === "interval"
                          ? t("检查间隔（秒）", "Check interval (seconds)")
                          : t("超时（毫秒）", "Timeout (milliseconds)")
                      }
                    >
                      <input
                        type="number"
                        min={key === "interval" ? 30 : 100}
                        max={key === "interval" ? 86400 : 30000}
                        value={clientProvider.healthCheck[key]}
                        onChange={(e) =>
                          setClientProvider({
                            ...clientProvider,
                            healthCheck: {
                              ...clientProvider.healthCheck,
                              [key]: Number(e.target.value),
                            },
                          })
                        }
                      />
                    </Field>
                  ))}
                </div>
                <CheckField
                  label={t("仅检查正在使用的节点", "Check only nodes in use")}
                  checked={clientProvider.healthCheck.lazy}
                  onChange={(lazy) =>
                    setClientProvider({
                      ...clientProvider,
                      healthCheck: { ...clientProvider.healthCheck, lazy },
                    })
                  }
                />
                <Field
                  label={t(
                    "期望 HTTP 状态码（可选）",
                    "Expected HTTP status (optional)",
                  )}
                >
                  <input
                    value={clientProvider.healthCheck.expectedStatus || ""}
                    onChange={(e) =>
                      setClientProvider({
                        ...clientProvider,
                        healthCheck: {
                          ...clientProvider.healthCheck,
                          expectedStatus: e.target.value,
                        },
                      })
                    }
                  />
                </Field>
              </>
            )}
          </>
        )}
        <p className="muted">
          {t(
            "客户端读取已发布的节点快照；上游地址保留在服务端。这里的 proxy 与健康检查由客户端执行。",
            "Clients read published snapshots. Upstream URLs remain server-side; proxy and health checks here run in the client.",
          )}
        </p>
      </details>
      <CheckField
        label={t("启用订阅", "Enable source")}
        checked={enabled}
        onChange={setEnabled}
      />
      <div className="form-footer">
        <Button primary type="submit">
          {t("保存订阅", "Save source")}
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
          value
            ? t("编辑链接", "Edit link")
            : t("创建订阅链接", "Create subscription link")
        }
      />
      <Field label={t("链接名称（如手机）", "Link name (e.g. phone)")}>
        <input
          autoFocus
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      {profiles.length > 1 && (
        <Field
          label={t("绑定配置", "Profile")}
          hint={t(
            "切换配置保持订阅 URL 不变，客户端刷新后生效。",
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
      )}
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
          {t("保存链接", "Save link")}
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
  error,
  profile,
  state,
  busy,
  t,
  time,
  onRun,
  onDelete,
  onSource,
  onNode,
  onSourceNodes,
  onRemove,
  onLink,
  onNewLink,
  onEditLink,
  onDirtyChange,
}: {
  profile: Profile;
  error: string;
  state: State;
  busy: boolean;
  t: T;
  time: (v?: string) => string;
  onRun: Run;
  onDelete: () => void;
  onSource: (v?: Source) => unknown;
  onNode: (v?: ManualNode) => unknown;
  onSourceNodes: (v: Source) => unknown;
  onRemove: (kind: string, v: Meta) => void;
  onLink: (v: Device) => unknown;
  onNewLink: () => unknown;
  onEditLink: (v: Device) => unknown;
  onDirtyChange: (v: boolean) => void;
}) {
  const { catalog } = useCatalog();
  const [draft, setDraft] = useState<Profile>(copy(profile) as Profile);
  const [tab, setTab] = useState(
    state.sources.length || state.nodes.length ? "routing" : "sources",
  );
  const [manageRulesets, setManageRulesets] = useState(false);
  const [resourcesDirty, setResourcesDirty] = useState(false);
  const [rulesDirty, setRulesDirty] = useState(false);
  const [poolsDirty, setPoolsDirty] = useState(false);
  const [allowStale, setAllowStale] = useState(false);
  const [preview, setPreview] = useState<{
    config: string;
    members: Record<string, string[]>;
    stale: string[];
  } | null>(null);
  useEffect(() => {
    setDraft(copy(profile) as Profile);
  }, [profile.id, profile.revision]);
  useEffect(() => setPreview(null), [profile.id]);
  const dirty = JSON.stringify(draft) !== JSON.stringify(profile);
  const pendingChanges = dirty || resourcesDirty || rulesDirty || poolsDirty;
  const releases = state.releases
    .filter((r) => r.profileId === profile.id)
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  const active = releases.find((r) => r.id === state.active[profile.id]);
  const needsApply =
    dirty ||
    !active ||
    active.profileRevision !== profile.revision ||
    state.pendingProfiles?.[profile.id] !== false;
  useEffect(() => {
    onDirtyChange(pendingChanges);
    return () => onDirtyChange(false);
  }, [pendingChanges, onDirtyChange]);
  useEffect(() => {
    const guard = (event: BeforeUnloadEvent) => {
      if (pendingChanges) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [pendingChanges]);
  const saveIfDirty = async () =>
    dirty
      ? await api<Profile>(`profiles/${profile.id}`, "PUT", draft)
      : profile;
  const options = [...draft.groups.map((g) => g.name), "DIRECT", "REJECT"];
  return (
    <section className="panel profile-editor">
      <div className="editor-header">
        <div>
          <h2>{draft.name}</h2>
          <span className="pill subtle">
            {dirty
              ? t("有未保存修改", "Unsaved changes")
              : active &&
                  !state.pendingProfiles?.[profile.id] &&
                  active.profileRevision === profile.revision
                ? t("已生成，可供客户端更新", "Ready for client refresh")
                : t("有待应用更改", "Pending changes")}
          </span>
        </div>
        <div className="actions">
          <Button
            primary
            disabled={
              busy || rulesDirty || poolsDirty || resourcesDirty || !needsApply
            }
            onClick={() =>
              void onRun(
                async () => {
                  const saved = await saveIfDirty();
                  return api("profiles/" + profile.id + "/publish", "POST", {
                    revision: saved.revision,
                    allowStale,
                  });
                },
                t(
                  "校验通过，请在客户端刷新订阅",
                  "Validated. Refresh the subscription in your client.",
                ),
              )
            }
          >
            {t("应用更改", "Apply changes")}
          </Button>
          <Button onClick={() => setTab("links")}>
            {t("获取订阅链接", "Subscription links")}
          </Button>
          <details className="profile-more">
            <summary>{t("更多", "More")}</summary>
            <div className="profile-more-menu">
              <Button
                disabled={busy || !dirty}
                onClick={() =>
                  void onRun(
                    () => api("profiles/" + profile.id, "PUT", draft),
                    t("修改已保存", "Changes saved"),
                  )
                }
              >
                {t("保存修改", "Save changes")}
              </Button>

              <Field label={t("配置名称", "Profile name")}>
                <input
                  maxLength={160}
                  value={draft.name}
                  onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                />
              </Field>
              <Button
                disabled={busy}
                onClick={() =>
                  void onRun(async () => {
                    const saved = await saveIfDirty();
                    const value = await api<{
                      config: string;
                      members: Record<string, string[]>;
                      stale: string[];
                    }>("profiles/" + profile.id + "/preview", "POST", {
                      revision: saved.revision,
                      allowStale,
                    });
                    setPreview(value);
                    setTab("preview");
                  })
                }
              >
                {t("查看生成结果", "Preview generated configuration")}
              </Button>
              <Button onClick={() => setTab("preview")}>
                {t("版本记录与回滚", "Versions & rollback")}
              </Button>
              <Button
                disabled={busy}
                onClick={() =>
                  void onRun(
                    async () => {
                      const saved = await saveIfDirty();
                      return api("profiles/" + profile.id + "/copy", "POST", {
                        revision: saved.revision,
                      });
                    },
                    t("配置已复制", "Profile copied"),
                  )
                }
              >
                {t("复制配置", "Copy profile")}
              </Button>
              <Button danger disabled={busy} onClick={onDelete}>
                {t("删除配置", "Delete profile")}
              </Button>
              <CheckField
                label={t(
                  "允许使用更新失败订阅的旧缓存",
                  "Allow cached content after refresh failures",
                )}
                checked={allowStale}
                onChange={setAllowStale}
              />
            </div>
          </details>
        </div>
      </div>
      {dirty && (
        <div className="notice inline">
          {t(
            "应用更改会保存并校验；校验失败时设备继续使用当前版本。",
            "Apply saves and validates changes. Failures preserve the current device configuration.",
          )}
        </div>
      )}
      <div className="editor-tabs tabs">
        {[
          ["sources", t("订阅与节点", "Subscriptions & nodes")],
          ["routing", t("分流", "Routing")],
          ["dns", t("DNS 与连接", "DNS & connections")],
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
        {tab === "routing" && (
          <RoutingWorkspace
            profile={draft}
            state={state}
            t={t}
            onChange={setDraft}
            onDirtyChange={setRulesDirty}
            onManageResources={() => setManageRulesets(true)}
            renderGroup={(g, i, change, rules) => (
              <GroupForm
                value={g}
                index={i}
                groups={draft.groups}
                pools={draft.pools || []}
                state={state}
                t={t}
                onChange={change}
                onDelete={() => {}}
                rules={rules}
              />
            )}
          />
        )}
        {tab === "sources" && (
          <>
            <div className="section-intro">
              <div>
                <h3>{t("订阅", "Subscriptions")}</h3>
                <p>
                  {t(
                    "来源编辑会保存到共享库，应用更改后才影响本配置；新增订阅还需在分流中选择。",
                    "Source edits are saved to the shared library and take effect in this profile after Apply changes. Select new subscriptions in routing.",
                  )}
                </p>
              </div>
              <Button primary onClick={() => onSource()}>
                {t("添加订阅", "Add subscription")}
              </Button>
            </div>
            {state.profiles.length > 1 && (
              <p className="notice">
                {t(
                  "订阅和手工节点由配置共享；编辑来源会影响引用它的其他配置。订阅池仅属于当前配置。",
                  "Subscriptions and manual nodes are shared across configurations. Source edits affect configurations using them. Pools belong only to this configuration.",
                )}
              </p>
            )}
            {state.sources.length === 0 && (
              <p className="empty-list">
                {t(
                  "还没有订阅。添加订阅或下方手工节点即可开始。",
                  "Add a subscription or a manual node below to start.",
                )}
              </p>
            )}
            {state.sources.map((source) => (
              <div className="purpose-row" key={source.id}>
                <div>
                  <strong>{source.name}</strong>
                  <p className="muted">
                    {source.nodeCount} {t("个节点", "nodes")} ·{" "}
                    {source.enabled
                      ? t("启用", "Enabled")
                      : t("停用", "Disabled")}
                  </p>
                </div>
                <div className="muted">
                  {source.lastError
                    ? t(
                        "更新失败，保留旧内容",
                        "Refresh failed; cached content retained",
                      )
                    : time(source.lastSuccess)}
                </div>
                <div className="actions">
                  <Button onClick={() => onSourceNodes(source)}>
                    {t("查看节点", "View nodes")}
                  </Button>
                  <Button onClick={() => onSource(source)}>
                    {t("编辑", "Edit")}
                  </Button>
                  {source.kind === "http" && (
                    <Button
                      disabled={busy}
                      onClick={() =>
                        void onRun(
                          () =>
                            api(`sources/${source.id}/refresh`, "POST", {
                              revision: source.revision,
                            }),
                          t("订阅内容已更新", "Subscription content refreshed"),
                        )
                      }
                    >
                      {t("更新", "Refresh")}
                    </Button>
                  )}
                  <Button danger onClick={() => onRemove("sources", source)}>
                    {t("删除", "Delete")}
                  </Button>
                </div>
              </div>
            ))}
            <PoolEditor
              pools={draft.pools || []}
              groups={draft.groups}
              sources={state.sources}
              t={t}
              onChange={(pools, groups) =>
                setDraft({ ...draft, pools, groups })
              }
              onDirtyChange={setPoolsDirty}
            />
            <section className="pool-section">
              <div className="section-intro">
                <h3>{t("手工节点", "Manual nodes")}</h3>
                <Button onClick={() => onNode()}>
                  {t("添加节点", "Add node")}
                </Button>
              </div>
              {state.nodes.map((node) => (
                <div className="purpose-row" key={node.id}>
                  <strong>{node.node.name}</strong>
                  <span>{node.node.type}</span>
                  <div className="actions">
                    <Button onClick={() => onNode(node)}>
                      {t("编辑", "Edit")}
                    </Button>
                    <Button danger onClick={() => onRemove("nodes", node)}>
                      {t("删除", "Delete")}
                    </Button>
                  </div>
                </div>
              ))}
            </section>
          </>
        )}
        {tab === "links" && (
          <>
            <div className="section-intro">
              <div>
                <h3>{t("订阅链接", "Subscription links")}</h3>
                <p>
                  {t(
                    "每个链接可独立撤销；客户端刷新后获取当前已应用配置。",
                    "Each link can be revoked independently. Refresh the client to receive the applied configuration.",
                  )}
                </p>
              </div>
              <Button primary disabled={!active || busy} onClick={onNewLink}>
                {t("创建链接", "Create link")}
              </Button>
            </div>
            {!active && (
              <p className="notice">
                {t(
                  "请先应用一次配置，再创建链接。",
                  "Apply the configuration before creating a link.",
                )}
              </p>
            )}
            {!state.devices.some((d) => d.profileId === profile.id) && (
              <p className="empty-list">
                {t(
                  "还没有链接。为手机或电脑创建一个，便于分别管理。",
                  "Create a link for each phone or computer to manage them independently.",
                )}
              </p>
            )}
            {state.devices
              .filter((d) => d.profileId === profile.id)
              .map((d) => {
                const expired =
                  !!d.expiresAt && Date.parse(d.expiresAt) <= Date.now();
                const revoked = d.state === "revoked";
                const saveState = (state: string) =>
                  onRun(
                    () =>
                      api(`devices/${d.id}`, "PUT", {
                        id: d.id,
                        revision: d.revision,
                        name: d.name,
                        profileId: d.profileId,
                        state,
                        expiresAt: d.expiresAt || null,
                      }),
                    t("链接状态已更新", "Link state updated"),
                  );
                return (
                  <div className="purpose-row" key={d.id}>
                    <div>
                      <strong>{d.name}</strong>
                      <p className="muted">
                        {d.expiresAt
                          ? t("到期：", "Expires: ") + time(d.expiresAt)
                          : t("长期有效", "No expiry")}
                      </p>
                    </div>
                    <div>
                      <span
                        className={`pill ${revoked ? "subtle" : d.state === "paused" || expired ? "warning" : "success"}`}
                      >
                        {revoked
                          ? t("已撤销", "Revoked")
                          : d.state === "paused"
                            ? t("已暂停", "Paused")
                            : expired
                              ? t("已到期", "Expired")
                              : t("有效", "Active")}
                      </span>
                      <p className="muted">
                        {t("最近下载：", "Last download: ")}
                        {time(state.seen[d.id])}
                      </p>
                    </div>
                    <div className="actions">
                      <Button
                        disabled={
                          busy || revoked || expired || d.state !== "active"
                        }
                        onClick={() => onLink(d)}
                      >
                        {t("查看链接", "Show link")}
                      </Button>
                      <Button
                        disabled={busy || revoked}
                        onClick={() => onEditLink(d)}
                      >
                        {t("编辑", "Edit")}
                      </Button>
                      <details className="action-menu">
                        <summary>{t("更多操作", "More actions")}</summary>
                        <div>
                          <Button
                            disabled={busy || revoked}
                            onClick={() =>
                              void saveState(
                                d.state === "paused" ? "active" : "paused",
                              )
                            }
                          >
                            {d.state === "paused"
                              ? t("恢复", "Resume")
                              : t("暂停", "Pause")}
                          </Button>
                          <Button
                            disabled={busy || revoked}
                            onClick={() => {
                              if (
                                confirm(
                                  t(
                                    "更换后旧链接立即失效，需要重新导入新链接。",
                                    "The old link stops working immediately. Import the new link again.",
                                  ),
                                )
                              )
                                void onRun(
                                  () =>
                                    api(`devices/${d.id}/rotate`, "POST", {
                                      revision: d.revision,
                                    }),
                                  t(
                                    "链接已更换，请重新获取",
                                    "Link replaced. Retrieve the new link.",
                                  ),
                                );
                            }}
                          >
                            {t("更换链接", "Replace link")}
                          </Button>
                          <Button
                            danger
                            disabled={busy || revoked}
                            onClick={() => {
                              if (
                                confirm(
                                  t(
                                    "永久撤销此链接？其他链接不受影响。",
                                    "Permanently revoke this link? Other links are unaffected.",
                                  ),
                                )
                              )
                                void saveState("revoked");
                            }}
                          >
                            {t("撤销", "Revoke")}
                          </Button>
                          <Button
                            danger
                            disabled={busy}
                            onClick={() => onRemove("devices", d)}
                          >
                            {t("删除记录", "Delete record")}
                          </Button>
                        </div>
                      </details>
                    </div>
                  </div>
                );
              })}
          </>
        )}
        {manageRulesets && (
          <EditorDialog
            title={t("规则集", "Rulesets")}
            t={t}
            dirty={resourcesDirty}
            onClose={() => setManageRulesets(false)}
            footer={null}
          >
            {error && (
              <p role="alert" className="alert error">
                {error}
              </p>
            )}
            <ResourcesPanel
              values={state.resources || []}
              t={t}
              request={api}
              onRun={onRun}
              busy={busy}
              onDirtyChange={setResourcesDirty}
            />
          </EditorDialog>
        )}
        {tab === "dns" && (
          <div className="settings-form">
            {tab === "dns" && (
              <>
                <h3>{t("连接设置", "Connection settings")}</h3>
                <Field
                  label={t("混合端口（0 停用）", "Mixed port (0 disables)")}
                >
                  <input
                    type="number"
                    min={0}
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
                    setDraft({
                      ...draft,
                      settings: { ...draft.settings, ipv6 },
                    })
                  }
                />
              </>
            )}
            {tab === "dns" && (
              <>
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
                      label={t(
                        "解析器（每行一个）",
                        "Nameservers (one per line)",
                      )}
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
                {catalog && draft.settings.dnsEnabled && (
                  <>
                    <details>
                      <summary>
                        {t("完整 DNS 高级字段", "Complete DNS fields")}
                      </summary>
                      <ObjectForm
                        schema={catalog.dns}
                        exclude={[
                          "enable",
                          "enhanced-mode",
                          "nameserver",
                          "nameserver-policy",
                          "proxy-server-nameserver-policy",
                        ]}
                        value={draft.settings.dns || {}}
                        t={t}
                        path="settings.dns"
                        onChange={(dns) =>
                          setDraft({
                            ...draft,
                            settings: { ...draft.settings, dns },
                          })
                        }
                      />
                    </details>
                    {(
                      ["nameserverPolicy", "proxyNameserverPolicy"] as const
                    ).map((key) => (
                      <details key={key}>
                        <summary>
                          {key === "nameserverPolicy"
                            ? t("按域名选择解析器", "Nameserver policies")
                            : t(
                                "代理服务器域名解析规则",
                                "Proxy nameserver policies",
                              )}
                        </summary>
                        <p className="muted">
                          {t(
                            "按顺序匹配；每条可设置多个解析器。",
                            "Matched in order; each policy accepts multiple resolvers.",
                          )}
                        </p>
                        <ValueForm
                          schema={{
                            kind: "array",
                            items: {
                              kind: "object",
                              properties: {
                                pattern: { kind: "string", required: true },
                                nameservers: {
                                  kind: "array",
                                  required: true,
                                  items: { kind: "string" },
                                },
                              },
                            },
                          }}
                          value={draft.settings[key] || []}
                          t={t}
                          path={"settings." + key}
                          onChange={(v) =>
                            setDraft({
                              ...draft,
                              settings: { ...draft.settings, [key]: v },
                            })
                          }
                        />
                      </details>
                    ))}
                  </>
                )}
              </>
            )}
            <details>
              <summary>
                {t("自动更新与地理资源", "Automatic updates & geo resources")}
              </summary>
              <CheckField
                label={t(
                  "订阅内容更新后自动应用",
                  "Automatically apply subscription content updates",
                )}
                checked={!!draft.autoPublish}
                onChange={(autoPublish) => setDraft({ ...draft, autoPublish })}
              />
              <p className="muted">
                {t(
                  "自动应用已确认来源的新内容。手动修改订阅、节点或规则集后，需要应用更改；失败保留现行版本。",
                  "Apply refreshed content from confirmed sources. Manual source, node or ruleset edits require Apply changes. Failures retain the active release.",
                )}
              </p>
              {state.publishStatus
                ?.filter((v) => v.id === draft.id && v.error)
                .map((v) => (
                  <p className="alert error" key={v.id}>
                    {t(
                      "自动发布未成功，现行版本已保留：",
                      "Automatic publication failed; active release retained: ",
                    )}
                    {v.error}
                  </p>
                ))}
              {(["geoIpId", "geoSiteId", "geoAsnId"] as const).map((key) => (
                <Field
                  key={key}
                  label={
                    key === "geoIpId"
                      ? "GEOIP / Country.mmdb / GeoIP.dat"
                      : key === "geoAsnId"
                        ? "ASN / ASN.mmdb"
                        : "GEOSITE / GeoSite.dat"
                  }
                >
                  <select
                    value={draft[key] || ""}
                    onChange={(e) =>
                      setDraft({ ...draft, [key]: e.target.value })
                    }
                  >
                    <option value="">{t("不使用", "Unused")}</option>
                    {(state.resources || [])
                      .filter((r) =>
                        key === "geoIpId"
                          ? ["mmdb", "geoip"].includes(r.type)
                          : r.type === (key === "geoAsnId" ? "asn" : "geosite"),
                      )
                      .map((r) => (
                        <option key={r.id} value={r.id}>
                          {r.name}
                        </option>
                      ))}
                  </select>
                </Field>
              ))}
            </details>
            {catalog && (
              <details>
                <summary>
                  {t("客户端全局参数", "Global client settings")}
                </summary>
                <ObjectForm
                  schema={catalog.config}
                  exclude={["mixed-port", "allow-lan", "ipv6"]}
                  value={draft.settings.options || {}}
                  t={t}
                  path="settings.options"
                  onChange={(options) =>
                    setDraft({
                      ...draft,
                      settings: { ...draft.settings, options },
                    })
                  }
                />
              </details>
            )}
          </div>
        )}
        {tab === "preview" && (
          <>
            <div className="section-intro">
              <div>
                <h3>{t("只读预览", "Read-only preview")}</h3>
                <p>
                  {t(
                    "凭据及敏感传输参数已遮蔽。实际发布使用完整配置并执行内核校验。",
                    "Credentials and sensitive transport values are redacted. Publishing validates the complete configuration.",
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
    </section>
  );
}
function SourceNodeList({
  onChanged,
  source,
  nodes,
  t,
  onCopy,
  embedded = false,
  onDirtyChange,
}: {
  embedded?: boolean;
  onDirtyChange: (dirty: boolean) => void;
  onChanged: () => void;
  source: Source;
  nodes: Array<
    Pick<Node, "name" | "type" | "server" | "port"> & {
      identity?: string;
      ambiguous?: boolean;
      override?: {
        identity: string;
        name: string;
        excluded: boolean;
        notes: string;
      };
    }
  >;
  t: T;
  onCopy: (
    node: Pick<Node, "name" | "type" | "server" | "port"> & {
      identity?: string;
      ambiguous?: boolean;
      override?: {
        identity: string;
        name: string;
        excluded: boolean;
        notes: string;
      };
    },
  ) => void;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  const [localRevision, setLocalRevision] = useState(source.revision);
  const [localNodes, setLocalNodes] = useState(nodes);
  const [localOverrides, setLocalOverrides] = useState(source.overrides || []);
  const [overrideEdit, setOverrideEdit] = useState<(typeof nodes)[number]>();
  const [overrideValue, setOverrideValue] = useState({
    identity: "",
    name: "",
    excluded: false,
    notes: "",
  });
  const [localError, setLocalError] = useState("");
  const overrideOriginal = overrideEdit?.override || {
    identity: overrideEdit?.identity || "",
    name: "",
    excluded: false,
    notes: "",
  };
  const overrideDirty =
    !!overrideEdit &&
    (overrideValue.identity !== overrideOriginal.identity ||
      overrideValue.name !== overrideOriginal.name ||
      overrideValue.excluded !== overrideOriginal.excluded ||
      overrideValue.notes !== overrideOriginal.notes);
  useEffect(() => {
    onDirtyChange(overrideDirty);
    return () => onDirtyChange(false);
  }, [overrideDirty, onDirtyChange]);
  const discardOverride = () =>
    !overrideDirty ||
    confirm(t("放弃未保存的节点修改？", "Discard unsaved node changes?"));
  const saveOverride = async (remove = false, value = overrideValue) => {
    try {
      const saved = await api<Source>(
        `sources/${source.id}/overrides`,
        "POST",
        { revision: localRevision, override: value, remove },
      );
      setLocalRevision(saved.revision);
      setLocalOverrides(saved.overrides || []);
      onChanged();
      setLocalNodes(
        localNodes.map((n) =>
          n.identity === value.identity
            ? { ...n, override: remove ? undefined : value }
            : n,
        ),
      );
      setOverrideEdit(undefined);
      setLocalError("");
    } catch {
      setLocalError(
        t(
          "保存失败，订阅可能已更新；请关闭后重新打开。",
          "Save failed; the source may have changed. Reopen this dialog.",
        ),
      );
    }
  };
  const filtered = localNodes.filter((node) =>
    node.name.toLowerCase().includes(query.toLowerCase()),
  );
  const pages = Math.max(1, Math.ceil(filtered.length / 50));
  return (
    <div className={embedded ? "embedded-source-nodes" : undefined}>
      {embedded ? (
        <h2>{source.name + " · " + t("节点", "Nodes")}</h2>
      ) : (
        <ModalTitle title={source.name + " · " + t("节点", "Nodes")} />
      )}
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
      {localError && (
        <p role="alert" className="alert error">
          {localError}
        </p>
      )}
      {localOverrides
        .filter(
          (o) =>
            localNodes.filter((n) => n.identity === o.identity).length !== 1 ||
            localNodes.some((n) => n.identity === o.identity && n.ambiguous),
        )
        .map((o) => (
          <div className="alert warning" key={o.identity}>
            <span>
              {t(
                "上游节点缺失或身份歧义，请清除旧覆盖后重新设置。",
                "Upstream identity missing or ambiguous. Clear this override before reapplying.",
              )}{" "}
              {o.name || o.notes}
            </span>
            <Button
              onClick={() => {
                if (discardOverride()) void saveOverride(true, o);
              }}
            >
              {t("清除覆盖", "Clear override")}
            </Button>
          </div>
        ))}
      {overrideEdit && (
        <div className="panel">
          <h3>{overrideEdit.name}</h3>
          <Field
            label={t(
              "本地名称（留空使用上游）",
              "Local name (blank uses upstream)",
            )}
          >
            <input
              value={overrideValue.name}
              onChange={(e) =>
                setOverrideValue({ ...overrideValue, name: e.target.value })
              }
            />
          </Field>
          <Field label={t("备注", "Notes")}>
            <input
              value={overrideValue.notes}
              onChange={(e) =>
                setOverrideValue({ ...overrideValue, notes: e.target.value })
              }
            />
          </Field>
          <CheckField
            label={t("从所有配置排除", "Exclude from all profiles")}
            checked={overrideValue.excluded}
            onChange={(excluded) =>
              setOverrideValue({ ...overrideValue, excluded })
            }
          />
          <div className="actions">
            <Button onClick={() => void saveOverride()}>
              {t("保存覆盖", "Save override")}
            </Button>
            <Button
              onClick={() => {
                if (discardOverride()) void saveOverride(true);
              }}
            >
              {t("清除覆盖", "Clear override")}
            </Button>
            <Button
              onClick={() => {
                if (discardOverride()) setOverrideEdit(undefined);
              }}
            >
              {t("取消", "Cancel")}
            </Button>
          </div>
        </div>
      )}
      <div className="node-list">
        {filtered.slice(page * 50, (page + 1) * 50).map((node) => (
          <div className="node-row" key={node.name}>
            <div>
              <strong>
                {node.override?.name || node.name}
                {node.override?.excluded ? " · excluded" : ""}
              </strong>
              <small>
                {node.type} · {node.server}:{node.port}
              </small>
            </div>
            <Button
              disabled={node.ambiguous || !node.identity}
              onClick={() => {
                if (!discardOverride()) return;
                setOverrideEdit(node);
                setOverrideValue(
                  node.override?.identity
                    ? node.override
                    : {
                        identity: node.identity!,
                        name: "",
                        excluded: false,
                        notes: "",
                      },
                );
              }}
            >
              {node.ambiguous
                ? t("身份歧义，禁止覆盖", "Ambiguous identity")
                : t("本地覆盖", "Local override")}
            </Button>
            <Button
              onClick={() => {
                if (discardOverride()) {
                  setOverrideEdit(undefined);
                  onCopy(node);
                }
              }}
            >
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
  pools = [],
  state,
  t,
  onChange,
  rules,
}: {
  rules?: ReactNode;
  value: Group;
  index: number;
  groups: Group[];
  pools?: import("./types").SourcePool[];
  state: State;
  t: T;
  onChange: (g: Group) => void;
  onDelete: () => void;
}) {
  const set = <K extends keyof Group>(key: K, value: Group[K]) =>
    onChange({ ...g, [key]: value });
  const { catalog } = useCatalog();
  const [memberTab, setMemberTab] = useState("sources");
  const [search, setSearch] = useState("");
  const matches = (name: string) =>
    name.toLowerCase().includes(search.toLowerCase());
  const selectedNames = [
    ...state.sources
      .filter((s) => g.sourceIds.includes(s.id))
      .map((s) => s.name),
    ...(g.pools || []),
    ...state.nodes
      .filter((n) => g.nodeIds.includes(n.id))
      .map((n) => n.node.name),
    ...g.groups,
    ...g.builtins,
    ...(g.allSources ? [t("全部订阅", "All subscriptions")] : []),
    ...g.tags,
  ];
  const [membersOpen, setMembersOpen] = useState(selectedNames.length === 0);
  return (
    <div className="group-card">
      <div className="form-grid">
        <Field label={t("用途名称", "Purpose name")}>
          <input
            maxLength={160}
            value={g.name}
            onChange={(e) => set("name", e.target.value)}
          />
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
      {rules}
      <details
        className="member-picker"
        open={membersOpen}
        onToggle={(e) => setMembersOpen(e.currentTarget.open)}
      >
        <summary>
          {t("节点来源", "Node sources")}
          <span>
            {selectedNames.join("、") ||
              t("请选择订阅或节点", "Choose subscriptions or nodes")}
          </span>
        </summary>
        <div className="tabs">
          {[
            ["sources", t("订阅", "Subscriptions")],
            ["nodes", t("手工节点", "Manual nodes")],
            ["groups", t("其他组与直连", "Groups & direct")],
          ].map(([key, label]) => (
            <button
              type="button"
              key={key}
              className={memberTab === key ? "selected" : ""}
              onClick={() => setMemberTab(key)}
            >
              {label}
            </button>
          ))}
        </div>
        {state.sources.length + state.nodes.length + groups.length > 8 && (
          <Field label={t("搜索成员", "Search members")}>
            <input
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </Field>
        )}
        {memberTab === "sources" && (
          <div className="source-selector">
            <div className="selector-heading">
              <strong>{t("选择订阅", "Choose subscriptions")}</strong>
              <Button
                onClick={() =>
                  onChange({
                    ...g,
                    sourceIds: state.sources
                      .filter((s) => s.enabled)
                      .map((s) => s.id),
                    tags: [],
                    allSources: false,
                  })
                }
              >
                {t("选择当前全部", "Select current subscriptions")}
              </Button>
            </div>
            {(g.allSources || g.tags.length > 0) && (
              <div className="notice">
                <p>
                  {t(
                    "此用途使用动态来源。可明确转换为当前订阅列表，转换前保留原行为。",
                    "This purpose uses dynamic sources. Convert explicitly to a fixed selection; existing behavior is preserved until then.",
                  )}
                </p>
                <Button
                  onClick={() =>
                    onChange({
                      ...g,
                      sourceIds: [
                        ...new Set([
                          ...g.sourceIds,
                          ...state.sources
                            .filter(
                              (s) =>
                                s.enabled &&
                                (g.allSources ||
                                  s.tags?.some((tag) => g.tags.includes(tag))),
                            )
                            .map((s) => s.id),
                        ]),
                      ],
                      tags: [],
                      allSources: false,
                    })
                  }
                >
                  {t("改为明确选择", "Convert to explicit selection")}
                </Button>
              </div>
            )}
            <div className="checkbox-grid">
              {state.sources
                .filter((s) => matches(s.name))
                .map((s) => (
                  <CheckField
                    key={s.id}
                    label={`${s.name} (${s.nodeCount})${s.enabled ? "" : t(" · 已停用", " · Disabled")}`}
                    checked={g.sourceIds.includes(s.id)}
                    onChange={() => set("sourceIds", toggle(g.sourceIds, s.id))}
                  />
                ))}
              {state.sources.length === 0 && (
                <span className="muted">
                  {t("尚无订阅，请先添加。", "Add a source first.")}
                </span>
              )}
            </div>
            {pools.length > 0 && (
              <div className="pool-picker">
                <strong>
                  {t("或使用订阅池", "Or use a subscription pool")}
                </strong>
                <div className="checkbox-grid">
                  {pools.map((pool) => (
                    <CheckField
                      key={pool.name}
                      label={
                        pool.name +
                        " · " +
                        pool.sourceIds.length +
                        t(" 个订阅", " subscriptions")
                      }
                      checked={g.pools?.includes(pool.name) || false}
                      onChange={() =>
                        set("pools", toggle(g.pools || [], pool.name))
                      }
                    />
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
        {memberTab === "nodes" && (
          <div className="source-selector">
            <strong>{t("手工节点", "Manual nodes")}</strong>
            <div className="checkbox-grid">
              {state.nodes
                .filter((n) => matches(n.node.name))
                .map((n) => (
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
        {memberTab === "groups" && (
          <div className="source-selector">
            <div className="checkbox-grid">
              {groups
                .filter((v, i) => i !== index && matches(v.name))
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
          </div>
        )}
      </details>
      <details className="advanced">
        <summary>{t("筛选节点", "Filter nodes")}</summary>
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
        <details className="test-settings">
          <summary>{t("测速参数", "Health-check settings")}</summary>
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
        </details>
      )}
      {catalog?.groups[g.type] && (
        <details>
          <summary>{t("高级策略参数", "Advanced policy settings")}</summary>
          <ObjectForm
            schema={catalog.groups[g.type]}
            value={g.options || {}}
            t={t}
            path={"groups." + index + ".options"}
            onChange={(v) => set("options", v)}
          />
        </details>
      )}
    </div>
  );
}

class PageBoundary extends React.Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    if (this.state.failed) {
      const zh = document.documentElement.lang.startsWith("zh");
      return (
        <main className="center">
          <h1>{zh ? "页面暂时无法显示" : "The page could not be displayed"}</h1>
          <p>
            {zh
              ? "已保存的数据仍然保留。请重新载入。"
              : "Saved data is preserved. Reload to continue."}
          </p>
          <Button primary onClick={() => location.reload()}>
            {zh ? "重新载入" : "Reload"}
          </Button>
        </main>
      );
    }
    return this.props.children;
  }
}
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <PageBoundary>
      <App />
    </PageBoundary>
  </React.StrictMode>,
);

function NewProfileForm({
  state,
  t,
  onSave,
}: {
  state: State;
  t: T;
  onSave: (p: Profile) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [sourceIds, setSources] = useState<string[]>([]);
  const [nodeIds, setNodes] = useState<string[]>([]);
  const [direct, setDirect] = useState(false);
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        const p = emptyProfile(name.trim());
        p.autoPublish = true;
        p.groups[0] = {
          ...p.groups[0],
          name: t("默认节点", "Default nodes"),
          sourceIds,
          nodeIds,
          builtins: direct ? ["DIRECT"] : [],
        };
        p.rules[0].target = p.groups[0].name;
        void onSave(p);
      }}
    >
      <ModalTitle title={t("新建配置", "New profile")} />
      <Field label={t("配置名称", "Profile name")}>
        <input
          required
          maxLength={160}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      <p>
        {t(
          "选择这份配置使用的订阅和节点。之后可添加策略组和分流规则。",
          "Choose subscriptions and nodes. Add policy groups and routing rules later.",
        )}
      </p>
      <h3>{t("订阅", "Subscriptions")}</h3>
      <div className="member-options">
        {state.sources
          .filter((s) => s.enabled)
          .map((s) => (
            <CheckField
              key={s.id}
              label={s.name + " · " + s.nodeCount + t(" 个节点", " nodes")}
              checked={sourceIds.includes(s.id)}
              onChange={() => setSources(toggle(sourceIds, s.id))}
            />
          ))}
      </div>
      <h3>{t("手工节点", "Manual nodes")}</h3>
      <div className="member-options">
        {state.nodes
          .filter((n) => n.enabled)
          .map((n) => (
            <CheckField
              key={n.id}
              label={n.node.name}
              checked={nodeIds.includes(n.id)}
              onChange={() => setNodes(toggle(nodeIds, n.id))}
            />
          ))}
      </div>
      {!state.sources.length && !state.nodes.length && (
        <p className="notice">
          {t(
            "尚无订阅或节点。可以先添加，或创建仅直连的配置。",
            "Add a subscription or node first, or create a direct-only profile.",
          )}
        </p>
      )}
      <CheckField
        label={t("允许选择直连", "Include direct connection")}
        checked={direct}
        onChange={setDirect}
      />
      <p className="muted">
        {t(
          "仅使用本次选择；以后新增订阅不会自动加入。",
          "Only these selections are included; future subscriptions are not added automatically.",
        )}
      </p>
      <div className="form-footer">
        <Button
          primary
          type="submit"
          disabled={
            !name.trim() || (!sourceIds.length && !nodeIds.length && !direct)
          }
        >
          {t("创建配置", "Create profile")}
        </Button>
      </div>
    </form>
  );
}

function SubscriptionNodes({
  onDirtyChange,
  source,
  t,
  onChanged,
  onCopy,
}: {
  source: Source;
  t: T;
  onDirtyChange: (dirty: boolean) => void;
  onChanged: () => void;
  onCopy: (name: string) => void;
}) {
  const [nodes, setNodes] =
    useState<React.ComponentProps<typeof SourceNodeList>["nodes"]>();
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    setNodes(undefined);
    setFailed(false);
    api<React.ComponentProps<typeof SourceNodeList>["nodes"]>(
      "sources/" + source.id + "/nodes",
    )
      .then((v) => {
        if (active) setNodes(v);
      })
      .catch(() => {
        if (active) setFailed(true);
      });
    return () => {
      active = false;
    };
  }, [source.id]);
  return (
    <div className="padded">
      {failed ? (
        <p role="alert">
          {t(
            "节点加载失败，请刷新页面重试。",
            "Could not load nodes. Refresh to retry.",
          )}
        </p>
      ) : nodes ? (
        <SourceNodeList
          onDirtyChange={onDirtyChange}
          embedded
          source={source}
          nodes={nodes}
          t={t}
          onChanged={onChanged}
          onCopy={(n) => onCopy(n.name)}
        />
      ) : (
        <p>{t("正在加载节点…", "Loading nodes…")}</p>
      )}
    </div>
  );
}

function DeviceLink({
  device,
  url,
  t,
  onCopied,
}: {
  device: Device;
  url: string;
  t: T;
  onCopied: () => void;
}) {
  const [visible, setVisible] = useState(false);
  return (
    <div>
      <ModalTitle title={t("设备订阅链接", "Device subscription link")} />
      <p className="muted">
        {t(
          "此链接包含设备凭据，请仅提供给这台设备。普通发布和切换配置不会改变链接。",
          "This link contains a device credential. Publishing and profile switching preserve it.",
        )}
      </p>
      <div className="field">
        <label htmlFor="device-link-value">{device.name}</label>
        <div className="secret-control">
          <input
            id="device-link-value"
            type={visible ? "text" : "password"}
            readOnly
            value={url}
            onFocus={(e) => e.target.select()}
          />
          <Button
            aria-label={
              visible ? t("隐藏链接", "Hide link") : t("显示链接", "Show link")
            }
            aria-pressed={visible}
            onClick={() => setVisible(!visible)}
          >
            {visible ? t("隐藏", "Hide") : t("显示", "Show")}
          </Button>
        </div>
      </div>
      <div className="actions">
        <Button
          primary
          onClick={() => void navigator.clipboard.writeText(url).then(onCopied)}
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
    </div>
  );
}
