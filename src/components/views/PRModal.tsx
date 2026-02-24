import { useState } from "react";
import { GitPullRequest } from "lucide-react";
import { Modal } from "@/components/ui/Modal";

interface PRModalProps {
  onClose: () => void;
  onSubmit: (title: string, body: string, head: string, base: string) => Promise<string>;
  isLoading: boolean;
}

export function PRModal({ onClose, onSubmit, isLoading }: PRModalProps) {
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [head, setHead] = useState("");
  const [base, setBase] = useState("main");
  const [result, setResult] = useState<string | null>(null);

  async function handleSubmit() {
    try {
      const json = await onSubmit(title, body, head, base);
      const pr = JSON.parse(json);
      setResult(pr.html_url || "PR created successfully");
    } catch {
      /* error handled by store */
    }
  }

  const footer = (
    <div className="flex items-center justify-end gap-2">
      <button
        onClick={onClose}
        className="px-3 py-1.5 text-xs text-text-secondary hover:text-text-primary transition-colors"
      >
        Cancel
      </button>
      <button
        onClick={handleSubmit}
        disabled={!title.trim() || !head.trim() || isLoading}
        className="px-4 py-1.5 text-xs bg-accent-purple/15 text-accent-purple border border-accent-purple/30 rounded font-medium hover:bg-accent-purple/25 transition-colors disabled:opacity-50"
      >
        {isLoading ? "Creating..." : "Create PR"}
      </button>
    </div>
  );

  return (
    <Modal
      onClose={onClose}
      title="Create Pull Request"
      icon={<GitPullRequest size={14} className="text-accent-purple" />}
      footer={footer}
    >
      <div className="px-5 py-4 flex flex-col gap-3">
        <input
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="PR title"
          className="w-full bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-purple"
        />
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Description..."
          rows={4}
          className="w-full bg-bg-primary border border-bg-border rounded px-3 py-2 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-purple resize-none"
        />
        <div className="flex gap-2">
          <div className="flex-1">
            <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
              Head branch
            </label>
            <input
              type="text"
              value={head}
              onChange={(e) => setHead(e.target.value)}
              placeholder="feature-branch"
              className="w-full bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-purple"
            />
          </div>
          <div className="flex-1">
            <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
              Base branch
            </label>
            <input
              type="text"
              value={base}
              onChange={(e) => setBase(e.target.value)}
              placeholder="main"
              className="w-full bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-purple"
            />
          </div>
        </div>

        {result && (
          <div className="text-[11px] text-accent-green bg-accent-green/10 rounded px-3 py-2">
            {result}
          </div>
        )}
      </div>
    </Modal>
  );
}
