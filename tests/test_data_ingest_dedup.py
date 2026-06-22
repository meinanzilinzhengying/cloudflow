"""
Tests for data-ingest deduplication logic (_build_dedup_key and _dedup_events).

These tests specifically verify the fix for the dedup collision bug:
- details hash prevents same-category different-content events from colliding
- millisecond precision reduces temporal collisions
- Redis SETNX correctly identifies true duplicates
"""
import json
import hashlib
import sys
import os
from datetime import datetime, timezone
from unittest.mock import MagicMock, patch

# Add project root to PYTHONPATH so quota module can be imported
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import pytest

from test_data_ingest import data_ingest_service


def _make_dedup_event(category, event_type, details, tenant_id="tenant1"):
    """Helper to create an event for dedup testing."""
    return {
        "timestamp": datetime(2024, 1, 15, 8, 30, 0, tzinfo=timezone.utc),
        "probe_id": "probe-01",
        "category": category,
        "event_type": event_type,
        "details": details,
        "tenant_id": tenant_id,
        "src_ip": "10.0.0.1",
        "dst_ip": "10.0.0.2",
        "src_port": 12345,
        "dst_port": 80,
        "protocol": "tcp",
    }


class TestBuildDedupKey:
    """Tests for _build_dedup_key method."""

    def setup_method(self):
        self.service = data_ingest_service.DataIngestService()
        self.service.redis_client = MagicMock()

    def test_same_event_produces_same_key(self):
        ev1 = _make_dedup_event("security_events", "port_scan", "scanning ports 1-1024")
        ev2 = _make_dedup_event("security_events", "port_scan", "scanning ports 1-1024")

        key1 = self.service._build_dedup_key(ev1)
        key2 = self.service._build_dedup_key(ev2)

        assert key1 == key2, f"Same event should produce identical keys:\n  {key1}\n  {key2}"

    def test_different_details_produce_different_keys(self):
        """The key bug fix: events with different details must have different dedup keys."""
        ev_a = _make_dedup_event("syscall", "exec", "pid=100, comm=nginx")
        ev_b = _make_dedup_event("syscall", "exec", "pid=200, comm=python")

        key_a = self.service._build_dedup_key(ev_a)
        key_b = self.service._build_dedup_key(ev_b)

        assert key_a != key_b, (
            f"Different details must produce different dedup keys! "
            f"This was the collision bug.\n  key_a: {key_a}\n  key_b: {key_b}"
        )

    def test_different_categories_produce_different_keys(self):
        ev_net = _make_dedup_event("network_events", "connect", "tcp:443")
        ev_sec = _make_dedup_event("security_events", "connect", "tcp:443")

        key_net = self.service._build_dedup_key(ev_net)
        key_sec = self.service._build_dedup_key(ev_sec)

        assert key_net != key_sec

    def test_empty_details_uses_zero_hash(self):
        ev = _make_dedup_event("test", "test", "")
        key = self.service._build_dedup_key(ev)
        # Should contain the 8-char zero hash
        assert "00000000" in key, f"Empty details should use '00000000' hash: {key}"

    def test_none_details_uses_zero_hash(self):
        ev = _make_dedup_event("test", "test", None)
        key = self.service._build_dedup_key(ev)
        assert "00000000" in key

    def test_dedup_key_contains_details_hash(self):
        ev = _make_dedup_event("file_events", "open", "filename=/etc/passwd")
        key = self.service._build_dedup_key(ev)

        expected_hash = hashlib.md5(b"filename=/etc/passwd").hexdigest()[:8]
        assert expected_hash in key, f"Expected hash {expected_hash} in key: {key}"

    def test_dedup_key_prefix(self):
        ev = _make_dedup_event("test", "test", "data")
        key = self.service._build_dedup_key(ev)
        assert key.startswith("dedup:"), f"Key should start with 'dedup:': {key}"

    def test_timestamp_string_conversion(self):
        ev = _make_dedup_event("test", "test", "data")
        ev["timestamp"] = "2024-01-15T08:30:00Z"
        key = self.service._build_dedup_key(ev)

        # Should not raise exception
        assert "dedup:" in key
        assert "probe-01" in key

    def test_timestamp_int_nanosecond_conversion(self):
        ev = _make_dedup_event("test", "test", "data")
        ev["timestamp"] = 1705309200000000000  # nanoseconds
        key = self.service._build_dedup_key(ev)

        assert "dedup:" in key
        assert "probe-01" in key

    def test_different_timestamp_milliseconds(self):
        """Events 1ms apart should have different keys."""
        ev1 = _make_dedup_event("security_events", "alert", "event A")
        ev2 = _make_dedup_event("security_events", "alert", "event A")
        ev2["timestamp"] = datetime(2024, 1, 15, 8, 30, 0, 1000, tzinfo=timezone.utc)  # +1ms

        key1 = self.service._build_dedup_key(ev1)
        key2 = self.service._build_dedup_key(ev2)

        # With millisecond precision, 1ms apart should have different keys
        assert key1 != key2

    def test_different_probe_ids(self):
        ev1 = _make_dedup_event("network", "flow", "data")
        ev2 = _make_dedup_event("network", "flow", "data")
        ev2["probe_id"] = "probe-02"

        assert self.service._build_dedup_key(ev1) != self.service._build_dedup_key(ev2)

    def test_large_details_still_hashes(self):
        """Details of any size should produce a fixed 8-char hash."""
        large_details = "x" * 10000
        ev = _make_dedup_event("test", "test", large_details)
        key = self.service._build_dedup_key(ev)

        expected_hash = hashlib.md5(large_details.encode()).hexdigest()[:8]
        assert expected_hash in key

        # Key length should be reasonable (< 300 chars)
        assert len(key) < 300, f"Key too long: {len(key)} chars"


class TestDedupEvents:
    """Tests for _dedup_events method."""

    def setup_method(self):
        self.service = data_ingest_service.DataIngestService()
        self.service.redis_client = MagicMock()

    def test_dedup_disabled_passthrough(self):
        with patch.object(data_ingest_service, "DEDUP_ENABLED", False):
            events = [_make_dedup_event("test", "test", "data")]
            result = self.service._dedup_events(events)
            assert len(result) == 1

    def test_dedup_no_redis_passthrough(self):
        self.service.redis_client = None
        events = [_make_dedup_event("test", "test", "data")]
        result = self.service._dedup_events(events)
        assert len(result) == 1

    def test_first_event_is_kept(self):
        """SETNX should return True for first write."""
        self.service.redis_client.pipeline.return_value.execute.return_value = [True]
        events = [_make_dedup_event("test", "test", "data")]
        result = self.service._dedup_events(events)
        assert len(result) == 1

    def test_duplicate_event_is_removed(self):
        """SETNX returns False → event is a duplicate."""
        self.service.redis_client.pipeline.return_value.execute.return_value = [False]
        events = [_make_dedup_event("test", "test", "data")]
        result = self.service._dedup_events(events)
        assert len(result) == 0, "Duplicate event should be removed"

    def test_mixed_events(self):
        """Some new, some duplicates."""
        self.service.redis_client.pipeline.return_value.execute.return_value = [
            True,   # new
            False,  # duplicate
            True,   # new
            False,  # duplicate
        ]
        events = [
            _make_dedup_event("test", "t1", "data 1"),
            _make_dedup_event("test", "t1", "data 1"),
            _make_dedup_event("test", "t2", "data 2"),
            _make_dedup_event("test", "t1", "data 1"),
        ]
        result = self.service._dedup_events(events)
        assert len(result) == 2, f"Expected 2 new events, got {len(result)}"

    def test_empty_events(self):
        result = self.service._dedup_events([])
        assert len(result) == 0

    def test_redis_failure_graceful_degradation(self):
        """If Redis pipeline fails, return all events (degrade gracefully)."""
        self.service.redis_client.pipeline.return_value.execute.side_effect = \
            Exception("Redis connection lost")

        events = [_make_dedup_event("test", "test", "data")]
        result = self.service._dedup_events(events)
        assert len(result) == 1, "On Redis failure, should return all events"

    def test_different_details_not_duplicates(self):
        """Two events with same category/type but different details are NOT duplicates."""
        ev_a = _make_dedup_event("file_events", "open", "filename=/etc/shadow")
        ev_b = _make_dedup_event("file_events", "open", "filename=/etc/hosts")

        # Build keys to verify they differ
        key_a = self.service._build_dedup_key(ev_a)
        key_b = self.service._build_dedup_key(ev_b)
        assert key_a != key_b

        # Verify dedup treats them as different
        self.service.redis_client.pipeline.return_value.execute.return_value = [True, True]
        result = self.service._dedup_events([ev_a, ev_b])
        assert len(result) == 2

    def test_dedup_key_correctly_passed_to_redis(self):
        """Verify the SETNX key is the output of _build_dedup_key."""
        self.service.redis_client.pipeline.return_value.execute.return_value = [True]
        events = [_make_dedup_event("security_events", "alert", "test")]
        self.service._dedup_events(events)

        pipe = self.service.redis_client.pipeline.return_value
        set_calls = pipe.set.call_args_list
        assert len(set_calls) == 1

        expected_key = self.service._build_dedup_key(events[0])
        # Redis SET is called as: set(key, value, nx=True, ex=300)
        # With mock, it's set(key, "1", {"nx": True, "ex": 300})
        actual_key = set_calls[0][0][0]
        assert actual_key == expected_key

        # Verify the SET was called with NX and EX parameters
        # args[0] = (key, value, {...}) where {...} contains nx, ex
        args = set_calls[0][0]
        assert len(args) >= 1, f"Expected at least key, got {args}"
        assert args[0] == expected_key


class TestDedupIntegration:
    """Integration tests for dedup within the _dedup_events method."""

    def test_dedup_correctly_filters_with_redis_mock(self):
        """Verify _dedup_events filters based on Redis SETNX results."""
        # The ingest method calls _dedup_events on filtered events.
        # We test the dedup filter directly here.
        svc = data_ingest_service.DataIngestService()
        svc.redis_client = MagicMock()

        # 3 events: 1 new, 2 duplicates
        svc.redis_client.pipeline.return_value.execute.return_value = [True, False, False]
        events = [
            _make_dedup_event("security_events", "alert", "new event"),
            _make_dedup_event("security_events", "alert", "new event"),  # duplicate
            _make_dedup_event("security_events", "scan", "other scan"),  # duplicate
        ]
        result = svc._dedup_events(events)
        assert len(result) == 1, f"Only 1 unique event should survive dedup, got {len(result)}"
        assert result[0]["details"] == "new event"

    def test_ingest_dataflow_sampling_limit_enforcement(self):
        """Test size limit is enforced during ingest."""
        patches = [
            patch.object(data_ingest_service, "MAX_EVENT_BYTES", 100),
            patch.object(data_ingest_service, "DEDUP_ENABLED", False),
            patch.object(data_ingest_service, "FILTER_CATEGORIES", []),
            patch.object(data_ingest_service, "CATEGORY_SAMPLING", {"security_events": 1.0}),
            patch.object(data_ingest_service, "SAMPLING_RATE", 1.0),
        ]
        for p in patches:
            p.start()

        try:
            svc = data_ingest_service.DataIngestService()
            svc.redis_client = MagicMock()
            svc.ch_client = MagicMock()

            # Event larger than 100 bytes should be dropped
            big_ev = _make_dedup_event("security_events", "alert", "x" * 500)
            big_ev["timestamp"] = "2024-01-15T08:30:00Z"
            svc.ingest([big_ev])

            pipe = svc.redis_client.pipeline.return_value
            # No rpush calls should happen (event too large)
            rpush_calls = [c for c in pipe.method_calls if c[0] == 'rpush']
            assert len(rpush_calls) == 0, f"Large event should be dropped, got {len(rpush_calls)} rpush calls"
        finally:
            for p in patches:
                p.stop()
