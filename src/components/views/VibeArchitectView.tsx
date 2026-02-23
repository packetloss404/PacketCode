import { useState } from "react";
import { ExternalLink, ClipboardList } from "lucide-react";
import { SpecImportModal } from "./SpecImportModal";

const VIBE_ARCHITECT_URL = "https://specs-gen.vercel.app";

export function VibeArchitectView() {
  const [showImport, setShowImport] = useState(false);

  return (
    <div className="flex flex-col h-full bg-bg-primary">
      <div className="flex items-center gap-2 px-4 py-2 border-b border-bg-border bg-bg-secondary">
        <span className="text-xs font-semibold text-text-primary">
          Vibe Architect
        </span>
        <a
          href={VIBE_ARCHITECT_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="text-text-muted hover:text-accent-green transition-colors"
          title="Open in browser"
        >
          <ExternalLink size={11} />
        </a>
        <button
          onClick={() => setShowImport(true)}
          className="flex items-center gap-1 px-2 py-0.5 text-[10px] font-medium rounded border border-bg-border text-text-muted hover:text-accent-green hover:border-accent-green/40 transition-colors"
          title="Import spec to Issues board"
        >
          <ClipboardList size={10} />
          Import to Issues
        </button>
        <span className="text-[10px] text-text-muted ml-auto">
          AI Project Spec Generator
        </span>
      </div>
      <iframe
        src={VIBE_ARCHITECT_URL}
        className="flex-1 w-full border-none"
        title="Vibe Architect"
        allow="clipboard-read; clipboard-write"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-clipboard-read allow-clipboard-write"
      />
      {showImport && <SpecImportModal onClose={() => setShowImport(false)} />}
    </div>
  );
}
