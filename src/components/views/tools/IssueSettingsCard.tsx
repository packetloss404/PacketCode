import { Settings } from "lucide-react";

interface IssueSettingsCardProps {
  ticketPrefix: string;
  setTicketPrefix: (prefix: string) => void;
}

export function IssueSettingsCard({ ticketPrefix, setTicketPrefix }: IssueSettingsCardProps) {
  return (
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
  );
}
