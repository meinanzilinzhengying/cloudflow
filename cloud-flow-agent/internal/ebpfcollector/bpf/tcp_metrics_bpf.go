// tcp_metrics_bpf.go - Go绑定用于加载TCP指标eBPF程序
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

//go:embed tcp_metrics.bpf.o
var tcpMetricsBpfFS embed.FS

// TCPMetricsObjects 包含所有TCP指标eBPF对象
type TCPMetricsObjects struct {
	// Programs
	TraceTcpV4ConnectEntry  *ebpf.Program `ebpf:"trace_tcp_v4_connect_entry"`
	TraceTcpV6ConnectEntry  *ebpf.Program `ebpf:"trace_tcp_v6_connect_entry"`
	TraceTcpRcvStateProcess *ebpf.Program `ebpf:"trace_tcp_rcv_state_process"`
	TraceTcpRetransmitSkb   *ebpf.Program `ebpf:"trace_tcp_retransmit_skb"`
	TraceTcpAckUpdateWindow *ebpf.Program `ebpf:"trace_tcp_ack_update_window"`
	TraceTcpV4SynRecvSock   *ebpf.Program `ebpf:"trace_tcp_v4_syn_recv_sock"`
	TraceTcpDrop            *ebpf.Program `ebpf:"trace_tcp_drop"`
	TraceTcpClose           *ebpf.Program `ebpf:"trace_tcp_close"`
	TraceTcpSendmsg         *ebpf.Program `ebpf:"trace_tcp_sendmsg"`
	TraceTcpRecvmsg         *ebpf.Program `ebpf:"trace_tcp_recvmsg"`

	// Maps
	TcpFlowStatsMap      *ebpf.Map `ebpf:"tcp_flow_stats_map"`
	GlobalTcpMetricsMap  *ebpf.Map `ebpf:"global_tcp_metrics_map"`
}

// TcpConnKey TCP连接标识
type TcpConnKey struct {
	Saddr uint32
	Daddr uint32
	Sport uint16
	Dport uint16
	Pid   uint32
	Comm  [16]byte
}

// TcpFlowStats TCP流统一统计指标（镜像C端 tcp_flow_stats）
type TcpFlowStats struct {
	ConnectLatencyNs   uint64
	SynSentNs          uint64
	ConnectComplete    uint8
	Padding1           [7]byte
	RetransCount       uint64
	ZeroWindowCount    uint64
	QueueOverflowCount uint64
	ConnFailCount      uint64
	BytesSent          uint64
	BytesRecv          uint64
	PacketsSent        uint64
	PacketsRecv        uint64
	LastUpdate         uint64
}

// GlobalTcpMetrics 全局TCP指标汇总
type GlobalTcpMetrics struct {
	TotalConnections     uint64
	FailedConnections    uint64
	TotalRetrans         uint64
	ZeroWindowEvents     uint64
	QueueOverflowEvents  uint64
	AvgLatencyNs         uint64
	MaxLatencyNs         uint64
	MinLatencyNs         uint64
	LatencySamples       uint64
}

// LoadTCPMetrics 加载TCP指标eBPF程序
func LoadTCPMetrics(opts *ebpf.CollectionOptions) (*TCPMetricsObjects, error) {
	// 读取编译后的eBPF对象文件
	objData, err := tcpMetricsBpfFS.ReadFile("tcp_metrics.bpf.o")
	if err != nil {
		return nil, fmt.Errorf("读取TCP指标eBPF对象失败: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(objData))
	if err != nil {
		return nil, fmt.Errorf("加载TCP指标eBPF规格失败: %w", err)
	}

	var objs TCPMetricsObjects
	if err := spec.LoadAndAssign(&objs, opts); err != nil {
		return nil, fmt.Errorf("加载TCP指标eBPF对象失败: %w", err)
	}

	return &objs, nil
}

// Close 关闭所有eBPF对象
func (o *TCPMetricsObjects) Close() error {
	var errs []error

	if o.TraceTcpV4ConnectEntry != nil {
		if err := o.TraceTcpV4ConnectEntry.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceTcpV6ConnectEntry != nil {
		if err := o.TraceTcpV6ConnectEntry.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceTcpRcvStateProcess != nil {
		if err := o.TraceTcpRcvStateProcess.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceTcpRetransmitSkb != nil {
		if err := o.TraceTcpRetransmitSkb.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceTcpAckUpdateWindow != nil {
		if err := o.TraceTcpAckUpdateWindow.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceTcpV4SynRecvSock != nil {
		if err := o.TraceTcpV4SynRecvSock.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceTcpDrop != nil {
		if err := o.TraceTcpDrop.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceTcpClose != nil {
		if err := o.TraceTcpClose.Close(); err != nil {
			errs = append(errs, err)
		}
	}
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

	if o.TcpFlowStatsMap != nil {
		if err := o.TcpFlowStatsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.GlobalTcpMetricsMap != nil {
		if err := o.GlobalTcpMetricsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭TCP指标eBPF对象时发生错误: %v", errs)
	}
	return nil
}

// AttachTCPMetricsProbes 附加TCP指标kprobe探针
func AttachTCPMetricsProbes(objs *TCPMetricsObjects) ([]link.Link, error) {
	var links []link.Link

	// 附加tcp_v4_connect入口探针
	if objs.TraceTcpV4ConnectEntry != nil {
		l, err := link.Kprobe("tcp_v4_connect", objs.TraceTcpV4ConnectEntry, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_v4_connect kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_v6_connect入口探针
	if objs.TraceTcpV6ConnectEntry != nil {
		l, err := link.Kprobe("tcp_v6_connect", objs.TraceTcpV6ConnectEntry, nil)
		if err != nil {
			// IPv6可能不支持,不视为致命错误
			_ = err
		}
	}

	// 附加tcp_rcv_state_process探针
	if objs.TraceTcpRcvStateProcess != nil {
		l, err := link.Kprobe("tcp_rcv_state_process", objs.TraceTcpRcvStateProcess, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_rcv_state_process kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_retransmit_skb探针
	if objs.TraceTcpRetransmitSkb != nil {
		l, err := link.Kprobe("tcp_retransmit_skb", objs.TraceTcpRetransmitSkb, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_retransmit_skb kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_ack_update_window探针
	if objs.TraceTcpAckUpdateWindow != nil {
		l, err := link.Kprobe("tcp_ack_update_window", objs.TraceTcpAckUpdateWindow, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_ack_update_window kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_v4_syn_recv_sock探针
	if objs.TraceTcpV4SynRecvSock != nil {
		l, err := link.Kprobe("tcp_v4_syn_recv_sock", objs.TraceTcpV4SynRecvSock, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_v4_syn_recv_sock kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_drop探针
	if objs.TraceTcpDrop != nil {
		l, err := link.Kprobe("tcp_drop", objs.TraceTcpDrop, nil)
		if err != nil {
			// tcp_drop可能在某些内核中不可用
			_ = err
		} else {
			links = append(links, l)
		}
	}

	// 附加tcp_close探针
	if objs.TraceTcpClose != nil {
		l, err := link.Kprobe("tcp_close", objs.TraceTcpClose, nil)
		if err != nil {
			return links, fmt.Errorf("附加tcp_close kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加tcp_sendmsg探针
	if objs.TraceTcpSendmsg != nil {
		l, err := link.Kprobe("tcp_sendmsg", objs.TraceTcpSendmsg, nil)
		if err != nil {
			// tcp_sendmsg可能在某些内核中不可用
			_ = err
		} else {
			links = append(links, l)
		}
	}

	// 附加tcp_recvmsg探针
	if objs.TraceTcpRecvmsg != nil {
		l, err := link.Kprobe("tcp_recvmsg", objs.TraceTcpRecvmsg, nil)
		if err != nil {
			// tcp_recvmsg可能在某些内核中不可用
			_ = err
		} else {
			links = append(links, l)
		}
	}

	return links, nil
}

// GetGlobalTCPMetrics 获取全局TCP指标
func (o *TCPMetricsObjects) GetGlobalTCPMetrics() (*GlobalTcpMetrics, error) {
	var key uint32 = 0
	var metrics GlobalTcpMetrics

	err := o.GlobalTcpMetricsMap.Lookup(&key, &metrics)
	if err != nil {
		return nil, fmt.Errorf("获取全局TCP指标失败: %w", err)
	}

	return &metrics, nil
}

// IterateTcpFlowStats 遍历TCP流统计映射
func (o *TCPMetricsObjects) IterateTcpFlowStats() *ebpf.MapIterator {
	return o.TcpFlowStatsMap.Iterate()
}

// ClearGlobalMetrics 清零全局指标
func (o *TCPMetricsObjects) ClearGlobalMetrics() error {
	var key uint32 = 0
	var empty GlobalTcpMetrics

	return o.GlobalTcpMetricsMap.Put(&key, &empty)
}
