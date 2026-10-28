import React, { createContext, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { createPortal } from "react-dom";
export type T = (zh: string, en: string) => string;
export function EditorDialog({
  title,
  children,
  footer,
  dirty,
  onClose,
  t,
}: {
  title: string;
  children: ReactNode;
  footer: ReactNode;
  dirty: boolean;
  onClose: () => void;
  t: T;
}) {
  const ref = React.useRef<HTMLDialogElement>(null);
  const titleId = React.useId();
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    ref.current?.showModal();
    ref.current
      ?.querySelector<HTMLInputElement>("input,select,textarea")
      ?.focus();
    return () => {
      ref.current?.close();
      previous?.focus();
    };
  }, []);
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
  const close = () => {
    if (
      !dirty ||
      confirm(t("放弃未保存的表单修改？", "Discard unsaved form changes?"))
    )
      onClose();
  };
  return createPortal(
    <dialog
      className="editor-dialog"
      ref={ref}
      aria-labelledby={titleId}
      onCancel={(e) => {
        e.preventDefault();
        close();
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
            close();
        }
      }}
    >
      <header className="dialog-header">
        <h2 id={titleId}>{title}</h2>
        <Button aria-label={t("关闭对话框", "Close dialog")} onClick={close}>
          ×
        </Button>
      </header>
      <div className="dialog-body">{children}</div>
      <footer className="dialog-footer">
        <Button onClick={close}>{t("取消", "Cancel")}</Button>
        {footer}
      </footer>
    </dialog>,
    document.body,
  );
}
export const FormErrorContext = createContext({
  field: "",
  message: "",
  clear: () => {},
});
export function Button({
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
export function Field({
  label,
  hint,
  name,
  children,
}: {
  label: string;
  hint?: string;
  name?: string;
  children: ReactNode;
}) {
  const id = React.useId();
  const error = useContext(FormErrorContext);
  const invalid = !!name && error.field === name;
  const control = React.isValidElement(children)
    ? React.cloneElement(
        children as React.ReactElement<Record<string, unknown>>,
        {
          id,
          onChange: (event: unknown) => {
            if (invalid) error.clear();
            const handler = (
              children as React.ReactElement<Record<string, unknown>>
            ).props.onChange;
            if (typeof handler === "function") handler(event);
          },
          "aria-invalid": invalid || undefined,
          "aria-describedby":
            [hint ? id + "-hint" : "", invalid ? id + "-error" : ""]
              .filter(Boolean)
              .join(" ") || undefined,
        },
      )
    : children;
  return (
    <div className="field" data-field={name}>
      <label htmlFor={id}>{label}</label>
      {control}
      {hint && <small id={id + "-hint"}>{hint}</small>}
      {invalid && (
        <small className="field-error" id={id + "-error"}>
          {error.message}
        </small>
      )}
    </div>
  );
}
export function CheckField({
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
export function ModalTitle({ title }: { title: string }) {
  return (
    <div className="modal-title">
      <span className="eyebrow">SUBWEFT WORKSPACE</span>
      <h2 id="subweft-dialog-title">{title}</h2>
    </div>
  );
}

export function ErrorFieldLink({
  field,
  label,
}: {
  field: string;
  label: string;
}) {
  const [target, setTarget] = useState("");
  useEffect(() => {
    const control = field
      ? document.querySelector<HTMLInputElement>(
          `[data-field="${CSS.escape(field)}"] :is(input,select,textarea)`,
        )
      : null;
    setTarget(control?.id || "");
  }, [field]);
  return target ? (
    <a
      className="error-field-link"
      href={"#" + target}
      onClick={(e) => {
        e.preventDefault();
        document.getElementById(target)?.focus();
      }}
    >
      {label}
    </a>
  ) : null;
}
