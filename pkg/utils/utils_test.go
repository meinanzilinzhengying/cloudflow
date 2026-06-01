package utils

import "testing"

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "***"},
		{"short", "ab", "***"},
		{"exactly 8", "12345678", "***"},
		{"9 chars", "123456789", "1234***6789"},
		{"long", "abcdefghijklmnop", "abcd***mnop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskSecret(tt.input); got != tt.want {
				t.Errorf("MaskSecret(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{"integer", "42", 42.0, false},
		{"float", "3.14", 3.14, false},
		{"negative", "-1.5", -1.5, false},
		{"invalid", "abc", 0, true},
		{"empty", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFloat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFloat(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseFloat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"bytes", 500, "500 B"},
		{"kb", 1536, "1.50 KB"},
		{"mb", 1048576, "1.00 MB"},
		{"gb", 1073741824, "1.00 GB"},
		{"large", 274877906944, "256.00 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatBytes(tt.bytes); got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestSafeFloat64(t *testing.T) {
	v := 3.14
	p := SafeFloat64(v)
	if p == nil || *p != v {
		t.Fatalf("SafeFloat64(%v) = %v, want %v", v, p, &v)
	}
}

func TestSafeInt64(t *testing.T) {
	v := int64(42)
	p := SafeInt64(v)
	if p == nil || *p != v {
		t.Fatalf("SafeInt64(%v) = %v, want %v", v, p, &v)
	}
}

func TestSafeString(t *testing.T) {
	v := "hello"
	p := SafeString(v)
	if p == nil || *p != v {
		t.Fatalf("SafeString(%q) = %v, want %v", v, p, &v)
	}
}
