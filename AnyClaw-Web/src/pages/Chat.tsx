import { useState, useEffect, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { getToken, getWebSocketUrl } from '../api'

interface PicoMessage {
  type: string;
  id?: string;
  payload?: { content?: string; role?: string; message_id?: string; [key: string]: unknown };
}

interface ChatMessage {
  id: string;
  content: string;
  role?: string;
}

export default function Chat() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [typing, setTyping] = useState(false);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState('');
  const wsRef = useRef<WebSocket | null>(null);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const instanceId = parseInt(id ?? '', 10);
    if (isNaN(instanceId)) {
      setError('Invalid instance');
      return;
    }

    const token = getToken();
    if (!token) {
      navigate('/login');
      return;
    }

    const url = getWebSocketUrl(instanceId);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      setError('');
    };

    ws.onmessage = (event) => {
      try {
        const msg: PicoMessage = JSON.parse(event.data);
        switch (msg.type) {
          case 'message.create':
            if (msg.payload?.content != null) {
              const id = msg.payload.message_id ?? msg.id ?? String(Date.now());
              setMessages((prev) => [
                ...prev,
                {
                  id,
                  content: String(msg.payload!.content),
                  role: msg.payload!.role as string | undefined,
                },
              ]);
            }
            break;
          case 'message.update':
            if (msg.payload?.content != null) {
              const targetId = msg.payload.message_id ?? msg.id;
              if (targetId) {
                setMessages((prev) =>
                  prev.map((m) =>
                    m.id === targetId
                      ? { ...m, content: String(msg.payload!.content) }
                      : m
                  )
                );
              }
            }
            break;
          case 'typing.start':
            setTyping(true);
            break;
          case 'typing.stop':
            setTyping(false);
            break;
        }
      } catch {
        // ignore parse errors
      }
    };

    ws.onclose = (e) => {
      setConnected(false);
      if (!error) {
        const msg = e.code === 1006 || e.code === 1011
          ? '连接失败，宠物可能仍在启动中，请稍后重试'
          : '连接已断开';
        setError(msg);
      }
    };

    ws.onerror = () => {
      setError('连接失败，请检查网络或稍后重试');
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [id, navigate]);

  useEffect(() => {
    listRef.current?.scrollTo(0, listRef.current.scrollHeight);
  }, [messages, typing]);

  const sendMessage = (e: React.FormEvent) => {
    e.preventDefault();
    const content = input.trim();
    if (!content || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;

    wsRef.current.send(
      JSON.stringify({
        type: 'message.send',
        payload: { content },
      })
    );
    setInput('');
  };

  return (
    <div className="max-w-2xl mx-auto flex flex-col h-[calc(100dvh-10rem)] sm:h-[calc(100vh-8rem)]">
      <div className="flex items-center gap-3 mb-3">
        <button
          onClick={() => navigate('/')}
          className="flex items-center gap-1 px-3 py-2 -ml-2 text-slate-600 active:bg-slate-100 rounded-lg touch-target min-h-[44px]"
        >
          <span className="text-lg">←</span>
          <span className="text-sm">返回</span>
        </button>
        <span
          className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${
            connected ? 'bg-green-500' : 'bg-red-500'
          }`}
        />
        <span className="text-sm text-slate-500">
          {connected ? '已连接' : '未连接'}
        </span>
      </div>

      {error && (
        <p className="mb-3 text-sm text-red-600 bg-red-50 p-3 rounded-lg">{error}</p>
      )}

      <div
        ref={listRef}
        className="flex-1 bg-white border border-slate-200 rounded-xl p-4 overflow-y-auto mb-4 min-h-0"
      >
        {messages.length === 0 && !typing && (
          <p className="text-slate-500 text-sm py-8 text-center">发条消息试试</p>
        )}
        {messages.map((m) => (
          <div key={m.id} className="mb-4 last:mb-0">
            {m.role && (
              <span className="text-xs text-slate-500">{m.role}: </span>
            )}
            <p className="text-slate-800 whitespace-pre-wrap break-words">{m.content}</p>
          </div>
        ))}
        {typing && (
          <p className="text-slate-500 text-sm italic">输入中...</p>
        )}
      </div>

      <form onSubmit={sendMessage} className="flex gap-2 flex-shrink-0">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="输入消息..."
          disabled={!connected}
          className="flex-1 px-4 py-3 border border-slate-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-slate-400 disabled:opacity-50 min-h-[48px]"
        />
        <button
          type="submit"
          disabled={!connected || !input.trim()}
          className="px-5 py-3 bg-slate-800 text-white rounded-xl active:bg-slate-700 disabled:opacity-50 touch-target min-h-[48px]"
        >
          发送
        </button>
      </form>
    </div>
  );
}
