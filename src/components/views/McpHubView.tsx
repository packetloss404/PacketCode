import { useEffect, useState } from "react";
import { Plug, Plus, Pencil, Trash2, Globe, FolderOpen, RefreshCw } from "lucide-react";
import { useMcpStore } from "@/stores/mcpStore";
import { McpServerModal } from "./McpServerModal";
import type { McpServerEntry } from "@/types/mcp";

export function McpHubView() {
  const { servers, loading, error, fetchServers, addServer, updateServer, removeServer } =
    useMcpStore();
  const [showModal, setShowModal] = useState(false);
  const [editEntry, setEditEntry] = useState<McpServerEntry | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);

  useEffect(() => {
    fetchServers();
  }, [fetchServers]);

  const globalServers = servers.filter((s) => s.scope === "global");
  const projectServers = servers.filter((s) => s.scope === "project");

  function handleEdit(entry: McpServerEntry) {
    setEditEntry(entry);
    setShowModal(true);
  }

  function handleAdd() {
    setEditEntry(null);
    setShowModal(true);
  }

  async function handleSave(
    name: string,
    command: string,
    args: string[],
    env: Record<string, string>,
    scope: "global" | "project"
  ) {
    if (editEntry) {
      await updateServer(name, command, args, env, scope);
    } else {
      await addServer(name, command, args, env, scope);
    }
  }

  async function handleDelete(name: string, scope: "global" | "project") {
    await removeServer(name, scope);
    setDeleteConfirm(null);
  }

  return (
    <div className="flex flex-col h-full bg-bg-primary overflow-y-auto">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-bg-border bg-bg-secondary">
        <div className="flex items-center gap-2">
          <Plug size={14} className="text-accent-blue" />
          <h2 className="text-sm font-medium text-text-primary">MCP Servers</h2>
          <span className="text-[10px] text-text-muted px-1.5 py-0.5 bg-bg-elevated rounded">
            {servers.length} server{servers.length !== 1 ? "s" : ""}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchServers()}
            className="p-1 text-text-muted hover:text-text-primary transition-colors"
            title="Refresh"
          >
            <RefreshCw size={12} className={loading ? "animate-spin" : ""} />
          </button>
          <button
            onClick={handleAdd}
            className="flex items-center gap-1.5 px-2.5 py-1 text-xs bg-accent-green/20 text-accent-green rounded hover:bg-accent-green/30 transition-colors"
          >
            <Plus size={12} />
            Add Server
          </button>
        </div>
      </div>

      {error && (
        <div className="mx-4 mt-3 px-3 py-2 bg-red-500/10 border border-red-500/20 rounded text-xs text-red-400">
          {error}
        </div>
      )}

      <div className="p-4 space-y-4">
        {/* Global Servers */}
        <ServerGroup
          title="Global"
          icon={<Globe size={12} className="text-accent-green" />}
          servers={globalServers}
          onEdit={handleEdit}
          onDelete={handleDelete}
          deleteConfirm={deleteConfirm}
          setDeleteConfirm={setDeleteConfirm}
        />

        {/* Project Servers */}
        <ServerGroup
          title="Project"
          icon={<FolderOpen size={12} className="text-accent-blue" />}
          servers={projectServers}
          onEdit={handleEdit}
          onDelete={handleDelete}
          deleteConfirm={deleteConfirm}
          setDeleteConfirm={setDeleteConfirm}
        />

        {servers.length === 0 && !loading && (
          <div className="text-center py-12 text-text-muted text-xs">
            <Plug size={24} className="mx-auto mb-3 opacity-30" />
            <p>No MCP servers configured</p>
            <p className="mt-1">
              Add servers to extend Claude Code with custom tools
            </p>
          </div>
        )}
      </div>

      {showModal && (
        <McpServerModal
          onClose={() => {
            setShowModal(false);
            setEditEntry(null);
          }}
          onSave={handleSave}
          initial={
            editEntry
              ? {
                  name: editEntry.name,
                  command: editEntry.config.command,
                  args: editEntry.config.args ?? [],
                  env: editEntry.config.env ?? {},
                  scope: editEntry.scope as "global" | "project",
                }
              : undefined
          }
        />
      )}
    </div>
  );
}

function ServerGroup({
  title,
  icon,
  servers,
  onEdit,
  onDelete,
  deleteConfirm,
  setDeleteConfirm,
}: {
  title: string;
  icon: React.ReactNode;
  servers: McpServerEntry[];
  onEdit: (entry: McpServerEntry) => void;
  onDelete: (name: string, scope: "global" | "project") => void;
  deleteConfirm: string | null;
  setDeleteConfirm: (key: string | null) => void;
}) {
  if (servers.length === 0) return null;

  return (
    <div>
      <div className="flex items-center gap-1.5 mb-2">
        {icon}
        <h3 className="text-[11px] font-medium text-text-secondary uppercase tracking-wider">
          {title}
        </h3>
        <span className="text-[10px] text-text-muted">({servers.length})</span>
      </div>
      <div className="space-y-1.5">
        {servers.map((entry) => {
          const key = `${entry.scope}:${entry.name}`;
          return (
            <div
              key={key}
              className="flex items-center gap-3 px-3 py-2 bg-bg-secondary border border-bg-border rounded-lg group"
            >
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium text-text-primary">{entry.name}</span>
                  {entry.disabled && (
                    <span className="text-[9px] px-1 py-0.5 bg-bg-elevated text-text-muted rounded">
                      disabled
                    </span>
                  )}
                </div>
                <div className="text-[11px] text-text-muted mt-0.5 truncate">
                  {entry.config.command}
                  {entry.config.args?.length
                    ? ` ${entry.config.args.join(" ")}`
                    : ""}
                </div>
              </div>
              <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                <button
                  onClick={() => onEdit(entry)}
                  className="p-1 text-text-muted hover:text-accent-blue transition-colors"
                  title="Edit"
                >
                  <Pencil size={12} />
                </button>
                {deleteConfirm === key ? (
                  <button
                    onClick={() => onDelete(entry.name, entry.scope as "global" | "project")}
                    className="px-2 py-0.5 text-[10px] bg-red-500/20 text-red-400 rounded hover:bg-red-500/30 transition-colors"
                  >
                    Confirm
                  </button>
                ) : (
                  <button
                    onClick={() => setDeleteConfirm(key)}
                    className="p-1 text-text-muted hover:text-red-400 transition-colors"
                    title="Delete"
                  >
                    <Trash2 size={12} />
                  </button>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
