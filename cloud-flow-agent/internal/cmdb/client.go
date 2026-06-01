package cmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CMDBClient CMDB客户端接口
type CMDBClient interface {
	// FetchAssets 获取所有资产
	FetchAssets(ctx context.Context, filter *AssetFilter) ([]CMDBAsset, error)

	// FetchAsset 获取单个资产
	FetchAsset(ctx context.Context, assetID string) (*CMDBAsset, error)

	// FetchLabels 获取资产标签
	FetchLabels(ctx context.Context, assetIDs []string) (map[string]map[string]string, error)

	// FetchGroups 获取资产分组
	FetchGroups(ctx context.Context) ([]CMDBGroup, error)

	// FetchApps 获取应用列表
	FetchApps(ctx context.Context) ([]CMDBApp, error)

	// FetchIncremental 获取增量变更
	FetchIncremental(ctx context.Context, since time.Time) ([]CMDBAsset, error)

	// Ping 健康检查
	Ping(ctx context.Context) error

	// Close 关闭连接
	Close() error
}

// AssetFilter 资产过滤条件
type AssetFilter struct {
	AssetTypes  []string          `json:"asset_types"`
	Statuses    []string          `json:"statuses"`
	Labels      map[string]string `json:"labels"`
	Page        int               `json:"page"`
	PageSize    int               `json:"page_size"`
}

// HTTPClient HTTP实现的CMDB客户端
type HTTPClient struct {
	config     *CMDBSourceConfig
	httpClient *http.Client
	headers    map[string]string
}

// NewHTTPClient 创建HTTP CMDB客户端
func NewHTTPClient(config *CMDBSourceConfig) *HTTPClient {
	timeout := 30 * time.Second
	if config.Timeout > 0 {
		timeout = config.Timeout
	}

	client := &HTTPClient{
		config: config,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		headers: make(map[string]string),
	}

	// 设置认证头
	client.setupAuth()

	return client
}

// setupAuth 设置认证
func (c *HTTPClient) setupAuth() {
	switch c.config.AuthType {
	case "bearer", "token":
		if c.config.AuthToken != "" {
			c.headers["Authorization"] = "Bearer " + c.config.AuthToken
		}
	case "apikey":
		if c.config.APIKey != "" {
			c.headers["X-API-Key"] = c.config.APIKey
		}
	case "basic":
		if c.config.Username != "" {
			c.headers["Authorization"] = "Basic " + basicAuth(c.config.Username, c.config.Password)
		}
	}

	// 默认Content-Type
	c.headers["Content-Type"] = "application/json"
	c.headers["Accept"] = "application/json"
}

// FetchAssets 获取所有资产
func (c *HTTPClient) FetchAssets(ctx context.Context, filter *AssetFilter) ([]CMDBAsset, error) {
	if filter == nil {
		filter = &AssetFilter{Page: 1, PageSize: c.config.DefaultPageSize}
	}
	if filter.PageSize <= 0 {
		filter.PageSize = c.config.DefaultPageSize
	}
	if filter.PageSize > c.config.MaxPageSize && c.config.MaxPageSize > 0 {
		filter.PageSize = c.config.MaxPageSize
	}

	var allAssets []CMDBAsset
	page := filter.Page

	for {
		filter.Page = page
		assets, total, err := c.fetchAssetsPage(ctx, filter)
		if err != nil {
			return nil, err
		}

		allAssets = append(allAssets, assets...)

		// 检查是否还有下一页
		if len(allAssets) >= total || len(assets) < filter.PageSize {
			break
		}
		page++
	}

	return allAssets, nil
}

// fetchAssetsPage 获取单页资产
func (c *HTTPClient) fetchAssetsPage(ctx context.Context, filter *AssetFilter) ([]CMDBAsset, int, error) {
	path := c.config.QueryPath
	if path == "" {
		path = "/api/v1/assets"
	}

	// 构建查询参数
	params := url.Values{}
	params.Set("page", fmt.Sprintf("%d", filter.Page))
	params.Set("page_size", fmt.Sprintf("%d", filter.PageSize))

	if len(filter.AssetTypes) > 0 {
		params.Set("asset_type", strings.Join(filter.AssetTypes, ","))
	}
	if len(filter.Statuses) > 0 {
		params.Set("status", strings.Join(filter.Statuses, ","))
	}
	for k, v := range filter.Labels {
		params.Set("label_"+k, v)
	}

	// 合并配置中的过滤条件
	if len(c.config.AssetTypeFilter) > 0 {
		params.Set("asset_type", strings.Join(c.config.AssetTypeFilter, ","))
	}
	if len(c.config.StatusFilter) > 0 {
		params.Set("status", strings.Join(c.config.StatusFilter, ","))
	}
	for k, v := range c.config.LabelFilter {
		params.Set("label_"+k, v)
	}

	reqURL := fmt.Sprintf("%s%s?%s", c.config.Endpoint, path, params.Encode())

	body, err := c.doGet(ctx, reqURL)
	if err != nil {
		return nil, 0, fmt.Errorf("获取资产列表失败: %w", err)
	}

	// 解析响应
	var resp struct {
		Code    int          `json:"code"`
		Message string       `json:"message"`
		Data    struct {
			Items    []CMDBAsset `json:"items"`
			Total    int         `json:"total"`
			Page     int         `json:"page"`
			PageSize int         `json:"page_size"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("解析资产响应失败: %w", err)
	}

	if resp.Code != 0 && resp.Code != 200 {
		return nil, 0, fmt.Errorf("CMDB返回错误: code=%d, msg=%s", resp.Code, resp.Message)
	}

	return resp.Data.Items, resp.Data.Total, nil
}

// FetchAsset 获取单个资产
func (c *HTTPClient) FetchAsset(ctx context.Context, assetID string) (*CMDBAsset, error) {
	path := c.config.QueryPath
	if path == "" {
		path = "/api/v1/assets"
	}

	reqURL := fmt.Sprintf("%s%s/%s", c.config.Endpoint, path, assetID)

	body, err := c.doGet(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("获取资产详情失败: %w", err)
	}

	var resp struct {
		Code    int       `json:"code"`
		Message string    `json:"message"`
		Data    CMDBAsset `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析资产详情失败: %w", err)
	}

	if resp.Code != 0 && resp.Code != 200 {
		return nil, fmt.Errorf("CMDB返回错误: code=%d, msg=%s", resp.Code, resp.Message)
	}

	return &resp.Data, nil
}

// FetchLabels 获取资产标签
func (c *HTTPClient) FetchLabels(ctx context.Context, assetIDs []string) (map[string]map[string]string, error) {
	path := c.config.LabelPath
	if path == "" {
		path = "/api/v1/assets/labels"
	}

	reqURL := fmt.Sprintf("%s%s", c.config.Endpoint, path)

	// POST请求发送资产ID列表
	payload := map[string]interface{}{
		"asset_ids": assetIDs,
	}

	body, err := c.doPost(ctx, reqURL, payload)
	if err != nil {
		return nil, fmt.Errorf("获取标签失败: %w", err)
	}

	var resp struct {
		Code    int                        `json:"code"`
		Message string                     `json:"message"`
		Data    map[string]map[string]string `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析标签响应失败: %w", err)
	}

	if resp.Code != 0 && resp.Code != 200 {
		return nil, fmt.Errorf("CMDB返回错误: code=%d, msg=%s", resp.Code, resp.Message)
	}

	if resp.Data == nil {
		return make(map[string]map[string]string), nil
	}

	return resp.Data, nil
}

// FetchGroups 获取资产分组
func (c *HTTPClient) FetchGroups(ctx context.Context) ([]CMDBGroup, error) {
	path := c.config.GroupPath
	if path == "" {
		path = "/api/v1/groups"
	}

	reqURL := fmt.Sprintf("%s%s", c.config.Endpoint, path)

	body, err := c.doGet(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("获取分组失败: %w", err)
	}

	var resp struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    []CMDBGroup `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析分组响应失败: %w", err)
	}

	if resp.Code != 0 && resp.Code != 200 {
		return nil, fmt.Errorf("CMDB返回错误: code=%d, msg=%s", resp.Code, resp.Message)
	}

	return resp.Data, nil
}

// FetchApps 获取应用列表
func (c *HTTPClient) FetchApps(ctx context.Context) ([]CMDBApp, error) {
	path := c.config.AppPath
	if path == "" {
		path = "/api/v1/apps"
	}

	reqURL := fmt.Sprintf("%s%s", c.config.Endpoint, path)

	body, err := c.doGet(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("获取应用列表失败: %w", err)
	}

	var resp struct {
		Code    int       `json:"code"`
		Message string    `json:"message"`
		Data    []CMDBApp `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析应用响应失败: %w", err)
	}

	if resp.Code != 0 && resp.Code != 200 {
		return nil, fmt.Errorf("CMDB返回错误: code=%d, msg=%s", resp.Code, resp.Message)
	}

	return resp.Data, nil
}

// FetchIncremental 获取增量变更
func (c *HTTPClient) FetchIncremental(ctx context.Context, since time.Time) ([]CMDBAsset, error) {
	path := c.config.IncrementalPath
	if path == "" {
		path = "/api/v1/assets/incremental"
	}

	params := url.Values{}
	params.Set("since", since.Format(time.RFC3339))
	params.Set("page_size", fmt.Sprintf("%d", c.config.DefaultPageSize))

	reqURL := fmt.Sprintf("%s%s?%s", c.config.Endpoint, path, params.Encode())

	body, err := c.doGet(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("获取增量变更失败: %w", err)
	}

	var resp struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    []CMDBAsset `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析增量响应失败: %w", err)
	}

	if resp.Code != 0 && resp.Code != 200 {
		return nil, fmt.Errorf("CMDB返回错误: code=%d, msg=%s", resp.Code, resp.Message)
	}

	return resp.Data, nil
}

// Ping 健康检查
func (c *HTTPClient) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/api/v1/health", c.config.Endpoint)

	_, err := c.doGet(ctx, reqURL)
	if err != nil {
		return fmt.Errorf("CMDB健康检查失败: %w", err)
	}

	return nil
}

// Close 关闭连接
func (c *HTTPClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// doGet 发送GET请求
func (c *HTTPClient) doGet(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// doPost 发送POST请求
func (c *HTTPClient) doPost(ctx context.Context, reqURL string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// basicAuth 生成Basic认证字符串
func basicAuth(username, password string) string {
	// 简化实现，生产环境应使用 base64.StdEncoding
	return fmt.Sprintf("%s:%s", username, password)
}
