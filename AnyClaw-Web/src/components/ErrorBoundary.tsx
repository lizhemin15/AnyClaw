import { Component, type ReactNode } from 'react'

interface Props {
  children: ReactNode
  fallback?: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null }

  static getDerivedStateFromError(): Partial<State> {
    return { hasError: true }
  }

  componentDidCatch(err: Error) {
    this.setState({ error: err })
  }

  render() {
    if (this.state.hasError) {
      const { error } = this.state
      const errMsg = error?.message ?? error?.toString?.() ?? '未知错误'
      const hasCustomFallback = this.props.fallback != null
      return (
        <div className="p-4 text-center text-slate-600 text-sm space-y-3">
          {this.props.fallback ?? <p>加载出错，请刷新页面重试</p>}
          <div className="text-left max-w-full overflow-auto">
            <p className="font-medium text-red-600 text-xs mb-1">错误信息：</p>
            <pre className="text-xs text-slate-500 whitespace-pre-wrap break-words bg-slate-100 p-2 rounded">
              {errMsg}
            </pre>
          </div>
          {!hasCustomFallback && (
            <button
              type="button"
              onClick={() => window.location.reload()}
              className="text-indigo-600 underline"
            >
              刷新重试
            </button>
          )}
        </div>
      )
    }
    return this.props.children
  }
}
