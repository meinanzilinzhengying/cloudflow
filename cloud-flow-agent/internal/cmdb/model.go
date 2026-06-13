package cmdb

import (
	"time"
)

// CMDBAsset CMDB资产模型
type CMDBAsset struct {
	ID          string            `json:"id"`            // CMDB资产ID
	Name        string            `json:"name"`          // 资产名称
	AssetType   string            `json:"asset_type"`    // 资产类型: server/vm/container/network/db/middleware/app/service
	SerialNo    string            `json:"serial_no"`     // 序列号
	Status      CMDBAssetStatus   `json:"status"`        // 资产状态
	Lifecycle   CMDBLifecycle     `json:"lifecycle"`     // 生命周期

	// 网络信息
	IP          string            `json:"ip"`            // 主IP
	IPs         []string          `json:"ips"`           // 所有IP
	MAC         string            `json:"mac"`           // MAC地址
	Hostname    string            `json:"hostname"`      // 主机名
	FQDN        string            `json:"fqdn"`          // 完全域名

	// 硬件/系统信息
	OS          string            `json:"os"`            // 操作系统
	OSVersion   string            `json:"os_version"`    // 操作系统版本
	CPUCores    int               `json:"cpu_cores"`     // CPU核数
	MemoryGB    float64           `json:"memory_gb"`     // 内存(GB)
	DiskGB      float64           `json:"disk_gb"`       // 磁盘(GB)

	// 位置信息
	Datacenter  string            `json:"datacenter"`    // 数据中心
	Rack        string            `json:"rack"`          // 机架
	Zone        string            `json:"zone"`          // 可用区

	// 业务属性标签
	Labels      map[string]string `json:"labels"`        // 业务标签
	Business    string            `json:"business"`      // 业务线
	Department  string            `json:"department"`    // 部门
	Owner       string            `json:"owner"`         // 负责人
	Team        string            `json:"team"`          // 团队
	Environment string            `json:"environment"`   // 环境: production/staging/development/test
	ServiceLine string            `json:"service_line"`  // 服务线
	Project     string            `json:"project"`       // 项目
	Cluster     string            `json:"cluster"`       // 集群
	Tier        string            `json:"tier"`          // 分层: frontend/backend/middleware/data/cache

	// 关联关系
	ParentID    string            `json:"parent_id"`     // 父资产ID
	GroupIDs    []string          `json:"group_ids"`     // 所属分组
	AppIDs      []string          `json:"app_ids"`       // 关联应用

	// 时间戳
	CreatedAt   time.Time         `json:"created_at"`    // 创建时间
	UpdatedAt   time.Time         `json:"updated_at"`    // 更新时间
	SyncedAt    time.Time         `json:"synced_at"`     // 同步时间
}

// CMDBAssetStatus 资产状态
type CMDBAssetStatus string

const (
	CMDBStatusOnline    CMDBAssetStatus = "online"     // 在线
	CMDBStatusOffline   CMDBAssetStatus = "offline"    // 离线
	CMDBStatusMaintenance CMDBAssetStatus = "maintenance" // 维护中
	CMDBStatusDecommissioned CMDBAssetStatus = "decommissioned" // 已下线
	CMDBStatusUnknown   CMDBAssetStatus = "unknown"    // 未知
)

// CMDBLifecycle 资产生命周期
type CMDBLifecycle string

const (
	CMDBLifecyclePlanning    CMDBLifecycle = "planning"     // 规划中
	CMDBLifecycleProvisioning CMDBLifecycle = "provisioning" // 部署中
	CMDBLifecycleRunning     CMDBLifecycle = "running"      // 运行中
	CMDBLifecycleRetired     CMDBLifecycle = "retired"      // 已退役
)

// CMDBGroup CMDB资产分组
type CMDBGroup struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	ParentID    string            `json:"parent_id"`
	GroupType   string            `json:"group_type"`   // business/department/environment/custom
	Labels      map[string]string `json:"labels"`
	AssetCount  int               `json:"asset_count"`
}

// CMDBApp CMDB应用
type CMDBApp struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	AppType     string            `json:"app_type"`     // web/api/microservice/job
	Owner       string            `json:"owner"`
	Team        string            `json:"team"`
	Labels      map[string]string `json:"labels"`
	AssetIDs    []string          `json:"asset_ids"`
}

// SyncConfig 同步配置
type SyncConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	SyncInterval      time.Duration `yaml:"sync_interval" json:"sync_interval"`
	FullSyncOnStart   bool          `yaml:"full_sync_on_start" json:"full_sync_on_start"`
	IncrementalSync   bool          `yaml:"incremental_sync" json:"incremental_sync"`
	EnableLabelSync   bool          `yaml:"enable_label_sync" json:"enable_label_sync"`
	EnableConfigSync  bool          `yaml:"enable_config_sync" json:"enable_config_sync"`
	EnableRelationSync bool         `yaml:"enable_relation_sync" json:"enable_relation_sync"`
	ConflictPolicy    string        `yaml:"conflict_policy" json:"conflict_policy"` // cmdb_wins/agent_wins/merge
	MaxRetryCount     int           `yaml:"max_retry_count" json:"max_retry_count"`
	RetryInterval     time.Duration `yaml:"retry_interval" json:"retry_interval"`
	Timeout           time.Duration `yaml:"timeout" json:"timeout"`
	BatchSize         int           `yaml:"batch_size" json:"batch_size"`
}

// SyncResult 同步结果
type SyncResult struct {
	SyncID         string        `json:"sync_id"`
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time"`
	Duration       time.Duration `json:"duration"`
	SyncType       string        `json:"sync_type"`       // full/incremental
	TotalFetched   int           `json:"total_fetched"`
	TotalCreated   int           `json:"total_created"`
	TotalUpdated   int           `json:"total_updated"`
	TotalDeleted   int           `json:"total_deleted"`
	TotalUnchanged int           `json:"total_unchanged"`
	TotalErrors    int           `json:"total_errors"`
	Changes        []AssetChange `json:"changes"`
	Error          string        `json:"error,omitempty"`
}

// AssetChange 资产变更记录
type AssetChange struct {
	AssetID   string            `json:"asset_id"`
	AssetName string            `json:"asset_name"`
	ChangeType ChangeType       `json:"change_type"`
	Fields    map[string]FieldChange `json:"fields"`
	Timestamp time.Time         `json:"timestamp"`
}

// ChangeType 变更类型
type ChangeType string

const (
	ChangeTypeCreated  ChangeType = "created"
	ChangeTypeUpdated  ChangeType = "updated"
	ChangeTypeDeleted  ChangeType = "deleted"
	ChangeTypeLabelChanged ChangeType = "label_changed"
	ChangeTypeConfigChanged ChangeType = "config_changed"
	ChangeTypeStatusChanged ChangeType = "status_changed"
)

// FieldChange 字段变更
type FieldChange struct {
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}

// SyncStats 同步统计
type SyncStats struct {
	LastSyncTime     time.Time `json:"last_sync_time"`
	LastSyncType     string    `json:"last_sync_type"`
	LastSyncResult   string    `json:"last_sync_result"` // success/partial/failed
	TotalSyncs       int       `json:"total_syncs"`
	TotalAssets      int       `json:"total_assets"`
	TotalLabels      int       `json:"total_labels"`
	ConsecutiveFails int       `json:"consecutive_fails"`
}

// CMDBSourceConfig CMDB数据源配置
type CMDBSourceConfig struct {
	Type       string `yaml:"type" json:"type"`             // http/api/ldap/custom
	Endpoint   string `yaml:"endpoint" json:"endpoint"`     // CMDB服务地址
	AuthType   string `yaml:"auth_type" json:"auth_type"`   // none/basic/bearer/token/apikey
	AuthToken  string `yaml:"auth_token" json:"auth_token"`
	APIKey     string `yaml:"api_key" json:"api_key"`
	Username   string `yaml:"username" json:"username"`
	Password   string `yaml:"password" json:"password"`
	
	// 查询配置
	QueryPath      string `yaml:"query_path" json:"query_path"`           // 资产查询路径
	LabelPath      string `yaml:"label_path" json:"label_path"`           // 标签查询路径
	GroupPath      string `yaml:"group_path" json:"group_path"`           // 分组查询路径
	AppPath        string `yaml:"app_path" json:"app_path"`               // 应用查询路径
	IncrementalPath string `yaml:"incremental_path" json:"incremental_path"` // 增量查询路径
	
	// 过滤条件
	AssetTypeFilter []string `yaml:"asset_type_filter" json:"asset_type_filter"`
	StatusFilter    []string `yaml:"status_filter" json:"status_filter"`
	LabelFilter     map[string]string `yaml:"label_filter" json:"label_filter"`
	
	// 分页
	DefaultPageSize int `yaml:"default_page_size" json:"default_page_size"`
	MaxPageSize     int `yaml:"max_page_size" json:"max_page_size"`
}

// LabelUpdate 标签更新事件
type LabelUpdate struct {
	AssetID    string            `json:"asset_id"`
	Labels     map[string]string `json:"labels"`
	Added      []string          `json:"added"`
	Removed    []string          `json:"removed"`
	Modified   map[string]string `json:"modified"`
	Timestamp  time.Time         `json:"timestamp"`
}

// ConfigUpdate 配置更新事件
type ConfigUpdate struct {
	AssetID    string            `json:"asset_id"`
	Fields     map[string]interface{} `json:"fields"`
	Timestamp  time.Time         `json:"timestamp"`
}
