import { useState } from "react";
import { FileText, RefreshCw, Loader2, Search, AlertCircle } from "lucide-react";
import { useMemoryStore } from "@/stores/memoryStore";

export function FileMapTab({ projectPath }: { projectPath: string }) {
  const { memory, isScanning, scanError, scanCodebase } = useMemoryStore();
  const [searchQuery, setSearchQuery] = useState("");

  const filtered = memory.fileMap.filter(
    (f) =>
      f.path.toLowerCase().includes(searchQuery.toLowerCase()) ||
      f.summary.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div>
      <div className="flex items-center gap-3 mb-4">
        <button
          onClick={() => scanCodebase(projectPath)}
          disabled={isScanning}
          className="flex items-center gap-1.5 px-3 py-1.5 text-[11px] bg-accent-purple/15 text-accent-purple border border-accent-purple/30 rounded hover:bg-accent-purple/25 transition-colors disabled:opacity-50"
        >
          {isScanning ? <Loader2 size={11} className="animate-spin" /> : <RefreshCw size={11} />}
          {isScanning ? "Scanning..." : "Scan Codebase"}
        </button>
        {memory.lastScanAt && (
          <span className="text-[10px] text-text-muted">
            Last scan: {new Date(memory.lastScanAt).toLocaleString()}
          </span>
        )}
      </div>

      {scanError && (
        <div className="flex items-center gap-2 px-3 py-2 mb-3 bg-accent-red/10 rounded text-[11px] text-accent-red">
          <AlertCircle size={12} />
          {scanError}
        </div>
      )}

      {memory.fileMap.length > 0 && (
        <div className="mb-3">
          <div className="flex items-center gap-2 bg-bg-secondary border border-bg-border rounded px-2.5 py-1.5">
            <Search size={11} className="text-text-muted" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search files..."
              className="flex-1 bg-transparent text-[11px] text-text-primary placeholder:text-text-muted focus:outline-none"
            />
            <span className="text-[10px] text-text-muted">{filtered.length} files</span>
          </div>
        </div>
      )}

      <div className="flex flex-col gap-1">
        {filtered.map((file) => (
          <div key={file.path} className="flex items-start gap-3 px-3 py-2 bg-bg-secondary border border-bg-border rounded hover:bg-bg-hover transition-colors">
            <FileText size={12} className="text-text-muted mt-0.5 shrink-0" />
            <div className="min-w-0">
              <div className="text-[11px] text-text-primary font-mono truncate">{file.path}</div>
              <div className="text-[10px] text-text-muted mt-0.5">{file.summary}</div>
            </div>
          </div>
        ))}
        {memory.fileMap.length === 0 && !isScanning && (
          <div className="text-center py-12 text-[11px] text-text-muted">
            No file map yet. Click "Scan Codebase" to generate one.
          </div>
        )}
      </div>
    </div>
  );
}
