package protocol

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

type DNSRecord struct {
	Timestamp     time.Time
	ProbeID       string
	SrcIP         string
	DstIP         string
	SrcPort       uint16
	DstPort       uint16
	QueryName     string
	QueryType     string
	ResponseCode  int
	LatencyMs     float64
	IsNXDOMAIN    bool
	IsSuspicious  bool
	IsTunnel      bool
}

func ParseDNS(payload []byte, srcIP, dstIP string, srcPort, dstPort uint16) *DNSRecord {
	if len(payload) < 12 {
		return nil
	}
	rec := &DNSRecord{
		Timestamp: time.Now(),
		SrcIP:     srcIP, DstIP: dstIP,
		SrcPort: srcPort, DstPort: dstPort,
	}
	// Parse flags
	flags := binary.BigEndian.Uint16(payload[2:4])
	rec.ResponseCode = int(flags & 0x0F)
	rec.IsNXDOMAIN = rec.ResponseCode == 3
	// Parse questions
	questions := binary.BigEndian.Uint16(payload[4:6])
	if questions == 0 {
		return nil
	}
	offset := 12
	var nameParts []string
	for offset < len(payload) {
		length := int(payload[offset])
		if length == 0 {
			offset++
			break
		}
		if length&0xC0 == 0xC0 {
			offset += 2
			break
		}
		if offset+1+length > len(payload) {
			break
		}
		nameParts = append(nameParts, string(payload[offset+1:offset+1+length]))
		offset += 1 + length
	}
	rec.QueryName = strings.Join(nameParts, ".")
	rec.QueryType = queryTypeName(binary.BigEndian.Uint16(payload[offset:offset+2]))
	rec.IsSuspicious = DetectDGADomain(rec.QueryName)
	rec.IsTunnel = DetectDNSTunnel(rec.QueryName)
	return rec
}

func queryTypeName(qtype uint16) string {
	switch qtype {
	case 1:
		return "A"
	case 2:
		return "NS"
	case 5:
		return "CNAME"
	case 6:
		return "SOA"
	case 12:
		return "PTR"
	case 15:
		return "MX"
	case 16:
		return "TXT"
	case 28:
		return "AAAA"
	case 33:
		return "SRV"
	case 255:
		return "ANY"
	default:
		return fmt.Sprintf("TYPE%d", qtype)
	}
}

func DetectDGADomain(domain string) bool {
	if len(domain) > 50 {
		return true
	}
	parts := strings.Split(domain, ".")
	for _, part := range parts {
		if len(part) > 20 && isHighEntropy(part) {
			return true
		}
	}
	return false
}

func isHighEntropy(s string) bool {
	if len(s) < 10 {
		return false
	}
	var digits, letters int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			digits++
		} else if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			letters++
		}
	}
	if digits+letters < len(s)*8/10 {
		return false
	}
	return digits > len(s)/3 && digits < len(s)*2/3
}

func DetectDNSTunnel(domain string) bool {
	if len(domain) > 60 {
		return true
	}
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if len(label) > 30 {
			return true
		}
	}
	return false
}
