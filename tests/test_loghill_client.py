import importlib.util
import io
import json
import logging
import os
import sys
import tempfile
import threading
import unittest
from pathlib import Path
from unittest import mock


MODULE_PATH = Path(__file__).resolve().parents[1] / "examples" / "loghill.py"
SPEC = importlib.util.spec_from_file_location("loghill_client", MODULE_PATH)
assert SPEC and SPEC.loader
loghill = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = loghill
SPEC.loader.exec_module(loghill)


class CapturedStreamTests(unittest.TestCase):
    def test_preserves_output_and_captures_complete_non_empty_lines(self):
        terminal = io.StringIO()
        captured = []
        stream = loghill._CapturedStream(
            terminal,
            "stderr",
            lambda message, source: captured.append((message, source)),
        )

        stream.write("INFO:     Started server")
        stream.write(" process\n\n")
        stream.write("INFO:     Uvicorn running")
        stream.flush()

        self.assertEqual(
            terminal.getvalue(),
            "INFO:     Started server process\n\nINFO:     Uvicorn running",
        )
        self.assertEqual(
            captured,
            [
                ("INFO:     Started server process", "stderr"),
                ("INFO:     Uvicorn running", "stderr"),
            ],
        )


    def test_captures_handler_created_before_capture_through_real_descriptor(self):
        captured = []
        with tempfile.TemporaryFile(mode="w+", encoding="utf-8") as terminal:
            handler = logging.StreamHandler(terminal)
            handler.setFormatter(logging.Formatter("%(levelname)s:     %(message)s"))
            server_logger = logging.Logger("any-server", logging.INFO)
            server_logger.addHandler(handler)
            descriptor = loghill._CapturedFileDescriptor(
                terminal,
                "stderr",
                lambda message, source: captured.append((message, source)),
            )

            try:
                server_logger.info("Started server process [22300]")
                server_logger.info("Application startup complete.")
                os.write(descriptor._fd, b"native stderr write\n")
            finally:
                descriptor.stop()
                descriptor.close_bypass()

            terminal.seek(0)
            output = terminal.read()

        self.assertIn("INFO:     Started server process [22300]", output)
        self.assertEqual(
            captured,
            [
                ("INFO:     Started server process [22300]", "stderr"),
                ("INFO:     Application startup complete.", "stderr"),
                ("native stderr write", "stderr"),
            ],
        )


class InstrumentationTests(unittest.TestCase):
    def setUp(self):
        self.original_logger = loghill._instrumented_logger
        self.original_pid = loghill._instrumented_pid
        loghill._instrumented_logger = None
        loghill._instrumented_pid = None

    def tearDown(self):
        loghill._instrumented_logger = self.original_logger
        loghill._instrumented_pid = self.original_pid

    def test_instrument_and_create_logger_reuse_one_instance_per_process(self):
        logger = mock.Mock()
        logger._closed = False
        with mock.patch.object(loghill, "_build_logger", return_value=logger) as build:
            first = loghill.instrument(name="primeiro")
            second = loghill.instrument(name="segundo")
            compatible = loghill.create_logger(name="terceiro")

        self.assertIs(first, logger)
        self.assertIs(second, logger)
        self.assertIs(compatible, logger)
        build.assert_called_once_with(name="primeiro")

    def test_instrument_creates_new_instance_after_previous_logger_is_closed(self):
        closed = mock.Mock()
        closed._closed = True
        replacement = mock.Mock()
        replacement._closed = False
        loghill._instrumented_logger = closed
        loghill._instrumented_pid = os.getpid()

        with mock.patch.object(loghill, "_build_logger", return_value=replacement):
            current = loghill.instrument(name="replacement")

        self.assertIs(current, replacement)


class ConnectionConfigurationTests(unittest.TestCase):
    def test_uses_sender_name_without_sender_key(self):
        with tempfile.TemporaryDirectory() as directory, mock.patch.dict(
            os.environ,
            {"LOGHILL_API_URL": "http://localhost:8001", "LOGHILL_SENDER_NAME": "Worker Whom"},
            clear=True,
        ):
            api_url, sender_name = loghill._config(
                None,
                None,
                Path(directory) / "missing.env",
                "fallback",
            )

        self.assertEqual(api_url, "http://localhost:8001")
        self.assertEqual(sender_name, "Worker Whom")

    def test_logger_name_is_the_default_sender_name(self):
        with tempfile.TemporaryDirectory() as directory, mock.patch.dict(
            os.environ,
            {"LOGHILL_API_URL": "http://localhost:8001"},
            clear=True,
        ):
            _, sender_name = loghill._config(
                None,
                None,
                Path(directory) / "missing.env",
                "whom-worker",
            )

        self.assertEqual(sender_name, "whom-worker")

    def test_initializes_by_name_and_keeps_instance_token_in_memory(self):
        logger = loghill.LogHillLogger.__new__(loghill.LogHillLogger)
        logger.instance_id = ""
        logger.instance_token = ""
        logger.sender_id = ""
        logger.sender_name = "whom-worker"
        logger._closed = False
        logger._instance_lock = threading.Lock()
        logger._post = mock.Mock(
            return_value={
                "sender_id": "whom-worker",
                "instance_id": "ins_11111111111111111111111111111111",
                "instance_token": "inst_runtime_secret",
            }
        )
        logger._start_health_thread = mock.Mock()
        logger._report_started = mock.Mock()
        logger._clear_persistent_queue = mock.Mock(return_value=0)

        logger._initialize_instance()

        logger._post.assert_called_once_with(
            "/api/v1/instances/init",
            {"sender_name": "whom-worker"},
        )
        self.assertEqual(logger.sender_id, "whom-worker")
        self.assertEqual(logger.instance_token, "inst_runtime_secret")
        logger._clear_persistent_queue.assert_called_once_with()

    def test_clears_all_persisted_logs_and_preserves_memory_queue(self):
        with tempfile.TemporaryDirectory() as directory:
            logger = loghill.LogHillLogger.__new__(loghill.LogHillLogger)
            logger.sender_id = "whom-worker"
            logger._queue_path = Path(directory) / "fila.sqlite3"
            logger._persistent_queue_enabled = True
            logger._queue_lock = threading.Lock()
            logger._queue_drained = threading.Event()
            logger._queue_event = threading.Event()
            logger._memory_queue = loghill.deque(
                [
                    {"message": "memória atual", "_loghill_sender_id": "whom-worker"},
                    {"message": "memória antiga", "_loghill_sender_id": "sender-antigo"},
                    {"message": "memória legada"},
                ]
            )
            logger._report_error = mock.Mock()
            connection = logger._queue_connection()
            try:
                connection.execute(
                    "CREATE TABLE log_queue (id INTEGER PRIMARY KEY AUTOINCREMENT, payload TEXT NOT NULL, created_at TEXT NOT NULL)"
                )
                payloads = [
                    {"message": "disco atual", "_loghill_sender_id": "whom-worker"},
                    {"message": "disco antigo", "_loghill_sender_id": "sender-antigo"},
                    {"message": "disco legado"},
                ]
                connection.executemany(
                    "INSERT INTO log_queue (payload, created_at) VALUES (?, ?)",
                    [(json.dumps(payload), "2026-08-31T10:00:00Z") for payload in payloads],
                )
                connection.commit()
            finally:
                connection.close()

            removed = logger._clear_persistent_queue()

            self.assertEqual(removed, 3)
            self.assertEqual(
                [payload["message"] for payload in logger._memory_queue],
                ["memória atual", "memória antiga", "memória legada"],
            )
            connection = logger._queue_connection()
            try:
                remaining = [
                    json.loads(row[0])["message"]
                    for row in connection.execute(
                        "SELECT payload FROM log_queue ORDER BY id"
                    ).fetchall()
                ]
            finally:
                connection.close()
            self.assertEqual(remaining, [])
            logger._report_error.assert_not_called()

    def test_worker_sends_canonical_sender_id_instead_of_sender_name(self):
        logger = loghill.LogHillLogger.__new__(loghill.LogHillLogger)
        logger.sender_name = "Worker de exibição"
        logger.sender_id = "worker-canonical-id"
        logger.instance_id = "ins_11111111111111111111111111111111"
        logger.instance_token = "inst_runtime_secret"
        logger.retry_attempts = 0
        logger.retry_interval = 0.1
        logger._remote_enabled = True
        logger._stop = threading.Event()
        logger._peek_payload = mock.Mock(return_value=("memory", None, {"severity": "INFO", "message": "teste"}))
        captured = {}

        def post(path, payload, *, origin_instance_id=""):
            captured.update(
                {
                    "path": path,
                    "payload": payload,
                    "origin_instance_id": origin_instance_id,
                }
            )
            logger._stop.set()
            return {"accepted": True}

        logger._post = post
        logger._ack_payload = mock.Mock()
        logger._report_status = mock.Mock()
        logger._report_error = mock.Mock()

        logger._worker_loop()

        self.assertEqual(captured["path"], "/api/v1/logs")
        self.assertEqual(captured["payload"]["sender_id"], "worker-canonical-id")
        self.assertNotIn("sender", captured["payload"])
        self.assertEqual(
            captured["origin_instance_id"],
            "ins_11111111111111111111111111111111",
        )

    def test_queue_keeps_origin_identity_without_persisting_instance_token(self):
        logger = loghill.LogHillLogger.__new__(loghill.LogHillLogger)
        logger.sender_id = "worker-canonical-id"
        logger.instance_id = "ins_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
        logger.instance_token = "inst_must_not_be_persisted"
        logger._persistent_queue_enabled = False
        logger._memory_queue = loghill.deque()
        logger._queue_lock = threading.Lock()
        logger._queue_event = threading.Event()
        logger._queue_drained = threading.Event()
        logger._queue_drained.set()

        logger._enqueue_payload({"severity": "INFO", "message": "pendente"})

        queued = logger._memory_queue[0]
        self.assertEqual(queued["_loghill_sender_id"], "worker-canonical-id")
        self.assertEqual(
            queued["_loghill_instance_id"],
            "ins_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        )
        self.assertNotIn("instance_token", str(queued))
        self.assertNotIn("inst_must_not_be_persisted", str(queued))

    def test_worker_replays_pending_log_with_its_original_instance(self):
        logger = loghill.LogHillLogger.__new__(loghill.LogHillLogger)
        logger.sender_name = "Worker de exibiÃ§Ã£o"
        logger.sender_id = "worker-canonical-id"
        logger.instance_id = "ins_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
        logger.instance_token = "inst_current_runtime_secret"
        logger.retry_attempts = 0
        logger.retry_interval = 0.1
        logger._remote_enabled = True
        logger._stop = threading.Event()
        logger._peek_payload = mock.Mock(
            return_value=(
                "disk",
                7,
                {
                    "severity": "UNDEFINED",
                    "message": "INFO: Finished server process [26916]",
                    "_loghill_sender_id": "worker-canonical-id",
                    "_loghill_instance_id": "ins_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
                },
            )
        )
        captured = {}

        def post(path, payload, *, origin_instance_id=""):
            captured.update(
                {
                    "path": path,
                    "payload": payload,
                    "origin_instance_id": origin_instance_id,
                }
            )
            logger._stop.set()
            return {"accepted": True}

        logger._post = post
        logger._ack_payload = mock.Mock()
        logger._report_status = mock.Mock()
        logger._report_error = mock.Mock()

        logger._worker_loop()

        self.assertEqual(captured["payload"]["sender_id"], "worker-canonical-id")
        self.assertEqual(
            captured["origin_instance_id"],
            "ins_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        )
        self.assertNotIn("_loghill_sender_id", captured["payload"])
        self.assertNotIn("_loghill_instance_id", captured["payload"])

    def test_worker_discards_foreign_sender_without_sending_it(self):
        logger = loghill.LogHillLogger.__new__(loghill.LogHillLogger)
        logger.sender_id = "whom-worker"
        logger.instance_id = "ins_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
        logger.instance_token = "inst_current_runtime_secret"
        logger.retry_attempts = 0
        logger.retry_interval = 0.1
        logger._remote_enabled = True
        logger._stop = threading.Event()
        logger._peek_payload = mock.Mock(
            return_value=(
                "disk",
                214,
                {
                    "severity": "UNDEFINED",
                    "message": "registro antigo",
                    "_loghill_sender_id": "wk-whom-api-staging",
                    "_loghill_instance_id": "ins_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
                },
            )
        )
        logger._post = mock.Mock()
        logger._report_status = mock.Mock()
        logger._report_error = mock.Mock()

        def acknowledge(source, queue_id):
            logger._stop.set()

        logger._ack_payload = mock.Mock(side_effect=acknowledge)

        logger._worker_loop()

        logger._ack_payload.assert_called_once_with("disk", 214)
        logger._post.assert_not_called()
        logger._report_error.assert_not_called()

    def test_flush_waits_until_worker_acknowledges_last_pending_log(self):
        logger = loghill.LogHillLogger.__new__(loghill.LogHillLogger)
        logger._queue_lock = threading.Lock()
        logger._queue_event = threading.Event()
        logger._queue_drained = threading.Event()
        logger._persistent_queue_enabled = False
        logger._memory_queue = loghill.deque([{"message": "último log"}])
        logger._remote_enabled = True
        logger.shutdown_flush_timeout = 1.0

        def acknowledge_when_requested():
            logger._queue_event.wait()
            logger._ack_payload("memory", None)

        worker = threading.Thread(target=acknowledge_when_requested)
        logger._worker_thread = worker
        worker.start()
        try:
            self.assertTrue(logger.flush())
            self.assertEqual(logger.pending_count(), 0)
        finally:
            logger._queue_event.set()
            worker.join(timeout=1)


if __name__ == "__main__":
    unittest.main()
