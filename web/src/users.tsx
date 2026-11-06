import { useState, useEffect } from "react";
import { Button, Field, CheckField } from "./ui";
import type { T } from "./ui";
import type { Meta, Profile } from "./types";
export type User = Meta & {
  issuer: string;
  subject: string;
  name: string;
  role: string;
  enabled: boolean;
  profileIds: string[];
  bootstrap?: boolean;
};
export function UsersPanel({
  values,
  issuer,
  profiles,
  t,
  request,
  onRun,
  busy,
  onDirtyChange,
}: {
  values: User[];
  issuer: string;
  profiles: Profile[];
  t: T;
  request: <T>(path: string, method?: string, body?: unknown) => Promise<T>;
  onRun: (work: () => Promise<unknown>, success?: string) => Promise<void>;
  busy: boolean;
  onDirtyChange: (v: boolean) => void;
}) {
  const [edit, setEdit] = useState<User>();
  const [query, setQuery] = useState("");
  const [original, setOriginal] = useState<User>();
  const dirty = !!edit && JSON.stringify(edit) !== JSON.stringify(original);
  useEffect(() => {
    onDirtyChange(dirty);
    return () => onDirtyChange(false);
  }, [dirty, onDirtyChange]);
  const open = (value?: User) => {
    const initial = value || {
      id: "",
      revision: 0,
      issuer,
      subject: "",
      name: "",
      role: "user",
      enabled: true,
      profileIds: [],
    };
    setOriginal(initial);
    setEdit(initial);
  };
  return (
    <section>
      <div className="page-heading">
        <div>
          <span className="eyebrow">ACCESS</span>
          <h1>{t("使用者与授权", "Users & grants")}</h1>
          <p>
            {t(
              "使用身份提供方的用户标识授权，首次登录不会自动获得权限。",
              "Identities use OIDC issuer and sub; first login never grants access.",
            )}
          </p>
        </div>
        {!edit && (
          <Button primary onClick={() => open()}>
            {t("添加使用者", "Add user")}
          </Button>
        )}
      </div>
      {edit ? (
        <form
          className="panel settings-form"
          onSubmit={(e) => {
            e.preventDefault();
            void onRun(
              async () => {
                await request(
                  edit.id ? "users/" + edit.id : "users",
                  edit.id ? "PUT" : "POST",
                  edit,
                );
                setEdit(undefined);
                onDirtyChange(false);
              },
              t("授权已保存", "Access saved"),
            );
          }}
        >
          <Field label={t("显示名称", "Display name")}>
            <input
              required
              value={edit.name}
              onChange={(e) => setEdit({ ...edit, name: e.target.value })}
            />
          </Field>
          <Field
            label={t("身份提供方地址（Issuer）", "Identity provider (issuer)")}
          >
            <input value={edit.issuer} readOnly />
          </Field>
          <Field label={t("用户标识（Sub）", "User identifier (sub)")}>
            <input
              required
              value={edit.subject}
              readOnly={!!edit.id}
              onChange={(e) => setEdit({ ...edit, subject: e.target.value })}
            />
          </Field>
          <p className="muted">
            {t(
              "请填写身份提供方中该用户的稳定标识，不要使用昵称或邮箱。",
              "Enter the user’s stable identifier from your identity provider, not a display name or email.",
            )}
          </p>
          <Field label={t("角色", "Role")}>
            <select
              value={edit.role}
              onChange={(e) => setEdit({ ...edit, role: e.target.value })}
            >
              <option value="user">{t("使用者", "User")}</option>
              <option value="admin">{t("管理员", "Administrator")}</option>
            </select>
          </Field>
          <CheckField
            label={t("允许登录和下载", "Allow login and downloads")}
            checked={edit.enabled}
            onChange={(enabled) => setEdit({ ...edit, enabled })}
          />
          {edit.role === "user" && (
            <fieldset>
              <legend>{t("允许使用的配置方案", "Allowed profiles")}</legend>
              {profiles.map((p) => (
                <CheckField
                  key={p.id}
                  label={p.name}
                  checked={edit.profileIds.includes(p.id)}
                  onChange={(v) =>
                    setEdit({
                      ...edit,
                      profileIds: v
                        ? [...edit.profileIds, p.id]
                        : edit.profileIds.filter((id) => id !== p.id),
                    })
                  }
                />
              ))}
            </fieldset>
          )}
          <div className="form-footer">
            <Button
              onClick={() => {
                if (
                  !dirty ||
                  confirm(
                    t(
                      "放弃未保存的表单修改？",
                      "Discard unsaved form changes?",
                    ),
                  )
                ) {
                  setEdit(undefined);
                  onDirtyChange(false);
                }
              }}
            >
              {t("取消", "Cancel")}
            </Button>
            <Button primary type="submit" disabled={busy}>
              {t("保存授权", "Save access")}
            </Button>
          </div>
        </form>
      ) : (
        <>
          <Field label={t("搜索使用者", "Search users")}>
            <input value={query} onChange={(e) => setQuery(e.target.value)} />
          </Field>
          <div className="resource-grid">
            {values
              .filter((v) =>
                (v.name + " " + v.subject)
                  .toLowerCase()
                  .includes(query.toLowerCase()),
              )
              .map((v) => (
                <article className="panel" key={v.id}>
                  <h3>{v.name}</h3>
                  <p>
                    {v.role === "admin"
                      ? t("管理员", "Administrator")
                      : t("使用者", "User")}{" "}
                    ·{" "}
                    {v.enabled
                      ? t("已启用", "Enabled")
                      : t("已停用", "Disabled")}
                  </p>
                  <p className="muted wrap">{v.subject}</p>
                  {v.bootstrap ? (
                    <small>
                      {t(
                        "引导管理员由部署配置维护，作为恢复入口。",
                        "Bootstrap administrators are maintained by deployment configuration for recovery.",
                      )}
                    </small>
                  ) : (
                    <Button onClick={() => open(v)}>
                      {t("编辑授权", "Edit access")}
                    </Button>
                  )}
                </article>
              ))}
          </div>
        </>
      )}
    </section>
  );
}
