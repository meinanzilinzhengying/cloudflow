package bpf

import _ "embed"

//go:embed tc.bpf.o
var BuiltInBPF []byte
