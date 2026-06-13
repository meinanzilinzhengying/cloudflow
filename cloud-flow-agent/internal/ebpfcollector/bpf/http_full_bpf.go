// http_full_bpf.go - Go绑定用于加载HTTP全字段解析eBPF程序
//go:build linux
// +build linux

package bpf

import (
	"bytes"
	"embed"
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

//go:embed http_full.bpf.o
var httpFullBpfFS embed.FS

// HTTPFullObjects 包含所有HTTP全字段解析eBPF对象
type HTTPFullObjects struct {
	// Programs
	TraceHttpSendmsg   *ebpf.Program `ebpf:"trace_http_sendmsg"`
	TraceHttpRecvmsg   *ebpf.Program `ebpf:"trace_http_recvmsg"`
	TraceHttpTcpClose  *ebpf.Program `ebpf:"trace_http_tcp_close"`

	// Maps
	HttpRequestsMap     *ebpf.Map `ebpf:"http_requests"`
	HttpTransactionsMap *ebpf.Map `ebpf:"http_transactions"`
	HttpEventsMap       *ebpf.Map `ebpf:"http_events"`
	RequestIdGenMap     *ebpf.Map `ebpf:"request_id_gen"`
	HttpStatsCounterMap *ebpf.Map `ebpf:"http_stats_counter"`
}

// HTTPMethod HTTP请求方法类型
type HTTPMethod uint8

const (
	HTTP_GET     HTTPMethod = 1
	HTTP_POST    HTTPMethod = 2
	HTTP_PUT     HTTPMethod = 3
	HTTP_DELETE  HTTPMethod = 4
	HTTP_HEAD    HTTPMethod = 5
	HTTP_OPTIONS HTTPMethod = 6
	HTTP_PATCH   HTTPMethod = 7
	HTTP_CONNECT HTTPMethod = 8
	HTTP_TRACE   HTTPMethod = 9
	HTTP_UNKNOWN HTTPMethod = 10
)

// HTTPConnKey HTTP连接标识
type HTTPConnKey struct {
	Saddr uint32
	Daddr uint32
	Sport uint16
	Dport uint16
	Pid   uint32
	Netns uint32
}

// HTTPRequestFull HTTP请求完整信息
type HTTPRequestFull struct {
	// 基本信息
	TimestampNs uint64
	RequestId   uint64

	// 方法
	Method HTTPMethod

	// 路径
	Path    [256]byte
	PathLen uint16

	// Host头
	Host    [128]byte
	HostLen uint16

	// Cookie
	Cookie    [512]byte
	CookieLen uint16

	// User-Agent
	UserAgent [256]byte
	UaLen     uint16

	// Referer
	Referer    [256]byte
	RefererLen uint16

	// Content-Type
	ContentType    [64]byte
	ContentTypeLen uint16

	// 请求体大小
	ContentLength uint32

	// 连接信息
	IsHttps     uint8
	HttpVersion uint8
	Padding     [2]byte

	// 真实客户端IP（从代理头部提取）
	XForwardedFor    [64]byte
	XffLen           uint8
	XRealIP          [32]byte
	XriLen           uint8
}

// HTTPResponseFull HTTP响应完整信息
type HTTPResponseFull struct {
	// 基本信息
	TimestampNs uint64
	RequestId   uint64
	LatencyNs   uint64

	// 状态码
	StatusCode uint16

	// 状态描述
	StatusText    [32]byte
	StatusTextLen uint8

	// Content-Type
	ContentType    [64]byte
	ContentTypeLen uint16

	// Content-Length
	ContentLength uint32

	// Server
	Server    [64]byte
	ServerLen uint16

	// Set-Cookie
	SetCookie    [512]byte
	SetCookieLen uint16

	// 响应特征
	IsChunked uint8
	IsGzipped uint8
	IsCached  uint8
	Padding   uint8
}

// HTTPTransaction HTTP事务(请求+响应)
type HTTPTransaction struct {
	Request  HTTPRequestFull
	Response HTTPResponseFull
	Complete uint8
	Padding  [7]byte
}

// HTTPStats HTTP统计信息
type HTTPStats struct {
	TotalRequests   uint64
	TotalResponses  uint64
	SuccessCount    uint64
	ErrorCount      uint64
	AvgLatencyNs    uint64
	MaxLatencyNs    uint64
	MinLatencyNs    uint64
}

// LoadHTTPFull 加载HTTP全字段解析eBPF程序
func LoadHTTPFull(opts *ebpf.CollectionOptions) (*HTTPFullObjects, error) {
	// 读取编译后的eBPF对象文件
	objData, err := httpFullBpfFS.ReadFile("http_full.bpf.o")
	if err != nil {
		return nil, fmt.Errorf("读取HTTP全字段解析eBPF对象失败: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(objData))
	if err != nil {
		return nil, fmt.Errorf("加载HTTP全字段解析eBPF规格失败: %w", err)
	}

	var objs HTTPFullObjects
	if err := spec.LoadAndAssign(&objs, opts); err != nil {
		return nil, fmt.Errorf("加载HTTP全字段解析eBPF对象失败: %w", err)
	}

	return &objs, nil
}

// Close 关闭所有eBPF对象
func (o *HTTPFullObjects) Close() error {
	var errs []error

	if o.TraceHttpSendmsg != nil {
		if err := o.TraceHttpSendmsg.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceHttpRecvmsg != nil {
		if err := o.TraceHttpRecvmsg.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceHttpTcpClose != nil {
		if err := o.TraceHttpTcpClose.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if o.HttpRequestsMap != nil {
		if err := o.HttpRequestsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.HttpTransactionsMap != nil {
		if err := o.HttpTransactionsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.HttpEventsMap != nil {
		if err := o.HttpEventsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.RequestIdGenMap != nil {
		if err := o.RequestIdGenMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.HttpStatsCounterMap != nil {
		if err := o.HttpStatsCounterMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭HTTP全字段解析eBPF对象时发生错误: %v", errs)
	}
	return nil
}

// AttachHTTPFullProbes 附加HTTP全字段解析kprobe探针
func AttachHTTPFullProbes(objs *HTTPFullObjects) ([]link.Link, error) {
	var links []link.Link

	// 附加tcp_sendmsg探针
	if objs.TraceHttpSendmsg != nil {
		l, err := link.Kprobe("tcp_sendmsg", objs.TraceHttpSendmsg, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_sendmsg kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_recvmsg探针
	if objs.TraceHttpRecvmsg != nil {
		l, err := link.Kprobe("tcp_recvmsg", objs.TraceHttpRecvmsg, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_recvmsg kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_close探针
	if objs.TraceHttpTcpClose != nil {
		l, err := link.Kprobe("tcp_close", objs.TraceHttpTcpClose, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_close kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	return links, nil
}

// GetHTTPTransaction 从事件队列中获取一个HTTP事务
func (o *HTTPFullObjects) GetHTTPTransaction() (*HTTPTransaction, error) {
	var txn HTTPTransaction
	err := o.HttpEventsMap.LookupAndDelete(nil, &txn)
	if err != nil {
		return nil, fmt.Errorf("获取HTTP事务失败: %w", err)
	}
	return &txn, nil
}

// IterateHTTPTransactions 遍历HTTP事务映射
func (o *HTTPFullObjects) IterateHTTPTransactions() *ebpf.MapIterator {
	return o.HttpTransactionsMap.Iterate()
}

// IterateHTTPRequests 遍历HTTP请求映射
func (o *HTTPFullObjects) IterateHTTPRequests() *ebpf.MapIterator {
	return o.HttpRequestsMap.Iterate()
}

// GetHTTPStats 获取HTTP统计信息
func (o *HTTPFullObjects) GetHTTPStats() (*HTTPStats, error) {
	var key uint32 = 0
	var stats HTTPStats

	err := o.HttpStatsCounterMap.Lookup(&key, &stats)
	if err != nil {
		return nil, fmt.Errorf("获取HTTP统计信息失败: %w", err)
	}

	return &stats, nil
}

// ClearHTTPStats 清零HTTP统计信息
func (o *HTTPFullObjects) ClearHTTPStats() error {
	var key uint32 = 0
	var empty HTTPStats

	return o.HttpStatsCounterMap.Put(&key, &empty)
}

// GetMethodName 获取HTTP方法名称
func (m HTTPMethod) GetMethodName() string {
	switch m {
	case HTTP_GET:
		return "GET"
	case HTTP_POST:
		return "POST"
	case HTTP_PUT:
		return "PUT"
	case HTTP_DELETE:
		return "DELETE"
	case HTTP_HEAD:
		return "HEAD"
	case HTTP_OPTIONS:
		return "OPTIONS"
	case HTTP_PATCH:
		return "PATCH"
	case HTTP_CONNECT:
		return "CONNECT"
	case HTTP_TRACE:
		return "TRACE"
	default:
		return "UNKNOWN"
	}
}

// GetPath 获取请求路径字符串
func (r *HTTPRequestFull) GetPath() string {
	if r.PathLen == 0 || int(r.PathLen) > len(r.Path) {
		return ""
	}
	return string(r.Path[:r.PathLen])
}

// GetHost 获取Host头字符串
func (r *HTTPRequestFull) GetHost() string {
	if r.HostLen == 0 || int(r.HostLen) > len(r.Host) {
		return ""
	}
	return string(r.Host[:r.HostLen])
}

// GetUserAgent 获取User-Agent字符串
func (r *HTTPRequestFull) GetUserAgent() string {
	if r.UaLen == 0 || int(r.UaLen) > len(r.UserAgent) {
		return ""
	}
	return string(r.UserAgent[:r.UaLen])
}

// GetCookie 获取Cookie字符串
func (r *HTTPRequestFull) GetCookie() string {
	if r.CookieLen == 0 || int(r.CookieLen) > len(r.Cookie) {
		return ""
	}
	return string(r.Cookie[:r.CookieLen])
}

// GetReferer 获取Referer字符串
func (r *HTTPRequestFull) GetReferer() string {
	if r.RefererLen == 0 || int(r.RefererLen) > len(r.Referer) {
		return ""
	}
	return string(r.Referer[:r.RefererLen])
}

// GetContentType 获取Content-Type字符串
func (r *HTTPRequestFull) GetContentType() string {
	if r.ContentTypeLen == 0 || int(r.ContentTypeLen) > len(r.ContentType) {
		return ""
	}
	return string(r.ContentType[:r.ContentTypeLen])
}

// GetServer 获取Server字符串
func (r *HTTPResponseFull) GetServer() string {
	if r.ServerLen == 0 || int(r.ServerLen) > len(r.Server) {
		return ""
	}
	return string(r.Server[:r.ServerLen])
}

// GetResponseContentType 获取响应Content-Type字符串
func (r *HTTPResponseFull) GetResponseContentType() string {
	if r.ContentTypeLen == 0 || int(r.ContentTypeLen) > len(r.ContentType) {
		return ""
	}
	return string(r.ContentType[:r.ContentTypeLen])
}

// GetSetCookie 获取Set-Cookie字符串
func (r *HTTPResponseFull) GetSetCookie() string {
	if r.SetCookieLen == 0 || int(r.SetCookieLen) > len(r.SetCookie) {
		return ""
	}
	return string(r.SetCookie[:r.SetCookieLen])
}

// GetStatusText 获取状态描述字符串
func (r *HTTPResponseFull) GetStatusText() string {
	if r.StatusTextLen == 0 || int(r.StatusTextLen) > len(r.StatusText) {
		return ""
	}
	return string(r.StatusText[:r.StatusTextLen])
}

// GetHttpVersion 获取HTTP版本字符串
func (r *HTTPRequestFull) GetHttpVersion() string {
	switch r.HttpVersion {
	case 0:
		return "HTTP/1.0"
	case 1:
		return "HTTP/1.1"
	case 2:
		return "HTTP/2"
	default:
		return "UNKNOWN"
	}
}

// GetXForwardedFor 获取X-Forwarded-For头部值
func (r *HTTPRequestFull) GetXForwardedFor() string {
	if r.XffLen == 0 {
		return ""
	}
	return string(r.XForwardedFor[:r.XffLen])
}

// GetXRealIP 获取X-Real-IP头部值
func (r *HTTPRequestFull) GetXRealIP() string {
	if r.XriLen == 0 {
		return ""
	}
	return string(r.XRealIP[:r.XriLen])
}
