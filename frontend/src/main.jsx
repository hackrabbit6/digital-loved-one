import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import {
  Activity,
  AlertTriangle,
  Bot,
  CheckCircle2,
  ChevronDown,
  FileUp,
  BookmarkPlus,
  Loader2,
  MessageSquareText,
  Send,
  Server,
  Settings2,
  Trash2,
  UserRound,
} from 'lucide-react';
import './styles.css';

const DEFAULT_API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8080';

function App() {
  const [apiBase, setApiBase] = useState(() => localStorage.getItem('dlo_apiBase') || DEFAULT_API_BASE);
  const [personaId, setPersonaId] = useState(() => localStorage.getItem('dlo_personaId') || 'default');
  const [sourceType, setSourceType] = useState('text');
  const [health, setHealth] = useState({ status: 'unknown', detail: '尚未检测' });
  const [uploadState, setUploadState] = useState({ busy: false, result: null, error: '' });
  const [memoryState, setMemoryState] = useState({ busy: false, result: null, error: '' });
  const [memoryDraft, setMemoryDraft] = useState('');
  const [memoryTopic, setMemoryTopic] = useState('');
  const [chatState, setChatState] = useState({ busy: false, error: '' });
  const [query, setQuery] = useState('');
  const [dragOver, setDragOver] = useState(false);
  const [connOpen, setConnOpen] = useState(false);
  const [messages, setMessages] = useState([
    {
      role: 'assistant',
      text: '我可以先读取你上传或手动保存的记忆，再基于这些记忆回答。你也可以直接说："请记住：……"或者问我："你记住了什么？"',
      meta: { confidenceScore: 0, citations: [] },
    },
  ]);
  const fileInputRef = useRef(null);
  const messagesEndRef = useRef(null);

  const normalizedApiBase = useMemo(() => apiBase.replace(/\/+$/, ''), [apiBase]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  function updateApiBase(v) {
    setApiBase(v);
    localStorage.setItem('dlo_apiBase', v);
  }

  function updatePersonaId(v) {
    setPersonaId(v);
    localStorage.setItem('dlo_personaId', v);
  }

  async function checkHealth() {
    setHealth({ status: 'checking', detail: '检测中' });
    try {
      const res = await fetch(`${normalizedApiBase}/api/health`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setHealth({ status: 'ok', detail: data.status || 'ok' });
    } catch (error) {
      setHealth({ status: 'down', detail: error.message });
    }
  }

  useEffect(() => {
    checkHealth();
  }, []);

  async function handleUploadFile(file) {
    setUploadState({ busy: true, result: null, error: '' });
    const body = new FormData();
    body.append('file', file);
    body.append('personaId', personaId || 'default');
    body.append('sourceType', sourceType);

    try {
      const res = await fetch(`${normalizedApiBase}/api/upload`, { method: 'POST', body });
      const text = await res.text();
      if (!res.ok) throw new Error(text || `HTTP ${res.status}`);
      const data = JSON.parse(text);
      setUploadState({ busy: false, result: data, error: '' });
      setMessages((current) => [
        ...current,
        {
          role: 'assistant',
          text: `已导入 ${data.excerpts} 条记忆片段，personaId 为 ${data.personaId}。现在可以问我和这些资料相关的问题。`,
          meta: { confidenceScore: 1, citations: [] },
        },
      ]);
    } catch (error) {
      setUploadState({ busy: false, result: null, error: error.message });
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  }

  function handleUpload(event) {
    const file = event.target.files?.[0];
    if (file) handleUploadFile(file);
  }

  function handleDrop(event) {
    event.preventDefault();
    setDragOver(false);
    const file = event.dataTransfer.files?.[0];
    if (file) handleUploadFile(file);
  }

  async function sendMessage(event) {
    event.preventDefault();
    const trimmed = query.trim();
    if (!trimmed || chatState.busy) return;

    setMessages((current) => [...current, { role: 'user', text: trimmed }]);
    setQuery('');
    setChatState({ busy: true, error: '' });

    try {
      const res = await fetch(`${normalizedApiBase}/api/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ personaId: personaId || 'default', query: trimmed }),
      });
      const text = await res.text();
      if (!res.ok) throw new Error(text || `HTTP ${res.status}`);
      const data = JSON.parse(text);
      setMessages((current) => [
        ...current,
        {
          role: 'assistant',
          text: data.text,
          meta: {
            confidenceScore: data.confidenceScore,
            citations: data.citations || [],
            conflictsSurfaced: data.conflictsSurfaced || [],
            ambiguityFlags: data.ambiguityFlags || [],
          },
        },
      ]);
    } catch (error) {
      setChatState({ busy: false, error: error.message });
      setMessages((current) => [
        ...current,
        {
          role: 'assistant',
          text: '后端请求失败，请检查 API 地址和 Go 服务是否已经启动。',
          meta: { confidenceScore: 0, citations: [], error: error.message },
        },
      ]);
      return;
    }

    setChatState({ busy: false, error: '' });
  }

  async function rememberText(text, topic = memoryTopic) {
    const trimmed = text.trim();
    if (!trimmed || memoryState.busy) return;

    setMemoryState({ busy: true, result: null, error: '' });
    try {
      const res = await fetch(`${normalizedApiBase}/api/remember`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          personaId: personaId || 'default',
          text: trimmed,
          speaker: 'user',
          topic: topic.trim(),
        }),
      });
      const body = await res.text();
      if (!res.ok) throw new Error(body || `HTTP ${res.status}`);
      const data = JSON.parse(body);
      setMemoryState({ busy: false, result: data, error: '' });
      setMemoryDraft('');
      setMessages((current) => [
        ...current,
        {
          role: 'assistant',
          text: `我已经记住这条信息，归入 ${data.topic} 主题。`,
          meta: { confidenceScore: 1, citations: [] },
        },
      ]);
    } catch (error) {
      setMemoryState({ busy: false, result: null, error: error.message });
    }
  }

  function handleMemoryKeyDown(event) {
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
      event.preventDefault();
      rememberText(memoryDraft);
    }
  }

  function clearChat() {
    setMessages([
      {
        role: 'assistant',
        text: '对话已清空。你可以重新开始提问。',
        meta: { confidenceScore: 0, citations: [] },
      },
    ]);
  }

  return (
    <main className="app-shell">
      <section className="topbar">
        <div>
          <p className="eyebrow">Digital Loved One</p>
          <h1>本地记忆工作台</h1>
        </div>
        <StatusPill health={health} onRefresh={checkHealth} />
      </section>

      <section className="workspace">
        <aside className="side-panel">
          <CollapsibleSection
            icon={<Settings2 size={18} />}
            title="连接"
            open={connOpen}
            onToggle={() => setConnOpen((v) => !v)}
          >
            <label className="field">
              <span>API 地址</span>
              <input value={apiBase} onChange={(event) => updateApiBase(event.target.value)} />
            </label>
            <label className="field">
              <span>Persona ID</span>
              <input value={personaId} onChange={(event) => updatePersonaId(event.target.value)} />
            </label>
          </CollapsibleSection>

          <PanelTitle icon={<FileUp size={18} />} title="导入记忆" />
          <div className="segmented">
            {['text', 'whatsapp', 'wechat', 'audio'].map((type) => (
              <button
                className={sourceType === type ? 'active' : ''}
                key={type}
                onClick={() => setSourceType(type)}
                type="button"
              >
                {type}
              </button>
            ))}
          </div>
          <label
            className={`upload-box ${uploadState.busy ? 'busy' : ''} ${dragOver ? 'drag-over' : ''}`}
            onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
            onDragLeave={() => setDragOver(false)}
            onDrop={handleDrop}
          >
            {uploadState.busy ? <Loader2 className="spin" size={22} /> : <FileUp size={22} />}
            <span>
              {uploadState.busy ? '导入中' : dragOver ? '松开以上传' : '点击或拖拽文件上传'}
            </span>
            <input ref={fileInputRef} type="file" onChange={handleUpload} />
          </label>
          {uploadState.result && (
            <div className="notice success">
              <CheckCircle2 size={16} />
              <span>已导入 {uploadState.result.excerpts} 条片段</span>
            </div>
          )}
          {uploadState.error && (
            <div className="notice error">
              <AlertTriangle size={16} />
              <span>{uploadState.error}</span>
            </div>
          )}

          <PanelTitle icon={<BookmarkPlus size={18} />} title="对话记忆" />
          <label className="field">
            <span>主题</span>
            <input value={memoryTopic} onChange={(event) => setMemoryTopic(event.target.value)} placeholder="可选，例如 family" />
          </label>
          <label className="field">
            <span>要记住的内容</span>
            <textarea
              value={memoryDraft}
              onChange={(event) => setMemoryDraft(event.target.value)}
              onKeyDown={handleMemoryKeyDown}
              placeholder="例如：妈妈年轻时很喜欢唱歌。"
            />
            <span className="field-hint">Ctrl + Enter 保存</span>
          </label>
          <button
            className="memory-action"
            disabled={!memoryDraft.trim() || memoryState.busy}
            onClick={() => rememberText(memoryDraft)}
            type="button"
          >
            {memoryState.busy ? <Loader2 className="spin" size={17} /> : <BookmarkPlus size={17} />}
            <span>{memoryState.busy ? '保存中' : '记住'}</span>
          </button>
          {memoryState.result && (
            <div className="notice success">
              <CheckCircle2 size={16} />
              <span>已记入 {memoryState.result.topic}</span>
            </div>
          )}
          {memoryState.error && (
            <div className="notice error">
              <AlertTriangle size={16} />
              <span>{memoryState.error}</span>
            </div>
          )}
        </aside>

        <section className="chat-panel">
          <div className="chat-header">
            <PanelTitle icon={<MessageSquareText size={18} />} title="对话" />
            <div className="chat-header-right">
              <div className="persona-chip">
                <UserRound size={15} />
                <span>{personaId || 'default'}</span>
              </div>
              <button className="icon-btn" onClick={clearChat} title="清空对话" type="button">
                <Trash2 size={16} />
              </button>
            </div>
          </div>

          <div className="messages">
            {messages.map((message, index) => (
              <MessageBubble key={`${message.role}-${index}`} message={message} onRemember={rememberText} />
            ))}
            {chatState.busy && (
              <div className="message assistant">
                <div className="avatar">
                  <Bot size={17} />
                </div>
                <div className="bubble pending">
                  <Loader2 className="spin" size={17} />
                  <span>正在基于记忆检查证据</span>
                </div>
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>

          {chatState.error && (
            <div className="notice error chat-error">
              <AlertTriangle size={16} />
              <span>{chatState.error}</span>
            </div>
          )}

          <form className="composer" onSubmit={sendMessage}>
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="例如：请记住：我喜欢蓝莓芝士蛋糕。"
            />
            <button disabled={!query.trim() || chatState.busy} type="submit" title="发送">
              <Send size={18} />
            </button>
          </form>
        </section>
      </section>
    </main>
  );
}

function StatusPill({ health, onRefresh }) {
  const icon =
    health.status === 'ok'
      ? <CheckCircle2 size={16} />
      : health.status === 'checking'
      ? <Loader2 className="spin" size={16} />
      : <Server size={16} />;
  return (
    <button className={`status-pill ${health.status}`} onClick={onRefresh} type="button">
      {icon}
      <span>{health.status === 'ok' ? '后端在线' : health.status === 'checking' ? '检测中' : '后端未连接'}</span>
    </button>
  );
}

function PanelTitle({ icon, title }) {
  return (
    <div className="panel-title">
      {icon}
      <span>{title}</span>
    </div>
  );
}

function CollapsibleSection({ icon, title, open, onToggle, children }) {
  return (
    <div className="collapsible">
      <button className="collapsible-trigger" onClick={onToggle} type="button">
        <div className="panel-title">
          {icon}
          <span>{title}</span>
        </div>
        <ChevronDown size={16} className={`chevron ${open ? 'open' : ''}`} />
      </button>
      {open && <div className="collapsible-body">{children}</div>}
    </div>
  );
}

function MessageBubble({ message, onRemember }) {
  const isUser = message.role === 'user';
  const score = typeof message.meta?.confidenceScore === 'number' ? message.meta.confidenceScore : null;
  const conflicts = message.meta?.conflictsSurfaced || [];

  return (
    <div className={`message ${isUser ? 'user' : 'assistant'}`}>
      <div className="avatar">{isUser ? <UserRound size={17} /> : <Bot size={17} />}</div>
      <div className="bubble">
        <p>{message.text}</p>
        {!isUser && message.meta && (
          <div className="meta-row">
            {score !== null && (
              <span>
                <Activity size={13} />
                置信度 {Math.round(score * 100)}%
              </span>
            )}
            {message.meta.citations?.length > 0 && <span>引用 {message.meta.citations.length}</span>}
            {message.meta.ambiguityFlags?.length > 0 && <span>歧义 {message.meta.ambiguityFlags.length}</span>}
            {conflicts.length > 0 && <span className="conflict-badge">冲突 {conflicts.length}</span>}
          </div>
        )}
        {conflicts.length > 0 && (
          <div className="conflicts">
            {conflicts.map((c, i) => (
              <div key={i}>{c}</div>
            ))}
          </div>
        )}
        {message.meta?.citations?.length > 0 && (
          <div className="citations">
            {message.meta.citations.map((citation) => (
              <div key={citation.id}>{citation.text}</div>
            ))}
          </div>
        )}
        {isUser && (
          <button className="remember-inline" onClick={() => onRemember(message.text)} type="button">
            <BookmarkPlus size={13} />
            <span>记住</span>
          </button>
        )}
      </div>
    </div>
  );
}

createRoot(document.getElementById('root')).render(<App />);
