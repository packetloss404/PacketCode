import { useState } from "react";

interface TagListCardProps {
  title: string;
  items: string[];
  onAdd: (item: string) => void;
  tagClassName: string;
  placeholder: string;
}

export function TagListCard({ title, items, onAdd, tagClassName, placeholder }: TagListCardProps) {
  const [value, setValue] = useState("");

  function handleAdd() {
    if (value.trim()) {
      onAdd(value.trim());
      setValue("");
    }
  }

  return (
    <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
      <h3 className="text-xs font-semibold text-text-primary mb-3">
        {title}
      </h3>
      <div className="flex gap-1 mb-2">
        <input
          type="text"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder={placeholder}
          className="flex-1 bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
          onKeyDown={(e) => { if (e.key === "Enter") handleAdd(); }}
        />
        <button
          onClick={handleAdd}
          className="px-2 py-1 text-xs text-accent-green hover:bg-accent-green/15 rounded transition-colors"
        >
          Add
        </button>
      </div>
      <div className="flex flex-wrap gap-1">
        {items.map((item) => (
          <span key={item} className={`text-[10px] px-1.5 py-0.5 rounded ${tagClassName}`}>
            {item}
          </span>
        ))}
      </div>
    </div>
  );
}
