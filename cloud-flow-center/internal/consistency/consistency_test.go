//go:build linux

package consistency

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Mock 实现
// ============================================================================

type mockOutboxStore struct {
	mu     sync.Mutex
	events map[string]*OutboxEvent
}

func newMockOutboxStore() *mockOutboxStore {
	return &mockOutboxStore{events: make(map[string]*OutboxEvent)}
}

func (m *mockOutboxStore) Save(ctx context.Context, event *OutboxEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[event.ID] = event
	return nil
}

func (m *mockOutboxStore) GetPending(ctx context.Context, limit int) ([]*OutboxEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*OutboxEvent, 0)
	for _, e := range m.events {
		if e.Status == OutboxPending {
			result = append(result, e)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockOutboxStore) MarkPublished(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.events[id]; ok {
		e.Status = OutboxPublished
	}
	return nil
}

func (m *mockOutboxStore) MarkFailed(ctx context.Context, id string, retries int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.events[id]; ok {
		e.Status = OutboxFailed
		e.Retries = retries
	}
	return nil
}

func (m *mockOutboxStore) UpdateRetries(ctx context.Context, id string, retries int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.events[id]; ok {
		e.Retries = retries
		// 保持状态为 Pending
	}
	return nil
}

type mockPublisher struct {
	mu      sync.Mutex
	events  []*OutboxEvent
	failIDs map[string]bool
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{
		events:  make([]*OutboxEvent, 0),
		failIDs: make(map[string]bool),
	}
}

func (m *mockPublisher) Publish(ctx context.Context, event *OutboxEvent) error {
	if m.failIDs[event.ID] {
		return errors.New("publish failed")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockPublisher) setFail(id string) {
	m.failIDs[id] = true
}

type mockCacheStore struct {
	mu    sync.Mutex
	data  map[string]interface{}
	calls []string
}

func newMockCacheStore() *mockCacheStore {
	return &mockCacheStore{
		data:  make(map[string]interface{}),
		calls: make([]string, 0),
	}
}

func (m *mockCacheStore) Get(ctx context.Context, key string) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "GET:"+key)
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return nil, errors.New("cache miss")
}

func (m *mockCacheStore) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "SET:"+key)
	m.data[key] = value
	return nil
}

func (m *mockCacheStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "DEL:"+key)
	delete(m.data, key)
	return nil
}

func (m *mockCacheStore) getCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.calls))
	copy(result, m.calls)
	return result
}

type mockDBStore struct {
	mu   sync.Mutex
	data map[string]interface{}
}

func newMockDBStore() *mockDBStore {
	return &mockDBStore{data: make(map[string]interface{})}
}

func (m *mockDBStore) Get(ctx context.Context, key string) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}

func (m *mockDBStore) Set(ctx context.Context, key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockDBStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

type mockDataSource struct {
	name string
	data map[string]interface{}
}

func (m *mockDataSource) Name() string { return m.name }
func (m *mockDataSource) Get(ctx context.Context, key string) (interface{}, error) {
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}

type mockIdempotencyStore struct {
	mu   sync.Mutex
	keys map[string]bool
}

func newMockIdempotencyStore() *mockIdempotencyStore {
	return &mockIdempotencyStore{keys: make(map[string]bool)}
}

func (m *mockIdempotencyStore) Get(ctx context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.keys[key], nil
}

func (m *mockIdempotencyStore) Set(ctx context.Context, key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[key] = true
	return nil
}

// ============================================================================
// 测试用例
// ============================================================================

func TestSagaTransactionSuccess(t *testing.T) {
	so := NewSagaOrchestrator()
	tx := NewSagaTransaction("saga-1")

	step1Called := false
	step2Called := false

	tx.AddStep(&SagaStep{
		Name: "step1",
		Action: func(ctx context.Context) error {
			step1Called = true
			return nil
		},
	})
	tx.AddStep(&SagaStep{
		Name: "step2",
		Action: func(ctx context.Context) error {
			step2Called = true
			return nil
		},
	})

	err := so.Execute(context.Background(), tx)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !step1Called || !step2Called {
		t.Error("both steps should be called")
	}
	if tx.GetState() != SagaCommitted {
		t.Errorf("expected state committed, got %v", tx.GetState())
	}
}

func TestSagaTransactionCompensation(t *testing.T) {
	so := NewSagaOrchestrator()
	tx := NewSagaTransaction("saga-2")

	step1Action := false
	step1Compensate := false
	step2Action := false

	tx.AddStep(&SagaStep{
		Name: "step1",
		Action: func(ctx context.Context) error {
			step1Action = true
			return nil
		},
		Compensate: func(ctx context.Context) error {
			step1Compensate = true
			return nil
		},
	})
	tx.AddStep(&SagaStep{
		Name: "step2",
		Action: func(ctx context.Context) error {
			step2Action = true
			return errors.New("step2 failed")
		},
		Compensate: func(ctx context.Context) error {
			return nil
		},
	})

	err := so.Execute(context.Background(), tx)
	if err == nil {
		t.Fatal("expected error")
	}
	if !step1Action {
		t.Error("step1 action should be called")
	}
	if !step2Action {
		t.Error("step2 action should be called")
	}
	if !step1Compensate {
		t.Error("step1 compensate should be called")
	}
	if tx.GetState() != SagaCompensated {
		t.Errorf("expected state compensated, got %v", tx.GetState())
	}
}

func TestSagaTransactionPartialCompensation(t *testing.T) {
	so := NewSagaOrchestrator()
	tx := NewSagaTransaction("saga-3")

	step1Comp := false
	step2Comp := false

	tx.AddStep(&SagaStep{
		Name: "step1",
		Action: func(ctx context.Context) error { return nil },
		Compensate: func(ctx context.Context) error {
			step1Comp = true
			return nil
		},
	})
	tx.AddStep(&SagaStep{
		Name: "step2",
		Action: func(ctx context.Context) error { return nil },
		Compensate: func(ctx context.Context) error {
			step2Comp = true
			return nil
		},
	})
	tx.AddStep(&SagaStep{
		Name: "step3",
		Action: func(ctx context.Context) error { return errors.New("fail") },
	})

	err := so.Execute(context.Background(), tx)
	if err == nil {
		t.Fatal("expected error")
	}
	// step3 失败，step2 和 step1 的补偿应该被执行
	if !step2Comp {
		t.Error("step2 compensate should be called")
	}
	if !step1Comp {
		t.Error("step1 compensate should be called")
	}
}

func TestOutboxManagerDispatch(t *testing.T) {
	store := newMockOutboxStore()
	pub := newMockPublisher()
	om := NewOutboxManager(store, pub, 10, 100*time.Millisecond)

	// 添加事件
	ctx := context.Background()
	om.AddEvent(ctx, &OutboxEvent{ID: "evt-1", Topic: "topic1", Payload: []byte("data1")})
	om.AddEvent(ctx, &OutboxEvent{ID: "evt-2", Topic: "topic2", Payload: []byte("data2")})

	// 启动投递
	om.Start()
	defer om.Stop()

	// 等待投递
	time.Sleep(300 * time.Millisecond)

	if len(pub.events) != 2 {
		t.Errorf("expected 2 published events, got %d", len(pub.events))
	}

	store.mu.Lock()
	pending := 0
	for _, e := range store.events {
		if e.Status == OutboxPending {
			pending++
		}
	}
	store.mu.Unlock()
	if pending > 0 {
		t.Errorf("expected 0 pending events, got %d", pending)
	}
}

func TestOutboxManagerRetry(t *testing.T) {
	store := newMockOutboxStore()
	pub := newMockPublisher()
	pub.setFail("evt-fail")

	om := NewOutboxManager(store, pub, 10, 100*time.Millisecond)
	om.maxRetries = 2

	ctx := context.Background()
	om.AddEvent(ctx, &OutboxEvent{ID: "evt-fail", Topic: "topic", Payload: []byte("data")})

	om.Start()
	defer om.Stop()

	time.Sleep(500 * time.Millisecond)

	store.mu.Lock()
	evt := store.events["evt-fail"]
	store.mu.Unlock()
	if evt == nil {
		t.Fatal("event not found")
	}
	if evt.Status != OutboxFailed {
		t.Errorf("expected status failed, got %v", evt.Status)
	}
	if evt.Retries < 2 {
		t.Errorf("expected retries >= 2, got %d", evt.Retries)
	}
}

func TestCacheConsistencyReadThrough(t *testing.T) {
	cache := newMockCacheStore()
	db := newMockDBStore()
	ccm := NewCacheConsistencyManager(cache, db, 50*time.Millisecond)

	// 预置数据库数据
	db.Set(context.Background(), "key1", "value1")

	// 第一次读（缓存 miss）
	val1, err := ccm.Get(context.Background(), "key1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if val1 != "value1" {
		t.Errorf("expected value1, got %v", val1)
	}

	// 检查缓存被回填
	calls := cache.getCalls()
	hasGet := false
	hasSet := false
	for _, c := range calls {
		if c == "GET:key1" {
			hasGet = true
		}
		if c == "SET:key1" {
			hasSet = true
		}
	}
	if !hasGet {
		t.Error("expected cache GET")
	}
	if !hasSet {
		t.Error("expected cache SET after miss")
	}

	// 第二次读（缓存 hit）
	val2, err := ccm.Get(context.Background(), "key1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if val2 != "value1" {
		t.Errorf("expected value1, got %v", val2)
	}
}

func TestCacheConsistencyWriteDelayDelete(t *testing.T) {
	cache := newMockCacheStore()
	db := newMockDBStore()
	ccm := NewCacheConsistencyManager(cache, db, 50*time.Millisecond)

	// 预置数据
	db.Set(context.Background(), "key1", "old-value")
	cache.Set(context.Background(), "key1", "old-value", 5*time.Minute)

	// 写入新值
	err := ccm.Set(context.Background(), "key1", "new-value")
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}

	// 立即检查：缓存应该已被删除
	calls := cache.getCalls()
	lastCall := ""
	if len(calls) > 0 {
		lastCall = calls[len(calls)-1]
	}
	if lastCall != "DEL:key1" {
		t.Errorf("expected last call to be DEL, got %s", lastCall)
	}

	// 检查数据库已更新
	dbVal, _ := db.Get(context.Background(), "key1")
	if dbVal != "new-value" {
		t.Errorf("expected db new-value, got %v", dbVal)
	}

	// 等待延迟删除
	time.Sleep(100 * time.Millisecond)

	// 再次检查缓存应该已被删除（延迟删除）
	_, err = cache.Get(context.Background(), "key1")
	if err == nil {
		t.Error("expected cache to be empty after delay delete")
	}
}

func TestCacheConsistencyDelete(t *testing.T) {
	cache := newMockCacheStore()
	db := newMockDBStore()
	ccm := NewCacheConsistencyManager(cache, db, 50*time.Millisecond)

	// 预置数据
	db.Set(context.Background(), "key1", "value1")
	cache.Set(context.Background(), "key1", "value1", 5*time.Minute)

	err := ccm.Delete(context.Background(), "key1")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// 检查数据库已删除
	_, err = db.Get(context.Background(), "key1")
	if err == nil {
		t.Error("expected db to be empty")
	}

	// 等待延迟删除
	time.Sleep(100 * time.Millisecond)

	// 检查缓存已删除
	_, err = cache.Get(context.Background(), "key1")
	if err == nil {
		t.Error("expected cache to be empty after delay delete")
	}
}

func TestCompensationSchedulerSuccess(t *testing.T) {
	cs := NewCompensationScheduler()
	actionCalled := false

	cs.Register(&CompensationTask{
		ID:         "task-1",
		TargetID:   "target-1",
		Action: func(ctx context.Context) error {
			actionCalled = true
			return nil
		},
		MaxRetries: 3,
		Interval:   50 * time.Millisecond,
	})

	cs.Start()

	// 等待执行
	time.Sleep(150 * time.Millisecond)

	if !actionCalled {
		t.Error("action should be called")
	}
	if cs.GetTaskStatus("task-1") != CompensationSuccess {
		t.Errorf("expected success, got %v", cs.GetTaskStatus("task-1"))
	}
}

func TestCompensationSchedulerRetry(t *testing.T) {
	cs := NewCompensationScheduler()
	callCount := 0

	cs.Register(&CompensationTask{
		ID:         "task-2",
		TargetID:   "target-2",
		Action: func(ctx context.Context) error {
			callCount++
			if callCount < 3 {
				return errors.New("temporary failure")
			}
			return nil
		},
		MaxRetries: 5,
		Interval:   50 * time.Millisecond,
	})

	cs.Start()

	time.Sleep(400 * time.Millisecond)

	if callCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", callCount)
	}
	if cs.GetTaskStatus("task-2") != CompensationSuccess {
		t.Errorf("expected success after retries, got %v", cs.GetTaskStatus("task-2"))
	}
	if cs.GetTaskRetries("task-2") < 2 {
		t.Errorf("expected retries >= 2, got %d", cs.GetTaskRetries("task-2"))
	}
}

func TestCompensationSchedulerFailed(t *testing.T) {
	cs := NewCompensationScheduler()

	cs.Register(&CompensationTask{
		ID:         "task-3",
		TargetID:   "target-3",
		Action: func(ctx context.Context) error {
			return errors.New("permanent failure")
		},
		MaxRetries: 2,
		Interval:   50 * time.Millisecond,
	})

	cs.Start()

	time.Sleep(300 * time.Millisecond)

	if cs.GetTaskStatus("task-3") != CompensationFailed {
		t.Errorf("expected failed, got %v", cs.GetTaskStatus("task-3"))
	}
}

func TestConsistencyChecker(t *testing.T) {
	checker := NewConsistencyChecker()
	checker.RegisterSource(&mockDataSource{
		name: "db",
		data: map[string]interface{}{"key1": "value1"},
	})
	checker.RegisterSource(&mockDataSource{
		name: "cache",
		data: map[string]interface{}{"key1": "value1"},
	})

	result, err := checker.CheckConsistency(context.Background(), "key1")
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if !result.Consistent {
		t.Error("expected consistent")
	}
	if result.Values["db"] != "value1" || result.Values["cache"] != "value1" {
		t.Errorf("unexpected values: %v", result.Values)
	}
}

func TestConsistencyCheckerInconsistent(t *testing.T) {
	checker := NewConsistencyChecker()
	checker.RegisterSource(&mockDataSource{
		name: "db",
		data: map[string]interface{}{"key1": "value1"},
	})
	checker.RegisterSource(&mockDataSource{
		name: "cache",
		data: map[string]interface{}{"key1": "value2"},
	})

	result, err := checker.CheckConsistency(context.Background(), "key1")
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if result.Consistent {
		t.Error("expected inconsistent")
	}
}

func TestIdempotencyChecker(t *testing.T) {
	store := newMockIdempotencyStore()
	ic := NewIdempotencyChecker(store)

	// 第一次执行
	callCount := 0
	err := ic.Execute(context.Background(), "req-1", 5*time.Minute, func() error {
		callCount++
		return nil
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// 第二次执行（幂等）
	err = ic.Execute(context.Background(), "req-1", 5*time.Minute, func() error {
		callCount++
		return nil
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 call (idempotent), got %d", callCount)
	}

	// 检查已处理
	processed, _ := ic.Check(context.Background(), "req-1")
	if !processed {
		t.Error("expected req-1 to be processed")
	}
}

func TestIdempotencyCheckerNewRequest(t *testing.T) {
	store := newMockIdempotencyStore()
	ic := NewIdempotencyChecker(store)

	callCount := 0
	err := ic.Execute(context.Background(), "req-2", 5*time.Minute, func() error {
		callCount++
		return nil
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call for new request, got %d", callCount)
	}
}
