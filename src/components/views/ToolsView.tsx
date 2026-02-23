import { Wrench, GitBranch, FolderOpen, Settings, ClipboardList, User, Plus, Pencil, Trash2, Puzzle } from "lucide-react";
import { useLayoutStore } from "@/stores/layoutStore";
import { useGitInfo } from "@/hooks/useGitInfo";
import { useIssueStore } from "@/stores/issueStore";
import { useProfileStore } from "@/stores/profileStore";
import { useModuleStore } from "@/stores/moduleStore";
import { useAppStore, moduleViewId } from "@/stores/appStore";
import { getModulesSorted } from "@/modules/registry";
import { useState } from "react";
import { SpecImportModal } from "./SpecImportModal";

export function ToolsView() {
  const projectPath = useLayoutStore((s) => s.projectPath);
  const gitBranch = useGitInfo();
  const ticketPrefix = useIssueStore((s) => s.ticketPrefix);
  const setTicketPrefix = useIssueStore((s) => s.setTicketPrefix);
  const addEpic = useIssueStore((s) => s.addEpic);
  const addLabel = useIssueStore((s) => s.addLabel);
  const [newEpic, setNewEpic] = useState("");
  const [newLabel, setNewLabel] = useState("");
  const [showSpecImport, setShowSpecImport] = useState(false);

  const profiles = useProfileStore((s) => s.profiles);
  const addProfile = useProfileStore((s) => s.addProfile);
  const updateProfile = useProfileStore((s) => s.updateProfile);
  const deleteProfile = useProfileStore((s) => s.deleteProfile);
  const [editingProfile, setEditingProfile] = useState<string | null>(null);
  const [showNewProfile, setShowNewProfile] = useState(false);
  const [profileForm, setProfileForm] = useState({
    name: "",
    description: "",
    icon: "User",
    color: "text-accent-green",
    systemPrompt: "",
    defaultModel: "",
  });

  return (
    <div className="flex flex-col h-full bg-bg-primary p-4 overflow-y-auto">
      <div className="flex items-center gap-2 mb-4">
        <Wrench size={16} className="text-accent-amber" />
        <h2 className="text-sm font-semibold text-text-primary">Tools</h2>
      </div>

      <div className="grid grid-cols-2 gap-4 max-w-2xl">
        {/* Project Info */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <h3 className="text-xs font-semibold text-text-primary mb-3 flex items-center gap-2">
            <FolderOpen size={12} />
            Project
          </h3>
          <div className="flex flex-col gap-2 text-xs">
            <div>
              <span className="text-text-muted">Path: </span>
              <span className="text-text-secondary">{projectPath}</span>
            </div>
            {gitBranch && (
              <div className="flex items-center gap-1">
                <span className="text-text-muted">Branch: </span>
                <GitBranch size={10} className="text-accent-purple" />
                <span className="text-text-secondary">{gitBranch}</span>
              </div>
            )}
          </div>
        </div>

        {/* Issue Settings */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <h3 className="text-xs font-semibold text-text-primary mb-3 flex items-center gap-2">
            <Settings size={12} />
            Issue Settings
          </h3>
          <div className="flex flex-col gap-2">
            <div>
              <label className="text-[10px] text-text-muted uppercase tracking-wider">
                Ticket Prefix
              </label>
              <input
                type="text"
                value={ticketPrefix}
                onChange={(e) => setTicketPrefix(e.target.value.toUpperCase())}
                className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary focus:outline-none focus:border-accent-green mt-1"
                maxLength={6}
              />
            </div>
          </div>
        </div>

        {/* Epics */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <h3 className="text-xs font-semibold text-text-primary mb-3">
            Epics
          </h3>
          <div className="flex gap-1 mb-2">
            <input
              type="text"
              value={newEpic}
              onChange={(e) => setNewEpic(e.target.value)}
              placeholder="New epic..."
              className="flex-1 bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
              onKeyDown={(e) => {
                if (e.key === "Enter" && newEpic.trim()) {
                  addEpic(newEpic.trim());
                  setNewEpic("");
                }
              }}
            />
            <button
              onClick={() => {
                if (newEpic.trim()) {
                  addEpic(newEpic.trim());
                  setNewEpic("");
                }
              }}
              className="px-2 py-1 text-xs text-accent-green hover:bg-accent-green/15 rounded transition-colors"
            >
              Add
            </button>
          </div>
          <div className="flex flex-wrap gap-1">
            {useIssueStore.getState().epics.map((e) => (
              <span
                key={e}
                className="text-[10px] px-1.5 py-0.5 bg-accent-purple/15 text-accent-purple rounded"
              >
                {e}
              </span>
            ))}
          </div>
        </div>

        {/* Labels */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <h3 className="text-xs font-semibold text-text-primary mb-3">
            Labels
          </h3>
          <div className="flex gap-1 mb-2">
            <input
              type="text"
              value={newLabel}
              onChange={(e) => setNewLabel(e.target.value)}
              placeholder="New label..."
              className="flex-1 bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
              onKeyDown={(e) => {
                if (e.key === "Enter" && newLabel.trim()) {
                  addLabel(newLabel.trim());
                  setNewLabel("");
                }
              }}
            />
            <button
              onClick={() => {
                if (newLabel.trim()) {
                  addLabel(newLabel.trim());
                  setNewLabel("");
                }
              }}
              className="px-2 py-1 text-xs text-accent-green hover:bg-accent-green/15 rounded transition-colors"
            >
              Add
            </button>
          </div>
          <div className="flex flex-wrap gap-1">
            {useIssueStore.getState().labels.map((l) => (
              <span
                key={l}
                className="text-[10px] px-1.5 py-0.5 bg-bg-elevated text-text-muted rounded"
              >
                {l}
              </span>
            ))}
          </div>
        </div>

        {/* Spec2Tick */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <h3 className="text-xs font-semibold text-text-primary mb-3 flex items-center gap-2">
            <ClipboardList size={12} className="text-accent-green" />
            Spec2Tick
          </h3>
          <p className="text-[10px] text-text-muted mb-3">
            Paste any project spec and let Claude parse it into structured
            tickets on the Issues board.
          </p>
          <button
            onClick={() => setShowSpecImport(true)}
            className="px-3 py-1.5 text-[11px] font-medium rounded bg-accent-green text-bg-primary hover:opacity-90 transition-opacity"
          >
            Import Spec
          </button>
        </div>

        {/* Agent Profiles — spans full width */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4 col-span-2">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-xs font-semibold text-text-primary flex items-center gap-2">
              <User size={12} className="text-accent-purple" />
              Agent Profiles
            </h3>
            <button
              onClick={() => {
                setShowNewProfile(true);
                setProfileForm({ name: "", description: "", icon: "User", color: "text-accent-green", systemPrompt: "", defaultModel: "" });
              }}
              className="flex items-center gap-1 px-2 py-1 text-[10px] text-accent-green hover:bg-accent-green/15 rounded transition-colors"
            >
              <Plus size={10} />
              Create Profile
            </button>
          </div>

          {/* Profile list */}
          <div className="flex flex-col gap-2 mb-3">
            {profiles.map((p) => (
              <div
                key={p.id}
                className="flex items-center gap-3 px-3 py-2 bg-bg-primary border border-bg-border rounded"
              >
                <span className={`${p.color}`}>
                  <User size={12} />
                </span>
                <div className="flex-1 min-w-0">
                  <div className="text-[11px] text-text-primary font-medium">
                    {p.name}
                  </div>
                  <div className="text-[10px] text-text-muted truncate">
                    {p.description}
                  </div>
                </div>
                {p.isBuiltin && (
                  <span className="text-[9px] text-text-muted px-1.5 py-0.5 bg-bg-elevated rounded">
                    builtin
                  </span>
                )}
                <button
                  onClick={() => {
                    setEditingProfile(p.id);
                    setProfileForm({
                      name: p.name,
                      description: p.description,
                      icon: p.icon,
                      color: p.color,
                      systemPrompt: p.systemPrompt,
                      defaultModel: p.defaultModel,
                    });
                  }}
                  className="p-1 text-text-muted hover:text-text-primary transition-colors"
                  title="Edit"
                >
                  <Pencil size={10} />
                </button>
                {!p.isBuiltin && (
                  <button
                    onClick={() => deleteProfile(p.id)}
                    className="p-1 text-text-muted hover:text-accent-red transition-colors"
                    title="Delete"
                  >
                    <Trash2 size={10} />
                  </button>
                )}
              </div>
            ))}
          </div>

          {/* Create / Edit form */}
          {(showNewProfile || editingProfile) && (
            <ProfileForm
              form={profileForm}
              onChange={setProfileForm}
              onSave={() => {
                if (editingProfile) {
                  updateProfile(editingProfile, profileForm);
                  setEditingProfile(null);
                } else {
                  addProfile(profileForm);
                  setShowNewProfile(false);
                }
                setProfileForm({ name: "", description: "", icon: "User", color: "text-accent-green", systemPrompt: "", defaultModel: "" });
              }}
              onCancel={() => {
                setEditingProfile(null);
                setShowNewProfile(false);
              }}
              isEditing={!!editingProfile}
            />
          )}
        </div>

        {/* Modules — spans full width */}
        <ModulesCard />
      </div>

      {showSpecImport && (
        <SpecImportModal onClose={() => setShowSpecImport(false)} />
      )}
    </div>
  );
}

const CATEGORY_LABELS: Record<string, string> = {
  ai: "AI",
  analysis: "Analysis",
  integration: "Integration",
  utility: "Utility",
};

const CATEGORY_COLORS: Record<string, string> = {
  ai: "bg-accent-purple/15 text-accent-purple",
  analysis: "bg-accent-amber/15 text-accent-amber",
  integration: "bg-accent-blue/15 text-accent-blue",
  utility: "bg-bg-elevated text-text-muted",
};

function ModulesCard() {
  const modules = getModulesSorted();
  const { states, toggleModule } = useModuleStore();
  const activeView = useAppStore((s) => s.activeView);
  const setActiveView = useAppStore((s) => s.setActiveView);

  const enabledCount = modules.filter((mod) => states[mod.id]?.enabled).length;

  return (
    <div className="bg-bg-secondary border border-bg-border rounded-lg p-4 col-span-2">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-xs font-semibold text-text-primary flex items-center gap-2">
          <Puzzle size={12} className="text-accent-blue" />
          Modules
        </h3>
        <span className="text-[10px] text-text-muted px-1.5 py-0.5 bg-bg-elevated rounded">
          {enabledCount} of {modules.length} enabled
        </span>
      </div>

      <div className="flex flex-col gap-2">
        {modules.map((mod) => {
          const enabled = states[mod.id]?.enabled ?? false;
          const Icon = mod.icon;
          return (
            <div
              key={mod.id}
              className="flex items-center gap-3 px-3 py-2 bg-bg-primary border border-bg-border rounded"
            >
              <span className={enabled ? mod.iconColor : "text-text-muted"}>
                <Icon size={12} />
              </span>
              <div className="flex-1 min-w-0">
                <div className="text-[11px] text-text-primary font-medium flex items-center gap-2">
                  {mod.name}
                  <span className={`text-[9px] px-1.5 py-0.5 rounded ${CATEGORY_COLORS[mod.category] ?? "bg-bg-elevated text-text-muted"}`}>
                    {CATEGORY_LABELS[mod.category] ?? mod.category}
                  </span>
                </div>
                <div className="text-[10px] text-text-muted truncate">
                  {mod.description}
                  {mod.shortcutHint && (
                    <span className="ml-2 text-text-muted opacity-60">({mod.shortcutHint})</span>
                  )}
                </div>
              </div>
              <button
                onClick={() => {
                  toggleModule(mod.id);
                  // If disabling the currently active module, redirect to tools
                  if (enabled && activeView === moduleViewId(mod.id)) {
                    setActiveView("tools");
                  }
                }}
                className={`relative w-8 h-4 rounded-full transition-colors ${
                  enabled ? "bg-accent-green" : "bg-bg-elevated"
                }`}
                title={enabled ? "Disable" : "Enable"}
              >
                <span
                  className={`absolute top-0.5 w-3 h-3 rounded-full bg-white transition-transform ${
                    enabled ? "left-4" : "left-0.5"
                  }`}
                />
              </button>
            </div>
          );
        })}
      </div>
    </div>
  );
}

const COLOR_OPTIONS = [
  { label: "Green", value: "text-accent-green" },
  { label: "Blue", value: "text-accent-blue" },
  { label: "Purple", value: "text-accent-purple" },
  { label: "Amber", value: "text-accent-amber" },
  { label: "Red", value: "text-accent-red" },
];

const ICON_OPTIONS = ["User", "Zap", "Rocket", "Search", "Shield", "RefreshCw", "Code", "Brain", "Star", "Target"];

interface ProfileFormData {
  name: string;
  description: string;
  icon: string;
  color: string;
  systemPrompt: string;
  defaultModel: string;
}

function ProfileForm({
  form,
  onChange,
  onSave,
  onCancel,
  isEditing,
}: {
  form: ProfileFormData;
  onChange: (form: ProfileFormData) => void;
  onSave: () => void;
  onCancel: () => void;
  isEditing: boolean;
}) {
  return (
    <div className="bg-bg-elevated border border-bg-border rounded-lg p-3 flex flex-col gap-2">
      <div className="text-[10px] text-text-muted uppercase tracking-wider font-medium">
        {isEditing ? "Edit Profile" : "New Profile"}
      </div>
      <div className="flex gap-2">
        <input
          type="text"
          value={form.name}
          onChange={(e) => onChange({ ...form, name: e.target.value })}
          placeholder="Profile name..."
          className="flex-1 bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
        />
        <select
          value={form.icon}
          onChange={(e) => onChange({ ...form, icon: e.target.value })}
          className="bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary focus:outline-none focus:border-accent-green"
        >
          {ICON_OPTIONS.map((icon) => (
            <option key={icon} value={icon}>
              {icon}
            </option>
          ))}
        </select>
        <select
          value={form.color}
          onChange={(e) => onChange({ ...form, color: e.target.value })}
          className="bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary focus:outline-none focus:border-accent-green"
        >
          {COLOR_OPTIONS.map((c) => (
            <option key={c.value} value={c.value}>
              {c.label}
            </option>
          ))}
        </select>
      </div>
      <input
        type="text"
        value={form.description}
        onChange={(e) => onChange({ ...form, description: e.target.value })}
        placeholder="Short description..."
        className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
      />
      <textarea
        value={form.systemPrompt}
        onChange={(e) => onChange({ ...form, systemPrompt: e.target.value })}
        placeholder="System prompt (prepended to initial prompt)..."
        rows={3}
        className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green resize-none"
      />
      <input
        type="text"
        value={form.defaultModel}
        onChange={(e) => onChange({ ...form, defaultModel: e.target.value })}
        placeholder="Default model (leave empty for system default)..."
        className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
      />
      <div className="flex gap-2 mt-1">
        <button
          onClick={onSave}
          disabled={!form.name.trim()}
          className="px-3 py-1 text-[11px] bg-accent-green/15 text-accent-green rounded hover:bg-accent-green/25 transition-colors disabled:opacity-50"
        >
          {isEditing ? "Save Changes" : "Create"}
        </button>
        <button
          onClick={onCancel}
          className="px-3 py-1 text-[11px] text-text-muted hover:text-text-primary transition-colors"
        >
          Cancel
        </button>
      </div>
    </div>
  );
}
