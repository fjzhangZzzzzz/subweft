import { useEffect, useState } from "react";
import { Button, Field, CheckField } from "./ui";
import type { T } from "./ui";
import type { Source, ManualNode, Profile } from "./types";
import type { Resource } from "./resources";
type ImportFile = { name: string; content: string };
type Summary = {
  id: string;
  revision: number;
  name: string;
  expires: string;
  sources: Source[];
  nodes: ManualNode[];
  resources: Resource[];
  groups: {
    name: string;
    type: string;
    allSources: boolean;
    tags: string[];
    sourceIds: string[];
    manualNodes: number;
  }[];
  ruleCount: number;
  dnsEnabled: boolean;
  warnings: string[];
};
export function ImportWizard({
  t,
  request,
  onRun,
  busy,
  onDirtyChange,
}: {
  t: T;
  request: <T>(path: string, method?: string, body?: unknown) => Promise<T>;
  onRun: (work: () => Promise<unknown>, success?: string) => Promise<void>;
  busy: boolean;
  onDirtyChange: (v: boolean) => void;
}) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(t("导入配置", "Imported profile"));
  const [content, setContent] = useState("");
  const [refresh, setRefresh] = useState(true);
  const [summary, setSummary] = useState<Summary>();
  const [progress, setProgress] = useState("");
  const [created, setCreated] = useState("");
  const [fileError, setFileError] = useState("");
  useEffect(() => {
    onDirtyChange(open && !created && !!content);
    return () => onDirtyChange(false);
  }, [open, created, content, onDirtyChange]);
  const readFiles = async (
    list: FileList | null,
    set: (v: ImportFile[]) => void,
  ) => {
    if (!list) return;
    const files = Array.from(list);
    if (files.reduce((n, f) => n + f.size, 0) > 16 * 1024 * 1024) {
      setFileError(
        t(
          "所选文件合计不能超过 16 MiB。",
          "Selected files must total at most 16 MiB.",
        ),
      );
      return;
    }
    set(
      await Promise.all(
        files.map(async (f) => ({ name: f.name, content: await f.text() })),
      ),
    );
    setFileError("");
  };
  const warning: Record<string, string> = {
    import_creates_draft_only: t(
      "仅创建新草稿，不改变现行发布或设备。",
      "Creates new drafts; active releases and devices remain unchanged.",
    ),
    client_cache_paths_are_service_managed: t(
      "客户端缓存路径由服务统一生成。",
      "Client cache paths are managed by the service.",
    ),
    refresh_http_sources_and_resources_before_publish: t(
      "发布前须取得有效的来源和规则缓存。",
      "Valid source and rule caches are required before publishing.",
    ),
    review_managed_geodata_source_or_upload_existing_client_data: t(
      "GEOIP/GEOSITE 转为受管理资源。请核对数据来源，或上传原客户端数据文件。",
      "GEOIP/GEOSITE become managed resources. Review their origins or upload the existing client data.",
    ),
  };
  return (
    <section className="panel padded">
      <div className="section-intro">
        <div>
          <h2>{t("导入现有配置", "Import existing configuration")}</h2>
          <p>
            {t(
              "预览字段检查和对象转换，确认后一次创建独立草稿。",
              "Review field checks and object conversion, then create independent drafts atomically.",
            )}
          </p>
        </div>
        <Button disabled={busy} onClick={() => setOpen(!open)}>
          {open ? t("收起", "Collapse") : t("开始导入", "Start import")}
        </Button>
      </div>
      {open && (
        <>
          {created ? (
            <div role="status" className="notice inline">
              {created}
            </div>
          ) : summary ? (
            <>
              <h3>{summary.name}</h3>
              <dl>
                <dt>
                  {t(
                    "订阅 / 手工节点 / 规则集与地理数据",
                    "Sources / manual nodes / resources",
                  )}
                </dt>
                <dd>
                  {summary.sources.length} / {summary.nodes.length} /{" "}
                  {summary.resources.length}
                </dd>
                <dt>{t("规则 / DNS", "Rules / DNS")}</dt>
                <dd>
                  {summary.ruleCount} /{" "}
                  {summary.dnsEnabled
                    ? t("启用", "Enabled")
                    : t("停用", "Disabled")}
                </dd>
              </dl>
              <div className="resource-grid">
                {summary.sources.map((s) => (
                  <div className="schema-field" key={s.id}>
                    <strong>{s.name}</strong>
                    <p>
                      {s.enabled ? t("启用", "Enabled") : t("停用", "Disabled")}{" "}
                      · {s.tags?.join(" / ")}
                    </p>
                  </div>
                ))}
                {summary.groups.map((g) => (
                  <div className="schema-field" key={g.name}>
                    <strong>
                      {g.name} · {g.type}
                    </strong>
                    <p>
                      {g.allSources
                        ? t("全部启用来源", "All enabled sources")
                        : g.tags?.length
                          ? t("标签：", "Tags: ") + g.tags.join(" / ")
                          : (g.sourceIds?.length || 0) +
                            t(" 个订阅", " subscriptions")}{" "}
                      · {g.manualNodes} {t("手工节点", "manual nodes")}
                    </p>
                  </div>
                ))}
              </div>
              <ul>
                {summary.warnings.map((w) => (
                  <li key={w}>{warning[w] || w}</li>
                ))}
              </ul>
              <CheckField
                label={t(
                  "创建草稿后抓取已启用的 HTTP 来源和规则集与地理数据；不自动发布",
                  "Refresh enabled HTTP sources and resources after creation; do not publish",
                )}
                checked={refresh}
                onChange={setRefresh}
              />
              {progress && <p role="status">{progress}</p>}
              <div className="actions">
                <Button
                  disabled={busy}
                  onClick={() =>
                    void onRun(async () => {
                      await request("imports/" + summary.id, "DELETE", {
                        revision: summary.revision,
                      });
                      setSummary(undefined);
                    })
                  }
                >
                  {t("返回修改", "Back to edit")}
                </Button>
                <Button
                  primary
                  disabled={busy}
                  onClick={() =>
                    void onRun(
                      async () => {
                        const p = await request<Profile>(
                          "imports/" + summary.id + "/commit",
                          "POST",
                          { revision: summary.revision },
                        );
                        setCreated(
                          t("草稿已创建：", "Draft created: ") + p.name,
                        );
                        onDirtyChange(false);
                        let failed = 0;
                        if (refresh) {
                          for (const item of [
                            ...summary.sources
                              .filter((s) => s.enabled && s.kind === "http")
                              .map((s) => ({
                                kind: "sources",
                                id: s.id,
                                name: s.name,
                              })),
                            ...summary.resources
                              .filter((r) => r.enabled && r.kind === "http")
                              .map((r) => ({
                                kind: "resources",
                                id: r.id,
                                name: r.name,
                              })),
                          ]) {
                            setProgress(
                              t("正在抓取：", "Refreshing: ") + item.name,
                            );
                            try {
                              await request(
                                item.kind + "/" + item.id + "/refresh",
                                "POST",
                                { revision: 1 },
                              );
                            } catch {
                              failed++;
                            }
                          }
                        }
                        setProgress("");
                        if (failed)
                          setCreated(
                            t(
                              "草稿已创建，部分抓取失败。请到来源和规则集与地理数据页检查，再预览和发布。",
                              "Draft created; some refreshes failed. Check sources and resources before previewing and publishing.",
                            ),
                          );
                      },
                      t("导入草稿已创建", "Import draft created"),
                    )
                  }
                >
                  {t("确认创建导入草稿", "Create reviewed import draft")}
                </Button>
              </div>
            </>
          ) : (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void onRun(async () =>
                  setSummary(
                    await request<Summary>("imports/preview", "POST", {
                      name,
                      content,
                    }),
                  ),
                );
              }}
            >
              <Field label={t("新配置方案名称", "New profile name")}>
                <input
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </Field>
              <Field
                label={t(
                  "完整渲染后配置文件",
                  "Complete rendered configuration file",
                )}
              >
                <input
                  type="file"
                  accept=".yaml,.yml"
                  onChange={(e) =>
                    void readFiles(e.target.files, (files) =>
                      setContent(files[0]?.content || ""),
                    )
                  }
                />
              </Field>
              <Field
                label={t(
                  "或粘贴导入内容（仅用于导入）",
                  "Or paste import content (import only)",
                )}
              >
                <textarea
                  rows={5}
                  required
                  value={content}
                  onChange={(e) => setContent(e.target.value)}
                  spellCheck={false}
                />
              </Field>

              {fileError && <p role="alert">{fileError}</p>}
              <div className="form-footer">
                <Button primary type="submit" disabled={busy || !content}>
                  {t("检查并预览导入", "Check and preview import")}
                </Button>
              </div>
            </form>
          )}
        </>
      )}
    </section>
  );
}
