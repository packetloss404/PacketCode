import { useState } from "react";
import { X } from "lucide-react";
import type { DeployConfig } from "@/types/deploy";

interface DeployConfigModalProps {
  onClose: () => void;
  onSave: (config: DeployConfig) => void;
  initial?: DeployConfig;
}

export function DeployConfigModal({ onClose, onSave, initial }: DeployConfigModalProps) {
  const [name, setName] = useState(initial?.name ?? "");
  const [command, setCommand] = useState(initial?.command ?? "");

  function handleSave() {
    if (!name.trim() || !command.trim()) return;
    onSave({ name: name.trim(), command: command.trim() });
    onClose();
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
      <div className="bg-bg-secondary border border-bg-border rounded-lg w-[420px]">
        <div className="flex items-center justify-between px-4 py-3 border-b border-bg-border">
          <h3 className="text-sm font-medium text-text-primary">
            {initial ? "Edit Deploy Config" : "Add Deploy Config"}
          </h3>
          <button onClick={onClose} className="text-text-muted hover:text-text-primary">
            <X size={14} />
          </button>
        </div>

        <div className="p-4 space-y-3">
          <div>
            <label className="block text-[11px] text-text-secondary mb-1">Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Production Deploy"
              className="w-full px-3 py-1.5 bg-bg-primary border border-bg-border rounded text-xs text-text-primary focus:outline-none focus:border-accent-green"
              autoFocus
            />
          </div>

          <div>
            <label className="block text-[11px] text-text-secondary mb-1">Command</label>
            <input
              type="text"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              placeholder="e.g. npm run build && npx vercel --prod"
              className="w-full px-3 py-1.5 bg-bg-primary border border-bg-border rounded text-xs text-text-primary focus:outline-none focus:border-accent-green"
            />
            <p className="text-[10px] text-text-muted mt-1">
              Shell command to execute. Runs in the project directory.
            </p>
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
            disabled={!name.trim() || !command.trim()}
            className="px-3 py-1.5 text-xs text-bg-primary bg-accent-green rounded hover:bg-accent-green/80 transition-colors disabled:opacity-50"
          >
            {initial ? "Update" : "Add Config"}
          </button>
        </div>
      </div>
    </div>
  );
}
