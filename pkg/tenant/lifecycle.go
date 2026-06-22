// P25: 租户生命周期管理 — 状态机、禁用/启用、删除、数据清理
//
// 解决：租户生命周期管理不完善（创建/禁用/删除流程）
// 提供：
//   - 租户状态机（active/suspended/pending_deletion/deleted）
//   - 禁用/启用操作
//   - 软删除 + 数据清理流程
//   - 删除前的资源检查
//   - 生命周期事件钩子
//
package tenant

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	sharedTenant "github.com/meinanzilinzhengying/cloudflow/services/shared/tenant"
)

// ============================================================================
// 生命周期指标
// ============================================================================

var (
	// TenantLifecycleEventTotal 租户生命周期事件计数器
	TenantLifecycleEventTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudflow_tenant_lifecycle_event_total",
			Help: "Total number of tenant lifecycle events",
		},
		[]string{"event_type"},
	)

	// TenantStatusCount 各状态租户数量（Gauge）
	TenantStatusCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cloudflow_tenant_status_count",
			Help: "Current number of tenants by status",
		},
		[]string{"status"},
	)

	// TenantCleanupDuration 租户清理耗时
	TenantCleanupDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cloudflow_tenant_cleanup_duration_seconds",
			Help:    "Tenant data cleanup duration in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"tenant_id"},
	)
)

// ============================================================================
// 一、租户状态机
// ============================================================================

// TenantStatus 租户状态
type TenantStatus string

const (
	// TenantStatusActive 活跃状态：租户正常使用
	TenantStatusActive TenantStatus = "active"

	// TenantStatusSuspended 暂停状态：租户被暂停（欠费/违规），只读访问
	TenantStatusSuspended TenantStatus = "suspended"

	// TenantStatusPendingDeletion 待删除状态：租户已申请删除，进入宽限期
	TenantStatusPendingDeletion TenantStatus = "pending_deletion"

	// TenantStatusDeleted 已删除状态：租户数据已清理，仅保留审计记录
	TenantStatusDeleted TenantStatus = "deleted"

	// TenantStatusCreating 创建中状态：租户正在初始化
	TenantStatusCreating TenantStatus = "creating"

	// TenantStatusFailed 创建失败状态：租户创建失败
	TenantStatusFailed TenantStatus = "failed"
)

// String 返回状态可读名称
func (s TenantStatus) String() string {
	return string(s)
}

// IsActive 判断租户是否处于活跃状态
func (s TenantStatus) IsActive() bool {
	return s == TenantStatusActive
}

// IsSuspended 判断租户是否处于暂停状态
func (s TenantStatus) IsSuspended() bool {
	return s == TenantStatusSuspended
}

// IsPendingDeletion 判断租户是否处于待删除状态
func (s TenantStatus) IsPendingDeletion() bool {
	return s == TenantStatusPendingDeletion
}

// IsDeleted 判断租户是否已删除
func (s TenantStatus) IsDeleted() bool {
	return s == TenantStatusDeleted
}

// CanAccess 判断租户状态是否允许数据访问
func (s TenantStatus) CanAccess() bool {
	return s == TenantStatusActive || s == TenantStatusSuspended
}

// CanWrite 判断租户状态是否允许写入操作
func (s TenantStatus) CanWrite() bool {
	return s == TenantStatusActive
}

// CanDelete 判断租户是否可以被删除（待删除 -> 已删除）
func (s TenantStatus) CanDelete() bool {
	return s == TenantStatusPendingDeletion
}

// ============================================================================
// 二、租户生命周期信息
// ============================================================================

// TenantLifecycleInfo 租户生命周期信息
type TenantLifecycleInfo struct {
	TenantID      string
	Status        TenantStatus
	CreatedAt     int64
	UpdatedAt     int64
	SuspendedAt   *int64
	SuspendedReason string
	SuspendedBy     string
	DeletionRequestedAt *int64
	DeletionReason      string
	DeletionRequestedBy string
	DeletedAt     *int64
	DeletedBy     string
	GracePeriodDays int // 删除宽限期天数
	Version       int64 // 乐观锁版本
}

// ============================================================================
// 三、生命周期管理器（TenantLifecycleManager）
// ============================================================================

// LifecycleHook 生命周期钩子函数签名
type LifecycleHook func(ctx context.Context, tenantID string, from, to TenantStatus, info *TenantLifecycleInfo) error

// CleanupFunc 数据清理函数签名
type CleanupFunc func(ctx context.Context, tenantID string) error

// TenantLifecycleManager 租户生命周期管理器
type TenantLifecycleManager struct {
	// 租户状态存储
	infos map[string]*TenantLifecycleInfo
	mu    sync.RWMutex

	// 默认宽限期天数
	defaultGracePeriodDays int

	// 生命周期钩子
	hooks map[TenantStatus][]LifecycleHook

	// 数据清理函数
	cleanupFuncs []CleanupFunc

	// 配额管理器（用于清理前检查资源）
	quotaManager *QuotaManager
}

// NewTenantLifecycleManager 创建生命周期管理器
func NewTenantLifecycleManager(defaultGracePeriodDays int) *TenantLifecycleManager {
	return &TenantLifecycleManager{
		infos:                  make(map[string]*TenantLifecycleInfo),
		defaultGracePeriodDays: defaultGracePeriodDays,
		hooks:                  make(map[TenantStatus][]LifecycleHook),
		cleanupFuncs:           make([]CleanupFunc, 0),
	}
}

// SetQuotaManager 设置配额管理器（用于清理前检查资源）
func (m *TenantLifecycleManager) SetQuotaManager(qm *QuotaManager) {
	m.quotaManager = qm
}

// RegisterHook 注册生命周期钩子
// status: 目标状态（当租户进入此状态时触发钩子）
func (m *TenantLifecycleManager) RegisterHook(status TenantStatus, hook LifecycleHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks[status] = append(m.hooks[status], hook)
}

// RegisterCleanupFunc 注册数据清理函数
func (m *TenantLifecycleManager) RegisterCleanupFunc(fn CleanupFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupFuncs = append(m.cleanupFuncs, fn)
}

// RegisterTenant 注册租户到生命周期管理器
func (m *TenantLifecycleManager) RegisterTenant(tenantID string, status TenantStatus) {
	now := time.Now().Unix()
	info := &TenantLifecycleInfo{
		TenantID:        tenantID,
		Status:          status,
		CreatedAt:       now,
		UpdatedAt:       now,
		GracePeriodDays: m.defaultGracePeriodDays,
		Version:         1,
	}

	m.mu.Lock()
	m.infos[tenantID] = info
	m.mu.Unlock()

	m.updateStatusGauge()
}

// GetTenantInfo 获取租户生命周期信息
func (m *TenantLifecycleManager) GetTenantInfo(tenantID string) *TenantLifecycleInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.infos[tenantID]
}

// GetTenantStatus 获取租户状态
func (m *TenantLifecycleManager) GetTenantStatus(tenantID string) TenantStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if info, ok := m.infos[tenantID]; ok {
		return info.Status
	}
	return TenantStatusDeleted // 未注册视为已删除
}

// ============================================================================
// 四、生命周期操作
// ============================================================================

// SuspendTenant 暂停租户
// reason: 暂停原因（如 "payment_overdue", "policy_violation"）
// suspendedBy: 操作人
func (m *TenantLifecycleManager) SuspendTenant(ctx context.Context, tenantID, reason, suspendedBy string) error {
	m.mu.Lock()
	info, ok := m.infos[tenantID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	if info.Status == TenantStatusSuspended {
		m.mu.Unlock()
		return nil // 已暂停，幂等
	}

	if info.Status == TenantStatusDeleted || info.Status == TenantStatusPendingDeletion {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s is already deleted or pending deletion", tenantID)
	}

	oldStatus := info.Status
	now := time.Now().Unix()
	info.Status = TenantStatusSuspended
	info.SuspendedAt = &now
	info.SuspendedReason = reason
	info.SuspendedBy = suspendedBy
	info.UpdatedAt = now
	info.Version++
	m.mu.Unlock()

	// 触发钩子
	if err := m.triggerHooks(ctx, tenantID, oldStatus, TenantStatusSuspended, info); err != nil {
		return fmt.Errorf("suspend hook failed: %w", err)
	}

	TenantLifecycleEventTotal.WithLabelValues("suspend").Inc()
	m.updateStatusGauge()
	return nil
}

// ActivateTenant 恢复租户（从暂停状态恢复为活跃）
func (m *TenantLifecycleManager) ActivateTenant(ctx context.Context, tenantID string) error {
	m.mu.Lock()
	info, ok := m.infos[tenantID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	if info.Status == TenantStatusActive {
		m.mu.Unlock()
		return nil // 已活跃，幂等
	}

	if info.Status == TenantStatusDeleted || info.Status == TenantStatusPendingDeletion {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s is already deleted or pending deletion", tenantID)
	}

	oldStatus := info.Status
	now := time.Now().Unix()
	info.Status = TenantStatusActive
	info.SuspendedAt = nil
	info.SuspendedReason = ""
	info.SuspendedBy = ""
	info.UpdatedAt = now
	info.Version++
	m.mu.Unlock()

	if err := m.triggerHooks(ctx, tenantID, oldStatus, TenantStatusActive, info); err != nil {
		return fmt.Errorf("activate hook failed: %w", err)
	}

	TenantLifecycleEventTotal.WithLabelValues("activate").Inc()
	m.updateStatusGauge()
	return nil
}

// RequestDeletion 请求删除租户（进入待删除状态，开始宽限期）
// 宽限期内租户可取消删除
func (m *TenantLifecycleManager) RequestDeletion(ctx context.Context, tenantID, reason, requestedBy string) error {
	m.mu.Lock()
	info, ok := m.infos[tenantID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	if info.Status == TenantStatusDeleted {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s is already deleted", tenantID)
	}

	if info.Status == TenantStatusPendingDeletion {
		m.mu.Unlock()
		return nil // 已申请删除，幂等
	}

	oldStatus := info.Status
	now := time.Now().Unix()
	info.Status = TenantStatusPendingDeletion
	info.DeletionRequestedAt = &now
	info.DeletionReason = reason
	info.DeletionRequestedBy = requestedBy
	info.UpdatedAt = now
	info.Version++
	m.mu.Unlock()

	if err := m.triggerHooks(ctx, tenantID, oldStatus, TenantStatusPendingDeletion, info); err != nil {
		return fmt.Errorf("deletion request hook failed: %w", err)
	}

	TenantLifecycleEventTotal.WithLabelValues("request_deletion").Inc()
	m.updateStatusGauge()
	return nil
}

// CancelDeletion 取消删除（从待删除状态恢复为活跃）
func (m *TenantLifecycleManager) CancelDeletion(ctx context.Context, tenantID string) error {
	m.mu.Lock()
	info, ok := m.infos[tenantID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	if info.Status != TenantStatusPendingDeletion {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s is not pending deletion", tenantID)
	}

	oldStatus := info.Status
	now := time.Now().Unix()
	info.Status = TenantStatusActive
	info.DeletionRequestedAt = nil
	info.DeletionReason = ""
	info.DeletionRequestedBy = ""
	info.UpdatedAt = now
	info.Version++
	m.mu.Unlock()

	if err := m.triggerHooks(ctx, tenantID, oldStatus, TenantStatusActive, info); err != nil {
		return fmt.Errorf("cancel deletion hook failed: %w", err)
	}

	TenantLifecycleEventTotal.WithLabelValues("cancel_deletion").Inc()
	m.updateStatusGauge()
	return nil
}

// ExecuteDeletion 执行删除（从待删除状态转为已删除，清理数据）
// 此操作不可逆，必须在宽限期结束后执行
func (m *TenantLifecycleManager) ExecuteDeletion(ctx context.Context, tenantID, deletedBy string) error {
	m.mu.Lock()
	info, ok := m.infos[tenantID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	if info.Status == TenantStatusDeleted {
		m.mu.Unlock()
		return nil // 已删除，幂等
	}

	if info.Status != TenantStatusPendingDeletion {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s must be in pending_deletion status before deletion", tenantID)
	}

	// 检查宽限期是否已过
	if info.DeletionRequestedAt != nil {
		gracePeriodEnd := *info.DeletionRequestedAt + int64(info.GracePeriodDays*24*3600)
		if time.Now().Unix() < gracePeriodEnd {
			m.mu.Unlock()
			return fmt.Errorf("tenant %s grace period not expired yet (expires at %d)", tenantID, gracePeriodEnd)
		}
	}

	oldStatus := info.Status
	now := time.Now().Unix()
	info.Status = TenantStatusDeleted
	info.DeletedAt = &now
	info.DeletedBy = deletedBy
	info.UpdatedAt = now
	info.Version++
	m.mu.Unlock()

	// 清理数据
	start := time.Now()
	if err := m.cleanupTenantData(ctx, tenantID); err != nil {
		// 数据清理失败，但状态已更新为 deleted，记录错误
		TenantLifecycleEventTotal.WithLabelValues("deletion_cleanup_failed").Inc()
		return fmt.Errorf("tenant data cleanup failed: %w", err)
	}
	TenantCleanupDuration.WithLabelValues(tenantID).Observe(time.Since(start).Seconds())

	if err := m.triggerHooks(ctx, tenantID, oldStatus, TenantStatusDeleted, info); err != nil {
		return fmt.Errorf("deletion hook failed: %w", err)
	}

	TenantLifecycleEventTotal.WithLabelValues("execute_deletion").Inc()
	m.updateStatusGauge()
	return nil
}

// DeleteTenantImmediately 立即删除租户（跳过宽限期，仅平台管理员可用）
func (m *TenantLifecycleManager) DeleteTenantImmediately(ctx context.Context, tenantID, deletedBy string) error {
	// 检查操作者是否为平台管理员
	tc, ok := sharedTenant.FromContext(ctx)
	if !ok || tc == nil || !tc.IsPlatformAdmin {
		return fmt.Errorf("only platform admin can delete tenant immediately")
	}

	m.mu.Lock()
	info, ok := m.infos[tenantID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	if info.Status == TenantStatusDeleted {
		m.mu.Unlock()
		return nil
	}

	oldStatus := info.Status
	now := time.Now().Unix()
	info.Status = TenantStatusDeleted
	info.DeletedAt = &now
	info.DeletedBy = deletedBy
	info.DeletionRequestedAt = &now
	info.DeletionReason = "immediate deletion by admin"
	info.DeletionRequestedBy = deletedBy
	info.UpdatedAt = now
	info.Version++
	m.mu.Unlock()

	start := time.Now()
	if err := m.cleanupTenantData(ctx, tenantID); err != nil {
		TenantLifecycleEventTotal.WithLabelValues("immediate_deletion_cleanup_failed").Inc()
		return fmt.Errorf("immediate tenant data cleanup failed: %w", err)
	}
	TenantCleanupDuration.WithLabelValues(tenantID).Observe(time.Since(start).Seconds())

	if err := m.triggerHooks(ctx, tenantID, oldStatus, TenantStatusDeleted, info); err != nil {
		return fmt.Errorf("immediate deletion hook failed: %w", err)
	}

	TenantLifecycleEventTotal.WithLabelValues("immediate_deletion").Inc()
	m.updateStatusGauge()
	return nil
}

// ============================================================================
// 五、状态校验
// ============================================================================

// ValidateTenantStatus 校验租户状态是否允许当前操作
// 适用于所有业务操作前的校验
func (m *TenantLifecycleManager) ValidateTenantStatus(ctx context.Context, tenantID string) error {
	m.mu.RLock()
	info, ok := m.infos[tenantID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	switch info.Status {
	case TenantStatusActive:
		return nil
	case TenantStatusSuspended:
		return fmt.Errorf("tenant %s is suspended (reason: %s)", tenantID, info.SuspendedReason)
	case TenantStatusPendingDeletion:
		return fmt.Errorf("tenant %s is pending deletion (grace period expires soon)", tenantID)
	case TenantStatusDeleted:
		return fmt.Errorf("tenant %s has been deleted", tenantID)
	case TenantStatusCreating:
		return fmt.Errorf("tenant %s is being created", tenantID)
	case TenantStatusFailed:
		return fmt.Errorf("tenant %s creation failed", tenantID)
	default:
		return fmt.Errorf("tenant %s has unknown status: %s", tenantID, info.Status)
	}
}

// ValidateTenantWriteAccess 校验租户是否允许写入操作
func (m *TenantLifecycleManager) ValidateTenantWriteAccess(ctx context.Context, tenantID string) error {
	m.mu.RLock()
	info, ok := m.infos[tenantID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	if !info.Status.CanWrite() {
		return fmt.Errorf("tenant %s does not allow write operations (status: %s)", tenantID, info.Status)
	}

	return nil
}

// ValidateTenantAccess 校验租户是否允许数据访问（读取）
func (m *TenantLifecycleManager) ValidateTenantAccess(ctx context.Context, tenantID string) error {
	m.mu.RLock()
	info, ok := m.infos[tenantID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	if !info.Status.CanAccess() {
		return fmt.Errorf("tenant %s does not allow access (status: %s)", tenantID, info.Status)
	}

	return nil
}

// ============================================================================
// 六、数据清理
// ============================================================================

func (m *TenantLifecycleManager) cleanupTenantData(ctx context.Context, tenantID string) error {
	// 1. 清理配额数据
	if m.quotaManager != nil {
		m.quotaManager.DeleteTenant(tenantID)
	}

	// 2. 执行注册的清理函数
	for i, fn := range m.cleanupFuncs {
		if err := fn(ctx, tenantID); err != nil {
			return fmt.Errorf("cleanup function %d failed: %w", i, err)
		}
	}

	return nil
}

// ============================================================================
// 七、辅助函数
// ============================================================================

func (m *TenantLifecycleManager) triggerHooks(ctx context.Context, tenantID string, from, to TenantStatus, info *TenantLifecycleInfo) error {
	m.mu.RLock()
	hooks := m.hooks[to]
	m.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook(ctx, tenantID, from, to, info); err != nil {
			return err
		}
	}
	return nil
}

func (m *TenantLifecycleManager) updateStatusGauge() {
	m.mu.RLock()
	counts := make(map[TenantStatus]int)
	for _, info := range m.infos {
		counts[info.Status]++
	}
	m.mu.RUnlock()

	for _, status := range []TenantStatus{
		TenantStatusActive, TenantStatusSuspended, TenantStatusPendingDeletion,
		TenantStatusDeleted, TenantStatusCreating, TenantStatusFailed,
	} {
		TenantStatusCount.WithLabelValues(string(status)).Set(float64(counts[status]))
	}
}

// GetAllTenantsByStatus 获取指定状态的所有租户
func (m *TenantLifecycleManager) GetAllTenantsByStatus(status TenantStatus) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []string
	for tenantID, info := range m.infos {
		if info.Status == status {
			result = append(result, tenantID)
		}
	}
	return result
}

// GetExpiredPendingDeletionTenants 获取已过宽限期的待删除租户
func (m *TenantLifecycleManager) GetExpiredPendingDeletionTenants() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now().Unix()
	var result []string
	for tenantID, info := range m.infos {
		if info.Status == TenantStatusPendingDeletion && info.DeletionRequestedAt != nil {
			gracePeriodEnd := *info.DeletionRequestedAt + int64(info.GracePeriodDays*24*3600)
			if now >= gracePeriodEnd {
				result = append(result, tenantID)
			}
		}
	}
	return result
}

// GetTenantCount 返回租户总数
func (m *TenantLifecycleManager) GetTenantCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.infos)
}
