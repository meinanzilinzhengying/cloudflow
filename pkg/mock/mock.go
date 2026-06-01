package mock

import (
	"context"
	"sync"
	"time"
)

// Logger Mock Logger 实现
type Logger struct {
	mu           sync.RWMutex
	Infos        []string
	Warns        []string
	Errors       []string
	Debugs       []string
	InfofFunc    func(format string, args ...interface{})
	WarnfFunc    func(format string, args ...interface{})
	ErrorfFunc   func(format string, args ...interface{})
	DebugfFunc   func(format string, args ...interface{})
}

func NewLogger() *Logger {
	return &Logger{}
}

func (l *Logger) Info(args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(args) > 0 {
		if str, ok := args[0].(string); ok {
			l.Infos = append(l.Infos, str)
		}
	}
}

func (l *Logger) Infof(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Infos = append(l.Infos, format)
	if l.InfofFunc != nil {
		l.InfofFunc(format, args...)
	}
}

func (l *Logger) Warn(args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(args) > 0 {
		if str, ok := args[0].(string); ok {
			l.Warns = append(l.Warns, str)
		}
	}
}

func (l *Logger) Warnf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Warns = append(l.Warns, format)
	if l.WarnfFunc != nil {
		l.WarnfFunc(format, args...)
	}
}

func (l *Logger) Error(args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(args) > 0 {
		if str, ok := args[0].(string); ok {
			l.Errors = append(l.Errors, str)
		}
	}
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Errors = append(l.Errors, format)
	if l.ErrorfFunc != nil {
		l.ErrorfFunc(format, args...)
	}
}

func (l *Logger) Debug(args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(args) > 0 {
		if str, ok := args[0].(string); ok {
			l.Debugs = append(l.Debugs, str)
		}
	}
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Debugs = append(l.Debugs, format)
	if l.DebugfFunc != nil {
		l.DebugfFunc(format, args...)
	}
}

func (l *Logger) Sync() {}

// StorageEngine Mock 存储引擎
type StorageEngine struct {
	mu             sync.RWMutex
	Metrics        []map[string]interface{}
	Traces         []map[string]interface{}
	Nodes          []map[string]interface{}
	Users          []map[string]interface{}
	QueryError     error
	InsertError    error
	DeleteError    error
	UpdateError    error
	QueryCalled    int
	InsertCalled   int
	DeleteCalled   int
	UpdateCalled   int
}

func NewStorageEngine() *StorageEngine {
	return &StorageEngine{
		Metrics: make([]map[string]interface{}, 0),
		Traces:  make([]map[string]interface{}, 0),
		Nodes:   make([]map[string]interface{}, 0),
		Users:   make([]map[string]interface{}, 0),
	}
}

func (s *StorageEngine) QueryMetrics(day, probeID string, limit int) ([]map[string]interface{}, error) {
	s.mu.Lock()
	s.QueryCalled++
	s.mu.Unlock()
	if s.QueryError != nil {
		return nil, s.QueryError
	}
	return s.Metrics, nil
}

func (s *StorageEngine) QueryTraces(day, probeID string, limit int) ([]map[string]interface{}, error) {
	s.mu.Lock()
	s.QueryCalled++
	s.mu.Unlock()
	if s.QueryError != nil {
		return nil, s.QueryError
	}
	return s.Traces, nil
}

func (s *StorageEngine) QueryNodes() ([]map[string]interface{}, error) {
	s.mu.Lock()
	s.QueryCalled++
	s.mu.Unlock()
	if s.QueryError != nil {
		return nil, s.QueryError
	}
	return s.Nodes, nil
}

func (s *StorageEngine) VerifyUser(username, password string) (bool, string, error) {
	s.mu.Lock()
	s.QueryCalled++
	s.mu.Unlock()
	if s.QueryError != nil {
		return false, "", s.QueryError
	}
	for _, user := range s.Users {
		if user["username"] == username && user["password"] == password {
			return true, user["role"].(string), nil
		}
	}
	return false, "", nil
}

func (s *StorageEngine) InsertMetric(data map[string]interface{}) error {
	s.mu.Lock()
	s.InsertCalled++
	s.mu.Unlock()
	if s.InsertError != nil {
		return s.InsertError
	}
	s.Metrics = append(s.Metrics, data)
	return nil
}

func (s *StorageEngine) InsertTrace(data map[string]interface{}) error {
	s.mu.Lock()
	s.InsertCalled++
	s.mu.Unlock()
	if s.InsertError != nil {
		return s.InsertError
	}
	s.Traces = append(s.Traces, data)
	return nil
}

func (s *StorageEngine) StartCleanup()              {}
func (s *StorageEngine) Stop()                     {}
func (s *StorageEngine) DB() interface{}           { return nil }
func (s *StorageEngine) Close() error               { return nil }
func (s *StorageEngine) GetOverview(days int) (map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) GetMetricTrend(metric string, start, end int64) (map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) GetProtocolDistribution(day string) ([]map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) ListUsers() ([]map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) CreateUser(user map[string]interface{}) error { return nil }
func (s *StorageEngine) UpdateUser(id string, user map[string]interface{}) error { return nil }
func (s *StorageEngine) DeleteUser(id string) error { return nil }
func (s *StorageEngine) InsertFlow(data map[string]interface{}) error { return nil }
func (s *StorageEngine) InsertProfiling(data map[string]interface{}) error { return nil }
func (s *StorageEngine) InsertAlert(data map[string]interface{}) error { return nil }
func (s *StorageEngine) GetAlertHistory(start, end int64) ([]map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) CreateBusiness(data map[string]interface{}) error { return nil }
func (s *StorageEngine) ListBusinesses() ([]map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) UpdateBusiness(id string, data map[string]interface{}) error { return nil }
func (s *StorageEngine) DeleteBusiness(id string) error { return nil }
func (s *StorageEngine) CreateService(data map[string]interface{}) error { return nil }
func (s *StorageEngine) ListServices() ([]map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) UpdateService(id string, data map[string]interface{}) error { return nil }
func (s *StorageEngine) DeleteService(id string) error { return nil }
func (s *StorageEngine) GetService(id string) (map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) CreateCollector(data map[string]interface{}) error { return nil }
func (s *StorageEngine) ListCollectors() ([]map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) UpdateCollector(id string, data map[string]interface{}) error { return nil }
func (s *StorageEngine) DeleteCollector(id string) error { return nil }
func (s *StorageEngine) GetCollector(id string) (map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) CreateDataNode(data map[string]interface{}) error { return nil }
func (s *StorageEngine) ListDataNodes() ([]map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) UpdateDataNode(id string, data map[string]interface{}) error { return nil }
func (s *StorageEngine) DeleteDataNode(id string) error { return nil }
func (s *StorageEngine) GetDataNode(id string) (map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) GetSystemConfig() (map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) UpdateSystemConfig(config map[string]interface{}) error { return nil }
func (s *StorageEngine) ListAlertRules() ([]map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) CreateAlertRule(data map[string]interface{}) error { return nil }
func (s *StorageEngine) UpdateAlertRule(id string, data map[string]interface{}) error { return nil }
func (s *StorageEngine) DeleteAlertRule(id string) error { return nil }
func (s *StorageEngine) GetAlertRule(id string) (map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) ListAlertEvents(start, end int64) ([]map[string]interface{}, error) { return nil, nil }
func (s *StorageEngine) HandleAlertEvent(id string, handler, comment string) error { return nil }
func (s *StorageEngine) GetAlertEvent(id string) (map[string]interface{}, error) { return nil, nil }

// ConfigManager Mock 配置管理器
type ConfigManager struct {
	mu             sync.RWMutex
	config         map[string]interface{}
	reloadCalled   int
	reloadError    error
	changeCallback func(oldCfg, newCfg map[string]interface{})
}

func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		config: make(map[string]interface{}),
	}
}

func (c *ConfigManager) Reload() error {
	c.mu.Lock()
	c.reloadCalled++
	c.mu.Unlock()
	if c.reloadError != nil {
		return c.reloadError
	}
	return nil
}

func (c *ConfigManager) RegisterCallback(callback func(oldCfg, newCfg map[string]interface{})) {
	c.changeCallback = callback
}

func (c *ConfigManager) GetConfig() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

func (c *ConfigManager) Stop() {}

// TimeProvider Mock 时间提供者
type TimeProvider struct {
	now time.Time
}

func NewTimeProvider(t time.Time) *TimeProvider {
	return &TimeProvider{now: t}
}

func (tp *TimeProvider) Now() time.Time {
	return tp.now
}

// ContextProvider 提供测试用的 context
func ContextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func ContextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
