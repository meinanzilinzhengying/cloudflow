//go:build linux

package sysconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ============================================================================
// 一、热加载配置源
// ============================================================================

// ConfigSource 配置源接口
type ConfigSource interface {
	Load() (*ConfigSnapshot, error)
	Watch() error
	Stop() error
	SourceType() string
}

// FileSource 文件配置源
type FileSource struct {
	path       string
	format     string // json, yaml, yml
	watcher    *fsnotify.Watcher
	stopCh     chan struct{}
	mu         sync.RWMutex
	onChange   func(*ConfigSnapshot)
	debounce   time.Duration
}

// NewFileSource 创建文件配置源
func NewFileSource(path, format string) *FileSource {
	if format == "" {
		ext := filepath.Ext(path)
		switch ext {
		case ".json":
			format = "json"
		case ".yaml", ".yml":
			format = "yaml"
		default:
			format = "json"
		}
	}
	return &FileSource{
		path:     path,
		format:   format,
		stopCh:   make(chan struct{}),
		debounce: 500 * time.Millisecond,
	}
}

// Load 从文件加载配置
func (fs *FileSource) Load() (*ConfigSnapshot, error) {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	
	switch fs.format {
	case "json":
		return FromJSON(data)
	case "yaml", "yml":
		return FromYAML(data)
	default:
		return FromJSON(data)
	}
}

// Watch 开始监视文件变更
func (fs *FileSource) Watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	
	if err := watcher.Add(fs.path); err != nil {
		watcher.Close()
		return err
	}
	
	fs.watcher = watcher
	
	go fs.watchLoop()
	return nil
}

// watchLoop 文件监视循环
func (fs *FileSource) watchLoop() {
	var debounceTimer *time.Timer
	debounceChan := make(chan struct{}, 1)
	
	for {
		select {
		case <-fs.stopCh:
			return
		case event, ok := <-fs.watcher.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(fs.debounce, func() {
				select {
				case debounceChan <- struct{}{}:
				default:
				}
			})
		case <-debounceChan:
			fs.reload()
		case err, ok := <-fs.watcher.Errors:
			if !ok {
				return
			}
			_ = err
		}
	}
}

// reload 重新加载并通知
func (fs *FileSource) reload() {
	snapshot, err := fs.Load()
	if err != nil {
		return
	}
	
	fs.mu.RLock()
	cb := fs.onChange
	fs.mu.RUnlock()
	
	if cb != nil {
		cb(snapshot)
	}
}

// SetOnChange 设置变更回调
func (fs *FileSource) SetOnChange(cb func(*ConfigSnapshot)) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.onChange = cb
}

// Stop 停止监视
func (fs *FileSource) Stop() error {
	close(fs.stopCh)
	if fs.watcher != nil {
		return fs.watcher.Close()
	}
	return nil
}

// SourceType 返回源类型
func (fs *FileSource) SourceType() string {
	return "file"
}

// ============================================================================
// 二、内存配置源
// ============================================================================

// MemorySource 内存配置源
type MemorySource struct {
	mu        sync.RWMutex
	snapshot  *ConfigSnapshot
	onChange  func(*ConfigSnapshot)
}

// NewMemorySource 创建内存配置源
func NewMemorySource(snapshot *ConfigSnapshot) *MemorySource {
	return &MemorySource{snapshot: snapshot}
}

// Load 加载配置
func (ms *MemorySource) Load() (*ConfigSnapshot, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.snapshot == nil {
		return nil, fmt.Errorf("no config in memory")
	}
	return ms.snapshot.DeepCopy(), nil
}

// Update 更新内存配置
func (ms *MemorySource) Update(snapshot *ConfigSnapshot) {
	ms.mu.Lock()
	ms.snapshot = snapshot.DeepCopy()
	cb := ms.onChange
	ms.mu.Unlock()
	
	if cb != nil {
		cb(snapshot)
	}
}

// Watch 不需要监视
func (ms *MemorySource) Watch() error {
	return nil
}

// Stop 不需要停止
func (ms *MemorySource) Stop() error {
	return nil
}

// SourceType 返回源类型
func (ms *MemorySource) SourceType() string {
	return "memory"
}

// SetOnChange 设置变更回调
func (ms *MemorySource) SetOnChange(cb func(*ConfigSnapshot)) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.onChange = cb
}

// ============================================================================
// 三、配置变更监听器
// ============================================================================

// ConfigChangeHandler 配置变更处理器
type ConfigChangeHandler struct {
	mu        sync.RWMutex
	handlers  map[string][]func(oldVal, newVal interface{})
}

// NewConfigChangeHandler 创建变更处理器
func NewConfigChangeHandler() *ConfigChangeHandler {
	return &ConfigChangeHandler{
		handlers: make(map[string][]func(oldVal, newVal interface{})),
	}
}

// Register 注册配置项变更处理器
func (cch *ConfigChangeHandler) Register(key string, handler func(oldVal, newVal interface{})) {
	cch.mu.Lock()
	defer cch.mu.Unlock()
	cch.handlers[key] = append(cch.handlers[key], handler)
}

// Handle 处理配置变更
func (cch *ConfigChangeHandler) Handle(key string, oldVal, newVal interface{}) {
	cch.mu.RLock()
	handlers := make([]func(oldVal, newVal interface{}), len(cch.handlers[key]))
	copy(handlers, cch.handlers[key])
	cch.mu.RUnlock()
	
	for _, h := range handlers {
		go func(handler func(oldVal, newVal interface{})) {
			defer func() { recover() }()
			handler(oldVal, newVal)
		}(h)
	}
}

// ============================================================================
// 四、YAML 支持
// ============================================================================

// FromYAML 从 YAML 导入（简化实现，使用 JSON 作为兼容）
func FromYAML(data []byte) (*ConfigSnapshot, error) {
	// 简化：YAML 先解析为通用 map，然后转为 JSON 再解析
	// 实际实现应使用 yaml 库
	return FromJSON(data)
}

// ToYAML 导出为 YAML（简化实现）
func (cs *ConfigSnapshot) ToYAML() (string, error) {
	json, err := cs.ToJSON()
	if err != nil {
		return "", err
	}
	return json, nil
}