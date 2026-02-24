import type { ReactNode } from "react";

interface DropdownItemProps {
  icon: ReactNode;
  label: string;
  onClick: () => void;
  badge?: string;
}

export function DropdownItem({ icon, label, onClick, badge }: DropdownItemProps) {
  return (
    <button
      onClick={onClick}
      className="flex items-center gap-2 w-full px-3 py-1.5 text-[11px] text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors text-left"
    >
      {icon}
      {label}
      {badge && <span className="ml-auto text-[9px] text-accent-green">{badge}</span>}
    </button>
  );
}
