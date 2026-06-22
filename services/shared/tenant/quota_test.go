package tenant

import (
    "testing"
)

func TestCheckEventRateUnderLimit(t *testing.T) {
    qm := NewQuotaManager()
    qm.SetQuota(&TenantQuota{
        TenantID:        "tenant-1",
        MaxEventsPerSec: 5,
    })

    for i := 0; i < 3; i++ {
        if err := qm.CheckEventRate("tenant-1"); err != nil {
            t.Fatalf("unexpected rate limit error on event %d: %v", i, err)
        }
    }
}

func TestCheckEventRateOverLimit(t *testing.T) {
    qm := NewQuotaManager()
    qm.SetQuota(&TenantQuota{
        TenantID:        "tenant-2",
        MaxEventsPerSec: 2,
    })

    if err := qm.CheckEventRate("tenant-2"); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if err := qm.CheckEventRate("tenant-2"); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    err := qm.CheckEventRate("tenant-2")
    if err == nil {
        t.Fatal("expected rate limit error, got nil")
    }
}

func TestIsDisabled(t *testing.T) {
    qm := NewQuotaManager()
    qm.SetQuota(&TenantQuota{
        TenantID: "tenant-3",
        Disabled: true,
    })

    if !qm.IsDisabled("tenant-3") {
        t.Fatal("expected tenant-3 to be disabled")
    }

    if qm.IsDisabled("tenant-4") {
        t.Fatal("expected tenant-4 to not be disabled (default)")
    }
}

func TestCheckStorageOverLimit(t *testing.T) {
    qm := NewQuotaManager()
    qm.SetQuota(&TenantQuota{
        TenantID:        "tenant-5",
        MaxStorageBytes: 100,
    })

    qm.AddStorageUsage("tenant-5", 80)
    if err := qm.CheckStorage("tenant-5", 10); err != nil {
        t.Fatalf("unexpected storage error: %v", err)
    }

    err := qm.CheckStorage("tenant-5", 25)
    if err == nil {
        t.Fatal("expected storage limit error, got nil")
    }
}
