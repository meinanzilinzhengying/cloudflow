#ifndef LIBBPF_LOADER_H
#define LIBBPF_LOADER_H

#include <bpf/libbpf.h>
#include <bpf/bpf.h>
#include <bpf/btf.h>

// Stub declarations for libbpf-based eBPF loading
struct bpf_object_skeleton {
    const char *name;
    struct bpf_object **obj;
    int *map_fds;
    int *prog_fds;
};

#endif /* LIBBPF_LOADER_H */
