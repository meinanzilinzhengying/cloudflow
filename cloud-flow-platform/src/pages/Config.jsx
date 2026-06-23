import React, { useEffect, useState } from 'react';
import { fetchConfig, updateConfig, applyConfig } from '../api';

export default function Config() {
  const [config, setConfig] = useState(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [applying, setApplying] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  // 分类中文名
  const categoryNames = {
    'logs': '日志配置',
    'alerts': '告警配置',
    'collectors': '数据采集',
    'services': '服务管理',
  };

  // 类型中文名
  const typeNames = {
    'number': '数字',
    'boolean': '布尔',
    'select': '选项',
  };

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    setError('');
    try {
      const data = await fetchConfig();
      if (data) {
        setConfig(data);
      } else {
        setError('加载配置失败');
      }
    } catch (e) {
      setError(`加载配置失败: ${e.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 更新配置项
  const handleUpdate = async (category, key, value) => {
    setSaving(true);
    setMessage('');
    setError('');
    try {
      const result = await updateConfig(category, key, value);
      if (result.success) {
        setMessage(`配置项 ${key} 已更新`);
        // 更新本地状态
        setConfig(prev => ({
          ...prev,
          [category]: {
            ...prev[category],
            [key]: {
              ...prev[category][key],
              value: value,
            },
          },
        }));
      } else {
        setError(`更新失败: ${result.message}`);
      }
    } catch (e) {
      setError(`更新失败: ${e.message}`);
    } finally {
      setSaving(false);
    }
  };

  // 应用配置
  const handleApply = async () => {
    setApplying(true);
    setMessage('');
    setError('');
    try {
      const result = await applyConfig();
      if (result.success) {
        setMessage('配置已应用，部分服务可能需要重启');
      } else {
        setError(`应用失败: ${result.message}`);
      }
    } catch (e) {
      setError(`应用失败: ${e.message}`);
    } finally {
      setApplying(false);
    }
  };

  // 渲染配置项
  const renderConfigItem = (category, key, item) => {
    const { value, type, description, min, max, options } = item;

    return (
      <div key={key} style={{
        border: '1px solid #2e3a5c',
        borderRadius: 8,
        padding: 16,
        marginBottom: 12,
        backgroundColor: '#1a2236',
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div style={{ flex: 1 }}>
            <div style={{ fontWeight: 600, marginBottom: 4, color: '#e0e6f0', fontSize: 14 }}>{description}</div>
            <div style={{ fontSize: 12, color: '#6b7a90' }}>
              键: {key} | 类型: {typeNames[type] || type}
              {min !== undefined && ` | 最小值: ${min}`}
              {max !== undefined && ` | 最大值: ${max}`}
            </div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {/* 根据类型渲染输入框 */}
            {type === 'number' && (
              <input
                type="number"
                value={value}
                min={min}
                max={max}
                onChange={(e) => handleUpdate(category, key, parseFloat(e.target.value))}
                style={{
                  width: 100,
                  padding: '6px 10px',
                  borderRadius: 4,
                  border: '1px solid #2e3a5c',
                  backgroundColor: '#0d1320',
                  color: '#e0e6f0',
                  outline: 'none',
                }}
              />
            )}
            {type === 'boolean' && (
              <label style={{ display: 'flex', alignItems: 'center', cursor: 'pointer', color: '#e0e6f0' }}>
                <input
                  type="checkbox"
                  checked={value}
                  onChange={(e) => handleUpdate(category, key, e.target.checked)}
                  style={{ marginRight: 8 }}
                />
                {value ? '启用' : '禁用'}
              </label>
            )}
            {type === 'select' && (
              <select
                value={value}
                onChange={(e) => handleUpdate(category, key, e.target.value)}
                style={{
                  padding: '6px 10px',
                  borderRadius: 4,
                  border: '1px solid #2e3a5c',
                  backgroundColor: '#0d1320',
                  color: '#e0e6f0',
                  outline: 'none',
                }}
              >
                {options.map(opt => (
                  <option key={opt} value={opt}>{opt}</option>
                ))}
              </select>
            )}
            {saving && <span style={{ fontSize: 12, color: '#6b7a90' }}>保存中...</span>}
          </div>
        </div>
      </div>
    );
  };

  // 渲染分类
  const renderCategory = (category, configs) => {
    return (
      <div key={category} style={{ marginBottom: 32 }}>
        <h3 style={{
          fontSize: 18,
          fontWeight: 600,
          marginBottom: 16,
          paddingBottom: 8,
          borderBottom: '2px solid #1890ff',
          color: '#e0e6f0',
        }}>
          {categoryNames[category] || category}
        </h3>
        <div>
          {Object.entries(configs).map(([key, item]) => renderConfigItem(category, key, item))}
        </div>
      </div>
    );
  };

  // 初始加载
  useEffect(() => {
    loadConfig();
  }, []);

  if (loading) {
    return (
      <div style={{ padding: 24, textAlign: 'center', color: '#6b7a90' }}>
        <div>加载中...</div>
      </div>
    );
  }

  return (
    <div style={{ padding: 24, backgroundColor: '#0d1320', minHeight: '100vh' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 24, fontWeight: 700, color: '#e0e6f0' }}>⚙ 配置管理</h2>
        <div style={{ display: 'flex', gap: 12 }}>
          <button
            onClick={loadConfig}
            disabled={loading}
            style={{
              padding: '8px 16px',
              borderRadius: 6,
              border: '1px solid #2e3a5c',
              backgroundColor: '#1a2236',
              color: '#e0e6f0',
              cursor: 'pointer',
            }}
          >
            刷新
          </button>
          <button
            onClick={handleApply}
            disabled={applying}
            style={{
              padding: '8px 16px',
              borderRadius: 6,
              border: 'none',
              backgroundColor: '#1890ff',
              color: 'white',
              cursor: applying ? 'not-allowed' : 'pointer',
              opacity: applying ? 0.6 : 1,
            }}
          >
            {applying ? '应用中...' : '应用配置'}
          </button>
        </div>
      </div>

      {message && (
        <div style={{
          padding: '12px 16px',
          marginBottom: 16,
          borderRadius: 6,
          backgroundColor: '#162312',
          border: '1px solid #52c41a',
          color: '#52c41a',
        }}>
          {message}
        </div>
      )}

      {error && (
        <div style={{
          padding: '12px 16px',
          marginBottom: 16,
          borderRadius: 6,
          backgroundColor: '#2a1518',
          border: '1px solid #ff4d4f',
          color: '#ff4d4f',
        }}>
          {error}
        </div>
      )}

      {config && Object.entries(config).map(([category, configs]) => renderCategory(category, configs))}
    </div>
  );
}
