import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { getToken, getWebSocketUrl, getMessages, type ChatMessage as ApiMessage } from '../api'

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

export default function Chat() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const [input, setInput] = useState('')
  const [typing, setTyping] = useState(false)
  const [connected, setConnected] = useState(false)
  const [error, setError] = useState('')
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
  }, [instanceId, navigate, loadInitial])

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
              if (targetId) {
                setMessages((prev) =>
                  prev.map((m) =>
                    m.id === targetId ? { ...m, content } : m
                  )
                )
              } else {
                setMessages((prev) => [...prev, { id: 'u-' + Date.now(), content, role: 'assistant' }])
              }
            }
            break
          case 'typing.start':
            setTyping(true)
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
          ? '连接失败，宠物可能仍在启动中，请稍后重试'
          : '连接已断开'
        setError(msg)
      }
    }

    ws.onerror = () => setError('连接失败，请检查网络或稍后重试')

    return () => {
      ws.close()
      wsRef.current = null
    }
  }, [instanceId])

  useEffect(() => {
    listRef.current?.scrollTo(0, listRef.current?.scrollHeight ?? 0)
  }, [messages, typing])

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
    <div className="fixed inset-0 flex flex-col bg-slate-50 sm:bg-white sm:max-w-2xl sm:mx-auto sm:my-4 sm:rounded-2xl sm:shadow-lg sm:border sm:border-slate-200 sm:min-h-[80vh] z-10">
      {/* 顶部栏 - 移动端友好 */}
      <div className="flex items-center gap-3 px-3 py-3 sm:px-4 bg-white border-b border-slate-200 flex-shrink-0">
        <button
          onClick={() => navigate('/')}
          className="flex items-center gap-1 px-2 py-2.5 -ml-1 text-slate-600 active:bg-slate-100 rounded-lg min-h-[44px] min-w-[44px] touch-manipulation"
          aria-label="返回"
        >
          <span className="text-xl">←</span>
        </button>
        <span
          className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${
            connected ? 'bg-green-500' : 'bg-red-500'
          }`}
        />
        <span className="text-sm text-slate-500 flex-1">
          {connected ? '已连接' : '未连接'}
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
            <div className="animate-pulse text-slate-400 text-sm">加载中...</div>
          </div>
        ) : (
          <>
            {loadingMore && (
              <div className="flex justify-center py-2 text-slate-400 text-xs">加载更多...</div>
            )}
            {messages.length === 0 && !typing && (
              <p className="text-slate-400 text-sm py-12 text-center">发条消息试试</p>
            )}
            <div className="space-y-3">
              {messages.map((m) => {
                const isUser = m.role === 'user'
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
                      <p className="text-xs opacity-80 mb-0.5">
                        {isUser ? '我' : '助手'}
                      </p>
                      <p className="whitespace-pre-wrap break-words text-[15px] leading-relaxed">
                        {m.content}
                      </p>
                    </div>
                  </div>
                )
              })}
            </div>
            {typing && (
              <div className="flex justify-start mt-3">
                <div className="bg-white border border-slate-200 rounded-2xl rounded-bl-md px-4 py-2.5 shadow-sm">
                  <p className="text-slate-500 text-sm italic">输入中...</p>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      {/* 输入区 - 移动端大按钮 */}
      <form
        onSubmit={sendMessage}
        className="flex gap-2 p-3 sm:p-4 bg-white border-t border-slate-200 flex-shrink-0"
      >
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="输入消息..."
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
