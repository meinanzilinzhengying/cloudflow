import time
import threading
from collections import defaultdict

class TenantQuota:
    def __init__(self, tenant_id, max_events_per_sec=0, max_storage_bytes=0, max_query_per_min=0, disabled=False):
        self.tenant_id = tenant_id
        self.max_events_per_sec = max_events_per_sec
        self.max_storage_bytes = max_storage_bytes
        self.max_query_per_min = max_query_per_min
        self.disabled = disabled

class TenantUsage:
    def __init__(self, tenant_id):
        self.tenant_id = tenant_id
        self.events_this_sec = 0
        self.events_this_day = 0
        self.storage_used = 0
        self.queries_this_min = 0
        self.last_event_time = 0
        self.last_query_time = 0

class QuotaManager:
    def __init__(self):
        self._lock = threading.RLock()
        self.quotas = {}
        self.usage = defaultdict(lambda: None)

    def set_quota(self, quota):
        with self._lock:
            self.quotas[quota.tenant_id] = quota

    def is_disabled(self, tenant_id):
        with self._lock:
            q = self.quotas.get(tenant_id)
            return q.disabled if q else False

    def check_event_rate(self, tenant_id):
        with self._lock:
            q = self.quotas.get(tenant_id)
            if not q or q.max_events_per_sec <= 0:
                return True
            if q.disabled:
                return False

            usage = self.usage.get(tenant_id)
            if usage is None:
                usage = TenantUsage(tenant_id)
                self.usage[tenant_id] = usage

            now = time.time()
            if now - usage.last_event_time >= 1:
                usage.events_this_sec = 0
            usage.events_this_sec += 1
            usage.last_event_time = now

            return usage.events_this_sec <= q.max_events_per_sec

    def check_storage(self, tenant_id, additional_bytes):
        with self._lock:
            q = self.quotas.get(tenant_id)
            if not q or q.max_storage_bytes <= 0:
                return True
            usage = self.usage.get(tenant_id)
            if not usage:
                return True
            return usage.storage_used + additional_bytes <= q.max_storage_bytes

    def add_storage_usage(self, tenant_id, bytes_used):
        with self._lock:
            usage = self.usage.get(tenant_id)
            if usage is None:
                usage = TenantUsage(tenant_id)
                self.usage[tenant_id] = usage
            usage.storage_used += bytes_used
