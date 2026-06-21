package tenant

import (
    "fmt"
    "sync"
    "time"
)

// QuotaManager 租户配额管理器
type QuotaManager struct {
    mu       sync.RWMutex
    quotas   map[string]*TenantQuota // tenant_id -> quota
    usage    map[string]*TenantUsage // tenant_id -> usage
    // redis    RedisClient // optional, for distributed rate limiting
}

// TenantQuota 租户配额
type TenantQuota struct {
    TenantID        string
    MaxEventsPerSec int       // 每秒最大事件数
    MaxStorageBytes int64     // 最大存储字节数
    MaxQueryPerMin  int       // 每分钟最大查询数
    Disabled        bool      // 租户是否被禁用
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

// TenantUsage 租户当前使用量
type TenantUsage struct {
    TenantID        string
    EventsThisSec   int       // 当前秒事件数
    EventsThisDay   int       // 当天事件数
    StorageUsed     int64     // 已用存储
    QueriesThisMin  int       // 当前分钟查询数
    LastEventTime   time.Time
    LastQueryTime   time.Time
}

// NewQuotaManager 创建配额管理器
func NewQuotaManager() *QuotaManager {
    return &QuotaManager{
        quotas: make(map[string]*TenantQuota),
        usage:  make(map[string]*TenantUsage),
    }
}

// SetQuota 设置租户配额
func (qm *QuotaManager) SetQuota(q *TenantQuota) {
    qm.mu.Lock()
    defer qm.mu.Unlock()
    q.UpdatedAt = time.Now()
    qm.quotas[q.TenantID] = q
}

// GetQuota 获取租户配额
func (qm *QuotaManager) GetQuota(tenantID string) *TenantQuota {
    qm.mu.RLock()
    defer qm.mu.RUnlock()
    return qm.quotas[tenantID]
}

// IsDisabled 检查租户是否被禁用
func (qm *QuotaManager) IsDisabled(tenantID string) bool {
    qm.mu.RLock()
    defer qm.mu.RUnlock()
    q, ok := qm.quotas[tenantID]
    if !ok {
        return false // 默认不禁用
    }
    return q.Disabled
}

// CheckEventRate 检查事件写入速率
func (qm *QuotaManager) CheckEventRate(tenantID string) error {
    qm.mu.Lock()
    defer qm.mu.Unlock()

    quota, ok := qm.quotas[tenantID]
    if !ok {
        return nil // 无配额限制
    }

    if quota.Disabled {
        return fmt.Errorf("tenant %s is disabled", tenantID)
    }

    usage, ok := qm.usage[tenantID]
    if !ok {
        usage = &TenantUsage{TenantID: tenantID}
        qm.usage[tenantID] = usage
    }

    now := time.Now()
    if now.Sub(usage.LastEventTime) >= time.Second {
        usage.EventsThisSec = 0
    }
    usage.EventsThisSec++
    usage.LastEventTime = now

    if quota.MaxEventsPerSec > 0 && usage.EventsThisSec > quota.MaxEventsPerSec {
        return fmt.Errorf("tenant %s rate limit exceeded: %d/%d events/sec",
            tenantID, usage.EventsThisSec, quota.MaxEventsPerSec)
    }

    return nil
}

// CheckQueryRate 检查查询速率
func (qm *QuotaManager) CheckQueryRate(tenantID string) error {
    qm.mu.Lock()
    defer qm.mu.Unlock()

    quota, ok := qm.quotas[tenantID]
    if !ok {
        return nil
    }

    if quota.Disabled {
        return fmt.Errorf("tenant %s is disabled", tenantID)
    }

    usage, ok := qm.usage[tenantID]
    if !ok {
        usage = &TenantUsage{TenantID: tenantID}
        qm.usage[tenantID] = usage
    }

    now := time.Now()
    if now.Sub(usage.LastQueryTime) >= time.Minute {
        usage.QueriesThisMin = 0
    }
    usage.QueriesThisMin++
    usage.LastQueryTime = now

    if quota.MaxQueryPerMin > 0 && usage.QueriesThisMin > quota.MaxQueryPerMin {
        return fmt.Errorf("tenant %s query limit exceeded: %d/%d queries/min",
            tenantID, usage.QueriesThisMin, quota.MaxQueryPerMin)
    }

    return nil
}

// CheckStorage 检查存储配额
func (qm *QuotaManager) CheckStorage(tenantID string, additionalBytes int64) error {
    qm.mu.RLock()
    defer qm.mu.RUnlock()

    quota, ok := qm.quotas[tenantID]
    if !ok {
        return nil
    }

    if quota.MaxStorageBytes <= 0 {
        return nil
    }

    usage, ok := qm.usage[tenantID]
    if !ok {
        return nil
    }

    if usage.StorageUsed+additionalBytes > quota.MaxStorageBytes {
        return fmt.Errorf("tenant %s storage limit exceeded: %d/%d bytes",
            tenantID, usage.StorageUsed+additionalBytes, quota.MaxStorageBytes)
    }

    return nil
}

// AddStorageUsage 增加存储使用量
func (qm *QuotaManager) AddStorageUsage(tenantID string, bytes int64) {
    qm.mu.Lock()
    defer qm.mu.Unlock()

    usage, ok := qm.usage[tenantID]
    if !ok {
        usage = &TenantUsage{TenantID: tenantID}
        qm.usage[tenantID] = usage
    }
    usage.StorageUsed += bytes
}
