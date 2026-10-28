import React, { createContext, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
export type T = (zh: string, en: string) => string;
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
