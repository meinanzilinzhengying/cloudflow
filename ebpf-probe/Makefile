CLANG ?= clang
KERN  ?= /usr/src/kernels/$(shell uname -r)

BPF_CFLAGS = -g -O2 -target bpf -D__BPF_TRACING__ -D__TARGET_ARCH_x86 -I$(KERN)/include -I$(KERN)/arch/x86/include -I/usr/include

all: bpf/flow_tracker.bpf.o

bpf/flow_tracker.bpf.o: bpf/flow_tracker.bpf.c bpf/flow_tracker.h
	$(CLANG) $(BPF_CFLAGS) -c $< -o $@

clean:/n/trm -f bpf/*.o

.PHONY: all clean
