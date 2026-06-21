//go:build linux

package consistency

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// P9 数据一致性保证
// 解决：缺少分布式事务机制、多存储写入不一致、缓存双写不一致、缺少最终一致性保证
// ============================================================================

// ============================================================================
// 1. Saga 分布式事务编排
// ============================================================================

// SagaState Saga 事务状态
type SagaState int

const (
	SagaPending SagaState = iota
	SagaExecuting
	SagaCommitted
	SagaCompensating
	SagaCompensated
	SagaFailed
)

func (s SagaState) String() string {
	switch s {
	case SagaPending:
		return "pending"
	case SagaExecuting:
		return "executing"
	case SagaCommitted:
		return "committed"
	case SagaCompensating:
		return "compensating"
	case SagaCompensated:
		return "compensated"
	case SagaFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// SagaAction Saga 步骤动作
type SagaAction func(ctx context.Context) error

// SagaCompensate Saga 步骤补偿
type SagaCompensate func(ctx context.Context) error

// SagaStep Saga 步骤
type SagaStep struct {
	Name       string
	Action     SagaAction
	Compensate SagaCompensate
}

// SagaTransaction Saga 事务
type SagaTransaction struct {
	ID     string
	Steps  []*SagaStep
	State  SagaState
	Errors []error
	
	mu       sync.RWMutex
	results  map[string]interface{}
	stepIdx  int
}

// NewSagaTransaction 创建 Saga 事务
func NewSagaTransaction(id string) *SagaTransaction {
	return &SagaTransaction{
		ID:      id,
		State:   SagaPending,
		results: make(map[string]interface{}),
		Errors:  make([]error, 0),
	}
}

// AddStep 添加步骤
func (st *SagaTransaction) AddStep(step *SagaStep) {
	st.Steps = append(st.Steps, step)
}

// GetState 获取状态
func (st *SagaTransaction) GetState() SagaState {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.State
}

// GetResult 获取步骤结果
func (st *SagaTransaction) GetResult(key string) interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.results[key]
}

// SetResult 设置步骤结果
func (st *SagaTransaction) SetResult(key string, val interface{}) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.results[key] = val
}

// SagaOrchestrator Saga 编排器
type SagaOrchestrator struct {
	mu         sync.RWMutex
	transactions map[string]*SagaTransaction
}

// NewSagaOrchestrator 创建 Saga 编排器
func NewSagaOrchestrator() *SagaOrchestrator {
	return &SagaOrchestrator{
		transactions: make(map[string]*SagaTransaction),
	}
}

// Execute 执行 Saga 事务
func (so *SagaOrchestrator) Execute(ctx context.Context, tx *SagaTransaction) error {
	so.mu.Lock()
	so.transactions[tx.ID] = tx
	so.mu.Unlock()
	
	tx.mu.Lock()
	tx.State = SagaExecuting
	tx.mu.Unlock()
	
	for i, step := range tx.Steps {
		tx.stepIdx = i
		if err := step.Action(ctx); err != nil {
			// 记录失败步骤
			tx.Errors = append(tx.Errors, fmt.Errorf("step %s failed: %w", step.Name, err))
			
			// 回滚：按反向顺序执行补偿
			tx.mu.Lock()
			tx.State = SagaCompensating
			tx.mu.Unlock()
			
			for j := i - 1; j >= 0; j-- {
				compStep := tx.Steps[j]
				if compStep.Compensate != nil {
					if compErr := compStep.Compensate(ctx); compErr != nil {
						tx.Errors = append(tx.Errors, 
							fmt.Errorf("compensate step %s failed: %w", compStep.Name, compErr))
					}
				}
			}
			
			tx.mu.Lock()
			tx.State = SagaCompensated
			tx.mu.Unlock()
			return fmt.Errorf("saga %s failed at step %s: %w", tx.ID, step.Name, err)
		}
	}
	
	tx.mu.Lock()
	tx.State = SagaCommitted
	tx.mu.Unlock()
	return nil
}

// GetTransaction 获取事务
func (so *SagaOrchestrator) GetTransaction(id string) *SagaTransaction {
	so.mu.RLock()
	defer so.mu.RUnlock()
	return so.transactions[id]
}

// ============================================================================
// 2. Outbox 模式（多存储一致性）
// ============================================================================

// OutboxEvent 本地事务事件
type OutboxEvent struct {
	ID        string
	Topic     string
	Payload   []byte
	CreatedAt time.Time
	Status    OutboxStatus
	Retries   int
}

// OutboxStatus 事件状态
type OutboxStatus int

const (
	OutboxPending OutboxStatus = iota
	OutboxPublished
	OutboxFailed
)

// OutboxPublisher 事件发布器
type OutboxPublisher interface {
	Publish(ctx context.Context, event *OutboxEvent) error
}

// OutboxStore 事件存储接口
type OutboxStore interface {
	Save(ctx context.Context, event *OutboxEvent) error
	GetPending(ctx context.Context, limit int) ([]*OutboxEvent, error)
	MarkPublished(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, retries int) error
	UpdateRetries(ctx context.Context, id string, retries int) error
}

// OutboxManager Outbox 管理器
type OutboxManager struct {
	store     OutboxStore
	publisher OutboxPublisher
	batchSize int
	interval  time.Duration
	maxRetries int
	
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
}

// NewOutboxManager 创建 Outbox 管理器
func NewOutboxManager(store OutboxStore, publisher OutboxPublisher, batchSize int, interval time.Duration) *OutboxManager {
	return &OutboxManager{
		store:      store,
		publisher:  publisher,
		batchSize:  batchSize,
		interval:   interval,
		maxRetries: 3,
		stopCh:     make(chan struct{}),
	}
}

// Start 启动投递循环
func (om *OutboxManager) Start() {
	om.mu.Lock()
	if om.running {
		om.mu.Unlock()
		return
	}
	om.running = true
	om.mu.Unlock()
	
	go om.dispatchLoop()
}

// Stop 停止投递循环
func (om *OutboxManager) Stop() {
	om.mu.Lock()
	if !om.running {
		om.mu.Unlock()
		return
	}
	om.running = false
	om.mu.Unlock()
	close(om.stopCh)
}

// dispatchLoop 投递循环
func (om *OutboxManager) dispatchLoop() {
	ticker := time.NewTicker(om.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-om.stopCh:
			return
		case <-ticker.C:
			om.dispatchBatch()
		}
	}
}

// dispatchBatch 批量投递
func (om *OutboxManager) dispatchBatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	events, err := om.store.GetPending(ctx, om.batchSize)
	if err != nil {
		return
	}
	
	for _, event := range events {
		if event.Retries >= om.maxRetries {
			om.store.MarkFailed(ctx, event.ID, event.Retries)
			continue
		}
		
		if err := om.publisher.Publish(ctx, event); err != nil {
			newRetries := event.Retries + 1
			if newRetries >= om.maxRetries {
				om.store.MarkFailed(ctx, event.ID, newRetries)
			} else {
				om.store.UpdateRetries(ctx, event.ID, newRetries)
			}
			continue
		}
		
		om.store.MarkPublished(ctx, event.ID)
	}
}

// AddEvent 添加事件
func (om *OutboxManager) AddEvent(ctx context.Context, event *OutboxEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	if event.Status == 0 && event.Status != OutboxPending {
		event.Status = OutboxPending
	}
	return om.store.Save(ctx, event)
}

// ============================================================================
// 3. 缓存一致性（Cache-Aside + 延迟双删）
// ============================================================================

// CacheStore 缓存存储接口
type CacheStore interface {
	Get(ctx context.Context, key string) (interface{}, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// DBStore 数据库存储接口
type DBStore interface {
	Get(ctx context.Context, key string) (interface{}, error)
	Set(ctx context.Context, key string, value interface{}) error
	Delete(ctx context.Context, key string) error
}

// CacheConsistencyManager 缓存一致性管理器
type CacheConsistencyManager struct {
	cache   CacheStore
	db      DBStore
	
	mu        sync.RWMutex
	delayDelete time.Duration // 延迟删除时间
}

// NewCacheConsistencyManager 创建缓存一致性管理器
func NewCacheConsistencyManager(cache CacheStore, db DBStore, delayDelete time.Duration) *CacheConsistencyManager {
	if delayDelete <= 0 {
		delayDelete = 500 * time.Millisecond
	}
	return &CacheConsistencyManager{
		cache:       cache,
		db:          db,
		delayDelete: delayDelete,
	}
}

// Get 读取数据（Cache-Aside）
func (ccm *CacheConsistencyManager) Get(ctx context.Context, key string) (interface{}, error) {
	// 1. 先读缓存
	val, err := ccm.cache.Get(ctx, key)
	if err == nil && val != nil {
		return val, nil
	}
	
	// 2. 缓存 miss，读数据库
	val, err = ccm.db.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	
	// 3. 回填缓存
	ccm.cache.Set(ctx, key, val, 5*time.Minute)
	return val, nil
}

// Set 写入数据（延迟双删）
func (ccm *CacheConsistencyManager) Set(ctx context.Context, key string, value interface{}) error {
	// 1. 先删除缓存（防止脏读）
	ccm.cache.Delete(ctx, key)
	
	// 2. 更新数据库
	if err := ccm.db.Set(ctx, key, value); err != nil {
		return err
	}
	
	// 3. 再次删除缓存（延迟双删，防止并发写导致不一致）
	go func() {
		time.Sleep(ccm.delayDelete)
		ccm.cache.Delete(context.Background(), key)
	}()
	
	return nil
}

// Delete 删除数据（延迟双删）
func (ccm *CacheConsistencyManager) Delete(ctx context.Context, key string) error {
	// 1. 先删除缓存
	ccm.cache.Delete(ctx, key)
	
	// 2. 删除数据库
	if err := ccm.db.Delete(ctx, key); err != nil {
		return err
	}
	
	// 3. 延迟再次删除缓存
	go func() {
		time.Sleep(ccm.delayDelete)
		ccm.cache.Delete(context.Background(), key)
	}()
	
	return nil
}

// ============================================================================
// 4. 最终一致性补偿
// ============================================================================

// CompensationTask 补偿任务
type CompensationTask struct {
	ID        string
	TargetID  string
	Action    func(ctx context.Context) error
	MaxRetries int
	Interval  time.Duration
	Status    CompensationStatus
	LastError error
	Retries   int
}

// CompensationStatus 补偿状态
type CompensationStatus int

const (
	CompensationPending CompensationStatus = iota
	CompensationRunning
	CompensationSuccess
	CompensationFailed
)

// CompensationScheduler 补偿调度器
type CompensationScheduler struct {
	mu     sync.RWMutex
	tasks  map[string]*CompensationTask
	wg     sync.WaitGroup
	stopCh chan struct{}
}

// NewCompensationScheduler 创建补偿调度器
func NewCompensationScheduler() *CompensationScheduler {
	return &CompensationScheduler{
		tasks:  make(map[string]*CompensationTask),
		stopCh: make(chan struct{}),
	}
}

// Register 注册补偿任务
func (cs *CompensationScheduler) Register(task *CompensationTask) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.tasks[task.ID] = task
}

// Start 启动补偿调度
func (cs *CompensationScheduler) Start() {
	cs.mu.RLock()
	tasks := make([]*CompensationTask, 0, len(cs.tasks))
	for _, t := range cs.tasks {
		tasks = append(tasks, t)
	}
	cs.mu.RUnlock()
	
	for _, task := range tasks {
		cs.wg.Add(1)
		go cs.runTask(task)
	}
}

// Stop 停止补偿调度
func (cs *CompensationScheduler) Stop() {
	close(cs.stopCh)
	cs.wg.Wait()
}

// runTask 运行补偿任务
func (cs *CompensationScheduler) runTask(task *CompensationTask) {
	defer cs.wg.Done()
	
	if task.Interval <= 0 {
		task.Interval = 5 * time.Second
	}
	if task.MaxRetries <= 0 {
		task.MaxRetries = 5
	}
	
	ticker := time.NewTicker(task.Interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-cs.stopCh:
			return
		case <-ticker.C:
			if task.Status == CompensationSuccess || task.Status == CompensationFailed {
				return
			}
			
			task.Status = CompensationRunning
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := task.Action(ctx)
			cancel()
			
			if err == nil {
				task.Status = CompensationSuccess
				return
			}
			
			task.LastError = err
			task.Retries++
			if task.Retries >= task.MaxRetries {
				task.Status = CompensationFailed
				return
			}
		}
	}
}

// GetTaskStatus 获取任务状态
func (cs *CompensationScheduler) GetTaskStatus(taskID string) CompensationStatus {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if task, ok := cs.tasks[taskID]; ok {
		return task.Status
	}
	return CompensationFailed
}

// GetTaskRetries 获取任务重试次数
func (cs *CompensationScheduler) GetTaskRetries(taskID string) int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if task, ok := cs.tasks[taskID]; ok {
		return task.Retries
	}
	return 0
}

// ============================================================================
// 5. 一致性状态校验器
// ============================================================================

// ConsistencyChecker 一致性校验器
type ConsistencyChecker struct {
	mu     sync.RWMutex
	sources []DataSource
}

// DataSource 数据源接口
type DataSource interface {
	Name() string
	Get(ctx context.Context, key string) (interface{}, error)
}

// NewConsistencyChecker 创建一致性校验器
func NewConsistencyChecker() *ConsistencyChecker {
	return &ConsistencyChecker{
		sources: make([]DataSource, 0),
	}
}

// RegisterSource 注册数据源
func (cc *ConsistencyChecker) RegisterSource(source DataSource) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.sources = append(cc.sources, source)
}

// CheckConsistency 检查一致性
func (cc *ConsistencyChecker) CheckConsistency(ctx context.Context, key string) (*ConsistencyResult, error) {
	cc.mu.RLock()
	sources := make([]DataSource, len(cc.sources))
	copy(sources, cc.sources)
	cc.mu.RUnlock()
	
	if len(sources) < 2 {
		return nil, fmt.Errorf("need at least 2 data sources")
	}
	
	values := make(map[string]interface{})
	for _, source := range sources {
		val, err := source.Get(ctx, key)
		if err != nil {
			values[source.Name()] = fmt.Sprintf("ERROR: %v", err)
		} else {
			values[source.Name()] = val
		}
	}
	
	// 比较一致性
	consistent := true
	var firstVal interface{}
	var firstName string
	for name, val := range values {
		if firstName == "" {
			firstName = name
			firstVal = val
			continue
		}
		if fmt.Sprintf("%v", firstVal) != fmt.Sprintf("%v", val) {
			consistent = false
			break
		}
	}
	
	return &ConsistencyResult{
		Key:         key,
		Values:      values,
		Consistent:  consistent,
		CheckedAt:   time.Now(),
	}, nil
}

// ConsistencyResult 一致性校验结果
type ConsistencyResult struct {
	Key        string
	Values     map[string]interface{}
	Consistent bool
	CheckedAt  time.Time
}

// ============================================================================
// 6. 幂等性保证
// ============================================================================

// IdempotencyStore 幂等性存储接口
type IdempotencyStore interface {
	Get(ctx context.Context, key string) (bool, error) // 返回是否已处理
	Set(ctx context.Context, key string, ttl time.Duration) error
}

// IdempotencyChecker 幂等性检查器
type IdempotencyChecker struct {
	store IdempotencyStore
	mu    sync.RWMutex
}

// NewIdempotencyChecker 创建幂等性检查器
func NewIdempotencyChecker(store IdempotencyStore) *IdempotencyChecker {
	return &IdempotencyChecker{store: store}
}

// Check 检查是否已处理（返回 true 表示已处理，应跳过）
func (ic *IdempotencyChecker) Check(ctx context.Context, key string) (bool, error) {
	processed, err := ic.store.Get(ctx, key)
	if err != nil {
		return false, err
	}
	return processed, nil
}

// MarkProcessed 标记为已处理
func (ic *IdempotencyChecker) MarkProcessed(ctx context.Context, key string, ttl time.Duration) error {
	return ic.store.Set(ctx, key, ttl)
}

// Execute 幂等执行（如果已处理返回 nil，否则执行 fn 并标记）
func (ic *IdempotencyChecker) Execute(ctx context.Context, key string, ttl time.Duration, fn func() error) error {
	processed, err := ic.Check(ctx, key)
	if err != nil {
		return err
	}
	if processed {
		return nil // 已处理，幂等返回
	}
	
	if err := fn(); err != nil {
		return err
	}
	
	return ic.MarkProcessed(ctx, key, ttl)
}
