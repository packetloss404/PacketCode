import { useState } from "react";
import { User, Plus, Pencil, Trash2 } from "lucide-react";
import { useProfileStore } from "@/stores/profileStore";

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

const EMPTY_FORM: ProfileFormData = { name: "", description: "", icon: "User", color: "text-accent-green", systemPrompt: "", defaultModel: "" };

export function AgentProfilesCard() {
  const profiles = useProfileStore((s) => s.profiles);
  const addProfile = useProfileStore((s) => s.addProfile);
  const updateProfile = useProfileStore((s) => s.updateProfile);
  const deleteProfile = useProfileStore((s) => s.deleteProfile);

  const [editingProfile, setEditingProfile] = useState<string | null>(null);
  const [showNewProfile, setShowNewProfile] = useState(false);
  const [form, setForm] = useState<ProfileFormData>(EMPTY_FORM);

  function handleSave() {
    if (editingProfile) {
      updateProfile(editingProfile, form);
      setEditingProfile(null);
    } else {
      addProfile(form);
      setShowNewProfile(false);
    }
    setForm(EMPTY_FORM);
  }

  function handleCancel() {
    setEditingProfile(null);
    setShowNewProfile(false);
  }

  return (
    <div className="bg-bg-secondary border border-bg-border rounded-lg p-4 col-span-2">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-xs font-semibold text-text-primary flex items-center gap-2">
          <User size={12} className="text-accent-purple" />
          Agent Profiles
        </h3>
        <button
          onClick={() => { setShowNewProfile(true); setForm(EMPTY_FORM); }}
          className="flex items-center gap-1 px-2 py-1 text-[10px] text-accent-green hover:bg-accent-green/15 rounded transition-colors"
        >
          <Plus size={10} />
          Create Profile
        </button>
      </div>

      <div className="flex flex-col gap-2 mb-3">
        {profiles.map((p) => (
          <div key={p.id} className="flex items-center gap-3 px-3 py-2 bg-bg-primary border border-bg-border rounded">
            <span className={p.color}><User size={12} /></span>
            <div className="flex-1 min-w-0">
              <div className="text-[11px] text-text-primary font-medium">{p.name}</div>
              <div className="text-[10px] text-text-muted truncate">{p.description}</div>
            </div>
            {p.isBuiltin && (
              <span className="text-[9px] text-text-muted px-1.5 py-0.5 bg-bg-elevated rounded">builtin</span>
            )}
            <button
              onClick={() => {
                setEditingProfile(p.id);
                setForm({ name: p.name, description: p.description, icon: p.icon, color: p.color, systemPrompt: p.systemPrompt, defaultModel: p.defaultModel });
              }}
              className="p-1 text-text-muted hover:text-text-primary transition-colors"
              title="Edit"
            >
              <Pencil size={10} />
            </button>
            {!p.isBuiltin && (
              <button onClick={() => deleteProfile(p.id)} className="p-1 text-text-muted hover:text-accent-red transition-colors" title="Delete">
                <Trash2 size={10} />
              </button>
            )}
          </div>
        ))}
      </div>

      {(showNewProfile || editingProfile) && (
        <div className="bg-bg-elevated border border-bg-border rounded-lg p-3 flex flex-col gap-2">
          <div className="text-[10px] text-text-muted uppercase tracking-wider font-medium">
            {editingProfile ? "Edit Profile" : "New Profile"}
          </div>
          <div className="flex gap-2">
            <input type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Profile name..."
              className="flex-1 bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green" />
            <select value={form.icon} onChange={(e) => setForm({ ...form, icon: e.target.value })}
              className="bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary focus:outline-none focus:border-accent-green">
              {ICON_OPTIONS.map((icon) => <option key={icon} value={icon}>{icon}</option>)}
            </select>
            <select value={form.color} onChange={(e) => setForm({ ...form, color: e.target.value })}
              className="bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary focus:outline-none focus:border-accent-green">
              {COLOR_OPTIONS.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
            </select>
          </div>
          <input type="text" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} placeholder="Short description..."
            className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green" />
          <textarea value={form.systemPrompt} onChange={(e) => setForm({ ...form, systemPrompt: e.target.value })} placeholder="System prompt (prepended to initial prompt)..." rows={3}
            className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green resize-none" />
          <input type="text" value={form.defaultModel} onChange={(e) => setForm({ ...form, defaultModel: e.target.value })} placeholder="Default model (leave empty for system default)..."
            className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green" />
          <div className="flex gap-2 mt-1">
            <button onClick={handleSave} disabled={!form.name.trim()}
              className="px-3 py-1 text-[11px] bg-accent-green/15 text-accent-green rounded hover:bg-accent-green/25 transition-colors disabled:opacity-50">
              {editingProfile ? "Save Changes" : "Create"}
            </button>
            <button onClick={handleCancel} className="px-3 py-1 text-[11px] text-text-muted hover:text-text-primary transition-colors">
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
