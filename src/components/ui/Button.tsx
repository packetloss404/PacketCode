import type { ButtonHTMLAttributes, ReactNode } from "react";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "default" | "ghost" | "danger" | "accent";
  size?: "sm" | "md";
  children: ReactNode;
}

const variants = {
  default:
    "bg-bg-elevated text-text-primary hover:bg-bg-hover border border-bg-border",
  ghost: "text-text-secondary hover:text-text-primary hover:bg-bg-hover",
  danger: "bg-accent-red/20 text-accent-red hover:bg-accent-red/30",
  accent: "bg-accent-green/20 text-accent-green hover:bg-accent-green/30",
};

const sizes = {
  sm: "px-2 py-1 text-xs",
  md: "px-3 py-1.5 text-sm",
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
      className={`inline-flex items-center justify-center rounded transition-colors font-medium disabled:opacity-40 disabled:cursor-not-allowed ${variants[variant]} ${sizes[size]} ${className}`}
      {...props}
    >
      {children}
    </button>
  );
}
