// http_metrics_bpf.go - Go绑定用于加载HTTP指标eBPF程序
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

//go:embed http_metrics.bpf.o
var httpMetricsBpfFS embed.FS

// HTTPMetricsObjects 包含所有HTTP指标eBPF对象
type HTTPMetricsObjects struct {
	// Programs
	TraceTcpSendmsg   *ebpf.Program `ebpf:"trace_tcp_sendmsg"`
	TraceTcpRecvmsg   *ebpf.Program `ebpf:"trace_tcp_recvmsg"`
	TraceTcpDataQueue *ebpf.Program `ebpf:"trace_tcp_data_queue"`
	TraceHttpTcpClose *ebpf.Program `ebpf:"trace_http_tcp_close"`

	// Maps
	HttpRequestMap       *ebpf.Map `ebpf:"http_request_map"`
	HttpStatsMap         *ebpf.Map `ebpf:"http_stats_map"`
	GlobalHttpMetricsMap *ebpf.Map `ebpf:"global_http_metrics_map"`
	ErrorStatsMap        *ebpf.Map `ebpf:"error_stats_map"`
}

// HttpFlowKey HTTP流标识
type HttpFlowKey struct {
	Saddr uint32
	Daddr uint32
	Sport uint16
	Dport uint16
	Pid   uint32
}

// HttpRequest HTTP请求跟踪
type HttpRequest struct {
	RequestNs     uint64
	ResponseNs    uint64
	LatencyNs     uint64
	StatusCode    uint16
	HasResponse   uint8
	IsError       uint8
	RequestBytes  uint64
	ResponseBytes uint64
}

// HttpStats HTTP统计
type HttpStats struct {
	RequestCount       uint64
	ResponseCount      uint64
	SuccessCount       uint64
	ErrorCount         uint64
	TotalLatencyNs     uint64
	AvgLatencyNs       uint64
	MaxLatencyNs       uint64
	MinLatencyNs       uint64
	TotalRequestBytes  uint64
	TotalResponseBytes uint64
	LastUpdate         uint64
}

// GlobalHttpMetrics 全局HTTP指标
type GlobalHttpMetrics struct {
	TotalRequests     uint64
	TotalResponses    uint64
	SuccessResponses  uint64
	ErrorResponses    uint64
	AvgLatencyNs      uint64
	MaxLatencyNs      uint64
	MinLatencyNs      uint64
	LatencySamples    uint64
}

// LoadHTTPMetrics 加载HTTP指标eBPF程序
func LoadHTTPMetrics(opts *ebpf.CollectionOptions) (*HTTPMetricsObjects, error) {
	// 读取编译后的eBPF对象文件
	objData, err := httpMetricsBpfFS.ReadFile("http_metrics.bpf.o")
	if err != nil {
		return nil, fmt.Errorf("读取HTTP指标eBPF对象失败: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(objData))
	if err != nil {
		return nil, fmt.Errorf("加载HTTP指标eBPF规格失败: %w", err)
	}

	var objs HTTPMetricsObjects
	if err := spec.LoadAndAssign(&objs, opts); err != nil {
		return nil, fmt.Errorf("加载HTTP指标eBPF对象失败: %w", err)
	}

	return &objs, nil
}

// Close 关闭所有eBPF对象
func (o *HTTPMetricsObjects) Close() error {
	var errs []error

	if o.TraceTcpSendmsg != nil {
		if err := o.TraceTcpSendmsg.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceTcpRecvmsg != nil {
		if err := o.TraceTcpRecvmsg.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceTcpDataQueue != nil {
		if err := o.TraceTcpDataQueue.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceHttpTcpClose != nil {
		if err := o.TraceHttpTcpClose.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if o.HttpRequestMap != nil {
		if err := o.HttpRequestMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.HttpStatsMap != nil {
		if err := o.HttpStatsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.GlobalHttpMetricsMap != nil {
		if err := o.GlobalHttpMetricsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.ErrorStatsMap != nil {
		if err := o.ErrorStatsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭HTTP指标eBPF对象时发生错误: %v", errs)
	}
	return nil
}

// AttachHTTPMetricsProbes 附加HTTP指标kprobe探针
func AttachHTTPMetricsProbes(objs *HTTPMetricsObjects) ([]link.Link, error) {
	var links []link.Link

	// 附加tcp_sendmsg探针
	if objs.TraceTcpSendmsg != nil {
		l, err := link.Kprobe("tcp_sendmsg", objs.TraceTcpSendmsg, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_sendmsg kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_recvmsg探针
	if objs.TraceTcpRecvmsg != nil {
		l, err := link.Kprobe("tcp_recvmsg", objs.TraceTcpRecvmsg, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_recvmsg kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_data_queue探针
	if objs.TraceTcpDataQueue != nil {
		l, err := link.Kprobe("tcp_data_queue", objs.TraceTcpDataQueue, nil)
		if err != nil {
			// tcp_data_queue可能在某些内核中不可用
			_ = err
		} else {
			links = append(links, l)
		}
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

// GetGlobalHTTPMetrics 获取全局HTTP指标
func (o *HTTPMetricsObjects) GetGlobalHTTPMetrics() (*GlobalHttpMetrics, error) {
	var key uint32 = 0
	var metrics GlobalHttpMetrics

	err := o.GlobalHttpMetricsMap.Lookup(&key, &metrics)
	if err != nil {
		return nil, fmt.Errorf("获取全局HTTP指标失败: %w", err)
	}

	return &metrics, nil
}

// IterateHttpStats 遍历HTTP统计映射
func (o *HTTPMetricsObjects) IterateHttpStats() *ebpf.MapIterator {
	return o.HttpStatsMap.Iterate()
}

// IterateErrorStats 遍历异常统计映射
func (o *HTTPMetricsObjects) IterateErrorStats() *ebpf.MapIterator {
	return o.ErrorStatsMap.Iterate()
}

// ClearGlobalMetrics 清零全局指标
func (o *HTTPMetricsObjects) ClearGlobalMetrics() error {
	var key uint32 = 0
	var empty GlobalHttpMetrics

	return o.GlobalHttpMetricsMap.Put(&key, &empty)
}
