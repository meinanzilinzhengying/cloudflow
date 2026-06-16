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
	"github.com/sirupsen/logrus"
)

// ConfigChangeEvent 配置变更事件
type ConfigChangeEvent struct {
	OldConfig *Config
	NewConfig *Config
	Error     error
}

// ConfigChangeListener 配置变更监听器
type ConfigChangeListener func(event ConfigChangeEvent)

// ConfigWatcher 配置文件监视器
type ConfigWatcher struct {
	configPath  string
	watcher     *fsnotify.Watcher
	listeners   []ConfigChangeListener
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	isWatching  bool
	lastConfig  *Config
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
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ConfigWatcher{
		configPath:  absPath,
		watcher:     watcher,
		listeners:   make([]ConfigChangeListener, 0),
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
	defer w.mu.Unlock()

	if w.isWatching {
		return nil
	}

	// 监视配置文件所在目录
	configDir := filepath.Dir(w.configPath)
	if err := w.watcher.Add(configDir); err != nil {
		return fmt.Errorf("failed to watch directory %s: %w", configDir, err)
	}

	w.isWatching = true

	// 启动监视循环
	go w.watchLoop()

	// 启动SIGHUP信号处理
	go w.watchSignals()

	logrus.Infof("config watcher started for: %s", w.configPath)
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

	logrus.Info("config watcher stopped")
}

// watchLoop 监视循环
func (w *ConfigWatcher) watchLoop() {
	var debounceTimer *time.Timer

	for {
		select {
		case <-w.ctx.Done():
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// 只处理配置文件的写入事件
			if event.Name != w.configPath {
				continue
			}

			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			logrus.Infof("config file changed: %s, operation: %s", event.Name, event.Op)

			// 防抖处理
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(w.debounceDur, func() {
				w.reloadConfig()
			})

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			logrus.Errorf("config watcher error: %v", err)
		}
	}
}

// watchSignals 监视SIGHUP信号
func (w *ConfigWatcher) watchSignals() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-sigCh:
			logrus.Info("received SIGHUP signal, reloading config")
			w.reloadConfig()
		}
	}
}

// reloadConfig 重新加载配置
func (w *ConfigWatcher) reloadConfig() {
	w.mu.Lock()
	oldConfig := w.lastConfig
	w.mu.Unlock()

	// 读取新配置
	newConfig, err := LoadConfig(w.configPath)
	if err != nil {
		logrus.Errorf("failed to reload config: %v", err)
		w.notifyListeners(ConfigChangeEvent{
			OldConfig: oldConfig,
			NewConfig: nil,
			Error:     fmt.Errorf("load config failed: %w", err),
		})
		return
	}

	// 验证配置
	if err := newConfig.Validate(); err != nil {
		logrus.Errorf("invalid config: %v, keeping old config", err)
		w.notifyListeners(ConfigChangeEvent{
			OldConfig: oldConfig,
			NewConfig: nil,
			Error:     fmt.Errorf("config validation failed: %w", err),
		})
		return
	}

	// 更新缓存
	w.mu.Lock()
	w.lastConfig = newConfig
	w.mu.Unlock()

	logrus.Info("config reloaded successfully")

	// 通知所有监听器
	w.notifyListeners(ConfigChangeEvent{
		OldConfig: oldConfig,
		NewConfig: newConfig,
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
