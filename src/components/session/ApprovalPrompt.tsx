import { useState } from "react";
import { ShieldCheck, ShieldX } from "lucide-react";

interface ApprovalPromptProps {
  toolName: string;
  description: string;
  onApprove: () => void;
  onDeny: () => void;
}

export function ApprovalPrompt({
  toolName,
  description,
  onApprove,
  onDeny,
}: ApprovalPromptProps) {
  const [responded, setResponded] = useState(false);

  const handleApprove = () => {
    setResponded(true);
    onApprove();
  };

  const handleDeny = () => {
    setResponded(true);
    onDeny();
  };

  if (responded) return null;

  return (
    <div className="border border-accent-amber/40 rounded-md bg-bg-elevated p-3 my-2">
      <div className="flex items-center gap-2 mb-2">
        <ShieldCheck size={14} className="text-accent-amber" />
        <span className="text-accent-amber text-xs font-semibold uppercase tracking-wider">
          Permission Required
        </span>
      </div>
      <div className="text-text-primary text-sm mb-1">
        <span className="text-accent-blue font-medium">{toolName}</span> wants
        to execute:
      </div>
      <pre className="text-xs text-text-secondary bg-bg-primary rounded px-2 py-1.5 mb-3 overflow-x-auto whitespace-pre-wrap">
        {description}
      </pre>
      <div className="flex items-center gap-2">
        <button
          onClick={handleApprove}
          className="flex items-center gap-1.5 px-3 py-1.5 bg-accent-green/20 text-accent-green rounded text-xs font-medium hover:bg-accent-green/30 transition-colors"
        >
          <ShieldCheck size={12} />
          Allow
        </button>
        <button
          onClick={handleDeny}
          className="flex items-center gap-1.5 px-3 py-1.5 bg-accent-red/20 text-accent-red rounded text-xs font-medium hover:bg-accent-red/30 transition-colors"
        >
          <ShieldX size={12} />
          Deny
        </button>
      </div>
    </div>
  );
}
