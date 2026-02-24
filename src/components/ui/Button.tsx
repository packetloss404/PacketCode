import type { ButtonHTMLAttributes, ReactNode } from "react";

type ButtonVariant = "default" | "ghost" | "danger" | "green" | "blue" | "purple" | "amber";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: "xs" | "sm" | "md";
  children: ReactNode;
}

const variants: Record<ButtonVariant, string> = {
  default: "bg-bg-elevated text-text-primary hover:bg-bg-hover border border-bg-border",
  ghost: "text-text-secondary hover:text-text-primary hover:bg-bg-hover",
  danger: "bg-accent-red/15 text-accent-red border border-accent-red/30 hover:bg-accent-red/25",
  green: "bg-accent-green/15 text-accent-green border border-accent-green/30 hover:bg-accent-green/25",
  blue: "bg-accent-blue/15 text-accent-blue border border-accent-blue/30 hover:bg-accent-blue/25",
  purple: "bg-accent-purple/15 text-accent-purple border border-accent-purple/30 hover:bg-accent-purple/25",
  amber: "bg-accent-amber/15 text-accent-amber border border-accent-amber/30 hover:bg-accent-amber/25",
};

const sizes = {
  xs: "px-2 py-1 text-[11px]",
  sm: "px-3 py-1.5 text-xs",
  md: "px-4 py-2 text-sm",
};

export function Button({
  variant = "default",
  size = "sm",
  children,
  className = "",
  ...props
}: ButtonProps) {
  return (
    <button
      className={`inline-flex items-center justify-center gap-1.5 rounded transition-colors font-medium disabled:opacity-40 disabled:cursor-not-allowed ${variants[variant]} ${sizes[size]} ${className}`}
      {...props}
    >
      {children}
    </button>
  );
}
