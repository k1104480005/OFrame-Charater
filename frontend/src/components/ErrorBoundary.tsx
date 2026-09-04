// 全局错误边界（crash shield）：任何页面渲染/effect 异常不再白屏，
// 而是显示可读的错误信息与重试入口。错误信息同时输出到 console，
// 便于在开发/调试时定位根因。
import { Component } from "react";
import type { ErrorInfo, ReactNode } from "react";

interface Props {
  children: ReactNode;
  /** 出错区域名称，显示在错误卡上 */
  label?: string;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error(`[ErrorBoundary:${this.props.label ?? "app"}]`, error, info.componentStack);
  }

  render(): ReactNode {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <div className="empty-state" style={{ margin: "48px auto", maxWidth: 560 }}>
        <h4 className="mono">页面出现错误{this.props.label ? `（${this.props.label}）` : ""}</h4>
        <pre className="error-text" style={{ whiteSpace: "pre-wrap", textAlign: "left", margin: "12px 0" }}>
          {String(error.message || error)}
        </pre>
        <div className="row" style={{ justifyContent: "center" }}>
          <button
            className="pixel-btn pixel-btn--primary"
            onClick={() => this.setState({ error: null })}
          >
            重试
          </button>
        </div>
      </div>
    );
  }
}
