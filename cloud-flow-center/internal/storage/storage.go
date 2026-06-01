// Package storage 提供基于 TiDB 的数据持久化
package storage

import (
	"time"

	edge "cloud-flow/proto"
)

// StorageEngine 统一存储接口
type StorageEngine interface {
	SaveMetrics(probeID string, metrics interface{}) error
	SaveTraces(probeID string, spans interface{}) error
	SaveProfiling(probeID string, profiles interface{}) error
	SaveProbeInfo(edgeNodeID string, data interface{}) error
	QueryMetrics(day string, probeID string, limit int) ([]map[string]interface{}, error)
	QueryMetricsByAlert(day string, metricName string, operator string, threshold float64, limit int) ([]map[string]interface{}, error)
	QueryTraces(day string, probeID string, limit int) ([]map[string]interface{}, error)
	GetRecentMetrics(metricType string, limit int, timeWindow time.Duration) ([]*edge.MetricData, error)
	GetOverview() (map[string]interface{}, error)
	GetNodes() ([]map[string]interface{}, error)
	// 用户管理
	CreateUser(username, password, role string) error
	GetUser(username string) (map[string]interface{}, error)
	UpdateUser(username, password, role string) error
	UpdateUserRole(username, role string) error
	DeleteUser(username string) error
	ListUsers() ([]map[string]interface{}, error)
	VerifyUser(username, password string) (bool, string, error)
	ChangePassword(username, oldPassword, newPassword string) error
	SaveUserPreferences(username string, prefs map[string]interface{}) error
	GetUserPreferences(username string) (map[string]interface{}, error)
	// 业务管理
	ListBusiness(page, pageSize int) ([]map[string]interface{}, int, error)
	CreateBusiness(data map[string]interface{}) error
	GetBusiness(id string) (map[string]interface{}, error)
	UpdateBusiness(id string, data map[string]interface{}) error
	DeleteBusiness(id string) error
	// 服务管理
	ListService(page, pageSize int) ([]map[string]interface{}, int, error)
	CreateService(data map[string]interface{}) error
	GetService(id string) (map[string]interface{}, error)
	UpdateService(id string, data map[string]interface{}) error
	DeleteService(id string) error
	// 采集器管理
	ListCollector(page, pageSize int) ([]map[string]interface{}, int, error)
	CreateCollector(data map[string]interface{}) error
	GetCollector(id string) (map[string]interface{}, error)
	UpdateCollector(id string, data map[string]interface{}) error
	DeleteCollector(id string) error
	// 数据节点管理
	ListDataNode(page, pageSize int) ([]map[string]interface{}, int, error)
	CreateDataNode(data map[string]interface{}) error
	GetDataNode(id string) (map[string]interface{}, error)
	UpdateDataNode(id string, data map[string]interface{}) error
	DeleteDataNode(id string) error
	StartCleanup()
	Stop()
	DB() interface{}
}
