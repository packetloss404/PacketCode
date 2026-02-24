import type { InputHTMLAttributes, TextareaHTMLAttributes } from "react";

type InputProps = InputHTMLAttributes<HTMLInputElement> & {
  accentColor?: "green" | "blue" | "purple" | "amber";
};

const accentMap = {
  green: "focus:border-accent-green",
  blue: "focus:border-accent-blue",
  purple: "focus:border-accent-purple",
  amber: "focus:border-accent-amber",
};

const BASE = "w-full bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none";

export function Input({ className, accentColor = "green", ...props }: InputProps) {
  return (
    <input
      className={`${BASE} ${accentMap[accentColor]} ${className ?? ""}`}
      {...props}
    />
  );
}

type TextAreaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & {
  accentColor?: "green" | "blue" | "purple" | "amber";
};

export function TextArea({ className, accentColor = "green", ...props }: TextAreaProps) {
  return (
    <textarea
      className={`${BASE} py-2 resize-none ${accentMap[accentColor]} ${className ?? ""}`}
      {...props}
    />
  );
}
