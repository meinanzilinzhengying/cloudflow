package cmdb

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"
)

// SyncEventType 同步事件类型
type SyncEventType string

const (
	SyncEventAssetCreated  SyncEventType = "asset_created"
	SyncEventAssetUpdated  SyncEventType = "asset_updated"
	SyncEventAssetDeleted  SyncEventType = "asset_deleted"
	SyncEventLabelChanged  SyncEventType = "label_changed"
	SyncEventConfigChanged SyncEventType = "config_changed"
	SyncEventSyncStart     SyncEventType = "sync_start"
	SyncEventSyncComplete  SyncEventType = "sync_complete"
	SyncEventSyncFailed    SyncEventType = "sync_failed"
)

// SyncEvent 同步事件
type SyncEvent struct {
	Type      SyncEventType     `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	Asset     *CMDBAsset        `json:"asset,omitempty"`
	Change    *AssetChange      `json:"change,omitempty"`
	Result    *SyncResult       `json:"result,omitempty"`
}

// SyncEventHandler 同步事件处理函数
type SyncEventHandler func(event SyncEvent)

// SyncEngine 同步引擎
type SyncEngine struct {
	mu          sync.RWMutex
	client      CMDBClient
	config      *SyncConfig
	sourceConfig *CMDBSourceConfig

	// 本地缓存
	assets      map[string]*CMDBAsset    // 按ID索引
	assetsByIP  map[string]*CMDBAsset    // 按IP索引
	labels      map[string]map[string]string // 资产标签缓存
	groups      map[string]*CMDBGroup     // 分组缓存
	apps        map[string]*CMDBApp       // 应用缓存

	// 同步状态
	lastSyncTime time.Time
	stats        SyncStats
	cancelFunc   context.CancelFunc
	running      bool

	// 事件处理
	eventHandlers []SyncEventHandler
}

// NewSyncEngine 创建同步引擎
func NewSyncEngine(client CMDBClient, config *SyncConfig, sourceConfig *CMDBSourceConfig) *SyncEngine {
	return &SyncEngine{
		client:       client,
		config:       config,
		sourceConfig: sourceConfig,
		assets:       make(map[string]*CMDBAsset),
		assetsByIP:   make(map[string]*CMDBAsset),
		labels:       make(map[string]map[string]string),
		groups:       make(map[string]*CMDBGroup),
		apps:         make(map[string]*CMDBApp),
	}
}

// Start 启动同步引擎
func (e *SyncEngine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("同步引擎已在运行")
	}
	e.running = true
	e.mu.Unlock()

	// 健康检查
	if err := e.client.Ping(ctx); err != nil {
		slog.Warn("CMDB健康检查失败，将在后台重试", "error", err)
	} else {
		slog.Info("CMDB连接成功")
	}

	// 启动时全量同步
	if e.config.FullSyncOnStart {
		slog.Info("执行启动全量同步...")
		result := e.FullSync(ctx)
		slog.Info("启动全量同步完成",
			"created", result.TotalCreated,
			"updated", result.TotalUpdated,
			"deleted", result.TotalDeleted,
			"errors", result.TotalErrors,
		)
	}

	// 启动定时同步
	syncCtx, cancel := context.WithCancel(ctx)
	e.cancelFunc = cancel

	go e.syncLoop(syncCtx)

	slog.Info("CMDB同步引擎已启动", "interval", e.config.SyncInterval)
	return nil
}

// Stop 停止同步引擎
func (e *SyncEngine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.cancelFunc != nil {
		e.cancelFunc()
	}
	e.running = false
	e.client.Close()
	slog.Info("CMDB同步引擎已停止")
}

// syncLoop 同步循环
func (e *SyncEngine) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(e.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e.config.IncrementalSync && !e.lastSyncTime.IsZero() {
				// 增量同步
				result := e.IncrementalSync(ctx, e.lastSyncTime)
				slog.Debug("增量同步完成",
					"created", result.TotalCreated,
					"updated", result.TotalUpdated,
					"deleted", result.TotalDeleted,
				)
			} else {
				// 全量同步
				result := e.FullSync(ctx)
				slog.Debug("定时全量同步完成",
					"created", result.TotalCreated,
					"updated", result.TotalUpdated,
					"deleted", result.TotalDeleted,
				)
			}
		}
	}
}

// FullSync 全量同步
func (e *SyncEngine) FullSync(ctx context.Context) *SyncResult {
	result := &SyncResult{
		SyncID:    generateSyncID(),
		StartTime: time.Now(),
		SyncType:  "full",
		Changes:   make([]AssetChange, 0),
	}

	e.emitEvent(SyncEvent{
		Type:      SyncEventSyncStart,
		Timestamp: result.StartTime,
	})

	// 1. 拉取所有资产
	filter := &AssetFilter{
		AssetTypes: e.sourceConfig.AssetTypeFilter,
		Statuses:   e.sourceConfig.StatusFilter,
		Labels:     e.sourceConfig.LabelFilter,
	}

	remoteAssets, err := e.client.FetchAssets(ctx, filter)
	if err != nil {
		result.Error = fmt.Sprintf("拉取资产失败: %v", err)
		result.TotalErrors++
		e.emitEvent(SyncEvent{
			Type:      SyncEventSyncFailed,
			Timestamp: time.Now(),
			Result:    result,
		})
		e.updateStats(result, "failed")
		return result
	}

	result.TotalFetched = len(remoteAssets)
	slog.Info("从CMDB拉取资产", "count", len(remoteAssets))

	// 2. 构建远程资产索引
	remoteMap := make(map[string]CMDBAsset, len(remoteAssets))
	for _, a := range remoteAssets {
		remoteMap[a.ID] = a
	}

	// 3. 检测新增和更新
	for id, remote := range remoteMap {
		local, exists := e.assets[id]
		if !exists {
			// 新增资产
			e.assets[id] = &remote
			e.indexByIP(&remote)
			result.TotalCreated++

			change := AssetChange{
				AssetID:   id,
				AssetName: remote.Name,
				ChangeType: ChangeTypeCreated,
				Timestamp: time.Now(),
			}
			result.Changes = append(result.Changes, change)

			e.emitEvent(SyncEvent{
				Type:  SyncEventAssetCreated,
				Asset: &remote,
			})
		} else {
			// 检测变更
			changes := e.detectChanges(local, &remote)
			if len(changes) > 0 {
				e.assets[id] = &remote
				e.reindexByIP(local, &remote)
				result.TotalUpdated++

				change := AssetChange{
					AssetID:   id,
					AssetName: remote.Name,
					ChangeType: classifyChange(changes),
					Fields:    changes,
					Timestamp: time.Now(),
				}
				result.Changes = append(result.Changes, change)

				e.emitEvent(SyncEvent{
					Type:   SyncEventAssetUpdated,
					Asset:  &remote,
					Change: &change,
				})
			} else {
				result.TotalUnchanged++
			}
		}
	}

	// 4. 检测删除
	for id, local := range e.assets {
		if _, exists := remoteMap[id]; !exists {
			delete(e.assets, id)
			e.removeIPIndex(local)
			result.TotalDeleted++

			change := AssetChange{
				AssetID:   id,
				AssetName: local.Name,
				ChangeType: ChangeTypeDeleted,
				Timestamp: time.Now(),
			}
			result.Changes = append(result.Changes, change)

			e.emitEvent(SyncEvent{
				Type:  SyncEventAssetDeleted,
				Asset: local,
			})
		}
	}

	// 5. 同步标签
	if e.config.EnableLabelSync {
		e.syncLabels(ctx, result)
	}

	// 6. 同步配置
	if e.config.EnableConfigSync {
		e.syncConfigs(ctx, result)
	}

	// 7. 同步分组和关联
	if e.config.EnableRelationSync {
		e.syncRelations(ctx, result)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	e.lastSyncTime = result.EndTime

	e.emitEvent(SyncEvent{
		Type:   SyncEventSyncComplete,
		Result: result,
	})

	e.updateStats(result, "success")
	return result
}

// IncrementalSync 增量同步
func (e *SyncEngine) IncrementalSync(ctx context.Context, since time.Time) *SyncResult {
	result := &SyncResult{
		SyncID:    generateSyncID(),
		StartTime: time.Now(),
		SyncType:  "incremental",
		Changes:   make([]AssetChange, 0),
	}

	// 拉取增量变更
	changedAssets, err := e.client.FetchIncremental(ctx, since)
	if err != nil {
		result.Error = fmt.Sprintf("拉取增量变更失败: %v", err)
		result.TotalErrors++
		e.updateStats(result, "failed")
		return result
	}

	result.TotalFetched = len(changedAssets)
	slog.Info("从CMDB拉取增量变更", "count", len(changedAssets))

	for _, remote := range changedAssets {
		local, exists := e.assets[remote.ID]
		if !exists {
			// 新增
			e.assets[remote.ID] = &remote
			e.indexByIP(&remote)
			result.TotalCreated++
		} else {
			// 更新
			changes := e.detectChanges(local, &remote)
			if len(changes) > 0 {
				e.assets[remote.ID] = &remote
				e.reindexByIP(local, &remote)
				result.TotalUpdated++

				change := AssetChange{
					AssetID:   remote.ID,
					AssetName: remote.Name,
					ChangeType: classifyChange(changes),
					Fields:    changes,
					Timestamp: time.Now(),
				}
				result.Changes = append(result.Changes, change)
			} else {
				result.TotalUnchanged++
			}
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	e.lastSyncTime = result.EndTime

	e.updateStats(result, "success")
	return result
}

// syncLabels 同步标签
func (e *SyncEngine) syncLabels(ctx context.Context, result *SyncResult) {
	// 收集所有资产ID
	assetIDs := make([]string, 0, len(e.assets))
	for id := range e.assets {
		assetIDs = append(assetIDs, id)
	}

	// 批量获取标签
	batchSize := e.config.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	for i := 0; i < len(assetIDs); i += batchSize {
		end := i + batchSize
		if end > len(assetIDs) {
			end = len(assetIDs)
		}
		batch := assetIDs[i:end]

		remoteLabels, err := e.client.FetchLabels(ctx, batch)
		if err != nil {
			slog.Warn("批量获取标签失败", "batch_start", i, "error", err)
			result.TotalErrors++
			continue
		}

		// 比较并更新标签
		for assetID, newLabels := range remoteLabels {
			oldLabels := e.labels[assetID]

			added, removed, modified := diffLabels(oldLabels, newLabels)

			if len(added) > 0 || len(removed) > 0 || len(modified) > 0 {
				e.labels[assetID] = newLabels

				e.emitEvent(SyncEvent{
					Type: SyncEventLabelChanged,
					Asset: e.assets[assetID],
					Change: &AssetChange{
						AssetID:   assetID,
						ChangeType: ChangeTypeLabelChanged,
						Timestamp: time.Now(),
					},
				})
			}
		}
	}
}

// syncConfigs 同步配置
func (e *SyncEngine) syncConfigs(ctx context.Context, result *SyncResult) {
	// 配置同步在全量同步中已通过 detectChanges 处理
	// 此处可添加额外的配置同步逻辑
}

// syncRelations 同步关联关系
func (e *SyncEngine) syncRelations(ctx context.Context, result *SyncResult) {
	// 同步分组
	groups, err := e.client.FetchGroups(ctx)
	if err != nil {
		slog.Warn("获取分组失败", "error", err)
		result.TotalErrors++
	} else {
		for i := range groups {
			e.groups[groups[i].ID] = &groups[i]
		}
	}

	// 同步应用
	apps, err := e.client.FetchApps(ctx)
	if err != nil {
		slog.Warn("获取应用列表失败", "error", err)
		result.TotalErrors++
	} else {
		for i := range apps {
			e.apps[apps[i].ID] = &apps[i]
		}
	}
}

// detectChanges 检测资产变更
func (e *SyncEngine) detectChanges(local, remote *CMDBAsset) map[string]FieldChange {
	changes := make(map[string]FieldChange)

	// 比较配置字段
	configFields := []string{
		"Name", "AssetType", "Status", "IP", "IPs", "Hostname", "FQDN",
		"OS", "OSVersion", "CPUCores", "MemoryGB", "DiskGB",
		"Datacenter", "Rack", "Zone",
		"Business", "Department", "Owner", "Team", "Environment",
		"ServiceLine", "Project", "Cluster", "Tier",
		"ParentID", "GroupIDs", "AppIDs",
	}

	localVal := reflect.ValueOf(local).Elem()
	remoteVal := reflect.ValueOf(remote).Elem()

	for _, field := range configFields {
		localField := localVal.FieldByName(field)
		remoteField := remoteVal.FieldByName(field)

		if !localField.IsValid() || !remoteField.IsValid() {
			continue
		}

		if !reflect.DeepEqual(localField.Interface(), remoteField.Interface()) {
			changes[field] = FieldChange{
				OldValue: localField.Interface(),
				NewValue: remoteField.Interface(),
			}
		}
	}

	// 比较标签
	if !reflect.DeepEqual(local.Labels, remote.Labels) {
		changes["Labels"] = FieldChange{
			OldValue: local.Labels,
			NewValue: remote.Labels,
		}
	}

	return changes
}

// classifyChange 分类变更类型
func classifyChange(changes map[string]FieldChange) ChangeType {
	hasLabelChange := false
	hasConfigChange := false
	hasStatusChange := false

	for field := range changes {
		switch field {
		case "Labels":
			hasLabelChange = true
		case "Status":
			hasStatusChange = true
		default:
			hasConfigChange = true
		}
	}

	if hasStatusChange {
		return ChangeTypeStatusChanged
	}
	if hasLabelChange && hasConfigChange {
		return ChangeTypeUpdated
	}
	if hasLabelChange {
		return ChangeTypeLabelChanged
	}
	if hasConfigChange {
		return ChangeTypeConfigChanged
	}
	return ChangeTypeUpdated
}

// indexByIP 按IP索引
func (e *SyncEngine) indexByIP(asset *CMDBAsset) {
	if asset.IP != "" {
		e.assetsByIP[asset.IP] = asset
	}
	for _, ip := range asset.IPs {
		e.assetsByIP[ip] = asset
	}
}

// reindexByIP 重新按IP索引
func (e *SyncEngine) reindexByIP(oldAsset, newAsset *CMDBAsset) {
	e.removeIPIndex(oldAsset)
	e.indexByIP(newAsset)
}

// removeIPIndex 移除IP索引
func (e *SyncEngine) removeIPIndex(asset *CMDBAsset) {
	if asset.IP != "" {
		if existing, ok := e.assetsByIP[asset.IP]; ok && existing.ID == asset.ID {
			delete(e.assetsByIP, asset.IP)
		}
	}
	for _, ip := range asset.IPs {
		if existing, ok := e.assetsByIP[ip]; ok && existing.ID == asset.ID {
			delete(e.assetsByIP, ip)
		}
	}
}

// diffLabels 比较标签差异
func diffLabels(oldLabels, newLabels map[string]string) (added, removed []string, modified map[string]string) {
	if oldLabels == nil {
		oldLabels = make(map[string]string)
	}
	if newLabels == nil {
		newLabels = make(map[string]string)
	}

	modified = make(map[string]string)

	for k, v := range newLabels {
		if oldVal, exists := oldLabels[k]; !exists {
			added = append(added, k)
		} else if oldVal != v {
			modified[k] = v
		}
	}

	for k := range oldLabels {
		if _, exists := newLabels[k]; !exists {
			removed = append(removed, k)
		}
	}

	return
}

// updateStats 更新统计
func (e *SyncEngine) updateStats(result *SyncResult, status string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.stats.LastSyncTime = result.EndTime
	e.stats.LastSyncType = result.SyncType
	e.stats.LastSyncResult = status
	e.stats.TotalSyncs++
	e.stats.TotalAssets = len(e.assets)
	e.stats.TotalLabels = len(e.labels)

	if status == "failed" {
		e.stats.ConsecutiveFails++
	} else {
		e.stats.ConsecutiveFails = 0
	}
}

// GetAsset 按ID获取资产
func (e *SyncEngine) GetAsset(assetID string) (*CMDBAsset, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	asset, ok := e.assets[assetID]
	return asset, ok
}

// GetAssetByIP 按IP获取资产
func (e *SyncEngine) GetAssetByIP(ip string) (*CMDBAsset, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	asset, ok := e.assetsByIP[ip]
	return asset, ok
}

// GetLabels 获取资产标签
func (e *SyncEngine) GetLabels(assetID string) map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.labels[assetID]
}

// GetAllLabels 获取所有标签
func (e *SyncEngine) GetAllLabels() map[string]map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string]map[string]string, len(e.labels))
	for k, v := range e.labels {
		result[k] = v
	}
	return result
}

// GetAssetsByLabel 按标签查询资产
func (e *SyncEngine) GetAssetsByLabel(key, value string) []*CMDBAsset {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*CMDBAsset
	for _, asset := range e.assets {
		if v, ok := asset.Labels[key]; ok && v == value {
			result = append(result, asset)
		}
	}
	return result
}

// GetStats 获取同步统计
func (e *SyncEngine) GetStats() SyncStats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.stats
}

// GetLastSyncTime 获取最后同步时间
func (e *SyncEngine) GetLastSyncTime() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastSyncTime
}

// OnEvent 注册事件处理函数
func (e *SyncEngine) OnEvent(handler SyncEventHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.eventHandlers = append(e.eventHandlers, handler)
}

// emitEvent 发送事件
func (e *SyncEngine) emitEvent(event SyncEvent) {
	e.mu.RLock()
	handlers := make([]SyncEventHandler, len(e.eventHandlers))
	copy(handlers, e.eventHandlers)
	e.mu.RUnlock()

	for _, handler := range handlers {
		go func(h SyncEventHandler) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("同步事件处理器panic", "error", r)
				}
			}()
			h(event)
		}(handler)
	}
}

// generateSyncID 生成同步ID
func generateSyncID() string {
	return fmt.Sprintf("sync-%d", time.Now().UnixNano())
}
