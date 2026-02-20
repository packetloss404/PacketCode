import { useState, useEffect, useRef, useCallback } from "react";
import {
  X,
  FolderOpen,
  Folder,
  FileText,
  FileCode,
  FileJson,
  FileType,
  ChevronRight,
  ChevronDown,
  Image,
  Database,
  Settings,
  File,
} from "lucide-react";
import { invoke } from "@tauri-apps/api/core";
import { useLayoutStore } from "@/stores/layoutStore";

interface DirEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  extension: string | null;
}

interface FileExplorerProps {
  onClose: () => void;
}

function getFileIcon(ext: string | null, size: number) {
  if (!ext) return <File size={size} className="text-text-muted" />;
  switch (ext.toLowerCase()) {
    case "ts":
    case "tsx":
      return <FileCode size={size} className="text-accent-blue" />;
    case "js":
    case "jsx":
    case "mjs":
      return <FileCode size={size} className="text-[#f7df1e]" />;
    case "rs":
      return <FileCode size={size} className="text-[#dea584]" />;
    case "py":
      return <FileCode size={size} className="text-[#3572A5]" />;
    case "json":
      return <FileJson size={size} className="text-accent-amber" />;
    case "md":
    case "mdx":
      return <FileText size={size} className="text-accent-blue" />;
    case "html":
    case "htm":
      return <FileCode size={size} className="text-[#e34c26]" />;
    case "css":
    case "scss":
    case "sass":
      return <FileCode size={size} className="text-[#563d7c]" />;
    case "sql":
      return <Database size={size} className="text-accent-amber" />;
    case "png":
    case "jpg":
    case "jpeg":
    case "gif":
    case "svg":
    case "ico":
      return <Image size={size} className="text-accent-green" />;
    case "toml":
    case "yaml":
    case "yml":
      return <Settings size={size} className="text-text-muted" />;
    case "lock":
      return <File size={size} className="text-text-muted" />;
    default:
      return <FileType size={size} className="text-text-muted" />;
  }
}

function formatSize(bytes: number): string {
  if (bytes === 0) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// ─── Tree Node ──────────────────────────────────────────────

function TreeNode({ entry, depth }: { entry: DirEntry; depth: number }) {
  const [expanded, setExpanded] = useState(false);
  const [children, setChildren] = useState<DirEntry[] | null>(null);
  const [loading, setLoading] = useState(false);

  async function toggleExpand() {
    if (!entry.is_dir) return;

    if (!expanded && children === null) {
      setLoading(true);
      try {
        const entries = await invoke<DirEntry[]>("list_directory", {
          dirPath: entry.path,
        });
        setChildren(entries);
      } catch {
        setChildren([]);
      }
      setLoading(false);
    }
    setExpanded(!expanded);
  }

  const paddingLeft = 8 + depth * 14;

  return (
    <>
      <div
        className={`flex items-center gap-1.5 py-[3px] pr-2 cursor-pointer hover:bg-bg-hover transition-colors group`}
        style={{ paddingLeft }}
        onClick={toggleExpand}
      >
        {/* Expand chevron or spacer */}
        {entry.is_dir ? (
          <span className="flex-shrink-0 w-3">
            {expanded ? (
              <ChevronDown size={10} className="text-text-muted" />
            ) : (
              <ChevronRight size={10} className="text-text-muted" />
            )}
          </span>
        ) : (
          <span className="w-3 flex-shrink-0" />
        )}

        {/* Icon */}
        <span className="flex-shrink-0">
          {entry.is_dir ? (
            expanded ? (
              <FolderOpen size={13} className="text-accent-amber" />
            ) : (
              <Folder size={13} className="text-accent-amber" />
            )
          ) : (
            getFileIcon(entry.extension, 13)
          )}
        </span>

        {/* Name */}
        <span className="text-[11px] text-text-secondary truncate flex-1">
          {entry.name}
        </span>

        {/* Size for files */}
        {!entry.is_dir && entry.size > 0 && (
          <span className="text-[9px] text-text-muted opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0">
            {formatSize(entry.size)}
          </span>
        )}
      </div>

      {/* Children */}
      {expanded && entry.is_dir && (
        <>
          {loading && (
            <div
              className="text-[10px] text-text-muted py-1 animate-pulse"
              style={{ paddingLeft: paddingLeft + 18 }}
            >
              Loading...
            </div>
          )}
          {children && children.length === 0 && !loading && (
            <div
              className="text-[10px] text-text-muted py-1"
              style={{ paddingLeft: paddingLeft + 18 }}
            >
              Empty
            </div>
          )}
          {children &&
            children.map((child) => (
              <TreeNode key={child.path} entry={child} depth={depth + 1} />
            ))}
        </>
      )}
    </>
  );
}

// ─── Main Panel ─────────────────────────────────────────────

export function FileExplorer({ onClose }: FileExplorerProps) {
  const projectPath = useLayoutStore((s) => s.projectPath);
  const [rootEntries, setRootEntries] = useState<DirEntry[] | null>(null);
  const [loading, setLoading] = useState(true);

  // Dragging state
  const panelRef = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState({ x: 12, y: 80 });
  const dragRef = useRef<{ startX: number; startY: number; origX: number; origY: number } | null>(null);

  useEffect(() => {
    setLoading(true);
    invoke<DirEntry[]>("list_directory", { dirPath: projectPath })
      .then((entries) => {
        setRootEntries(entries);
        setLoading(false);
      })
      .catch(() => {
        setRootEntries([]);
        setLoading(false);
      });
  }, [projectPath]);

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    // Only drag from the title bar area
    if ((e.target as HTMLElement).closest("[data-drag-handle]")) {
      e.preventDefault();
      dragRef.current = {
        startX: e.clientX,
        startY: e.clientY,
        origX: position.x,
        origY: position.y,
      };
    }
  }, [position]);

  useEffect(() => {
    function handleMouseMove(e: MouseEvent) {
      if (!dragRef.current) return;
      const dx = e.clientX - dragRef.current.startX;
      const dy = e.clientY - dragRef.current.startY;
      setPosition({
        x: dragRef.current.origX + dx,
        y: dragRef.current.origY + dy,
      });
    }

    function handleMouseUp() {
      dragRef.current = null;
    }

    window.addEventListener("mousemove", handleMouseMove);
    window.addEventListener("mouseup", handleMouseUp);
    return () => {
      window.removeEventListener("mousemove", handleMouseMove);
      window.removeEventListener("mouseup", handleMouseUp);
    };
  }, []);

  const projectName = projectPath.split(/[/\\]/).pop() || "Project";

  return (
    <div
      ref={panelRef}
      className="fixed z-40 flex flex-col bg-bg-secondary border border-bg-border rounded-lg shadow-2xl overflow-hidden"
      style={{
        left: position.x,
        top: position.y,
        width: 280,
        height: "calc(100vh - 120px)",
        maxHeight: 700,
      }}
      onMouseDown={handleMouseDown}
    >
      {/* Title bar — drag handle */}
      <div
        data-drag-handle
        className="flex items-center justify-between px-3 py-2 bg-bg-tertiary border-b border-bg-border cursor-move select-none"
      >
        <div className="flex items-center gap-2">
          <FolderOpen size={13} className="text-accent-amber" />
          <span className="text-xs font-medium text-text-primary truncate">
            {projectName}
          </span>
        </div>
        <button
          onClick={onClose}
          className="p-0.5 text-text-muted hover:text-text-primary transition-colors"
        >
          <X size={14} />
        </button>
      </div>

      {/* Tree content */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden py-1">
        {loading && (
          <div className="text-[10px] text-text-muted text-center py-6 animate-pulse">
            Loading files...
          </div>
        )}
        {rootEntries && rootEntries.length === 0 && !loading && (
          <div className="text-[10px] text-text-muted text-center py-6">
            No files found
          </div>
        )}
        {rootEntries &&
          rootEntries.map((entry) => (
            <TreeNode key={entry.path} entry={entry} depth={0} />
          ))}
      </div>

      {/* Footer */}
      <div className="px-3 py-1.5 border-t border-bg-border">
        <span className="text-[9px] text-text-muted truncate block" title={projectPath}>
          {projectPath}
        </span>
      </div>
    </div>
  );
}
