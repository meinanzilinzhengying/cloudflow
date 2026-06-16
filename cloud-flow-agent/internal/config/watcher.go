package config

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// ConfigChangeEvent 配置变更事件
type ConfigChangeEvent struct {
	OldConfig *Config
	NewConfig *Config
	Error     error
}

// ConfigChangeListener 配置变更监听器
type ConfigChangeListener func(event ConfigChangeEvent)

// ConfigWatcher 配置监视器
type ConfigWatcher struct {
	configPath string
	watcher    *fsnotify.Watcher
	listeners  []ConfigChangeListener
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	isWatching bool
	lastConfig *Config
	debounceDur time.Duration
}

// NewConfigWatcher 创建配置监视器
func NewConfigWatcher(configPath string) (*ConfigWatcher, error) {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	if err := watcher.Add(filepath.Dir(absPath)); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to add watch: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ConfigWatcher{
		configPath:  absPath,
		watcher:     watcher,
		ctx:         ctx,
		cancel:      cancel,
		debounceDur: 500 * time.Millisecond,
	}, nil
}

// AddListener 添加配置变更监听器
func (w *ConfigWatcher) AddListener(listener ConfigChangeListener) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.listeners = append(w.listeners, listener)
}

// Start 开始监视
func (w *ConfigWatcher) Start() error {
	w.mu.Lock()
	if w.isWatching {
		w.mu.Unlock()
		return nil
	}
	w.isWatching = true
	w.mu.Unlock()

	// 加载初始配置
	initialCfg, err := LoadConfig(w.configPath)
	if err != nil {
		return fmt.Errorf("failed to load initial config: %w", err)
	}
	w.lastConfig = initialCfg

	go w.watchLoop()
	go w.watchSignals()

	zap.L().Info("config watcher started", zap.String("path", w.configPath))
	return nil
}

// Stop 停止监视
func (w *ConfigWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.isWatching {
		return
	}

	w.cancel()
	w.watcher.Close()
	w.isWatching = false

	zap.L().Info("config watcher stopped")
}

// watchLoop 文件系统监视循环
func (w *ConfigWatcher) watchLoop() {
	var debounceTimer *time.Timer
	debounceChan := make(chan struct{}, 1)

	for {
		select {
		case <-w.ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// 只关注配置文件的变更
			if event.Name != w.configPath {
				continue
			}

			// 只处理写入和创建事件
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			// 防抖处理
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(w.debounceDur, func() {
				debounceChan <- struct{}{}
			})

		case <-debounceChan:
			w.reloadConfig()

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			zap.L().Error("config watcher error", zap.Error(err))
		}
	}
}

// watchSignals 监听SIGHUP信号
func (w *ConfigWatcher) watchSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-sigChan:
			zap.L().Info("received SIGHUP, reloading config")
			w.reloadConfig()
		}
	}
}

// reloadConfig 重新加载配置
func (w *ConfigWatcher) reloadConfig() {
	w.mu.RLock()
	oldCfg := w.lastConfig
	w.mu.RUnlock()

	zap.L().Info("reloading config", zap.String("path", w.configPath))

	newCfg, err := LoadConfig(w.configPath)
	if err != nil {
		zap.L().Error("failed to reload config", zap.Error(err))
		w.notifyListeners(ConfigChangeEvent{
			OldConfig: oldCfg,
			NewConfig: nil,
			Error:     err,
		})
		return
	}

	// 验证新配置
	if err := newCfg.Validate(); err != nil {
		zap.L().Error("new config validation failed, keeping old config", zap.Error(err))
		w.notifyListeners(ConfigChangeEvent{
			OldConfig: oldCfg,
			NewConfig: nil,
			Error:     fmt.Errorf("validation failed: %w", err),
		})
		return
	}

	// 更新配置
	w.mu.Lock()
	w.lastConfig = newCfg
	w.mu.Unlock()

	zap.L().Info("config reloaded successfully")

	// 通知监听器
	w.notifyListeners(ConfigChangeEvent{
		OldConfig: oldCfg,
		NewConfig: newCfg,
		Error:     nil,
	})
}

// notifyListeners 通知所有监听器
func (w *ConfigWatcher) notifyListeners(event ConfigChangeEvent) {
	w.mu.RLock()
	listeners := make([]ConfigChangeListener, len(w.listeners))
	copy(listeners, w.listeners)
	w.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// SetDebounce 设置防抖时间
func (w *ConfigWatcher) SetDebounce(dur time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.debounceDur = dur
}
