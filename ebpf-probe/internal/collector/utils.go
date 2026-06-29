package collector

import "strings"

// extractKernelFunc maps BPF program name to kernel function name
func extractKernelFunc(sectionName string) string {
	// First try to extract from section name (e.g., "kprobe/tcp_connect" -> "tcp_connect")
	if idx := strings.LastIndex(sectionName, "/"); idx >= 0 {
		return sectionName[idx+1:]
	}
	// Map BPF function names to kernel function names
	knownFuncs := map[string]string{
		"trace_connect_entry":    "tcp_connect",
		"trace_connect_exit":     "tcp_connect",
		"trace_file_open":        "do_filp_open",
		"trace_file_open_entry":  "do_filp_open",
		"trace_file_open_exit":   "do_filp_open",
		"trace_syscall_entry":    "tcp_connect",
		"trace_syscall_exit":     "tcp_connect",
		"trace_net_entry":        "tcp_connect",
		"trace_net_exit":         "tcp_connect",
		"trace_close_entry":      "__arm64_sys_close",
		"trace_close_exit":       "__arm64_sys_close",
		"trace_schedule":         "__schedule",
		"trace_schedule_ret":     "__schedule",
	}
	if kf, ok := knownFuncs[sectionName]; ok {
		return kf
	}
	return sectionName
}
