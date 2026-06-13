// dns_full_bpf.go - Go绑定用于加载DNS全字段解析eBPF程序
//go:build linux
// +build linux

package bpf

import (
	"bytes"
	"embed"
	"fmt"
	"net"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

//go:embed dns_full.bpf.o
var dnsFullBpfFS embed.FS

// DNSFullObjects 包含所有DNS全字段解析eBPF对象
type DNSFullObjects struct {
	// Programs
	TraceDNSSendmsg  *ebpf.Program `ebpf:"trace_dns_sendmsg"`
	TraceDNSRecvmsg  *ebpf.Program `ebpf:"trace_dns_recvmsg"`

	// Maps
	DNSQueriesMap *ebpf.Map `ebpf:"dns_queries"`
	DNSEventsMap  *ebpf.Map `ebpf:"dns_events"`
	DNSStatsMap   *ebpf.Map `ebpf:"dns_stats"`
}

// DNS记录类型常量
const (
	DNS_TYPE_A     uint16 = 1
	DNS_TYPE_NS    uint16 = 2
	DNS_TYPE_CNAME uint16 = 5
	DNS_TYPE_SOA   uint16 = 6
	DNS_TYPE_PTR   uint16 = 12
	DNS_TYPE_MX    uint16 = 15
	DNS_TYPE_TXT   uint16 = 16
	DNS_TYPE_AAAA  uint16 = 28
	DNS_TYPE_SRV   uint16 = 33
	DNS_TYPE_ANY   uint16 = 255
)

// DNS响应码常量
const (
	DNS_RCODE_NOERROR  uint8 = 0
	DNS_RCODE_FORMERR  uint8 = 1
	DNS_RCODE_SERVFAIL uint8 = 2
	DNS_RCODE_NXDOMAIN uint8 = 3
	DNS_RCODE_NOTIMP   uint8 = 4
	DNS_RCODE_REFUSED  uint8 = 5
)

// DNSConnKey DNS连接标识
type DNSConnKey struct {
	Saddr         uint32
	Daddr         uint32
	Sport         uint16
	Dport         uint16
	Pid           uint32
	TransactionId uint16
}

// DNSQuestion DNS问题(查询)
type DNSQuestion struct {
	Name    [256]byte
	NameLen uint16
	Qtype   uint16
	Qclass  uint16
}

// DNSRecord DNS资源记录
type DNSRecord struct {
	Name     [256]byte
	NameLen  uint16
	Rtype    uint16
	Rclass   uint16
	TTL      uint32
	Rdlength uint16
	Rdata    [256]byte
	RdataLen uint16
}

// DNSRequestFull DNS请求完整信息
type DNSRequestFull struct {
	TimestampNs          uint64
	TransactionId        uint16
	Flags                uint16
	Opcode               uint16
	RecursionDesired     uint8
	QuestionCount        uint16
	AnswerCount          uint16
	AuthorityCount       uint16
	AdditionalCount      uint16
	Questions            [4]DNSQuestion
	QuestionCountActual  uint16
}

// DNSResponseFull DNS响应完整信息
type DNSResponseFull struct {
	TimestampNs           uint64
	LatencyNs             uint64
	TransactionId         uint16
	Flags                 uint16
	IsResponse            uint8
	Authoritative         uint8
	Truncated             uint8
	RecursionAvailable    uint8
	Rcode                 uint8
	RcodeText             [16]byte
	QuestionCount         uint16
	AnswerCount           uint16
	AuthorityCount        uint16
	AdditionalCount       uint16
	Questions             [4]DNSQuestion
	Answers               [10]DNSRecord
	AnswerCountActual     uint16
	Authorities           [4]DNSRecord
	AuthorityCountActual  uint16
	Additionals           [4]DNSRecord
	AdditionalCountActual uint16
}

// DNSTransaction DNS事务
type DNSTransaction struct {
	Request  DNSRequestFull
	Response DNSResponseFull
	Complete uint8
	Padding  [7]byte
}

// LoadDNSFull 加载DNS全字段解析eBPF程序
func LoadDNSFull(opts *ebpf.CollectionOptions) (*DNSFullObjects, error) {
	// 读取编译后的eBPF对象文件
	objData, err := dnsFullBpfFS.ReadFile("dns_full.bpf.o")
	if err != nil {
		return nil, fmt.Errorf("读取DNS全字段解析eBPF对象失败: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(objData))
	if err != nil {
		return nil, fmt.Errorf("加载DNS全字段解析eBPF规格失败: %w", err)
	}

	var objs DNSFullObjects
	if err := spec.LoadAndAssign(&objs, opts); err != nil {
		return nil, fmt.Errorf("加载DNS全字段解析eBPF对象失败: %w", err)
	}

	return &objs, nil
}

// Close 关闭所有eBPF对象
func (o *DNSFullObjects) Close() error {
	var errs []error

	if o.TraceDNSSendmsg != nil {
		if err := o.TraceDNSSendmsg.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.TraceDNSRecvmsg != nil {
		if err := o.TraceDNSRecvmsg.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if o.DNSQueriesMap != nil {
		if err := o.DNSQueriesMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.DNSEventsMap != nil {
		if err := o.DNSEventsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if o.DNSStatsMap != nil {
		if err := o.DNSStatsMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭DNS全字段解析eBPF对象时发生错误: %v", errs)
	}
	return nil
}

// AttachDNSFullProbes 附加DNS全字段解析kprobe探针
func AttachDNSFullProbes(objs *DNSFullObjects) ([]link.Link, error) {
	var links []link.Link

	// 附加udp_sendmsg探针
	if objs.TraceDNSSendmsg != nil {
		l, err := link.Kprobe("udp_sendmsg", objs.TraceDNSSendmsg, nil)
		if err != nil {
			return links, fmt.Errorf("附加udp_sendmsg kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	// 附加udp_recvmsg探针
	if objs.TraceDNSRecvmsg != nil {
		l, err := link.Kprobe("udp_recvmsg", objs.TraceDNSRecvmsg, nil)
		if err != nil {
			return links, fmt.Errorf("附加udp_recvmsg kprobe失败: %w", err)
		}
		links = append(links, l)
	}

	return links, nil
}

// GetDNSTransaction 从事件队列中获取一个DNS事务
func (o *DNSFullObjects) GetDNSTransaction() (*DNSTransaction, error) {
	var txn DNSTransaction
	err := o.DNSEventsMap.LookupAndDelete(nil, &txn)
	if err != nil {
		return nil, fmt.Errorf("获取DNS事务失败: %w", err)
	}
	return &txn, nil
}

// IterateDNSQueries 遍历DNS查询映射
func (o *DNSFullObjects) IterateDNSQueries() *ebpf.MapIterator {
	return o.DNSQueriesMap.Iterate()
}

// GetDNSStats 获取DNS统计信息
func (o *DNSFullObjects) GetDNSStats(statKey uint32) (uint64, error) {
	var count uint64
	err := o.DNSStatsMap.Lookup(&statKey, &count)
	if err != nil {
		return 0, fmt.Errorf("获取DNS统计信息失败: %w", err)
	}
	return count, nil
}

// GetRcodeName 获取响应码名称
func GetRcodeName(rcode uint8) string {
	switch rcode {
	case DNS_RCODE_NOERROR:
		return "NOERROR"
	case DNS_RCODE_FORMERR:
		return "FORMERR"
	case DNS_RCODE_SERVFAIL:
		return "SERVFAIL"
	case DNS_RCODE_NXDOMAIN:
		return "NXDOMAIN"
	case DNS_RCODE_NOTIMP:
		return "NOTIMP"
	case DNS_RCODE_REFUSED:
		return "REFUSED"
	default:
		return "UNKNOWN"
	}
}

// GetRecordTypeName 获取记录类型名称
func GetRecordTypeName(rtype uint16) string {
	switch rtype {
	case DNS_TYPE_A:
		return "A"
	case DNS_TYPE_NS:
		return "NS"
	case DNS_TYPE_CNAME:
		return "CNAME"
	case DNS_TYPE_SOA:
		return "SOA"
	case DNS_TYPE_PTR:
		return "PTR"
	case DNS_TYPE_MX:
		return "MX"
	case DNS_TYPE_TXT:
		return "TXT"
	case DNS_TYPE_AAAA:
		return "AAAA"
	case DNS_TYPE_SRV:
		return "SRV"
	case DNS_TYPE_ANY:
		return "ANY"
	default:
		return "UNKNOWN"
	}
}

// GetQName 获取查询域名
func (q *DNSQuestion) GetQName() string {
	if q.NameLen == 0 || int(q.NameLen) > len(q.Name) {
		return ""
	}
	return string(q.Name[:q.NameLen])
}

// GetRcodeText 获取响应码文本
func (r *DNSResponseFull) GetRcodeText() string {
	// 查找字符串结束位置
	for i := 0; i < len(r.RcodeText); i++ {
		if r.RcodeText[i] == 0 {
			return string(r.RcodeText[:i])
		}
	}
	return string(r.RcodeText[:])
}

// GetRName 获取记录名称
func (r *DNSRecord) GetRName() string {
	if r.NameLen == 0 || int(r.NameLen) > len(r.Name) {
		return ""
	}
	return string(r.Name[:r.NameLen])
}

// GetRDataIP 获取A记录IP地址
func (r *DNSRecord) GetRDataIP() net.IP {
	if r.Rtype == DNS_TYPE_A && r.RdataLen == 4 {
		return net.IPv4(r.Rdata[0], r.Rdata[1], r.Rdata[2], r.Rdata[3])
	}
	return nil
}

// GetRDataString 获取RData字符串表示
func (r *DNSRecord) GetRDataString() string {
	switch r.Rtype {
	case DNS_TYPE_A:
		ip := r.GetRDataIP()
		if ip != nil {
			return ip.String()
		}
	case DNS_TYPE_CNAME, DNS_TYPE_PTR, DNS_TYPE_NS:
		if r.RdataLen > 0 && int(r.RdataLen) <= len(r.Rdata) {
			return string(r.Rdata[:r.RdataLen])
		}
	default:
		if r.RdataLen > 0 && int(r.RdataLen) <= len(r.Rdata) {
			return fmt.Sprintf("[%d bytes]", r.RdataLen)
		}
	}
	return ""
}

// IsSuccess 检查DNS查询是否成功
func (r *DNSResponseFull) IsSuccess() bool {
	return r.Rcode == DNS_RCODE_NOERROR
}

// GetPrimaryQuestion 获取主查询问题
func (r *DNSRequestFull) GetPrimaryQuestion() *DNSQuestion {
	if r.QuestionCountActual > 0 {
		return &r.Questions[0]
	}
	return nil
}

// GetPrimaryAnswer 获取主回答记录
func (r *DNSResponseFull) GetPrimaryAnswer() *DNSRecord {
	if r.AnswerCountActual > 0 {
		return &r.Answers[0]
	}
	return nil
}
