import { useState, useEffect, useLayoutEffect, useRef, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  getToken,
  getWebSocketUrl,
  getMessages,
  getInstance,
  markInstanceRead,
  fetchProxyText,
  uploadMedia,
  broadcastCollabEvent,
  type ChatMessage as ApiMessage,
} from '../api'

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
  timestamp?: number
  payload?: { content?: string; role?: string; message_id?: string; [key: string]: unknown }
}

interface ChatMessage {
  id: string | number
  content: string
  role?: string
  created_at?: string
}

// 初始只加载约两屏，其余通过上拉加载
const PAGE_SIZE = 10

const TYPING_PHRASES = ['嗯...', '想想看...', '稍等一下下～', '快好啦～', '马上就好～']

/** 从展示内容中移除 Thinking... 占位符行（供 mergeMediaIntoPrevious 等使用） */
function stripThinkingFromContent(content: string): string {
  return content
    .split('\n')
    .filter((line) => !/^Thinking\.\.\./i.test(line.trim()))
    .join('\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

/** 将连续的 assistant 消息中，第二条为媒体内容时合并到第一条，避免刷新后链接丢失。合并时移除 Thinking... */
function mergeMediaIntoPrevious(list: ChatMessage[]): ChatMessage[] {
  const result: ChatMessage[] = []
  for (const m of list) {
    const last = result[result.length - 1]
    if (last && m.role === 'assistant' && last.role === 'assistant' && isMediaContent(m.content) && !isMediaContent(last.content)) {
      const cleanMedia = stripThinkingFromContent(m.content)
      if (cleanMedia) {
        result[result.length - 1] = {
          ...last,
          content: (stripThinkingFromContent(last.content) + '\n\n' + cleanMedia).trim(),
        }
      } else {
        result.push(m)
      }
    } else {
      const cleaned = stripThinkingFromContent(m.content) || m.content
      if (m.role === 'assistant' && !cleaned.trim()) continue
      result.push({ ...m, content: cleaned })
    }
  }
  return result
}

function formatMessageTime(iso: string): string {
  try {
    const d = new Date(iso)
    if (isNaN(d.getTime())) return ''

    const tz = 'Asia/Shanghai'
    const parts = new Intl.DateTimeFormat('en-US', {
      timeZone: tz,
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
    }).formatToParts(d)
    const get = (t: string) => parts.find(p => p.type === t)?.value ?? ''
    const time = `${get('hour').padStart(2, '0')}:${get('minute').padStart(2, '0')}`
    const msgYmd = `${get('year')}-${get('month')}-${get('day')}`

    const nowParts = new Intl.DateTimeFormat('en-US', {
      timeZone: tz, year: 'numeric', month: '2-digit', day: '2-digit',
    }).formatToParts(new Date())
    const getN = (t: string) => nowParts.find(p => p.type === t)?.value ?? ''
    const todayYmd = `${getN('year')}-${getN('month')}-${getN('day')}`

    if (msgYmd === todayYmd) return time
    const diffDays = Math.round(
      (new Date(todayYmd).getTime() - new Date(msgYmd).getTime()) / 86400000
    )
    if (diffDays === 1) return `昨天 ${time}`
    if (diffDays > 1 && diffDays < 7) return `${diffDays}天前 ${time}`
    return `${parseInt(get('month'))}月${parseInt(get('day'))}日 ${time}`
  } catch {
    return ''
  }
}

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
  const raw = typeof content === 'string' ? content : String(content ?? '')
  const s = isUser ? raw : stripThinkingFromContent(raw)
  const hasEmbeddedMedia = /\]\(data:/.test(s)
  const isLong = !hasEmbeddedMedia && s.length > COLLAPSE_THRESHOLD
  const showCollapsed = isLong && !expanded
  const remainingChars = s.length - COLLAPSE_THRESHOLD
  const displayContent = showCollapsed ? s.slice(0, COLLAPSE_THRESHOLD) : s

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
        <div className={`msg-content-inner${showCollapsed ? ' msg-content-collapsed' : ''}`}>
          <div className="whitespace-pre-wrap break-words">{displayContent || '\u00A0'}</div>
        </div>
        {isLong && (
          <button type="button" onClick={onToggleExpand} className="msg-expand-btn">
            {expanded ? (
              <>
                <svg className="msg-expand-icon msg-expand-icon-up" viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z" clipRule="evenodd" /></svg>
                <span>收起</span>
              </>
            ) : (
              <>
                <svg className="msg-expand-icon" viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clipRule="evenodd" /></svg>
                <span>展开剩余 {remainingChars} 字</span>
              </>
            )}
          </button>
        )}
      </div>
    }>
      <div className={`msg-markdown ${wrapClass}`}>
        <div className={`msg-content-inner${showCollapsed ? ' msg-content-collapsed' : ''}${isLong && expanded ? ' msg-content-expanded' : ''}`}>
          <ReactMarkdown remarkPlugins={supportsGfm ? [remarkGfm] : []} components={markdownComponents}>{displayContent || '\u00A0'}</ReactMarkdown>
        </div>
        {isLong && (
          <button type="button" onClick={onToggleExpand} className="msg-expand-btn">
            {expanded ? (
              <>
                <svg className="msg-expand-icon msg-expand-icon-up" viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z" clipRule="evenodd" /></svg>
                <span>收起</span>
              </>
            ) : (
              <>
                <svg className="msg-expand-icon" viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clipRule="evenodd" /></svg>
                <span>展开剩余 {remainingChars} 字</span>
              </>
            )}
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
  const [showScrollBtn, setShowScrollBtn] = useState(false)
  // Voice recording state
  const [isRecording, setIsRecording] = useState(false)
  const [recordingDuration, setRecordingDuration] = useState(0)
  const [isCancelGesture, setIsCancelGesture] = useState(false)
  const [recordedBlob, setRecordedBlob] = useState<Blob | null>(null)
  const [voiceUploading, setVoiceUploading] = useState(false)
  const [voiceError, setVoiceError] = useState('')
  const [imageUploading, setImageUploading] = useState(false)
  const [imageError, setImageError] = useState('')

  const wsRef = useRef<WebSocket | null>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const loadingMoreRef = useRef(false)
  const markReadTimeoutRef = useRef<number | null>(null)
  const isAtBottomRef = useRef(true)
  const scrollRestorationRef = useRef<{ prevScrollHeight: number; prevScrollTop: number } | null>(null)
  const knownMsgIdsRef = useRef(new Set<string | number>())
  const initialLoadDoneRef = useRef(false)
  // Voice recording refs
  const mediaRecorderRef = useRef<MediaRecorder | null>(null)
  const audioChunksRef = useRef<Blob[]>([])
  const recordingTimerRef = useRef<number | null>(null)
  const recordingStartYRef = useRef(0)
  const touchUsedRef = useRef(false)
  const streamRef = useRef<MediaStream | null>(null)
  const imageInputRef = useRef<HTMLInputElement>(null)
  const isMobile = typeof window !== 'undefined' && ('ontouchstart' in window || navigator.maxTouchPoints > 0)
  const MAX_RECORDING_SECONDS = 60

  const instanceId = parseInt(id ?? '', 10)

  const scheduleMarkRead = useCallback(() => {
    if (markReadTimeoutRef.current) clearTimeout(markReadTimeoutRef.current)
    markReadTimeoutRef.current = window.setTimeout(() => {
      markInstanceRead(instanceId).catch(() => {})
      markReadTimeoutRef.current = null
    }, 500)
  }, [instanceId])

  const loadMessages = useCallback(
    async (before?: number): Promise<{ list: ChatMessage[]; rawCount: number }> => {
      if (isNaN(instanceId)) return { list: [], rawCount: 0 }
      try {
        const { messages: list } = await getMessages(instanceId, PAGE_SIZE, before)
        const arr = Array.isArray(list) ? list : []
        const mapped = arr.map((m: ApiMessage) => ({
          id: m.id,
          content: m.content ?? '',
          role: m.role,
          created_at: m.created_at,
        }))
        return { list: mergeMediaIntoPrevious(mapped), rawCount: arr.length }
      } catch {
        return { list: [], rawCount: 0 }
      }
    },
    [instanceId]
  )

  const loadInitial = useCallback(async () => {
    setLoading(true)
    const { list, rawCount } = await loadMessages()
    const filtered = list.filter((m) => !(m.role === 'assistant' && isThinkingPlaceholder(m.content ?? '')))
    setMessages([...filtered].reverse())
    setHasMore(rawCount >= PAGE_SIZE)
    setLoading(false)
  }, [loadMessages])

  const loadOlder = useCallback(async () => {
    if (loadingMoreRef.current || !hasMore || messages.length === 0) return
    const oldest = messages.find((m) => typeof m.id === 'number')
    const oldestId = oldest?.id as number | undefined
    if (oldestId == null) return
    loadingMoreRef.current = true
    setLoadingMore(true)
    const el = listRef.current
    const prevScrollHeight = el?.scrollHeight ?? 0
    const prevScrollTop = el?.scrollTop ?? 0
    const { list, rawCount } = await loadMessages(oldestId as number)
    const filtered = list.filter((m) => !(m.role === 'assistant' && isThinkingPlaceholder(m.content ?? '')))
    if (filtered.length === 0) {
      loadingMoreRef.current = false
      setLoadingMore(false)
      setHasMore(rawCount >= PAGE_SIZE)
      return
    }
    scrollRestorationRef.current = { prevScrollHeight, prevScrollTop }
    setMessages((prev) => [...[...filtered].reverse(), ...prev])
    setHasMore(rawCount >= PAGE_SIZE)
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

  // 以 API 为权威来源：API 是 DB 合并后的结果，直接采用；仅追加未落库的乐观 user
  const mergeMessagesFromServer = useCallback(
    (list: ChatMessage[], setter: React.Dispatch<React.SetStateAction<ChatMessage[]>>) => {
      const apiList = mergeMediaIntoPrevious(Array.isArray(list) ? list : []).filter(
        (m) => !(m.role === 'assistant' && isThinkingPlaceholder(m.content ?? ''))
      )
      if (apiList.length === 0) return
      const apiChronological = [...apiList].reverse()
      setter((prev) => {
        const apiNumericIds = apiChronological.filter((m) => typeof m.id === 'number').map((m) => m.id as number)
        const apiMinId = apiNumericIds.length > 0 ? Math.min(...apiNumericIds) : Infinity
        const olderMessages = prev.filter((m) => typeof m.id === 'number' && (m.id as number) < apiMinId)
        const apiUserContents = new Set(apiChronological.filter((m) => m.role === 'user').map((m) => (m.content ?? '').trim()))
        const optimisticUsers = prev.filter((m) => m.role === 'user' && String(m.id).startsWith('u-') && !apiUserContents.has((m.content ?? '').trim()))
        return [...olderMessages, ...apiChronological, ...optimisticUsers]
      })
    },
    []
  )

  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === 'visible' && !isNaN(instanceId)) {
        loadMessages().then(({ list }) => {
          mergeMessagesFromServer(list, setMessages)
        })
      }
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  }, [instanceId, loadMessages, mergeMessagesFromServer])

  // 等待回答时轮询拉取，兜底 WebSocket 漏传
  useEffect(() => {
    if (!typing || isNaN(instanceId)) return
    const timer = setInterval(() => {
      loadMessages().then(({ list }) => mergeMessagesFromServer(list, setMessages))
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
        const wsTime = msg.timestamp ? new Date(msg.timestamp).toISOString() : new Date().toISOString()
        switch (msg.type) {
          case 'message.create':
            if (msg.payload?.content != null) {
              const mid = msg.payload.message_id ?? msg.id ?? 'a-' + Date.now()
              const content = String(msg.payload.content)
              setMessages((prev) => {
                if (prev.some((m) => m.id === mid)) return prev
                const sameContentIdx = prev.findIndex((m) => m.role === 'assistant' && m.content === content)
                if (sameContentIdx >= 0) {
                  return prev.map((m, i) => (i === sameContentIdx ? { ...m, id: mid, created_at: m.created_at || wsTime } : m))
                }
                const lastIdx = prev.map((m, i) => (m.role === 'assistant' ? i : -1)).filter((i) => i >= 0).pop()
                if (isThinkingPlaceholder(content)) {
                  return [...prev, { id: mid, content, role: (msg.payload!.role as string) || 'assistant', created_at: wsTime }]
                }
                if (lastIdx != null && isThinkingPlaceholder(prev[lastIdx]?.content ?? '')) {
                  return prev.map((m, i) => (i === lastIdx ? { ...m, id: mid, content, created_at: m.created_at || wsTime } : m))
                }
                if (isMediaContent(content) && lastIdx != null) {
                  const lastContent = prev[lastIdx]?.content ?? ''
                  if (lastContent.includes(content)) return prev
                  if (!isMediaContent(lastContent)) {
                    const merged = (lastContent + '\n\n' + content).trim()
                    return prev.map((m, i) => (i === lastIdx ? { ...m, content: merged } : m))
                  }
                }
                return [...prev, { id: mid, content, role: (msg.payload!.role as string) || 'assistant', created_at: wsTime }]
              })
              scheduleMarkRead()
            }
            break
          case 'message.update':
            if (msg.payload?.content != null) {
              const targetId = msg.payload.message_id ?? msg.id
              const content = String(msg.payload.content)
              setMessages((prev) => {
                if (targetId) {
                  const idx = prev.findIndex((m) => m.id === targetId)
                  if (idx >= 0) {
                    return prev.map((m) => (m.id === targetId ? { ...m, content } : m))
                  }
                  const thinkingIdx = prev.findIndex((m) => m.role === 'assistant' && isThinkingPlaceholder(m.content))
                  if (thinkingIdx >= 0) {
                    return prev.map((m, i) => (i === thinkingIdx ? { ...m, id: targetId, content, created_at: m.created_at || wsTime } : m))
                  }
                }
                const lastAssistantIdx = prev.map((m, i) => (m.role === 'assistant' ? i : -1)).filter((i) => i >= 0).pop()
                if (lastAssistantIdx != null) {
                  const lastContent = prev[lastAssistantIdx]?.content ?? ''
                  if (isMediaContent(lastContent)) {
                    return [...prev, { id: targetId || 'a-' + Date.now(), content, role: 'assistant', created_at: wsTime }]
                  }
                  return prev.map((m, i) => (i === lastAssistantIdx ? { ...m, content } : m))
                }
                return [...prev, { id: targetId || 'a-' + Date.now(), content, role: 'assistant', created_at: wsTime }]
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
            loadMessages().then(({ list }) => mergeMessagesFromServer(list, setMessages))
            break
          case 'collab.internal_mail':
            broadcastCollabEvent('internal_mail', instanceId)
            break
          case 'collab.instance_mail':
            broadcastCollabEvent('instance_mail', instanceId)
            break
          case 'collab.topology_updated':
            broadcastCollabEvent('topology', instanceId)
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

  // loadOlder 滚动恢复：在浏览器绘制前同步调整，避免可见跳动
  useLayoutEffect(() => {
    const restore = scrollRestorationRef.current
    const el = listRef.current
    if (restore && el) {
      el.scrollTop = el.scrollHeight - restore.prevScrollHeight + restore.prevScrollTop
      scrollRestorationRef.current = null
      loadingMoreRef.current = false
      setLoadingMore(false)
    }
  }, [messages])

  // 追踪已知消息 ID，用于判断哪些消息需要淡入动画
  useLayoutEffect(() => {
    knownMsgIdsRef.current = new Set(messages.map(m => m.id))
    if (!initialLoadDoneRef.current && !loading && messages.length > 0) {
      initialLoadDoneRef.current = true
    }
  }, [messages, loading])

  useEffect(() => {
    if (loadingMoreRef.current) return
    if (isAtBottomRef.current) {
      listRef.current?.scrollTo(0, listRef.current?.scrollHeight ?? 0)
    }
  }, [messages, typing])

  useEffect(() => {
    if (loading || loadingMore || !hasMore) return
    const el = listRef.current
    if (!el) return
    if (el.scrollHeight <= el.clientHeight) {
      loadOlder()
    }
  }, [loading, loadingMore, hasMore, messages, loadOlder])

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
      isAtBottomRef.current = true
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
    if (!el) return
    const threshold = 100
    const distFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
    isAtBottomRef.current = distFromBottom < threshold
    setShowScrollBtn(distFromBottom > 400)
    if (!loadingMoreRef.current && hasMore && el.scrollTop < 80) loadOlder()
  }

  const scrollToBottom = useCallback(() => {
    const el = listRef.current
    if (!el) return
    isAtBottomRef.current = true
    setShowScrollBtn(false)
    el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
  }, [])

  const doSend = useCallback(() => {
    const content = input.trim()
    if (!content || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return
    isAtBottomRef.current = true
    const userMsgId = 'u-' + Date.now()
    setMessages((prev) => [...prev, { id: userMsgId, content, role: 'user', created_at: new Date().toISOString() }])
    wsRef.current.send(JSON.stringify({ type: 'message.send', payload: { content } }))
    setInput('')
  }, [input])

  const sendMessage = (e: React.FormEvent) => {
    e.preventDefault()
    doSend()
  }

  const cleanupRecording = useCallback(() => {
    if (recordingTimerRef.current) {
      clearInterval(recordingTimerRef.current)
      recordingTimerRef.current = null
    }
    if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
      mediaRecorderRef.current.stop()
    }
    mediaRecorderRef.current = null
    if (streamRef.current) {
      streamRef.current.getTracks().forEach(t => t.stop())
      streamRef.current = null
    }
    audioChunksRef.current = []
  }, [])

  useEffect(() => {
    return () => cleanupRecording()
  }, [cleanupRecording])

  const sendVoiceBlob = useCallback(async (blob: Blob) => {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return
    setVoiceUploading(true)
    setVoiceError('')
    try {
      const ext = blob.type.includes('ogg') ? 'ogg' : 'webm'
      const filename = `voice_${Date.now()}.${ext}`
      const { url } = await uploadMedia(instanceId, blob, filename)

      isAtBottomRef.current = true
      const userMsgId = 'u-' + Date.now()
      const content = `[🔊 语音消息](${url})`
      setMessages((prev) => [...prev, { id: userMsgId, content, role: 'user', created_at: new Date().toISOString() }])
      wsRef.current.send(JSON.stringify({
        type: 'message.send',
        payload: { content, media_url: url, media_type: 'audio' },
      }))
      setRecordedBlob(null)
    } catch (e) {
      setVoiceError(e instanceof Error ? e.message : '语音发送失败')
    } finally {
      setVoiceUploading(false)
    }
  }, [instanceId])

  const sendImageFile = useCallback(async (file: File) => {
    if (!file.type.startsWith('image/')) {
      setImageError('请选择图片文件')
      return
    }
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      setImageError('未连接或连接已断开')
      return
    }
    const caption = (inputRef.current?.value ?? '').trim()
    setImageError('')
    setImageUploading(true)
    try {
      const { url } = await uploadMedia(instanceId, file, file.name || `image_${Date.now()}.jpg`)
      const display = caption ? `${caption}\n\n![图片](${url})` : `![图片](${url})`
      setInput('')
      isAtBottomRef.current = true
      const userMsgId = 'u-' + Date.now()
      setMessages((prev) => [...prev, { id: userMsgId, content: display, role: 'user', created_at: new Date().toISOString() }])
      const ws = wsRef.current
      if (!ws || ws.readyState !== WebSocket.OPEN) {
        setImageError('发送失败：连接已断开')
        setMessages((prev) => prev.filter((m) => m.id !== userMsgId))
        return
      }
      ws.send(JSON.stringify({
        type: 'message.send',
        payload: { content: caption, media_url: url, media_type: 'image' },
      }))
    } catch (e) {
      setImageError(e instanceof Error ? e.message : '图片发送失败')
    } finally {
      setImageUploading(false)
    }
  }, [instanceId])

  const onImageInputChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    e.target.value = ''
    if (f) void sendImageFile(f)
  }, [sendImageFile])

  const sendVoiceBlobRef = useRef(sendVoiceBlob)
  sendVoiceBlobRef.current = sendVoiceBlob

  const stopRecordingRef = useRef<(cancel: boolean) => void>(() => {})

  const stopRecording = useCallback((cancel: boolean) => {
    if (!mediaRecorderRef.current || mediaRecorderRef.current.state === 'inactive') {
      cleanupRecording()
      setIsRecording(false)
      return
    }

    const recorder = mediaRecorderRef.current
    recorder.onstop = () => {
      if (cancel) {
        cleanupRecording()
        setIsRecording(false)
        return
      }
      const mimeType = recorder.mimeType || 'audio/webm'
      const blob = new Blob(audioChunksRef.current, { type: mimeType })
      if (blob.size < 1000) {
        cleanupRecording()
        setIsRecording(false)
        return
      }
      if (isMobile) {
        sendVoiceBlobRef.current(blob)
      } else {
        setRecordedBlob(blob)
      }
      if (streamRef.current) {
        streamRef.current.getTracks().forEach(t => t.stop())
        streamRef.current = null
      }
      if (recordingTimerRef.current) {
        clearInterval(recordingTimerRef.current)
        recordingTimerRef.current = null
      }
      mediaRecorderRef.current = null
      audioChunksRef.current = []
      setIsRecording(false)
    }
    recorder.stop()
  }, [cleanupRecording, isMobile])

  stopRecordingRef.current = stopRecording

  const startRecording = useCallback(async () => {
    setVoiceError('')
    setRecordedBlob(null)
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      streamRef.current = stream
      let mimeType = 'audio/webm;codecs=opus'
      if (!MediaRecorder.isTypeSupported(mimeType)) {
        mimeType = 'audio/webm'
        if (!MediaRecorder.isTypeSupported(mimeType)) {
          mimeType = 'audio/ogg;codecs=opus'
          if (!MediaRecorder.isTypeSupported(mimeType)) {
            mimeType = ''
          }
        }
      }
      const recorder = mimeType
        ? new MediaRecorder(stream, { mimeType })
        : new MediaRecorder(stream)
      mediaRecorderRef.current = recorder
      audioChunksRef.current = []

      recorder.ondataavailable = (e) => {
        if (e.data.size > 0) audioChunksRef.current.push(e.data)
      }

      recorder.start(200)
      setIsRecording(true)
      setRecordingDuration(0)
      setIsCancelGesture(false)

      const startTime = Date.now()
      recordingTimerRef.current = window.setInterval(() => {
        const elapsed = Math.floor((Date.now() - startTime) / 1000)
        setRecordingDuration(elapsed)
        if (elapsed >= MAX_RECORDING_SECONDS) {
          stopRecordingRef.current(false)
        }
      }, 200)
    } catch {
      setVoiceError('无法访问麦克风')
    }
  }, [])

  const cancelPreview = useCallback(() => {
    setRecordedBlob(null)
    setVoiceError('')
  }, [])

  const handleMicTouchStart = useCallback((e: React.TouchEvent) => {
    e.preventDefault()
    touchUsedRef.current = true
    recordingStartYRef.current = e.touches[0].clientY
    startRecording()
  }, [startRecording])

  const handleMicTouchMove = useCallback((e: React.TouchEvent) => {
    if (!isRecording) return
    const dy = recordingStartYRef.current - e.touches[0].clientY
    setIsCancelGesture(dy > 60)
  }, [isRecording])

  const handleMicTouchEnd = useCallback(() => {
    if (!isRecording) return
    stopRecording(isCancelGesture)
    setIsCancelGesture(false)
  }, [isRecording, isCancelGesture, stopRecording])

  const handleMicClick = useCallback(() => {
    if (touchUsedRef.current) {
      touchUsedRef.current = false
      return
    }
    if (isRecording) {
      stopRecording(false)
    } else {
      startRecording()
    }
  }, [isRecording, startRecording, stopRecording])

  const supportsRecording = typeof window !== 'undefined' && !!navigator.mediaDevices?.getUserMedia && typeof MediaRecorder !== 'undefined'

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
          <button
            type="button"
            onClick={() => navigate('/', { state: { orchestrateInstanceId: instanceId } })}
            className="text-xs px-2 py-1 rounded-lg bg-violet-50 text-violet-700 border border-violet-200 hover:bg-violet-100 flex-shrink-0"
            title="回首页：编排连线与内部邮件"
          >
            协作
          </button>
          <span className={`flex items-center gap-1.5 text-xs ${connected ? 'text-emerald-600' : 'text-slate-400'}`}>
            <span className={`w-2 h-2 rounded-full flex-shrink-0 ${connected ? 'bg-emerald-500 animate-pulse' : 'bg-slate-300'}`} />
            <span className="hidden sm:inline">{connected ? '在线' : '离线'}</span>
          </span>
        </div>
        <button
          type="button"
          onClick={() => {
            loadMessages().then(({ list }) => {
              mergeMessagesFromServer(list, setMessages)
            })
          }}
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
      <div className="relative flex-1 min-h-0">
      <div
        ref={listRef}
        onScroll={handleScroll}
        className="h-full overflow-y-auto overscroll-contain px-3 py-4 sm:px-5 chat-scroll"
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
          <div className="space-y-4 py-2">
            {[0, 1, 2, 3, 4].map((i) => (
              <div key={i} className={`flex gap-2 ${i % 3 === 0 ? 'flex-row-reverse' : 'flex-row'}`}>
                <div className="w-8 h-8 rounded-full bg-slate-200/60 flex-shrink-0 msg-skeleton-bar" />
                <div
                  className={`rounded-2xl px-4 py-3 ${i % 3 === 0 ? 'bg-indigo-50/40 rounded-br-md' : 'bg-slate-100/60 rounded-bl-md'}`}
                  style={{ width: `${45 + (i * 13) % 30}%` }}
                >
                  <div className="h-3.5 msg-skeleton-bar mb-2" style={{ width: `${60 + (i * 17) % 40}%`, animationDelay: `${i * 0.15}s` }} />
                  {i % 2 === 0 && <div className="h-3.5 msg-skeleton-bar" style={{ width: `${40 + (i * 23) % 30}%`, animationDelay: `${i * 0.15 + 0.1}s` }} />}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="msg-list-appear">
            {loadingMore && (
              <div className="flex justify-center py-3">
                <span className="inline-flex items-center gap-1.5 text-slate-400 text-xs">
                  <span className="w-3 h-3 rounded-full border border-slate-300 border-t-slate-500 animate-spin" />
                  加载更多...
                </span>
              </div>
            )}
            {!hasMore && messages.length > 0 && (
              <div className="flex justify-center py-3">
                <span className="text-slate-300 text-xs">— 没有更多消息了 —</span>
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
                .reduce(
                  (acc: { list: ChatMessage[]; ids: Set<string | number> }, m) => {
                    if (acc.ids.has(m.id as string | number)) return acc
                    const last = acc.list[acc.list.length - 1]
                    if (last && m.role === 'assistant' && last.role === 'assistant' && m.content !== last.content) {
                      const lastTrim = (last.content ?? '').trim()
                      if (lastTrim && (m.content ?? '').startsWith(lastTrim) && (m.content ?? '').length > lastTrim.length) {
                        acc.list[acc.list.length - 1] = m
                        acc.ids.add(m.id as string | number)
                        return acc
                      }
                    }
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
                const isNewMsg = initialLoadDoneRef.current && !loadingMoreRef.current && !knownMsgIdsRef.current.has(m.id)
                return (
                  <div
                    key={String(m.id)}
                    className={`flex gap-2 ${isUser ? 'flex-row-reverse' : 'flex-row'}${isNewMsg ? ' msg-fade-in' : ''}`}
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
                      {m.created_at && (
                        <div className={`mt-1.5 text-[10px] ${isUser ? 'text-indigo-200' : 'text-slate-400'}`}>
                          {formatMessageTime(m.created_at)}
                        </div>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
            {typing && !messages.some((m) => m.role === 'assistant' && isThinkingPlaceholder(m.content ?? '')) && (
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
          </div>
        )}
        </ErrorBoundary>
      </div>

      {/* 跳转到最新消息 */}
      <button
        type="button"
        onClick={scrollToBottom}
        className={`absolute right-3 bottom-3 w-10 h-10 rounded-full bg-white border border-slate-200 shadow-lg flex items-center justify-center text-slate-500 hover:text-indigo-600 hover:border-indigo-200 active:bg-indigo-50 transition-all touch-manipulation ${
          showScrollBtn ? 'opacity-100 scale-100' : 'opacity-0 scale-75 pointer-events-none'
        }`}
        aria-label="跳转到最新消息"
      >
        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
        </svg>
      </button>
      </div>

      {/* 手机端录音全屏覆盖层 */}
      {isRecording && isMobile && (
        <div className="fixed inset-0 z-50 flex flex-col items-center justify-end pb-32 bg-black/40 voice-overlay-in">
          <div className={`flex flex-col items-center gap-4 transition-transform ${isCancelGesture ? 'scale-90 opacity-60' : ''}`}>
            <div className="voice-wave-container">
              {[...Array(5)].map((_, i) => (
                <div key={i} className="voice-wave-bar" style={{ animationDelay: `${i * 0.12}s` }} />
              ))}
            </div>
            <span className="text-white text-2xl font-medium tabular-nums">
              {Math.floor(recordingDuration / 60)}:{String(recordingDuration % 60).padStart(2, '0')}
            </span>
            <span className={`text-sm transition-all ${isCancelGesture ? 'text-red-400 font-medium' : 'text-white/70'}`}>
              {isCancelGesture ? '松开取消' : '上滑取消'}
            </span>
          </div>
        </div>
      )}

      {/* 输入区 */}
      {recordedBlob && !isMobile ? (
        <div className="flex items-center gap-2 p-3 sm:p-4 pb-[max(0.75rem,env(safe-area-inset-bottom))] bg-white/95 backdrop-blur-sm border-t border-slate-200/80 flex-shrink-0">
          <audio src={URL.createObjectURL(recordedBlob)} controls className="flex-1 min-w-0 h-10" />
          <button
            type="button"
            onClick={cancelPreview}
            disabled={voiceUploading}
            className="flex-shrink-0 w-11 h-11 flex items-center justify-center text-slate-500 hover:text-red-500 hover:bg-red-50 rounded-xl transition-colors touch-manipulation"
            aria-label="取消"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
          <button
            type="button"
            onClick={() => sendVoiceBlob(recordedBlob)}
            disabled={voiceUploading || !connected}
            className="flex-shrink-0 w-11 h-11 sm:w-12 sm:h-12 flex items-center justify-center bg-indigo-600 text-white rounded-xl hover:bg-indigo-700 active:bg-indigo-800 disabled:opacity-50 transition-colors touch-manipulation"
            aria-label="发送语音"
          >
            {voiceUploading ? (
              <span className="w-5 h-5 rounded-full border-2 border-white border-t-transparent animate-spin" />
            ) : (
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
              </svg>
            )}
          </button>
        </div>
      ) : (
        <form
          onSubmit={sendMessage}
          className="flex gap-2 p-3 sm:p-4 pb-[max(0.75rem,env(safe-area-inset-bottom))] bg-white/95 backdrop-blur-sm border-t border-slate-200/80 flex-shrink-0"
        >
          <div className="flex-1 min-w-0 flex items-end gap-2 bg-slate-100 rounded-2xl px-2 sm:px-4 py-2 focus-within:ring-2 focus-within:ring-indigo-500/50 focus-within:bg-white transition-all">
            <input
              ref={imageInputRef}
              type="file"
              accept="image/*"
              className="sr-only"
              tabIndex={-1}
              aria-hidden
              onChange={onImageInputChange}
            />
            {!isRecording || isMobile ? (
              <button
                type="button"
                onClick={() => imageInputRef.current?.click()}
                disabled={!connected || imageUploading || isRecording}
                className="flex-shrink-0 w-9 h-9 mb-0.5 flex items-center justify-center text-slate-500 hover:text-indigo-600 hover:bg-white/80 rounded-xl transition-colors disabled:opacity-40 touch-manipulation"
                title="发送图片"
                aria-label="发送图片"
              >
                {imageUploading ? (
                  <span className="w-4 h-4 rounded-full border-2 border-current border-t-transparent animate-spin" />
                ) : (
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                  </svg>
                )}
              </button>
            ) : null}
            {isRecording && !isMobile ? (
              <div className="flex-1 flex items-center gap-3 min-h-[44px] py-2.5">
                <span className="voice-rec-dot" />
                <span className="text-slate-700 text-base tabular-nums">
                  {Math.floor(recordingDuration / 60)}:{String(recordingDuration % 60).padStart(2, '0')}
                </span>
                <span className="text-slate-400 text-sm">录音中...</span>
              </div>
            ) : (
              <textarea
                ref={inputRef}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
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
            )}
          </div>
          {input.trim() || !supportsRecording ? (
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
          ) : (
            <button
              type="button"
              disabled={!connected || voiceUploading}
              onTouchStart={isMobile ? handleMicTouchStart : undefined}
              onTouchMove={isMobile ? handleMicTouchMove : undefined}
              onTouchEnd={isMobile ? handleMicTouchEnd : undefined}
              onClick={handleMicClick}
              className={`flex-shrink-0 w-11 h-11 sm:w-12 sm:h-12 flex items-center justify-center rounded-xl transition-colors touch-manipulation select-none ${
                isRecording
                  ? 'bg-red-500 text-white hover:bg-red-600 active:bg-red-700 voice-rec-pulse'
                  : 'bg-slate-200 text-slate-600 hover:bg-slate-300 active:bg-slate-400'
              } disabled:opacity-50`}
              aria-label={isRecording ? '停止录音' : '语音消息'}
            >
              {voiceUploading ? (
                <span className="w-5 h-5 rounded-full border-2 border-current border-t-transparent animate-spin" />
              ) : isRecording ? (
                <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
                  <rect x="6" y="6" width="12" height="12" rx="2" />
                </svg>
              ) : (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4M12 15a3 3 0 003-3V5a3 3 0 00-6 0v7a3 3 0 003 3z" />
                </svg>
              )}
            </button>
          )}
        </form>
      )}
      {(voiceError || voiceUploading || imageError || imageUploading) && (
        <div className={`px-4 py-1.5 text-xs text-center flex-shrink-0 ${voiceError || imageError ? 'bg-rose-50 text-rose-600' : 'bg-indigo-50 text-indigo-600'}`}>
          {[voiceError, imageError].filter(Boolean).join(' ') || (imageUploading ? '图片上传中...' : '') || (voiceUploading ? '语音上传中...' : '')}
        </div>
      )}
    </div>
  )
}
