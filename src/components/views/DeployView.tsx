import { useEffect, useState, useCallback } from "react";
import {
  Rocket,
  Plus,
  Play,
  Trash2,
  Clock,
  CheckCircle2,
  XCircle,
  Loader2,
  RefreshCw,
} from "lucide-react";
import { invoke } from "@tauri-apps/api/core";
import { useDeployStore } from "@/stores/deployStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { DeployConfigModal } from "./DeployConfigModal";
import { DeployTerminal } from "./DeployTerminal";
import type { DeployConfig } from "@/types/deploy";

export function DeployView() {
  const {
    configs,
    configSource,
    loading,
    error,
    runs,
    activeRunId,
    fetchConfigs,
    addConfig,
    removeConfig,
    startRun,
    finishRun,
    setActiveRunId,
  } = useDeployStore();
  const projectPath = useLayoutStore((s) => s.projectPath);
  const [showConfigModal, setShowConfigModal] = useState(false);

  useEffect(() => {
    fetchConfigs();
  }, [fetchConfigs, projectPath]);

  const activeRun = runs.find((r) => r.id === activeRunId);

  const handleDeploy = useCallback(
    async (config: DeployConfig) => {
      try {
        // Determine shell command based on platform
        const isWindows = navigator.userAgent.includes("Windows") || navigator.platform.includes("Win");
        const command = isWindows ? "cmd" : "bash";
        const args = isWindows ? ["/c", config.command] : ["-c", config.command];

        const sessionId = await invoke<string>("create_pty_session", {
          command,
          args,
          cwd: projectPath,
          env: config.env ?? {},
        });

        startRun(config, sessionId);
      } catch (e) {
        console.error("Deploy failed to start:", e);
      }
    },
    [projectPath, startRun]
  );

  const handleExit = useCallback(
    (code: number) => {
      if (activeRunId) {
        finishRun(activeRunId, code === 0 ? "success" : "failed");
      }
    },
    [activeRunId, finishRun]
  );

  async function handleAddConfig(config: DeployConfig) {
    await addConfig(config);
  }

  return (
    <div className="flex flex-col h-full bg-bg-primary overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-bg-border bg-bg-secondary shrink-0">
        <div className="flex items-center gap-2">
          <Rocket size={14} className="text-accent-amber" />
          <h2 className="text-sm font-medium text-text-primary">Deploy</h2>
          {configSource !== "none" && (
            <span className="text-[10px] text-text-muted px-1.5 py-0.5 bg-bg-elevated rounded">
              {configSource}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchConfigs()}
            className="p-1 text-text-muted hover:text-text-primary transition-colors"
            title="Refresh configs"
          >
            <RefreshCw size={12} className={loading ? "animate-spin" : ""} />
          </button>
          <button
            onClick={() => setShowConfigModal(true)}
            className="flex items-center gap-1.5 px-2.5 py-1 text-xs bg-accent-green/20 text-accent-green rounded hover:bg-accent-green/30 transition-colors"
          >
            <Plus size={12} />
            Add Config
          </button>
        </div>
      </div>

      {error && (
        <div className="mx-4 mt-3 px-3 py-2 bg-red-500/10 border border-red-500/20 rounded text-xs text-red-400 shrink-0">
          {error}
        </div>
      )}

      <div className="flex flex-1 min-h-0 overflow-hidden">
        {/* Left panel — configs + history */}
        <div className="w-64 border-r border-bg-border flex flex-col shrink-0 overflow-y-auto">
          {/* Deploy configs */}
          <div className="p-3">
            <h3 className="text-[11px] font-medium text-text-secondary uppercase tracking-wider mb-2">
              Configurations
            </h3>
            {configs.length === 0 && !loading && (
              <p className="text-[11px] text-text-muted py-4 text-center">
                No deploy configs found
              </p>
            )}
            <div className="space-y-1.5">
              {configs.map((config) => (
                <div
                  key={config.name}
                  className="flex items-center gap-2 px-2.5 py-2 bg-bg-secondary border border-bg-border rounded-lg group"
                >
                  <div className="flex-1 min-w-0">
                    <div className="text-xs font-medium text-text-primary truncate">
                      {config.name}
                    </div>
                    <div className="text-[10px] text-text-muted truncate mt-0.5">
                      {config.command}
                    </div>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    <button
                      onClick={() => handleDeploy(config)}
                      disabled={activeRun?.status === "running"}
                      className="p-1 text-accent-green hover:text-accent-green/80 transition-colors disabled:opacity-40"
                      title="Deploy"
                    >
                      <Play size={12} />
                    </button>
                    <button
                      onClick={() => removeConfig(config.name)}
                      className="p-1 text-text-muted hover:text-red-400 transition-colors opacity-0 group-hover:opacity-100"
                      title="Remove"
                    >
                      <Trash2 size={10} />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Run history */}
          {runs.length > 0 && (
            <div className="p-3 border-t border-bg-border">
              <h3 className="text-[11px] font-medium text-text-secondary uppercase tracking-wider mb-2">
                History
              </h3>
              <div className="space-y-1">
                {runs.map((run) => (
                  <button
                    key={run.id}
                    onClick={() => setActiveRunId(run.id)}
                    className={`flex items-center gap-2 w-full px-2.5 py-1.5 rounded text-left transition-colors ${
                      activeRunId === run.id
                        ? "bg-bg-elevated text-text-primary"
                        : "text-text-secondary hover:bg-bg-hover"
                    }`}
                  >
                    <RunStatusIcon status={run.status} />
                    <div className="flex-1 min-w-0">
                      <div className="text-[11px] truncate">{run.configName}</div>
                      <div className="text-[9px] text-text-muted">
                        {formatTime(run.startedAt)}
                      </div>
                    </div>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Right panel — terminal output */}
        <div className="flex-1 flex flex-col min-h-0 p-3">
          {activeRun ? (
            <>
              <div className="flex items-center gap-2 mb-2 shrink-0">
                <RunStatusIcon status={activeRun.status} />
                <span className="text-xs font-medium text-text-primary">
                  {activeRun.configName}
                </span>
                <span className="text-[10px] text-text-muted">
                  {activeRun.command}
                </span>
                {activeRun.status === "running" && (
                  <span className="text-[10px] text-accent-amber ml-auto flex items-center gap-1">
                    <Loader2 size={10} className="animate-spin" />
                    Running...
                  </span>
                )}
                {activeRun.finishedAt && (
                  <span className="text-[10px] text-text-muted ml-auto">
                    {Math.round((activeRun.finishedAt - activeRun.startedAt) / 1000)}s
                  </span>
                )}
              </div>
              <DeployTerminal
                sessionId={activeRun.sessionId}
                onExit={handleExit}
              />
            </>
          ) : (
            <div className="flex flex-col items-center justify-center h-full text-text-muted">
              <Rocket size={24} className="mb-3 opacity-30" />
              <p className="text-xs">Select a config and click Deploy to start</p>
              {configs.length === 0 && (
                <p className="text-[11px] mt-1">
                  Or add a <code className="text-accent-green">packetcode.deploy.json</code> to your project
                </p>
              )}
            </div>
          )}
        </div>
      </div>

      {showConfigModal && (
        <DeployConfigModal
          onClose={() => setShowConfigModal(false)}
          onSave={handleAddConfig}
        />
      )}
    </div>
  );
}

function RunStatusIcon({ status }: { status: string }) {
  switch (status) {
    case "running":
      return <Loader2 size={12} className="text-accent-amber animate-spin" />;
    case "success":
      return <CheckCircle2 size={12} className="text-accent-green" />;
    case "failed":
      return <XCircle size={12} className="text-red-400" />;
    default:
      return <Clock size={12} className="text-text-muted" />;
  }
}

function formatTime(ts: number): string {
  const d = new Date(ts);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}
