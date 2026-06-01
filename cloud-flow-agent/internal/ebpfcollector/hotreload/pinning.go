// Package hotreload eBPF 程序热更新
//
// 设计目标:
//   - 无需重启 agent 更新 probe
//   - flow 零中断
//   - Map 数据保留 (state preserve)
//   - 版本管理与回退
//
// 热更新流程:
//
//	1. 加载新 eBPF object (新 maps + 新 programs)
//	2. Pin 新 maps 到 bpffs
//	3. 迁移旧 map 数据到新 map (state preserve)
//	4. 原子替换: detach 旧 program → attach 新 program
//	5. 健康检查: 验证新 program 正常工作
//	6. 清理旧 maps (unpin + close)
//	7. 失败则回退: re-attach 旧 program
//
// bpffs 布局:
//
//	/sys/fs/bpf/cloudflow/
//	  ├── tc/network_map
//	  ├── tcp/tcp_flow_stats_map
//	  ├── tcp/global_tcp_metrics_map
//	  ├── http/http_stats_map
//	  ├── http/http_transactions
//	  ├── dns/dns_queries
//	  ├── mysql/mysql_connections
//	  └── meta/version
package hotreload

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"
)

// DefaultBpffsMountPoint is the default mount point for the CloudFlow BPF filesystem.
const DefaultBpffsMountPoint = "/sys/fs/bpf/cloudflow"

// versionFileName is the filename used to store version metadata in bpffs.
const versionFileName = "version"

// ---------------------------------------------------------------------------
// BpffsManager
// ---------------------------------------------------------------------------

// BpffsManager manages the lifecycle of a BPF filesystem (bpffs) mount.
// It ensures the mount point exists and is properly mounted, and provides
// utilities for creating subdirectories and cleaning up.
type BpffsManager struct {
	mountPoint string
	mounted    bool
	mu         sync.Mutex
}

// NewBpffsManager creates a new BpffsManager. If mountPoint is empty,
// DefaultBpffsMountPoint is used.
func NewBpffsManager(mountPoint string) *BpffsManager {
	if mountPoint == "" {
		mountPoint = DefaultBpffsMountPoint
	}
	return &BpffsManager{
		mountPoint: mountPoint,
	}
}

// EnsureMounted checks whether the bpffs is already mounted at the configured
// mount point and mounts it if necessary. It creates the mount-point directory
// when it does not exist.
func (b *BpffsManager) EnsureMounted() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.mounted {
		return nil
	}

	// Create the mount-point directory (and parents) if absent.
	if err := os.MkdirAll(b.mountPoint, 0700); err != nil {
		return fmt.Errorf("failed to create bpffs mount point %s: %w", b.mountPoint, err)
	}

	// Check whether bpffs is already mounted (e.g. by a previous process).
	mounted, err := isBpffsMounted(b.mountPoint)
	if err != nil {
		return fmt.Errorf("failed to check bpffs mount status: %w", err)
	}
	if mounted {
		b.mounted = true
		return nil
	}

	// Mount the BPF filesystem.
	if err := unix.Mount("none", b.mountPoint, "bpf", 0, ""); err != nil {
		return fmt.Errorf("failed to mount bpffs at %s: %w", b.mountPoint, err)
	}

	b.mounted = true
	return nil
}

// IsMounted returns whether the bpffs is currently mounted.
func (b *BpffsManager) IsMounted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.mounted
}

// Cleanup unmounts the bpffs. It is safe to call multiple times.
func (b *BpffsManager) Cleanup() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.mounted {
		return nil
	}

	if err := unix.Unmount(b.mountPoint, 0); err != nil {
		return fmt.Errorf("failed to unmount bpffs at %s: %w", b.mountPoint, err)
	}

	b.mounted = false
	return nil
}

// MkdirAll creates a subdirectory (and any necessary parents) under the
// bpffs mount point. For example, MkdirAll("tcp") creates
// /sys/fs/bpf/cloudflow/tcp/.
func (b *BpffsManager) MkdirAll(subdir string) error {
	fullPath := filepath.Join(b.mountPoint, subdir)
	if err := os.MkdirAll(fullPath, 0700); err != nil {
		return fmt.Errorf("failed to create bpffs subdirectory %s: %w", fullPath, err)
	}
	return nil
}

// MountPoint returns the configured mount point path.
func (b *BpffsManager) MountPoint() string {
	return b.mountPoint
}

// isBpffsMounted checks whether a BPF filesystem is mounted at the given path
// by inspecting /proc/self/mountinfo.
func isBpffsMounted(path string) (bool, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}

	data, err := ioutil.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false, err
	}

	// Each line in mountinfo has the format:
	//   36 35 98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw
	// We look for lines whose mount point matches absPath and whose
	// filesystem type (the field after "-") is "bpf".
	for _, line := range splitLines(string(data)) {
		fields := splitFields(line)
		if len(fields) < 10 {
			continue
		}
		// The mount point is field index 4.
		if fields[4] != absPath {
			continue
		}
		// The filesystem type is after the "-" separator.
		for i, f := range fields {
			if f == "-" && i+1 < len(fields) && fields[i+1] == "bpf" {
				return true, nil
			}
		}
	}

	return false, nil
}

// ---------------------------------------------------------------------------
// MapPinning
// ---------------------------------------------------------------------------

// MapPinning provides helpers for pinning / unpinning / loading BPF maps
// to and from the BPF filesystem.
type MapPinning struct {
	bpffs *BpffsManager
}

// NewMapPinning creates a new MapPinning backed by the given BpffsManager.
func NewMapPinning(bpffs *BpffsManager) *MapPinning {
	return &MapPinning{bpffs: bpffs}
}

// PinPath returns the full bpffs path for a map: /sys/fs/bpf/cloudflow/{mapName}.
func (mp *MapPinning) PinPath(mapName string) string {
	return filepath.Join(mp.bpffs.MountPoint(), mapName)
}

// SubsystemPinPath returns the full bpffs path for a map within a subsystem
// subdirectory: /sys/fs/bpf/cloudflow/{subsystem}/{mapName}.
func (mp *MapPinning) SubsystemPinPath(subsystem, mapName string) string {
	return filepath.Join(mp.bpffs.MountPoint(), subsystem, mapName)
}

// PinMap pins a BPF map to bpffs at PinPath(mapName).
func (mp *MapPinning) PinMap(m *ebpf.Map, mapName string) error {
	path := mp.PinPath(mapName)
	if err := m.Pin(path); err != nil {
		return fmt.Errorf("failed to pin map %s to %s: %w", mapName, path, err)
	}
	return nil
}

// PinMapToSubsystem pins a BPF map to bpffs under a subsystem subdirectory.
// The subdirectory is created automatically if it does not exist.
func (mp *MapPinning) PinMapToSubsystem(m *ebpf.Map, subsystem, mapName string) error {
	if err := mp.bpffs.MkdirAll(subsystem); err != nil {
		return fmt.Errorf("failed to create subsystem directory %s: %w", subsystem, err)
	}
	path := mp.SubsystemPinPath(subsystem, mapName)
	if err := m.Pin(path); err != nil {
		return fmt.Errorf("failed to pin map %s/%s to %s: %w", subsystem, mapName, path, err)
	}
	return nil
}

// UnpinMap removes the pinned map at PinPath(mapName).
func (mp *MapPinning) UnpinMap(mapName string) error {
	path := mp.PinPath(mapName)
	if err := ebpf.RemovePin(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to unpin map %s at %s: %w", mapName, path, err)
	}
	return nil
}

// GetPinnedMap loads a previously pinned BPF map from bpffs.
func (mp *MapPinning) GetPinnedMap(mapName string) (*ebpf.Map, error) {
	path := mp.PinPath(mapName)
	m, err := ebpf.LoadPinnedMap(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load pinned map %s from %s: %w", mapName, path, err)
	}
	return m, nil
}

// IsPinned checks whether a map with the given name is currently pinned in bpffs.
func (mp *MapPinning) IsPinned(mapName string) bool {
	path := mp.PinPath(mapName)
	_, err := os.Stat(path)
	return err == nil
}

// ListPinnedMaps returns the names of all entries (maps and directories) pinned
// directly under the bpffs mount point.
func (mp *MapPinning) ListPinnedMaps() ([]string, error) {
	entries, err := ioutil.ReadDir(mp.bpffs.MountPoint())
	if err != nil {
		return nil, fmt.Errorf("failed to list pinned maps in %s: %w", mp.bpffs.MountPoint(), err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// UnpinAll removes all pinned entries under the bpffs mount point recursively.
func (mp *MapPinning) UnpinAll() error {
	// First pass: remove all pinned map files.
	err := filepath.Walk(mp.bpffs.MountPoint(), func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if path == mp.bpffs.MountPoint() {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if err := ebpf.RemovePin(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove pin %s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to unpin all maps: %w", err)
	}

	// Second pass: remove empty subdirectories (bottom-up via reverse walk).
	entries, err := ioutil.ReadDir(mp.bpffs.MountPoint())
	if err != nil {
		return fmt.Errorf("failed to read mount point for cleanup: %w", err)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if !e.IsDir() {
			continue
		}
		dirPath := filepath.Join(mp.bpffs.MountPoint(), e.Name())
		if rerr := os.RemoveAll(dirPath); rerr != nil && !os.IsNotExist(rerr) {
			return fmt.Errorf("failed to remove directory %s: %w", dirPath, rerr)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// MapMigration (state preserve)
// ---------------------------------------------------------------------------

// MapMigration provides utilities for migrating data between BPF maps,
// which is essential for preserving state during hot reload.
type MapMigration struct{}

// NewMapMigration creates a new MapMigration.
func NewMapMigration() *MapMigration {
	return &MapMigration{}
}

// MigrateMap iterates all entries from oldMap and inserts them into newMap.
// It returns the number of entries successfully migrated.
func (mm *MapMigration) MigrateMap(oldMap, newMap *ebpf.Map) (int, error) {
	return mm.MigrateMapWithFilter(oldMap, newMap, nil)
}

// MigrateMapWithFilter iterates all entries from oldMap and inserts those that
// pass the filter into newMap. If filter is nil, all entries are migrated.
// It returns the number of entries successfully migrated.
func (mm *MapMigration) MigrateMapWithFilter(oldMap, newMap *ebpf.Map, filter func(key, value []byte) bool) (int, error) {
	count := 0

	var (
		keySize   = oldMap.KeySize()
		valueSize = oldMap.ValueSize()
		key       = make([]byte, keySize)
		value     = make([]byte, valueSize)
	)

	iter := oldMap.Iterate()
	for iter.Next(&key, &value) {
		if filter != nil && !filter(key, value) {
			continue
		}
		if err := newMap.Put(key, value); err != nil {
			return count, fmt.Errorf("failed to migrate entry (key=%x): %w", key, err)
		}
		count++
	}
	if err := iter.Err(); err != nil {
		return count, fmt.Errorf("map iteration failed during migration: %w", err)
	}

	return count, nil
}

// BackupMap reads all entries from the given map and returns them as a slice
// of concatenated key+value byte slices. The caller can later use RestoreMap
// to write them back (for rollback scenarios).
func (mm *MapMigration) BackupMap(m *ebpf.Map) ([][]byte, error) {
	var (
		keySize   = m.KeySize()
		valueSize = m.ValueSize()
		key       = make([]byte, keySize)
		value     = make([]byte, valueSize)
	)

	var entries [][]byte

	iter := m.Iterate()
	for iter.Next(&key, &value) {
		// Pre-allocate a single buffer: [keySize | valueSize].
		buf := make([]byte, 0, keySize+valueSize)
		buf = append(buf, key...)
		buf = append(buf, value...)
		entries = append(entries, buf)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("map iteration failed during backup: %w", err)
	}

	return entries, nil
}

// RestoreMap writes previously backed-up entries into the given map.
// Each entry in the entries slice is expected to be a concatenation of
// key (first keySize bytes) and value (remaining bytes).
func (mm *MapMigration) RestoreMap(m *ebpf.Map, entries [][]byte) error {
	keySize := int(m.KeySize())

	for i, entry := range entries {
		if len(entry) < keySize {
			return fmt.Errorf("restore entry %d is too short: got %d bytes, need at least %d", i, len(entry), keySize)
		}
		key := entry[:keySize]
		value := entry[keySize:]
		if err := m.Put(key, value); err != nil {
			return fmt.Errorf("failed to restore entry %d (key=%x): %w", i, key, err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// VersionMetadata
// ---------------------------------------------------------------------------

// VersionMetadata stores version information for the currently loaded eBPF
// program. It is serialised as JSON and stored in bpffs so that the agent can
// determine which version is active and support rollback.
type VersionMetadata struct {
	Version         string `json:"version"`
	ProgramHash     string `json:"program_hash"`     // SHA-256 of the BPF ELF bytecode
	LoadedAt        int64  `json:"loaded_at"`        // Unix nano timestamp
	PreviousVersion string `json:"previous_version"` // empty for the first load
}

// SaveVersion serialises the version metadata to JSON and writes it to the
// bpffs meta/version file. It also preserves the current version as
// PreviousVersion for rollback support.
func (vm *VersionMetadata) SaveVersion(bpffs *BpffsManager, version, programHash string) error {
	// Preserve the current version for rollback.
	if vm.Version != "" {
		vm.PreviousVersion = vm.Version
	}

	vm.Version = version
	vm.ProgramHash = programHash
	vm.LoadedAt = time.Now().UnixNano()

	data, err := json.MarshalIndent(vm, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal version metadata: %w", err)
	}

	metaDir := filepath.Join(bpffs.MountPoint(), "meta")
	if err := bpffs.MkdirAll("meta"); err != nil {
		return fmt.Errorf("failed to create meta directory: %w", err)
	}

	versionPath := filepath.Join(metaDir, versionFileName)
	if err := ioutil.WriteFile(versionPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write version metadata to %s: %w", versionPath, err)
	}

	return nil
}

// LoadVersion reads the version metadata from the bpffs meta/version file.
// It returns an error if the file does not exist or cannot be parsed.
func LoadVersion(bpffs *BpffsManager) (*VersionMetadata, error) {
	versionPath := filepath.Join(bpffs.MountPoint(), "meta", versionFileName)

	data, err := ioutil.ReadFile(versionPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read version metadata from %s: %w", versionPath, err)
	}

	var vm VersionMetadata
	if err := json.Unmarshal(data, &vm); err != nil {
		return nil, fmt.Errorf("failed to parse version metadata: %w", err)
	}

	return &vm, nil
}

// ComputeProgramHash computes the SHA-256 hash of the given BPF bytecode.
// This can be used to generate the programHash field for VersionMetadata.
func ComputeProgramHash(bytecode []byte) string {
	h := sha256.Sum256(bytecode)
	return fmt.Sprintf("%x", h)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// splitLines splits a string by newline characters.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// splitFields splits a line by whitespace, collapsing consecutive whitespace.
func splitFields(line string) []string {
	var fields []string
	start := 0
	inField := false
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' || line[i] == '\t' {
			if inField {
				fields = append(fields, line[start:i])
				inField = false
			}
		} else {
			if !inField {
				start = i
				inField = true
			}
		}
	}
	if inField {
		fields = append(fields, line[start:])
	}
	return fields
}
