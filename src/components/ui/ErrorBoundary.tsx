import { Component, type ReactNode } from "react";
import { AlertTriangle, RotateCcw } from "lucide-react";

interface Props {
  children: ReactNode;
  fallbackMessage?: string;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex flex-col items-center justify-center h-full gap-3 p-6 text-center">
          <AlertTriangle size={32} className="text-accent-amber" />
          <p className="text-text-primary text-sm font-medium">
            {this.props.fallbackMessage || "Something went wrong"}
          </p>
          <p className="text-text-muted text-xs max-w-md">
            {this.state.error?.message}
          </p>
          <button
            onClick={() => this.setState({ hasError: false, error: null })}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-bg-elevated text-text-primary rounded text-xs hover:bg-bg-hover transition-colors mt-2"
          >
            <RotateCcw size={12} />
            Try Again
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}
