import json
import os
import sys
import random
import tempfile
import shutil
import importlib.util
from datetime import datetime
from unittest.mock import MagicMock, patch, mock_open, call
from io import BytesIO

import pytest


# --------------------------------------------------------------------------- #
# Mock external dependencies BEFORE importing the service module
# --------------------------------------------------------------------------- #
redis_mock = MagicMock()
redis_mock.Redis = MagicMock
sys.modules["redis"] = redis_mock

ch_driver_mock = MagicMock()
ch_driver_mock.Client = MagicMock
sys.modules["clickhouse_driver"] = ch_driver_mock

ch_errors_mock = MagicMock()
ch_errors_mock.NetworkError = Exception
ch_errors_mock.SocketTimeoutError = Exception
ch_errors_mock.ServerException = Exception
sys.modules["clickhouse_driver.errors"] = ch_errors_mock

# Load data-ingest-service.py as a module
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
_MODULE_PATH = os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
    "data-ingest-service.py",
)
_spec = importlib.util.spec_from_file_location("data_ingest_service", _MODULE_PATH)
data_ingest_service = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(data_ingest_service)


# --------------------------------------------------------------------------- #
# Helpers
# --------------------------------------------------------------------------- #
def _make_event(category="security_events", tenant_id="tenant1", extra=None):
    ev = {
        "category": category,
        "tenant_id": tenant_id,
        "timestamp": "2024-01-15T08:30:00Z",
        "probe_id": "probe-01",
        "event_type": "test",
        "src_ip": "10.0.0.1",
        "dst_ip": "10.0.0.2",
        "src_port": 12345,
        "dst_port": 80,
        "protocol": "tcp",
        "bytes": 100,
        "packets": 2,
        "latency_ms": 5,
        "service": "test-svc",
        "details": "unit test event",
        "tags": "test",
    }
    if extra:
        ev.update(extra)
    return ev


# --------------------------------------------------------------------------- #
# 1. Unit tests for ingest()
# --------------------------------------------------------------------------- #
class TestIngest:
    @pytest.fixture(autouse=True)
    def setup(self, tmp_path):
        self.backup_dir = tmp_path / "backup"
        self.backup_dir.mkdir()

        # Patch configuration values to deterministic defaults
        self.patches = [
            patch.object(data_ingest_service, "REDIS_BACKUP_DIR", str(self.backup_dir)),
            patch.object(data_ingest_service, "FILTER_CATEGORIES", []),
            patch.object(data_ingest_service, "CATEGORY_SAMPLING", {
                "file_events": 1.0,
                "network_events": 1.0,
                "process_events": 1.0,
                "security_events": 1.0,
            }),
            patch.object(data_ingest_service, "SAMPLING_RATE", 1.0),
            patch.object(data_ingest_service, "MAX_EVENT_BYTES", 1024),
            patch.object(data_ingest_service, "MAX_REDIS_BACKUP_SIZE_MB", 500),
        ]
        for p in self.patches:
            p.start()

        self.service = data_ingest_service.DataIngestService()
        self.service.redis_client = MagicMock()
        self.service.ch_client = MagicMock()

        yield

        for p in self.patches:
            p.stop()

        if self.service._backup_file:
            self.service._backup_file.close()

    def test_empty_events_list_returns_true(self):
        assert self.service.ingest([]) is True
        self.service.redis_client.pipeline.return_value.execute.assert_not_called()

    def test_size_limit_filtering(self):
        big_ev = _make_event(category="network_events")
        big_ev["payload"] = "x" * 2048  # exceeds 1KB default
        assert self.service.ingest([big_ev]) is True
        self.service.redis_client.pipeline.return_value.execute.assert_not_called()

    def test_category_filtering(self):
        with patch.object(data_ingest_service, "FILTER_CATEGORIES", ["security_events"]):
            svc = data_ingest_service.DataIngestService()
            svc.redis_client = MagicMock()
            svc.ch_client = MagicMock()

            allowed = _make_event(category="security_events")
            dropped = _make_event(category="network_events")

            svc.ingest([allowed, dropped])

            pipe = svc.redis_client.pipeline.return_value
            calls = pipe.rpush.call_args_list
            assert len(calls) == 1
            written = json.loads(calls[0][0][1])
            assert written["category"] == "security_events"

    def test_sampling(self):
        with patch.object(data_ingest_service, "CATEGORY_SAMPLING", {"security_events": 0.0}):
            svc = data_ingest_service.DataIngestService()
            svc.redis_client = MagicMock()
            svc.ch_client = MagicMock()

            ev = _make_event(category="security_events")
            svc.ingest([ev])

            pipe = svc.redis_client.pipeline.return_value
            pipe.rpush.assert_not_called()

    def test_tenant_id_default_value(self):
        ev = _make_event(category="security_events")
        del ev["tenant_id"]

        self.service.ingest([ev])

        pipe = self.service.redis_client.pipeline.return_value
        calls = pipe.rpush.call_args_list
        assert len(calls) == 1
        written = json.loads(calls[0][0][1])
        assert written["tenant_id"] == "default"

    def test_successful_redis_write(self):
        events = [_make_event(category="security_events")]
        self.service.redis_client.pipeline.return_value.execute.return_value = [None, None]
        result = self.service.ingest(events)
        assert result is True
        self.service.redis_client.pipeline.return_value.execute.assert_called_once()

    def test_redis_failure_falls_back_to_backup(self):
        events = [_make_event(category="security_events")]
        self.service.redis_client.pipeline.return_value.execute.side_effect = Exception("Redis down")

        with patch.object(self.service, "_backup_to_local") as mock_backup:
            result = self.service.ingest(events)
            assert result is False
            mock_backup.assert_called_once()

    def test_redis_unavailable_falls_back_to_backup(self):
        events = [_make_event(category="security_events")]
        self.service.redis_client = None

        with patch.object(self.service, "_backup_to_local") as mock_backup:
            result = self.service.ingest(events)
            assert result is False
            mock_backup.assert_called_once()


# --------------------------------------------------------------------------- #
# 2. Unit tests for _flush()
# --------------------------------------------------------------------------- #
class TestFlush:
    @pytest.fixture(autouse=True)
    def setup(self, tmp_path):
        self.backup_dir = tmp_path / "backup"
        self.backup_dir.mkdir()

        self.patches = [
            patch.object(data_ingest_service, "REDIS_BACKUP_DIR", str(self.backup_dir)),
            patch.object(data_ingest_service, "BATCH_SIZE", 1000),
            patch.object(data_ingest_service, "CLICKHOUSE_RETRY_MAX", 3),
            patch.object(data_ingest_service, "CLICKHOUSE_RETRY_DELAY", 0.01),
        ]
        for p in self.patches:
            p.start()

        self.service = data_ingest_service.DataIngestService()
        self.service.redis_client = MagicMock()
        self.service.ch_client = MagicMock()

        yield

        for p in self.patches:
            p.stop()

        if self.service._backup_file:
            self.service._backup_file.close()

    def test_no_events_in_redis_early_return(self):
        self.service.redis_client.pipeline.return_value.execute.return_value = ([], None)
        self.service._flush()
        self.service.ch_client.execute.assert_not_called()

    def test_successful_clickhouse_flush(self):
        ev = _make_event()
        self.service.redis_client.pipeline.return_value.execute.return_value = (
            [json.dumps(ev)],
            None,
        )
        self.service.ch_client.execute.return_value = None
        self.service._flush()
        self.service.ch_client.execute.assert_called_once()

    def test_clickhouse_failure_with_redis_push_back(self):
        ev = _make_event()
        self.service.redis_client.pipeline.return_value.execute.side_effect = [
            ([json.dumps(ev)], None),   # lrange + ltrim
            None,                         # lpush push-back
        ]
        self.service.ch_client.execute.side_effect = Exception("CH down")
        self.service._reconnect_clickhouse = MagicMock(return_value=False)
        self.service._flush()
        # lpush should be called in the second pipeline execution
        pipe = self.service.redis_client.pipeline.return_value
        lpush_calls = [c for c in pipe.lpush.call_args_list if c[0][0] == "cloudflow:events"]
        assert len(lpush_calls) == 1

    def test_clickhouse_failure_and_redis_failure_local_backup(self):
        ev = _make_event()
        self.service.redis_client.pipeline.return_value.execute.side_effect = [
            ([json.dumps(ev)], None),   # lrange + ltrim
            Exception("Redis push back failed"),
        ]
        self.service.ch_client.execute.side_effect = Exception("CH down")
        self.service._reconnect_clickhouse = MagicMock(return_value=False)

        with patch.object(self.service, "_backup_to_local") as mock_backup:
            self.service._flush()
            mock_backup.assert_called_once()

    def test_json_decode_error_handling(self):
        self.service.redis_client.pipeline.return_value.execute.return_value = (
            ["not valid json"],
            None,
        )
        self.service._flush()
        # Should not raise; ch execute should NOT be called because no valid events
        self.service.ch_client.execute.assert_not_called()


# --------------------------------------------------------------------------- #
# 3. Unit tests for _cleanup_disk()
# --------------------------------------------------------------------------- #
class TestCleanupDisk:
    @pytest.fixture(autouse=True)
    def setup(self, tmp_path):
        self.backup_dir = tmp_path / "backup"
        self.backup_dir.mkdir()

        self.patches = [
            patch.object(data_ingest_service, "REDIS_BACKUP_DIR", str(self.backup_dir)),
        ]
        for p in self.patches:
            p.start()

        self.service = data_ingest_service.DataIngestService()
        self.service.ch_client = MagicMock()

        yield

        for p in self.patches:
            p.stop()

        if self.service._backup_file:
            self.service._backup_file.close()

    def test_disk_below_85_percent_no_cleanup(self):
        mock_usage = MagicMock(used=50, total=100)
        with patch("shutil.disk_usage", return_value=mock_usage):
            self.service._cleanup_disk()
            self.service.ch_client.execute.assert_not_called()

    def test_disk_above_85_percent_cleanup_executed(self):
        mock_usage = MagicMock(used=90, total=100)
        with patch("shutil.disk_usage", return_value=mock_usage):
            self.service._cleanup_disk()
            assert self.service.ch_client.execute.call_count == 2  # DELETE + OPTIMIZE

    def test_clickhouse_unavailable_during_cleanup(self):
        mock_usage = MagicMock(used=90, total=100)
        self.service.ch_client.execute.side_effect = Exception("CH down")
        with patch("shutil.disk_usage", return_value=mock_usage):
            # Should not raise
            self.service._cleanup_disk()


# --------------------------------------------------------------------------- #
# 4. Unit tests for _backup_to_local()
# --------------------------------------------------------------------------- #
class TestBackupToLocal:
    @pytest.fixture(autouse=True)
    def setup(self, tmp_path):
        self.backup_dir = tmp_path / "backup"
        self.backup_dir.mkdir()

        self.patches = [
            patch.object(data_ingest_service, "REDIS_BACKUP_DIR", str(self.backup_dir)),
            patch.object(data_ingest_service, "MAX_REDIS_BACKUP_SIZE_MB", 1),
        ]
        for p in self.patches:
            p.start()

        self.service = data_ingest_service.DataIngestService()
        self.service.ch_client = MagicMock()

        yield

        for p in self.patches:
            p.stop()

        if self.service._backup_file:
            self.service._backup_file.close()

    def test_backup_file_creation(self):
        events = [_make_event()]
        self.service._backup_to_local(events)
        files = list(os.listdir(self.backup_dir))
        assert len(files) == 1
        with open(os.path.join(self.backup_dir, files[0])) as f:
            lines = f.readlines()
        assert len(lines) == 1
        assert json.loads(lines[0]) == events[0]

    def test_backup_size_tracking(self):
        events = [_make_event()]
        self.service._backup_to_local(events)
        assert self.service._backup_size > 0

    def test_old_backup_cleanup(self):
        # Create an old backup file
        old_path = os.path.join(self.backup_dir, "events_20200101.jsonl")
        with open(old_path, "w") as f:
            f.write("x" * 1024 * 1024)  # 1 MB
        os.utime(old_path, (1000000, 1000000))  # old mtime
        # Create a new backup that should trigger cleanup
        events = [_make_event()]
        with patch.object(data_ingest_service, "MAX_REDIS_BACKUP_SIZE_MB", 0):
            self.service._backup_to_local(events)
        # Old file should be removed
        assert not os.path.exists(old_path)


# --------------------------------------------------------------------------- #
# 5. Integration tests
# --------------------------------------------------------------------------- #
class TestIntegration:
    @pytest.fixture(autouse=True)
    def setup(self, tmp_path):
        self.backup_dir = tmp_path / "backup"
        self.backup_dir.mkdir()

        self.patches = [
            patch.object(data_ingest_service, "REDIS_BACKUP_DIR", str(self.backup_dir)),
            patch.object(data_ingest_service, "HTTP_PORT", 0),
            patch.object(data_ingest_service, "FILTER_CATEGORIES", []),
            patch.object(data_ingest_service, "CATEGORY_SAMPLING", {
                "file_events": 1.0,
                "network_events": 1.0,
                "process_events": 1.0,
                "security_events": 1.0,
            }),
            patch.object(data_ingest_service, "SAMPLING_RATE", 1.0),
            patch.object(data_ingest_service, "MAX_EVENT_BYTES", 1024),
        ]
        for p in self.patches:
            p.start()

        self.service = data_ingest_service.DataIngestService()
        self.service.redis_client = MagicMock()
        self.service.ch_client = MagicMock()

        yield

        for p in self.patches:
            p.stop()

        if self.service._backup_file:
            self.service._backup_file.close()

    def test_http_health_endpoint(self):
        from http.server import HTTPServer
        import threading
        data_ingest_service.IngestHandler.service = self.service
        server = HTTPServer(("127.0.0.1", 0), data_ingest_service.IngestHandler)
        port = server.server_address[1]
        t = threading.Thread(target=server.serve_forever)
        t.daemon = True
        t.start()
        try:
            import urllib.request
            resp = urllib.request.urlopen(f"http://127.0.0.1:{port}/health", timeout=5)
            assert resp.status == 200
            body = json.loads(resp.read())
            assert body["status"] == "healthy"
        finally:
            server.shutdown()
            server.server_close()

    def test_http_ingest_valid_json(self):
        from http.server import HTTPServer
        import threading
        data_ingest_service.IngestHandler.service = self.service
        server = HTTPServer(("127.0.0.1", 0), data_ingest_service.IngestHandler)
        port = server.server_address[1]
        t = threading.Thread(target=server.serve_forever)
        t.daemon = True
        t.start()
        try:
            import urllib.request
            payload = json.dumps([_make_event()]).encode()
            req = urllib.request.Request(
                f"http://127.0.0.1:{port}/api/v1/ingest",
                data=payload,
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            resp = urllib.request.urlopen(req, timeout=5)
            assert resp.status == 200
            body = json.loads(resp.read())
            assert body["status"] == "ok"
        finally:
            server.shutdown()
            server.server_close()

    def test_http_ingest_invalid_json(self):
        from http.server import HTTPServer
        import threading
        data_ingest_service.IngestHandler.service = self.service
        server = HTTPServer(("127.0.0.1", 0), data_ingest_service.IngestHandler)
        port = server.server_address[1]
        t = threading.Thread(target=server.serve_forever)
        t.daemon = True
        t.start()
        try:
            import urllib.request
            payload = b"not json"
            req = urllib.request.Request(
                f"http://127.0.0.1:{port}/api/v1/ingest",
                data=payload,
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            try:
                urllib.request.urlopen(req, timeout=5)
                assert False, "Should have raised HTTPError"
            except urllib.error.HTTPError as e:
                assert e.code == 400
        finally:
            server.shutdown()
            server.server_close()

    def test_http_ingest_503_when_redis_fails(self):
        from http.server import HTTPServer
        import threading
        self.service.redis_client = None
        data_ingest_service.IngestHandler.service = self.service
        server = HTTPServer(("127.0.0.1", 0), data_ingest_service.IngestHandler)
        port = server.server_address[1]
        t = threading.Thread(target=server.serve_forever)
        t.daemon = True
        t.start()
        try:
            import urllib.request
            payload = json.dumps([_make_event()]).encode()
            req = urllib.request.Request(
                f"http://127.0.0.1:{port}/api/v1/ingest",
                data=payload,
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            try:
                urllib.request.urlopen(req, timeout=5)
                assert False, "Should have raised HTTPError"
            except urllib.error.HTTPError as e:
                assert e.code == 503
                body = json.loads(e.read())
                assert body["status"] == "error"
                assert body["message"] == "redis_unavailable"
        finally:
            server.shutdown()
            server.server_close()
