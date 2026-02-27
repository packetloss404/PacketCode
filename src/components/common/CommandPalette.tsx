import { useState, useEffect, useRef, useMemo } from "react";
import { Search, MessageSquare, Ticket, Clock, Wrench, Github, Brain, BarChart3, Rocket, Zap, DollarSign } from "lucide-react";
import { useAppStore, moduleViewId } from "@/stores/appStore";
import { useModuleStore } from "@/stores/moduleStore";
import { getModulesSorted } from "@/modules/registry";

interface PaletteAction {
  id: string;
  label: string;
  description?: string;
  icon: React.ReactNode;
  action: () => void;
  keywords?: string[];
}

export function CommandPalette() {
  const [query, setQuery] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const [selectedIndex, setSelectedIndex] = useState(0);

  const setActiveView = useAppStore((s) => s.setActiveView);
  const setCommandPaletteOpen = useAppStore((s) => s.setCommandPaletteOpen);
  const quickStartSession = useAppStore((s) => s.quickStartSession);
  const moduleStates = useModuleStore((s) => s.states);

  const actions = useMemo<PaletteAction[]>(() => {
    const items: PaletteAction[] = [
      {
        id: "quick-claude",
        label: "Quick Claude Session",
        description: "Start a new Claude session with defaults",
        icon: <Zap size={14} className="text-accent-green" />,
        action: () => quickStartSession("claude"),
        keywords: ["new", "session", "quick", "claude"],
      },
      {
        id: "quick-codex",
        label: "Quick Codex Session",
        description: "Start a new Codex session with defaults",
        icon: <Zap size={14} className="text-accent-blue" />,
        action: () => quickStartSession("codex"),
        keywords: ["new", "session", "quick", "codex"],
      },
      {
        id: "sessions",
        label: "Sessions",
        description: "View active sessions",
        icon: <MessageSquare size={14} className="text-accent-green" />,
        action: () => setActiveView("claude"),
        keywords: ["claude", "codex", "terminal", "pane"],
      },
      {
        id: "issues",
        label: "Issues Board",
        description: "Kanban issue tracker",
        icon: <Ticket size={14} className="text-accent-amber" />,
        action: () => setActiveView("issues"),
        keywords: ["kanban", "tickets", "board", "todo"],
      },
      {
        id: "history",
        label: "Session History",
        description: "Browse past sessions",
        icon: <Clock size={14} className="text-text-secondary" />,
        action: () => setActiveView("history"),
        keywords: ["past", "log", "previous"],
      },
      {
        id: "insights",
        label: "Insights Chat",
        description: "AI-powered project insights",
        icon: <MessageSquare size={14} className="text-accent-blue" />,
        action: () => setActiveView("insights"),
        keywords: ["chat", "ai", "ask", "question"],
      },
      {
        id: "github",
        label: "GitHub",
        description: "GitHub integration",
        icon: <Github size={14} className="text-text-primary" />,
        action: () => setActiveView("github"),
        keywords: ["git", "repo", "pr", "pull request"],
      },
      {
        id: "memory",
        label: "Memory",
        description: "AI memory and file map",
        icon: <Brain size={14} className="text-accent-purple" />,
        action: () => setActiveView("memory"),
        keywords: ["context", "knowledge", "files"],
      },
      {
        id: "analytics",
        label: "Analytics",
        description: "Session analytics dashboard",
        icon: <BarChart3 size={14} className="text-accent-green" />,
        action: () => setActiveView("analytics"),
        keywords: ["stats", "usage", "metrics"],
      },
      {
        id: "deploy",
        label: "Deploy Pipeline",
        description: "Deployment configuration",
        icon: <Rocket size={14} className="text-accent-amber" />,
        action: () => setActiveView("deploy"),
        keywords: ["ship", "release", "ci", "cd"],
      },
      {
        id: "cost",
        label: "Cost Dashboard",
        description: "Track API usage costs",
        icon: <DollarSign size={14} className="text-accent-amber" />,
        action: () => setActiveView("cost"),
        keywords: ["money", "spending", "budget", "usage"],
      },
      {
        id: "settings",
        label: "Settings",
        description: "Project and app settings",
        icon: <Wrench size={14} className="text-text-muted" />,
        action: () => setActiveView("tools"),
        keywords: ["config", "preferences", "options"],
      },
    ];

    // Add enabled modules
    for (const mod of getModulesSorted()) {
      if (moduleStates[mod.id]?.enabled) {
        const Icon = mod.icon;
        items.push({
          id: `mod-${mod.id}`,
          label: mod.name,
          description: mod.description,
          icon: <Icon size={14} className={mod.iconColor} />,
          action: () => setActiveView(moduleViewId(mod.id)),
          keywords: [mod.id, mod.category],
        });
      }
    }

    return items;
  }, [setActiveView, quickStartSession, moduleStates]);

  const filtered = useMemo(() => {
    if (!query.trim()) return actions;
    const q = query.toLowerCase();
    return actions.filter(
      (a) =>
        a.label.toLowerCase().includes(q) ||
        a.description?.toLowerCase().includes(q) ||
        a.keywords?.some((k) => k.includes(q))
    );
  }, [actions, query]);

  // Reset selection when filtered list changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [filtered.length]);

  // Auto-focus input
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  function close() {
    setCommandPaletteOpen(false);
  }

  function execute(action: PaletteAction) {
    action.action();
    close();
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Escape") {
      e.preventDefault();
      close();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelectedIndex((i) => Math.min(i + 1, filtered.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelectedIndex((i) => Math.max(i - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (filtered[selectedIndex]) {
        execute(filtered[selectedIndex]);
      }
    }
  }

  // Scroll selected item into view
  useEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const item = list.children[selectedIndex] as HTMLElement;
    if (item) {
      item.scrollIntoView({ block: "nearest" });
    }
  }, [selectedIndex]);

  return (
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center pt-[15vh]"
      onClick={close}
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/50" />

      {/* Palette */}
      <div
        className="relative w-[480px] bg-bg-secondary border border-bg-border rounded-xl shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {/* Search input */}
        <div className="flex items-center gap-2 px-4 py-3 border-b border-bg-border">
          <Search size={14} className="text-text-muted flex-shrink-0" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Type a command..."
            className="flex-1 bg-transparent text-sm text-text-primary placeholder:text-text-muted focus:outline-none"
          />
          <kbd className="text-[10px] text-text-muted bg-bg-elevated px-1.5 py-0.5 rounded border border-bg-border">
            ESC
          </kbd>
        </div>

        {/* Results */}
        <div ref={listRef} className="max-h-[320px] overflow-y-auto py-1">
          {filtered.length === 0 ? (
            <div className="px-4 py-6 text-center text-xs text-text-muted">
              No matching commands
            </div>
          ) : (
            filtered.map((action, i) => (
              <button
                key={action.id}
                onClick={() => execute(action)}
                onMouseEnter={() => setSelectedIndex(i)}
                className={`flex items-center gap-3 w-full px-4 py-2.5 text-left transition-colors ${
                  i === selectedIndex
                    ? "bg-accent-green/10 text-text-primary"
                    : "text-text-secondary hover:bg-bg-hover"
                }`}
              >
                {action.icon}
                <div className="flex-1 min-w-0">
                  <div className="text-xs font-medium truncate">
                    {action.label}
                  </div>
                  {action.description && (
                    <div className="text-[10px] text-text-muted truncate">
                      {action.description}
                    </div>
                  )}
                </div>
              </button>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
