import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { getToken, getWebSocketUrl, getMessages, getInstance, type ChatMessage as ApiMessage } from '../api'

const COLLAPSE_THRESHOLD = 400

interface PicoMessage {
  type: string
  id?: string
  payload?: { content?: string; role?: string; message_id?: string; [key: string]: unknown }
}

interface ChatMessage {
  id: string | number
  content: string
  role?: string
}

const PAGE_SIZE = 20

const TYPING_PHRASES = ['嗯...', '想想...', '正在琢磨...', '快好了～', '有了！']

function MessageContent({
  content,
  isUser,
  expanded,
  onToggleExpand,
}: {
  content: string
  isUser: boolean
  expanded: boolean
  onToggleExpand: () => void
}) {
  const isLong = content.length > COLLAPSE_THRESHOLD
  const showCollapsed = isLong && !expanded
  const displayContent = showCollapsed ? content.slice(0, COLLAPSE_THRESHOLD) + '...' : content

  const wrapClass = isUser ? 'msg-md-user' : 'msg-md-assistant'

  return (
    <div className={`msg-markdown ${wrapClass}`}>
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{displayContent}</ReactMarkdown>
      {isLong && (
        <button
          type="button"
          onClick={onToggleExpand}
          className="msg-expand-btn"
        >
          {expanded ? '收起' : '展开'}
        </button>
      )}
    </div>
  )
}
const TYPING_ROTATE_MS = 2500

export default function Chat() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const [input, setInput] = useState('')
  const [typing, setTyping] = useState(false)
  const [typingPhraseIndex, setTypingPhraseIndex] = useState(0)
  const [connected, setConnected] = useState(false)
  const [error, setError] = useState('')
  const [instanceName, setInstanceName] = useState('')
  const [expandedIds, setExpandedIds] = useState<Set<string | number>>(new Set())
  const wsRef = useRef<WebSocket | null>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const loadingMoreRef = useRef(false)

  const instanceId = parseInt(id ?? '', 10)

  const loadMessages = useCallback(
    async (before?: number) => {
      if (isNaN(instanceId)) return []
      try {
        const { messages: list } = await getMessages(instanceId, PAGE_SIZE, before)
        return list.map((m: ApiMessage) => ({
          id: m.id,
          content: m.content,
          role: m.role,
        }))
      } catch {
        return []
      }
    },
    [instanceId]
  )

  const loadInitial = useCallback(async () => {
    setLoading(true)
    const list = await loadMessages()
    setMessages(list.reverse())
    setHasMore(list.length >= PAGE_SIZE)
    setLoading(false)
  }, [loadMessages])

  const loadOlder = useCallback(async () => {
    if (loadingMoreRef.current || !hasMore || messages.length === 0) return
    const oldest = messages.find((m) => typeof m.id === 'number')
    const oldestId = oldest?.id as number | undefined
    if (oldestId == null) return
    loadingMoreRef.current = true
    setLoadingMore(true)
    const list = await loadMessages(oldestId as number)
    setMessages((prev) => [...list.reverse(), ...prev])
    setHasMore(list.length >= PAGE_SIZE)
    loadingMoreRef.current = false
    setLoadingMore(false)
  }, [loadMessages, hasMore, messages])

  useEffect(() => {
    if (isNaN(instanceId)) {
      setError('Invalid instance')
      return
    }
    const token = getToken()
    if (!token) {
      navigate('/login')
      return
    }
    loadInitial()
    getInstance(instanceId)
      .then((inst) => setInstanceName(inst.name || '宠物'))
      .catch(() => setInstanceName(''))
  }, [instanceId, navigate, loadInitial])

  // 页面重新可见时拉取最新消息（多标签/后台时可能漏收 WebSocket）
  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === 'visible' && !isNaN(instanceId)) {
        loadMessages().then((list) => {
          if (list.length > 0) setMessages(list.reverse())
        })
      }
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  }, [instanceId, loadMessages])

  useEffect(() => {
    if (isNaN(instanceId)) return
    const token = getToken()
    if (!token) return

    const url = getWebSocketUrl(instanceId)
    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onopen = () => {
      setConnected(true)
      setError('')
    }

    ws.onmessage = (event) => {
      try {
        const msg: PicoMessage = JSON.parse(event.data)
        switch (msg.type) {
          case 'message.create':
            if (msg.payload?.content != null) {
              const mid = msg.payload.message_id ?? msg.id ?? String(Date.now())
              const content = String(msg.payload.content)
              if (content.startsWith('Thinking')) return
              setMessages((prev) => {
                if (prev.some((m) => m.id === mid || (m.content === content && m.role === 'assistant'))) return prev
                return [...prev, { id: mid, content, role: (msg.payload!.role as string) || 'assistant' }]
              })
            }
            break
          case 'message.update':
            if (msg.payload?.content != null) {
              const targetId = msg.payload.message_id ?? msg.id
              const content = String(msg.payload.content)
              if (content.startsWith('Thinking')) return
              setMessages((prev) => {
                if (targetId) {
                  const idx = prev.findIndex((m) => m.id === targetId)
                  if (idx >= 0) {
                    return prev.map((m) => (m.id === targetId ? { ...m, content } : m))
                  }
                }
                return [...prev, { id: targetId || 'u-' + Date.now(), content, role: 'assistant' }]
              })
            }
            break
          case 'typing.start':
            setTyping(true)
            setTypingPhraseIndex(0)
            break
          case 'typing.stop':
            setTyping(false)
            break
        }
      } catch {
        /* ignore */
      }
    }

    ws.onclose = (e) => {
      setConnected(false)
      if (!error) {
        const msg = e.code === 1006 || e.code === 1011
          ? 'TA 还在准备中，稍后再试试～'
          : '连接断开了，刷新试试'
        setError(msg)
      }
    }

    ws.onerror = () => setError('网络不太顺畅，稍后再试～')

    return () => {
      ws.close()
      wsRef.current = null
    }
  }, [instanceId])

  useEffect(() => {
    listRef.current?.scrollTo(0, listRef.current?.scrollHeight ?? 0)
  }, [messages, typing])

  // 等待回答时轮换提示语，减少干等感
  useEffect(() => {
    if (!typing) return
    const interval = setInterval(() => {
      setTypingPhraseIndex((i) => (i + 1) % TYPING_PHRASES.length)
    }, TYPING_ROTATE_MS)
    return () => clearInterval(interval)
  }, [typing])

  // 手机键盘弹出时滚动到底部，保持最新消息可见（类似微信）
  const inputRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const list = listRef.current
    const input = inputRef.current
    if (!list || !input) return
    const scrollToBottom = () => {
      requestAnimationFrame(() => list.scrollTo({ top: list.scrollHeight, behavior: 'smooth' }))
    }
    const onFocus = () => scrollToBottom()
    input.addEventListener('focus', onFocus)
    const vv = window.visualViewport
    if (vv) {
      const onResize = () => {
        if (document.activeElement === input) scrollToBottom()
      }
      vv.addEventListener('resize', onResize)
      return () => {
        input.removeEventListener('focus', onFocus)
        vv.removeEventListener('resize', onResize)
      }
    }
    return () => input.removeEventListener('focus', onFocus)
  }, [])

  const handleScroll = () => {
    const el = listRef.current
    if (!el || loadingMoreRef.current || !hasMore) return
    if (el.scrollTop < 80) loadOlder()
  }

  const sendMessage = (e: React.FormEvent) => {
    e.preventDefault()
    const content = input.trim()
    if (!content || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return

    const userMsgId = 'u-' + Date.now()
    setMessages((prev) => [...prev, { id: userMsgId, content, role: 'user' }])
    wsRef.current.send(JSON.stringify({ type: 'message.send', payload: { content } }))
    setInput('')
  }

  if (isNaN(instanceId)) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <p className="text-red-600">Invalid instance</p>
      </div>
    )
  }

  return (
    <div className="fixed inset-0 flex flex-col overflow-hidden bg-slate-50 sm:bg-white sm:max-w-2xl sm:mx-auto sm:my-4 sm:rounded-2xl sm:shadow-lg sm:border sm:border-slate-200 sm:min-h-[80vh] z-10 h-[100dvh] sm:h-auto">
      {/* 顶部栏 - 显示宠物名，移动端连接时隐藏状态 */}
      <div className="flex items-center gap-3 px-3 py-2.5 sm:py-3 sm:px-4 bg-white border-b border-slate-200 flex-shrink-0">
        <button
          onClick={() => navigate('/')}
          className="flex items-center gap-1 px-2 py-2 -ml-1 text-slate-600 active:bg-slate-100 rounded-lg min-h-[40px] min-w-[40px] touch-manipulation sm:min-h-[44px] sm:min-w-[44px]"
          aria-label="返回"
        >
          <span className="text-xl">←</span>
        </button>
        <span className="flex-1 font-medium text-slate-800 truncate">
          {instanceName || '...'}
        </span>
        <span className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${connected ? 'bg-green-500' : 'bg-red-500'} ${connected ? 'hidden sm:block' : ''}`} />
        <span className={`text-sm text-slate-500 ${connected ? 'hidden sm:block' : ''}`}>
          {connected ? '在线' : '离线'}
        </span>
      </div>

      {error && (
        <div className="px-4 py-2 bg-red-50 border-b border-red-100 text-red-700 text-sm flex-shrink-0">
          {error}
        </div>
      )}

      {/* 消息列表 - 可滚动，支持上拉加载 */}
      <div
        ref={listRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto overscroll-contain px-3 py-4 sm:px-4 min-h-0"
      >
        {loading ? (
          <div className="flex justify-center py-12">
            <div className="animate-pulse text-slate-400 text-sm">马上就好～</div>
          </div>
        ) : (
          <>
            {loadingMore && (
              <div className="flex justify-center py-2 text-slate-400 text-xs">
                <span className="sm:hidden">···</span>
                <span className="hidden sm:inline">加载更多...</span>
              </div>
            )}
            {messages.length === 0 && !typing && (
              <p className="text-slate-400 text-sm py-12 text-center">打个招呼吧～</p>
            )}
            <div className="space-y-3">
              {messages.map((m) => {
                const isUser = m.role === 'user'
                const expanded = expandedIds.has(m.id)
                const toggleExpand = () => {
                  setExpandedIds((prev) => {
                    const next = new Set(prev)
                    if (next.has(m.id)) next.delete(m.id)
                    else next.add(m.id)
                    return next
                  })
                }
                return (
                  <div
                    key={String(m.id)}
                    className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}
                  >
                    <div
                      className={`max-w-[85%] sm:max-w-[75%] rounded-2xl px-4 py-2.5 ${
                        isUser
                          ? 'bg-indigo-600 text-white rounded-br-md'
                          : 'bg-white border border-slate-200 text-slate-800 rounded-bl-md shadow-sm'
                      }`}
                    >
                      <MessageContent
                        content={m.content}
                        isUser={!!isUser}
                        expanded={expanded}
                        onToggleExpand={toggleExpand}
                      />
                    </div>
                  </div>
                )
              })}
            </div>
            {typing && (
              <div className="flex justify-start mt-3">
                <div className="typing-breathe bg-white border border-slate-200 rounded-2xl rounded-bl-md px-4 py-2.5 shadow-sm flex items-center gap-1">
                  <span className="text-slate-500 text-sm italic">{TYPING_PHRASES[typingPhraseIndex]}</span>
                  <span className="flex gap-0.5">
                    {[1, 2, 3].map((i) => (
                      <span key={i} className="typing-dot w-1 h-1 rounded-full bg-slate-400 inline-block" />
                    ))}
                  </span>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      {/* 输入区 - 移动端大按钮，底部安全区 */}
      <form
        onSubmit={sendMessage}
        className="flex gap-2 p-3 sm:p-4 pb-[max(0.75rem,env(safe-area-inset-bottom))] bg-white border-t border-slate-200 flex-shrink-0"
      >
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="说点什么～"
          disabled={!connected}
          className="flex-1 px-4 py-3.5 sm:py-3 border border-slate-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent disabled:opacity-50 text-base min-h-[48px]"
        />
        <button
          type="submit"
          disabled={!connected || !input.trim()}
          className="px-5 py-3.5 sm:py-3 bg-indigo-600 text-white rounded-xl active:bg-indigo-700 disabled:opacity-50 font-medium min-h-[48px] touch-manipulation"
        >
          发送
        </button>
      </form>
    </div>
  )
}
