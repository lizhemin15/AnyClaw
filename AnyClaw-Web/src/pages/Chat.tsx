import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { getToken, getWebSocketUrl, getMessages, getInstance, markInstanceRead, fetchProxyText, type ChatMessage as ApiMessage } from '../api'

// remark-gfm 使用 lookbehind 正则，Safari 16.4 以下不支持，会报 invalid group specifier name
const supportsGfm = typeof window !== 'undefined' && (() => {
  try {
    new RegExp('(?<=a)b').test('ab')
    return true
  } catch {
    return false
  }
})()
import { ErrorBoundary } from '../components/ErrorBoundary'

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

// 初始只加载约两屏，其余通过上拉加载
const PAGE_SIZE = 10

const TYPING_PHRASES = ['嗯...', '想想看...', '稍等一下下～', '快好啦～', '马上就好～']

function isThinkingPlaceholder(content: string): boolean {
  const s = String(content ?? '').trim().toLowerCase()
  return s.startsWith('thinking')
}

function isMediaContent(content: string): boolean {
  const s = String(content ?? '')
  return s.includes('![') || s.includes('[📎') || s.includes('[📹') || s.includes('[🔊')
}

function TextPreviewModal({ url, filename, onClose }: { url: string; filename: string; onClose: () => void }) {
  const [content, setContent] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const ext = (filename.split('.').pop() || '').toLowerCase()
  const isMd = ext === 'md' || ext === 'markdown'

  useEffect(() => {
    if (url.startsWith('data:')) {
      const comma = url.indexOf(',')
      if (comma >= 0) {
        try {
          const data = url.slice(comma + 1)
          const text = url.slice(0, comma).toLowerCase().includes(';base64')
            ? new TextDecoder().decode(Uint8Array.from(atob(data), (c) => c.charCodeAt(0)))
            : decodeURIComponent(data)
          setContent(text)
        } catch {
          setError('无法解析 data URL')
        }
      }
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    fetchProxyText(url)
      .then(setContent)
      .catch((e) => {
        const msg = e instanceof Error ? e.message : '加载失败'
        setError(msg)
      })
      .finally(() => setLoading(false))
  }, [url])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50" onClick={onClose}>
      <div
        className="bg-white rounded-lg shadow-xl max-w-3xl w-full max-h-[80vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-4 py-3 border-b border-slate-200 flex items-center justify-between shrink-0">
          <span className="font-medium text-slate-800 truncate">{filename}</span>
          <div className="flex gap-2 shrink-0">
            <a
              href={url}
              download={filename}
              className="px-3 py-1.5 text-sm bg-indigo-600 text-white rounded-lg hover:bg-indigo-700"
            >
              下载
            </a>
            <button type="button" onClick={onClose} className="px-3 py-1.5 text-sm border border-slate-300 rounded-lg hover:bg-slate-50">
              关闭
            </button>
          </div>
        </div>
        <div className="flex-1 min-h-0 overflow-auto p-4 bg-slate-50">
          {loading && <div className="text-slate-500">加载中...</div>}
          {error && <div className="text-red-600">{error}</div>}
          {content !== null && !error && (
            isMd ? (
              <div className="msg-markdown msg-md-assistant prose prose-slate max-w-none">
                <ReactMarkdown remarkPlugins={supportsGfm ? [remarkGfm] : []}>{content}</ReactMarkdown>
              </div>
            ) : (
              <pre className="text-sm text-slate-800 whitespace-pre-wrap break-words font-mono">{content}</pre>
            )
          )}
        </div>
      </div>
    </div>
  )
}

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
  const s = typeof content === 'string' ? content : String(content ?? '')
  const hasEmbeddedMedia = /\]\(data:/.test(s)
  const isLong = !hasEmbeddedMedia && s.length > COLLAPSE_THRESHOLD
  const showCollapsed = isLong && !expanded
  const displayContent = showCollapsed ? s.slice(0, COLLAPSE_THRESHOLD) + '...' : s

  const wrapClass = isUser ? 'msg-md-user' : 'msg-md-assistant'

  const getExt = (url: string) => (url.split('?')[0].split('#')[0].match(/\.([a-zA-Z0-9]+)$/) || [])[1]?.toLowerCase()
  const videoExts = new Set(['mp4', 'webm', 'mov', 'ogg'])
  const audioExts = new Set(['mp3', 'wav', 'ogg', 'm4a', 'webm'])
  const textExts = new Set(['md', 'txt', 'json', 'csv', 'log', 'xml', 'yaml', 'yml', 'js', 'ts', 'tsx', 'jsx', 'py', 'html', 'htm', 'css', 'scss', 'sh', 'bash', 'sql', 'ini', 'cfg', 'conf'])
  const [previewModal, setPreviewModal] = useState<{ url: string; filename: string } | null>(null)
  const isSafeHref = (h: string) => {
    const lower = h.toLowerCase()
    if (lower.startsWith('https://') || lower.startsWith('http://')) return true
    if (!lower.startsWith('data:')) return false
    const m = lower.slice(5, 50)
    return (m.startsWith('image/') && !m.startsWith('image/svg')) || m.startsWith('audio/') || m.startsWith('video/') || m.startsWith('application/octet-stream')
  }
  const linkText = (c: React.ReactNode): string => {
    if (c == null) return ''
    if (typeof c === 'string') return c
    if (Array.isArray(c)) return c.map(linkText).join('')
    return ''
  }

  const markdownComponents = {
    img: ({ src, alt, ...props }: React.ImgHTMLAttributes<HTMLImageElement>) =>
      src && isSafeHref(src) ? <img src={src} alt={alt ?? ''} className="max-w-full max-h-80 rounded" {...props} /> : null,
    a: ({ href, children, ...props }: React.AnchorHTMLAttributes<HTMLAnchorElement>) => {
      if (!href) return <a {...props}>{children}</a>
      if (!isSafeHref(href)) return <span {...props}>{children}</span>
      if (href.startsWith('data:')) return <a href={href} download {...props}>{children}</a>
      const text = linkText(children)
      const filenameFromText = text.replace(/^[📎📹🔊]\s*/, '').trim()
      const ext = getExt(href) || (filenameFromText.split('.').pop()?.toLowerCase() ?? '')
      const filename = filenameFromText || (href.split('/').pop() || 'file').split('?')[0]
      // 优先按 bridge 的 emoji 标识：📹 video、🔊 audio
      if (text.includes('📹')) return <video src={href} controls className="max-w-full max-h-80 rounded" />
      if (text.includes('🔊')) return <audio src={href} controls className="max-w-full" />
      // 无 emoji 时按扩展名推断，ogg 多为音频、webm 多为视频
      if (ext && videoExts.has(ext) && ext !== 'ogg') return <video src={href} controls className="max-w-full max-h-80 rounded" />
      if (ext && audioExts.has(ext)) return <audio src={href} controls className="max-w-full" />
      const isTextFile = ext && textExts.has(ext)
      if (isTextFile) {
        return (
          <a
            href={href}
            onClick={(e) => { e.preventDefault(); setPreviewModal({ url: href, filename }) }}
            className="cursor-pointer"
            style={{ touchAction: 'manipulation' }}
          >
            {children}
          </a>
        )
      }
      const isFile = !!ext
      return <a href={href} {...(isFile ? { download: true } : {})} target="_blank" rel="noopener noreferrer" {...props}>{children}</a>
    },
  }

  return (
  <>
    <ErrorBoundary fallback={
      <div className={`msg-markdown ${wrapClass}`}>
        <div className="whitespace-pre-wrap break-words">{displayContent || '\u00A0'}</div>
        {isLong && (
          <button type="button" onClick={onToggleExpand} className="msg-expand-btn">
            {expanded ? '收起' : '展开'}
          </button>
        )}
      </div>
    }>
      <div className={`msg-markdown ${wrapClass}`}>
        <ReactMarkdown remarkPlugins={supportsGfm ? [remarkGfm] : []} components={markdownComponents}>{displayContent || '\u00A0'}</ReactMarkdown>
        {isLong && (
          <button type="button" onClick={onToggleExpand} className="msg-expand-btn">
            {expanded ? '收起' : '展开'}
          </button>
        )}
      </div>
    </ErrorBoundary>
    {previewModal && (
      <TextPreviewModal
        url={previewModal.url}
        filename={previewModal.filename}
        onClose={() => setPreviewModal(null)}
      />
    )}
  </>
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
  const markReadTimeoutRef = useRef<number | null>(null)

  const instanceId = parseInt(id ?? '', 10)

  const scheduleMarkRead = useCallback(() => {
    if (markReadTimeoutRef.current) clearTimeout(markReadTimeoutRef.current)
    markReadTimeoutRef.current = window.setTimeout(() => {
      markInstanceRead(instanceId).catch(() => {})
      markReadTimeoutRef.current = null
    }, 500)
  }, [instanceId])

  const loadMessages = useCallback(
    async (before?: number) => {
      if (isNaN(instanceId)) return []
      try {
        const { messages: list } = await getMessages(instanceId, PAGE_SIZE, before)
        const arr = Array.isArray(list) ? list : []
        return arr.map((m: ApiMessage) => ({
          id: m.id,
          content: m.content ?? '',
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
    const arr = Array.isArray(list) ? list : []
    const filtered = arr.filter((m) => !(m.role === 'assistant' && isThinkingPlaceholder(m.content ?? '')))
    setMessages([...filtered].reverse())
    setHasMore(arr.length >= PAGE_SIZE)
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
    const arr = Array.isArray(list) ? list : []
    const filtered = arr.filter((m) => !(m.role === 'assistant' && isThinkingPlaceholder(m.content ?? '')))
    setMessages((prev) => [...[...filtered].reverse(), ...prev])
    setHasMore(arr.length >= PAGE_SIZE)
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
      .then((inst) => setInstanceName(inst.name || '员工'))
      .catch(() => setInstanceName(''))
    markInstanceRead(instanceId).catch(() => {})
  }, [instanceId, navigate, loadInitial])

  // 合并服务端消息，避免 DB 与 WS 重复：user 按 content+u- 替换；assistant 按 content 替换或去重
  const mergeMessagesFromServer = useCallback(
    (list: ChatMessage[], setter: React.Dispatch<React.SetStateAction<ChatMessage[]>>) => {
      const arr = Array.isArray(list) ? list : []
      if (arr.length === 0) return
      const reversed = [...arr].reverse()
      setter((prev) => {
        const prevIds = new Set(prev.map((m) => m.id))
        let merged = [...prev]
        for (const m of reversed) {
          const role = m.role ?? 'assistant'
          const content = m.content ?? ''
          if (role === 'assistant' && isThinkingPlaceholder(content)) continue
          if (role === 'user') {
            const idx = merged.findIndex((x) => x.role === 'user' && x.content === content && String(x.id).startsWith('u-'))
            if (idx >= 0) {
              merged[idx] = { id: m.id, content, role }
              continue
            }
          }
          if (role === 'assistant') {
            const sameContentIdx = merged.findIndex((x) => x.role === 'assistant' && x.content === content)
            if (sameContentIdx >= 0) {
              merged[sameContentIdx] = { ...merged[sameContentIdx], id: m.id }
              continue
            }
          }
          if (!prevIds.has(m.id)) {
            merged = [...merged, { id: m.id, content, role }]
            prevIds.add(m.id)
          }
        }
        return merged
      })
    },
    []
  )

  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === 'visible' && !isNaN(instanceId)) {
        loadMessages().then((list) => mergeMessagesFromServer(list, setMessages))
      }
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  }, [instanceId, loadMessages, mergeMessagesFromServer])

  // 等待回答时轮询拉取，兜底 WebSocket 漏传
  useEffect(() => {
    if (!typing || isNaN(instanceId)) return
    const timer = setInterval(() => {
      loadMessages().then((list) => mergeMessagesFromServer(list, setMessages))
    }, 3000)
    return () => clearInterval(timer)
  }, [typing, instanceId, loadMessages, mergeMessagesFromServer])

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
              const mid = msg.payload.message_id ?? msg.id ?? 'a-' + Date.now()
              const content = String(msg.payload.content)
              if (isThinkingPlaceholder(content)) return
              setMessages((prev) => {
                if (prev.some((m) => m.id === mid)) return prev
                const sameContentIdx = prev.findIndex((m) => m.role === 'assistant' && m.content === content)
                if (sameContentIdx >= 0) {
                  return prev.map((m, i) => (i === sameContentIdx ? { ...m, id: mid } : m))
                }
                // 媒体消息且上一条 assistant 是纯文本：追加到上一条，不单独成条
                if (isMediaContent(content)) {
                  const lastIdx = prev.map((m, i) => (m.role === 'assistant' ? i : -1)).filter((i) => i >= 0).pop()
                  if (lastIdx != null && !isMediaContent(prev[lastIdx]?.content ?? '')) {
                    const merged = (prev[lastIdx]!.content + '\n\n' + content).trim()
                    return prev.map((m, i) => (i === lastIdx ? { ...m, content: merged } : m))
                  }
                }
                return [...prev, { id: mid, content, role: (msg.payload!.role as string) || 'assistant' }]
              })
              scheduleMarkRead()
            }
            break
          case 'message.update':
            if (msg.payload?.content != null) {
              const targetId = msg.payload.message_id ?? msg.id
              const content = String(msg.payload.content)
              if (isThinkingPlaceholder(content)) return
              setMessages((prev) => {
                if (targetId) {
                  const idx = prev.findIndex((m) => m.id === targetId)
                  if (idx >= 0) {
                    return prev.map((m) => (m.id === targetId ? { ...m, content } : m))
                  }
                  const thinkingIdx = prev.findIndex((m) => m.role === 'assistant' && isThinkingPlaceholder(m.content))
                  if (thinkingIdx >= 0) {
                    return prev.map((m, i) => (i === thinkingIdx ? { ...m, id: targetId, content } : m))
                  }
                }
                // 无 targetId 时：若最后一条是媒体消息，不覆盖，追加新消息；否则更新最后一条
                const lastAssistantIdx = prev.map((m, i) => (m.role === 'assistant' ? i : -1)).filter((i) => i >= 0).pop()
                if (lastAssistantIdx != null) {
                  const lastContent = prev[lastAssistantIdx]?.content ?? ''
                  if (isMediaContent(lastContent)) {
                    return [...prev, { id: targetId || 'a-' + Date.now(), content, role: 'assistant' }]
                  }
                  return prev.map((m, i) => (i === lastAssistantIdx ? { ...m, content } : m))
                }
                return [...prev, { id: targetId || 'a-' + Date.now(), content, role: 'assistant' }]
              })
              scheduleMarkRead()
            }
            break
          case 'typing.start':
            setTyping(true)
            setTypingPhraseIndex(0)
            break
          case 'typing.stop':
            setTyping(false)
            loadMessages().then((list) => mergeMessagesFromServer(list, setMessages))
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
      if (markReadTimeoutRef.current) clearTimeout(markReadTimeoutRef.current)
    }
  }, [instanceId, scheduleMarkRead, loadMessages, mergeMessagesFromServer])

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
  const inputRef = useRef<HTMLTextAreaElement>(null)

  // 输入框内容变化时自动调整高度（最多约 4 行）
  useEffect(() => {
    const el = inputRef.current
    if (!el) return
    el.style.height = 'auto'
    const h = Math.max(44, Math.min(el.scrollHeight, 128))
    el.style.height = `${h}px`
  }, [input])

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

  const doSend = useCallback(() => {
    const content = input.trim()
    if (!content || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return
    const userMsgId = 'u-' + Date.now()
    setMessages((prev) => [...prev, { id: userMsgId, content, role: 'user' }])
    wsRef.current.send(JSON.stringify({ type: 'message.send', payload: { content } }))
    setInput('')
  }, [input])

  const sendMessage = (e: React.FormEvent) => {
    e.preventDefault()
    doSend()
  }

  if (isNaN(instanceId)) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <p className="text-red-600">Invalid instance</p>
      </div>
    )
  }

  return (
    <div className="fixed inset-0 flex flex-col overflow-hidden bg-gradient-to-b from-slate-50 to-white sm:bg-white sm:max-w-2xl sm:mx-auto sm:my-4 sm:rounded-2xl sm:shadow-xl sm:border sm:border-slate-200/80 sm:min-h-[80vh] z-10 chat-mobile-h sm:h-auto sm:overflow-hidden">
      {/* 顶部栏 - 简洁紧凑 */}
      <div className="flex items-center gap-2 px-3 py-2.5 sm:py-3 sm:px-4 bg-white/95 backdrop-blur-sm border-b border-slate-200/80 flex-shrink-0">
        <button
          onClick={() => navigate('/')}
          className="flex items-center justify-center w-9 h-9 -ml-1 text-slate-600 hover:text-slate-800 hover:bg-slate-100 active:bg-slate-200 rounded-xl transition-colors touch-manipulation"
          aria-label="返回"
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" />
          </svg>
        </button>
        <div className="flex-1 min-w-0 flex items-center gap-2">
          <span className="font-semibold text-slate-800 truncate">{instanceName || '...'}</span>
          <span className={`flex items-center gap-1.5 text-xs ${connected ? 'text-emerald-600' : 'text-slate-400'}`}>
            <span className={`w-2 h-2 rounded-full flex-shrink-0 ${connected ? 'bg-emerald-500 animate-pulse' : 'bg-slate-300'}`} />
            <span className="hidden sm:inline">{connected ? '在线' : '离线'}</span>
          </span>
        </div>
        <button
          type="button"
          onClick={() => loadMessages().then((list) => mergeMessagesFromServer(list, setMessages))}
          className="p-2 text-slate-400 hover:text-indigo-600 hover:bg-indigo-50 rounded-lg transition-colors"
          title="刷新消息"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
        </button>
      </div>

      {error && (
        <div className="px-4 py-2.5 bg-rose-50/95 border-b border-rose-100 text-rose-700 text-sm flex-shrink-0 flex items-center gap-2">
          <svg className="w-4 h-4 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
          </svg>
          {error}
        </div>
      )}

      {/* 消息列表 */}
      <div
        ref={listRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto overscroll-contain px-3 py-4 sm:px-5 min-h-0 chat-scroll"
      >
        <ErrorBoundary fallback={
          <div className="py-8 px-4 text-center text-slate-600 text-sm">
            加载出错，请
            <button type="button" onClick={() => window.location.reload()} className="text-indigo-600 underline ml-1">
              刷新
            </button>
            重试
          </div>
        }>
        {loading ? (
          <div className="flex flex-col items-center justify-center py-16 gap-3">
            <div className="w-10 h-10 rounded-full border-2 border-indigo-200 border-t-indigo-600 animate-spin" />
            <p className="text-slate-500 text-sm">马上就好～</p>
          </div>
        ) : (
          <>
            {loadingMore && (
              <div className="flex justify-center py-3">
                <span className="inline-flex items-center gap-1.5 text-slate-400 text-xs">
                  <span className="w-3 h-3 rounded-full border border-slate-300 border-t-slate-500 animate-spin" />
                  加载更多...
                </span>
              </div>
            )}
            {messages.length === 0 && !typing && (
              <ErrorBoundary fallback={
                <div className="py-12 px-4 text-center">
                  <p className="text-slate-600 text-sm">打个招呼吧～</p>
                </div>
              }>
                <div className="py-12 px-4 text-center">
                  <div className="w-20 h-20 mx-auto mb-4 rounded-2xl bg-gradient-to-br from-indigo-100 to-slate-100 flex items-center justify-center shadow-inner">
                    <img src="/10003.png" alt="" className="w-14 h-14 object-contain" aria-hidden />
                  </div>
                  <p className="text-slate-700 font-medium mb-1">打个招呼吧～</p>
                  <p className="text-slate-500 text-sm mb-6">发送任意消息开始对话</p>
                  <div className="text-left max-w-sm mx-auto p-4 bg-white/80 rounded-2xl border border-slate-200/80 shadow-sm text-xs text-slate-600 space-y-2.5">
                    <p className="font-semibold text-slate-700">OpenClaw：效率工具，一人公司的底座</p>
                    <p>· 擅长复杂任务，会调用工具、查资料、执行操作</p>
                    <p>· 拥有超长记忆，会记住你们的对话与约定</p>
                    <p>· 每位员工都有独特的专长，会随协作而成长</p>
                    <p>· 回答前会深入思考，请耐心等待～</p>
                  </div>
                </div>
              </ErrorBoundary>
            )}
            <div className="space-y-4">
              {messages
                .filter((m) => !isThinkingPlaceholder(m.content ?? ''))
                .reduce(
                  (acc: { list: ChatMessage[]; ids: Set<string | number> }, m) => {
                    if (acc.ids.has(m.id as string | number)) return acc
                    const last = acc.list[acc.list.length - 1]
                    if (last && m.role === 'assistant' && last.role === 'assistant' && m.content === last.content) return acc
                    acc.ids.add(m.id as string | number)
                    return { list: [...acc.list, m], ids: acc.ids }
                  },
                  { list: [] as ChatMessage[], ids: new Set<string | number>() }
                ).list
                .map((m) => {
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
                    className={`flex gap-2 ${isUser ? 'flex-row-reverse' : 'flex-row'}`}
                  >
                    <div className={`flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center text-sm ${
                      isUser ? 'bg-indigo-500 text-white' : 'bg-slate-200 text-slate-600'
                    }`}>
                      {isUser ? '我' : '🦞'}
                    </div>
                    <div
                      className={`max-w-[78%] sm:max-w-[70%] rounded-2xl px-4 py-3 transition-shadow ${
                        isUser
                          ? 'bg-indigo-600 text-white rounded-br-md shadow-md shadow-indigo-200/50'
                          : 'bg-white border border-slate-200/80 text-slate-800 rounded-bl-md shadow-sm hover:shadow-md'
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
              <ErrorBoundary fallback={
                <div className="flex gap-2 mt-4">
                  <div className="w-8 h-8 rounded-full bg-slate-200 flex items-center justify-center text-sm">🦞</div>
                  <div className="bg-white border border-slate-200 rounded-2xl rounded-bl-md px-4 py-3 shadow-sm">
                    <span className="text-slate-500 text-sm">思考中...</span>
                  </div>
                </div>
              }>
                <div className="flex flex-col gap-2 mt-4">
                  <div className="flex gap-2">
                    <div className="w-8 h-8 rounded-full bg-slate-200 flex items-center justify-center text-sm flex-shrink-0">🦞</div>
                    <div className="typing-breathe bg-white border border-slate-200/80 rounded-2xl rounded-bl-md px-4 py-3 shadow-sm flex items-center gap-2 min-w-[120px]">
                      <span className="text-slate-500 text-sm">{TYPING_PHRASES[typingPhraseIndex]}</span>
                      <span className="flex gap-1">
                        {[1, 2, 3].map((i) => (
                          <span key={i} className="typing-dot w-1.5 h-1.5 rounded-full bg-indigo-400 inline-block" />
                        ))}
                      </span>
                    </div>
                  </div>
                  <p className="text-slate-400 text-xs text-center px-2">
                    可离开页面，回答会继续。请耐心等待～
                  </p>
                </div>
              </ErrorBoundary>
            )}
          </>
        )}
        </ErrorBoundary>
      </div>

      {/* 输入区 */}
      <form
        onSubmit={sendMessage}
        className="flex gap-2 p-3 sm:p-4 pb-[max(0.75rem,env(safe-area-inset-bottom))] bg-white/95 backdrop-blur-sm border-t border-slate-200/80 flex-shrink-0"
      >
        <div className="flex-1 min-w-0 flex items-end gap-2 bg-slate-100 rounded-2xl px-4 py-2 focus-within:ring-2 focus-within:ring-indigo-500/50 focus-within:bg-white transition-all">
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                doSend()
              }
            }}
            placeholder="说点什么～"
            disabled={!connected}
            rows={1}
            className="flex-1 min-h-[44px] max-h-32 py-2.5 bg-transparent border-none focus:outline-none resize-none disabled:opacity-50 text-base text-slate-800 placeholder-slate-400"
            style={{ fontSize: '16px' }}
          />
        </div>
        <button
          type="submit"
          disabled={!connected || !input.trim()}
          className="flex-shrink-0 w-11 h-11 sm:w-12 sm:h-12 flex items-center justify-center bg-indigo-600 text-white rounded-xl hover:bg-indigo-700 active:bg-indigo-800 disabled:opacity-50 disabled:hover:bg-indigo-600 transition-colors touch-manipulation"
          aria-label="发送"
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
          </svg>
        </button>
      </form>
    </div>
  )
}
