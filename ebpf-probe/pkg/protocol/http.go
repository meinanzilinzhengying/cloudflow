package protocol

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
	"time"
)

type HTTPRecord struct {
	Timestamp   time.Time
	ProbeID     string
	SrcIP       string
	DstIP       string
	SrcPort     uint16
	DstPort     uint16
	Method      string
	Host        string
	URL         string
	StatusCode  int
	Bytes       uint64
	LatencyMs   float64
	ContentType string
	UserAgent   string
	Referer     string
	IsChunked   bool
	IsGzip      bool
	IsError     bool
	IsSlow      bool
}

func ParseHTTP(data []byte, srcIP, dstIP string, srcPort, dstPort uint16, direction uint8) *HTTPRecord {
	if len(data) < 10 {
		return nil
	}
	rec := &HTTPRecord{
		Timestamp: time.Now(),
		SrcIP:     srcIP, DstIP: dstIP,
		SrcPort: srcPort, DstPort: dstPort,
	}
	if direction == 1 {
		rec.SrcIP, rec.DstIP = rec.DstIP, rec.SrcIP
		rec.SrcPort, rec.DstPort = rec.DstPort, rec.SrcPort
	}
	return parseHTTPPayload(data, rec)
}

func parseHTTPPayload(data []byte, rec *HTTPRecord) *HTTPRecord {
	reader := bufio.NewReader(strings.NewReader(string(data)))
	line, err := reader.ReadString('\n')
	if err != nil || len(line) < 8 {
		return nil
	}
	if strings.HasPrefix(line, "GET ") || strings.HasPrefix(line, "POST ") || strings.HasPrefix(line, "PUT ") || strings.HasPrefix(line, "DELETE ") || strings.HasPrefix(line, "HEAD ") || strings.HasPrefix(line, "PATCH ") || strings.HasPrefix(line, "OPTIONS ") {
		parts := strings.SplitN(line, " ", 3)
		if len(parts) >= 2 {
			rec.Method = parts[0]
			rec.URL = parts[1]
		}
	} else if strings.HasPrefix(line, "HTTP/") {
		parts := strings.SplitN(line, " ", 3)
		if len(parts) >= 2 {
			fmt.Sscanf(parts[1], "%d", &rec.StatusCode)
			rec.IsError = rec.StatusCode >= 400
		}
	} else {
		return nil
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" || line == "\n" {
			break
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])
		switch key {
		case "host":
			rec.Host = val
		case "content-type":
			rec.ContentType = val
		case "user-agent":
			rec.UserAgent = val
		case "referer":
			rec.Referer = val
		case "transfer-encoding":
			rec.IsChunked = strings.Contains(strings.ToLower(val), "chunked")
		case "content-encoding":
			rec.IsGzip = strings.Contains(strings.ToLower(val), "gzip")
		}
	}
	return rec
}

func DetectGzip(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	return data[0] == 0x1f && data[1] == 0x8b
}

func DecompressGzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func SanitizeURL(url string) string {
	idx := strings.Index(url, "?")
	if idx >= 0 {
		url = url[:idx] + "?..."
	}
	return url
}

func IsSlowRequest(latencyMs float64) bool {
	return latencyMs > 1000.0
}
