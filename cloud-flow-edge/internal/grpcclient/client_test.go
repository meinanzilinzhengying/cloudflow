package grpcclient

import (
	"testing"
)

// TestSplitHostPort 测试地址分离函数
func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{"标准地址", "localhost:8080", "localhost", false},
		{"IP 地址", "192.168.1.1:50051", "192.168.1.1", false},
		{"带端口的主机名", "my-host:3000", "my-host", false},
		{"IPv6 地址", "[::1]:8080", "::1", false},
		{"只有主机无端口", "localhost", "", true},
		{"空字符串", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitHostPort(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("splitHostPort(%q) error = %v, wantErr %v", tt.addr, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("splitHostPort(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}
