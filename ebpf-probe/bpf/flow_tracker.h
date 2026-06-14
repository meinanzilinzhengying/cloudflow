#ifndef __FLOW_TRACKER_H
#define __FLOW_TRACKER_H

#define MAX_FLOWS 8192

struct flow_key {
    __u32 src_ip;
    __u32 dst_ip;
    __u16 src_port;
    __u16 dst_port;
    __u8  protocol;
};

struct flow_value {
    __u64 bytes;
    __u64 packets;
    __u64 first_seen;
    __u64 last_seen;
};

#endif
