import { useState } from "react";
import { X, Plus, Trash2 } from "lucide-react";

interface McpServerModalProps {
  onClose: () => void;
  onSave: (
    name: string,
    command: string,
    args: string[],
    env: Record<string, string>,
    scope: "global" | "project"
  ) => void;
  initial?: {
    name: string;
    command: string;
    args: string[];
    env: Record<string, string>;
    scope: "global" | "project";
  };
}

export function McpServerModal({ onClose, onSave, initial }: McpServerModalProps) {
  const [name, setName] = useState(initial?.name ?? "");
  const [command, setCommand] = useState(initial?.command ?? "");
  const [args, setArgs] = useState(initial?.args?.join(" ") ?? "");
  const [scope, setScope] = useState<"global" | "project">(initial?.scope ?? "global");
  const [envPairs, setEnvPairs] = useState<{ key: string; value: string }[]>(
    initial?.env
      ? Object.entries(initial.env).map(([key, value]) => ({ key, value }))
      : []
  );
  const [saving, setSaving] = useState(false);

  const isEdit = !!initial;

  async function handleSave() {
    if (!name.trim() || !command.trim()) return;
    setSaving(true);
    try {
      const env: Record<string, string> = {};
      for (const pair of envPairs) {
        if (pair.key.trim()) env[pair.key.trim()] = pair.value;
      }
      const argList = args
        .trim()
        .split(/\s+/)
        .filter((a) => a.length > 0);
      await onSave(name.trim(), command.trim(), argList, env, scope);
      onClose();
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
      <div className="bg-bg-secondary border border-bg-border rounded-lg w-[480px] max-h-[80vh] overflow-y-auto">
        <div className="flex items-center justify-between px-4 py-3 border-b border-bg-border">
          <h3 className="text-sm font-medium text-text-primary">
            {isEdit ? "Edit MCP Server" : "Add MCP Server"}
          </h3>
          <button onClick={onClose} className="text-text-muted hover:text-text-primary">
            <X size={14} />
          </button>
        </div>

        <div className="p-4 space-y-3">
          <div>
            <label className="block text-[11px] text-text-secondary mb-1">Server Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={isEdit}
              placeholder="e.g. my-mcp-server"
              className="w-full px-3 py-1.5 bg-bg-primary border border-bg-border rounded text-xs text-text-primary focus:outline-none focus:border-accent-green disabled:opacity-50"
            />
          </div>

          <div>
            <label className="block text-[11px] text-text-secondary mb-1">Command</label>
            <input
              type="text"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              placeholder="e.g. npx or node"
              className="w-full px-3 py-1.5 bg-bg-primary border border-bg-border rounded text-xs text-text-primary focus:outline-none focus:border-accent-green"
            />
          </div>

          <div>
            <label className="block text-[11px] text-text-secondary mb-1">
              Arguments (space-separated)
            </label>
            <input
              type="text"
              value={args}
              onChange={(e) => setArgs(e.target.value)}
              placeholder="e.g. -y @modelcontextprotocol/server-filesystem /path"
              className="w-full px-3 py-1.5 bg-bg-primary border border-bg-border rounded text-xs text-text-primary focus:outline-none focus:border-accent-green"
            />
          </div>

          <div>
            <label className="block text-[11px] text-text-secondary mb-1">Scope</label>
            <div className="flex gap-2">
              <button
                onClick={() => setScope("global")}
                className={`px-3 py-1 text-xs rounded transition-colors ${
                  scope === "global"
                    ? "bg-accent-green/20 text-accent-green"
                    : "bg-bg-primary text-text-secondary hover:text-text-primary"
                }`}
              >
                Global
              </button>
              <button
                onClick={() => setScope("project")}
                className={`px-3 py-1 text-xs rounded transition-colors ${
                  scope === "project"
                    ? "bg-accent-blue/20 text-accent-blue"
                    : "bg-bg-primary text-text-secondary hover:text-text-primary"
                }`}
              >
                Project
              </button>
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="text-[11px] text-text-secondary">Environment Variables</label>
              <button
                onClick={() => setEnvPairs([...envPairs, { key: "", value: "" }])}
                className="text-text-muted hover:text-accent-green"
              >
                <Plus size={12} />
              </button>
            </div>
            {envPairs.map((pair, i) => (
              <div key={i} className="flex gap-2 mb-1.5">
                <input
                  type="text"
                  value={pair.key}
                  onChange={(e) => {
                    const next = [...envPairs];
                    next[i] = { ...next[i], key: e.target.value };
                    setEnvPairs(next);
                  }}
                  placeholder="KEY"
                  className="flex-1 px-2 py-1 bg-bg-primary border border-bg-border rounded text-xs text-text-primary focus:outline-none focus:border-accent-green"
                />
                <input
                  type="text"
                  value={pair.value}
                  onChange={(e) => {
                    const next = [...envPairs];
                    next[i] = { ...next[i], value: e.target.value };
                    setEnvPairs(next);
                  }}
                  placeholder="value"
                  className="flex-1 px-2 py-1 bg-bg-primary border border-bg-border rounded text-xs text-text-primary focus:outline-none focus:border-accent-green"
                />
                <button
                  onClick={() => setEnvPairs(envPairs.filter((_, j) => j !== i))}
                  className="text-text-muted hover:text-red-400"
                >
                  <Trash2 size={12} />
                </button>
              </div>
            ))}
          </div>
        </div>

        <div className="flex justify-end gap-2 px-4 py-3 border-t border-bg-border">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-xs text-text-secondary hover:text-text-primary bg-bg-primary rounded transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !name.trim() || !command.trim()}
            className="px-3 py-1.5 text-xs text-bg-primary bg-accent-green rounded hover:bg-accent-green/80 transition-colors disabled:opacity-50"
          >
            {saving ? "Saving..." : isEdit ? "Update" : "Add Server"}
          </button>
        </div>
      </div>
    </div>
  );
}
