import { createContext, useContext, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'

type SaveHandler = () => Promise<void>

type UnsavedConfigContextType = {
  hasUnsaved: boolean
  setHasUnsaved: (v: boolean) => void
  registerSaveHandler: (fn: SaveHandler | null) => void
  tryLeave: (to: string, e: React.MouseEvent) => void
}

const UnsavedConfigContext = createContext<UnsavedConfigContextType | null>(null)

export function UnsavedConfigProvider({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate()
  const [hasUnsaved, setHasUnsaved] = useState(false)
  const [saveHandler, setSaveHandler] = useState<SaveHandler | null>(null)
  const [pending, setPending] = useState<{ to: string } | null>(null)

  const registerSaveHandler = useCallback((fn: SaveHandler | null) => {
    setSaveHandler(() => fn)
  }, [])

  const tryLeave = useCallback(
    (to: string, e: React.MouseEvent) => {
      if (!hasUnsaved) return
      e.preventDefault()
      setPending({ to })
    },
    [hasUnsaved]
  )

  const doLeave = useCallback(
    (saveFirst: boolean) => {
      const to = pending?.to
      if (!to) return
      setPending(null)

      const go = () => {
        setHasUnsaved(false)
        navigate(to)
      }

      if (saveFirst && saveHandler) {
        saveHandler().then(go).catch(() => {})
      } else {
        go()
      }
    },
    [pending, saveHandler, navigate]
  )

  const cancelLeave = useCallback(() => setPending(null), [])

  return (
    <UnsavedConfigContext.Provider
      value={{ hasUnsaved, setHasUnsaved, registerSaveHandler, tryLeave }}
    >
      {children}
      {pending && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="bg-white rounded-xl shadow-xl max-w-md w-full p-6">
            <h3 className="text-lg font-semibold text-slate-800">配置有改动未保存</h3>
            <p className="mt-2 text-sm text-slate-600">是否保存后再离开？</p>
            <div className="mt-6 flex flex-col sm:flex-row gap-3">
              <button
                type="button"
                onClick={() => doLeave(true)}
                className="flex-1 px-4 py-2 rounded-lg bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-700"
              >
                保存并离开
              </button>
              <button
                type="button"
                onClick={() => doLeave(false)}
                className="flex-1 px-4 py-2 rounded-lg border border-slate-300 text-slate-700 text-sm hover:bg-slate-50"
              >
                不保存离开
              </button>
              <button
                type="button"
                onClick={cancelLeave}
                className="flex-1 px-4 py-2 rounded-lg border border-slate-300 text-slate-700 text-sm hover:bg-slate-50"
              >
                取消
              </button>
            </div>
          </div>
        </div>
      )}
    </UnsavedConfigContext.Provider>
  )
}

export function useUnsavedConfig() {
  const ctx = useContext(UnsavedConfigContext)
  return ctx
}
