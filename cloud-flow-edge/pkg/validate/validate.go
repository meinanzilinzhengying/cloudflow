// Package validate 提供 gRPC 请求参数校验
// 防止恶意探针发送超大/非法数据耗尽资源
package validate

import (
	"fmt"

	edge "cloud-flow/proto"
)

const (
	maxProbeIDLen     = 128
	maxHostnameLen    = 256
	maxIPAddrLen      = 64
	maxVersionLen     = 32
	maxProtocolLen    = 16
	maxTagKeyLen      = 128
	maxTagValueLen    = 512
	maxTagsCount      = 50
	maxMetricsPerBatch = 500
	maxSpansPerBatch  = 200
	maxProfilesPerBatch = 200
)

// RegisterProbeRequest 校验探针注册请求
func RegisterProbeRequest(req *edge.RegisterProbeRequest) error {
	if req.GetProbeId() == "" {
		return fmt.Errorf("probe_id 不能为空")
	}
	if len(req.GetProbeId()) > maxProbeIDLen {
		return fmt.Errorf("probe_id 长度超限: %d > %d", len(req.GetProbeId()), maxProbeIDLen)
	}
	if len(req.GetHostname()) > maxHostnameLen {
		return fmt.Errorf("hostname 长度超限: %d > %d", len(req.GetHostname()), maxHostnameLen)
	}
	if len(req.GetHostIp()) > maxIPAddrLen {
		return fmt.Errorf("host_ip 长度超限: %d > %d", len(req.GetHostIp()), maxIPAddrLen)
	}
	if len(req.GetVersion()) > maxVersionLen {
		return fmt.Errorf("version 长度超限: %d > %d", len(req.GetVersion()), maxVersionLen)
	}
	return nil
}

// HeartbeatRequest 校验心跳请求
func HeartbeatRequest(req *edge.HeartbeatRequest) error {
	if req.GetProbeId() == "" {
		return fmt.Errorf("probe_id 不能为空")
	}
	if len(req.GetProbeId()) > maxProbeIDLen {
		return fmt.Errorf("probe_id 长度超限: %d > %d", len(req.GetProbeId()), maxProbeIDLen)
	}
	return nil
}

// MetricsBatch 校验指标数据批量
func MetricsBatch(batch *edge.MetricsBatch) error {
	if batch.GetProbeId() == "" {
		return fmt.Errorf("probe_id 不能为空")
	}
	if len(batch.GetProbeId()) > maxProbeIDLen {
		return fmt.Errorf("probe_id 长度超限")
	}
	metrics := batch.GetMetrics()
	if len(metrics) == 0 {
		return fmt.Errorf("metrics 不能为空")
	}
	if len(metrics) > maxMetricsPerBatch {
		return fmt.Errorf("metrics 数量超限: %d > %d", len(metrics), maxMetricsPerBatch)
	}
	for i, m := range metrics {
		if err := metricData(m, i); err != nil {
			return err
		}
	}
	return nil
}

func metricData(m *edge.MetricData, idx int) error {
	if len(m.GetProtocol()) > maxProtocolLen {
		return fmt.Errorf("metrics[%d].protocol 长度超限", idx)
	}
	if len(m.GetSrcIp()) > maxIPAddrLen || len(m.GetDstIp()) > maxIPAddrLen {
		return fmt.Errorf("metrics[%d].ip 长度超限", idx)
	}
	if err := tags(m.GetTags(), fmt.Sprintf("metrics[%d].tags", idx)); err != nil {
		return err
	}
	return nil
}

// TraceBatch 校验链路追踪数据批量
func TraceBatch(batch *edge.TraceBatch) error {
	if batch.GetProbeId() == "" {
		return fmt.Errorf("probe_id 不能为空")
	}
	if len(batch.GetProbeId()) > maxProbeIDLen {
		return fmt.Errorf("probe_id 长度超限")
	}
	spans := batch.GetSpans()
	if len(spans) == 0 {
		return fmt.Errorf("spans 不能为空")
	}
	if len(spans) > maxSpansPerBatch {
		return fmt.Errorf("spans 数量超限: %d > %d", len(spans), maxSpansPerBatch)
	}
	for i, sp := range spans {
		if len(sp.GetTraceId()) > maxProbeIDLen {
			return fmt.Errorf("spans[%d].trace_id 长度超限", i)
		}
		if len(sp.GetService()) > maxHostnameLen {
			return fmt.Errorf("spans[%d].service 长度超限", i)
		}
		if err := tags(sp.GetTags(), fmt.Sprintf("spans[%d].tags", i)); err != nil {
			return err
		}
	}
	return nil
}

// ProfilingBatch 校验性能分析数据批量
func ProfilingBatch(batch *edge.ProfilingBatch) error {
	if batch.GetProbeId() == "" {
		return fmt.Errorf("probe_id 不能为空")
	}
	if len(batch.GetProbeId()) > maxProbeIDLen {
		return fmt.Errorf("probe_id 长度超限")
	}
	profiles := batch.GetProfiles()
	if len(profiles) == 0 {
		return fmt.Errorf("profiles 不能为空")
	}
	if len(profiles) > maxProfilesPerBatch {
		return fmt.Errorf("profiles 数量超限: %d > %d", len(profiles), maxProfilesPerBatch)
	}
	for i, p := range profiles {
		if len(p.GetType()) > maxProtocolLen {
			return fmt.Errorf("profiles[%d].type 长度超限", i)
		}
		// stack 字段可能很大，限制为 64KB
		if len(p.GetStack()) > 65536 {
			return fmt.Errorf("profiles[%d].stack 长度超限: %d > 65536", i, len(p.GetStack()))
		}
		if err := tags(p.GetLabels(), fmt.Sprintf("profiles[%d].labels", i)); err != nil {
			return err
		}
	}
	return nil
}

func tags(m map[string]string, prefix string) error {
	if len(m) > maxTagsCount {
		return fmt.Errorf("%s 数量超限: %d > %d", prefix, len(m), maxTagsCount)
	}
	for k, v := range m {
		if len(k) > maxTagKeyLen {
			return fmt.Errorf("%s key '%s...' 长度超限", prefix, k[:20])
		}
		if len(v) > maxTagValueLen {
			return fmt.Errorf("%s[%s] value 长度超限: %d > %d", prefix, k, len(v), maxTagValueLen)
		}
	}
	return nil
}
