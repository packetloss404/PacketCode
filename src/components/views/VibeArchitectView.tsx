import { ExternalLink } from "lucide-react";

const VIBE_ARCHITECT_URL = "https://specs-gen.vercel.app";

export function VibeArchitectView() {
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
        <span className="text-[10px] text-text-muted ml-auto">
          AI Project Spec Generator
        </span>
      </div>
      <iframe
        src={VIBE_ARCHITECT_URL}
        className="flex-1 w-full border-none"
        title="Vibe Architect"
        allow="clipboard-read; clipboard-write"
      />
    </div>
  );
}
