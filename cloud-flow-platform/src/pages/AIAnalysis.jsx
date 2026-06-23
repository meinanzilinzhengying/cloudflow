import { useState } from 'react';
import axios from 'axios';

export default function AIAnalysis() {
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [aiStatus, setAiStatus] = useState(null);
  const [error, setError] = useState('');

  const checkHealth = async () => {
    try {
      const res = await axios.get('/api/ai/health', { timeout: 5000 });
      setAiStatus(res.status === 200 ? 'connected' : 'error');
      setError('');
    } catch {
      setAiStatus('disconnected');
      setError('AI 服务未连接 (端口 8082)');
    }
  };

  useState(() => { checkHealth(); }, []);

  const sendMessage = async () => {
    if (!input.trim() || loading) return;
    const userMsg = { role: 'user', content: input };
    setMessages(prev => [...prev, userMsg]);
    setInput('');
    setLoading(true);
    setError('');

    try {
      const res = await axios.post('/api/ai/v1/analyze', {
        query: input,
        context: 'cloudflow 平台运维分析',
      }, { timeout: 30000 });

      const reply = res.data?.result || res.data?.response || res.data?.answer || JSON.stringify(res.data);
      setMessages(prev => [...prev, { role: 'assistant', content: reply }]);
    } catch (e) {
      const errMsg = e.response?.data?.error || e.message || '请求失败';
      setError('AI 分析失败: ' + errMsg);
      setMessages(prev => [...prev, { role: 'system', content: '错误: ' + errMsg }]);
    } finally {
      setLoading(false);
    }
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2>AI 智能分析</h2>
        <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
          <span className={aiStatus === 'connected' ? 'status-online' : 'status-offline'}>
            {aiStatus === 'connected' ? '● AI已连接' : aiStatus === 'disconnected' ? '● 未连接' : '检测中...'}
          </span>
          <button onClick={checkHealth} className="btn-refresh">↻ 重检</button>
        </div>
      </div>

      <div className="card" style={{ display: 'flex', flexDirection: 'column', height: 'calc(100vh - 200px)' }}>
        {/* Messages */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '0 0 16px 0', display: 'flex', flexDirection: 'column', gap: 12 }}>
          {messages.length === 0 && (
            <div className="empty-state" style={{ marginTop: 100 }}>
              <div style={{ fontSize: 48, marginBottom: 16 }}>🤖</div>
              <div style={{ fontSize: 16, marginBottom: 8 }}>CloudFlow AI 助手</div>
              <div style={{ color: '#64748b', fontSize: 14 }}>
                输入问题，AI 将分析平台数据并提供洞察<br />
                示例: "分析最近的网络安全事件"
              </div>
            </div>
          )}
          {messages.map((msg, i) => (
            <div key={i} className={`chat-msg ${msg.role}`}>
              <div className="chat-role">{msg.role === 'user' ? '👤 你' : msg.role === 'assistant' ? '🤖 AI' : '⚠️ 系统'}</div>
              <div className="chat-content">{msg.content}</div>
            </div>
          ))}
          {loading && (
            <div className="chat-msg assistant">
              <div className="chat-role">🤖 AI</div>
              <div className="chat-content">
                <span className="typing">思考中...</span>
              </div>
            </div>
          )}
        </div>

        {error && (
          <div style={{ padding: '8px 16px', background: 'rgba(239,68,68,0.1)', color: '#ef4444', borderRadius: 6, marginBottom: 8, fontSize: 13 }}>
            {error}
          </div>
        )}

        {/* Input */}
        <div style={{ display: 'flex', gap: 12, borderTop: '1px solid #334155', paddingTop: 12 }}>
          <textarea
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="输入分析问题，Enter发送，Shift+Enter换行..."
            rows={3}
            style={{
              flex: 1,
              background: '#0f172a',
              border: '1px solid #334155',
              borderRadius: 8,
              color: '#e2e8f0',
              padding: '12px 16px',
              resize: 'none',
              fontFamily: 'inherit',
              fontSize: 14,
            }}
          />
          <button
            onClick={sendMessage}
            disabled={loading || !input.trim()}
            className="btn btn-primary"
            style={{ alignSelf: 'flex-end', height: 44 }}
          >
            {loading ? '...' : '发送'}
          </button>
        </div>
      </div>
    </div>
  );
}
