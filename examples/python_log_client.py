"""Cliente Python de exemplo para o LogHill.

Crie o sender pela interface, copie ID e chave e configure:

    LOG_API_URL=http://localhost:8080
    LOG_SENDER_KEY=snd_...

Depois execute ``python examples/python_log_client.py``.
"""

from __future__ import annotations

import json
import logging
import os
import re
import threading
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from typing import Any


EVENT_KEY_PATTERN = re.compile(r"^[a-z0-9][a-z0-9_-]{2,79}$")


class LogHillLogger(logging.Logger):
    """Logger compatível com ``event=`` e ``metadata=`` em cada chamada."""

    def _log(
        self,
        level: int,
        msg: object,
        args: tuple[Any, ...],
        exc_info: Any = None,
        extra: dict[str, Any] | None = None,
        stack_info: bool = False,
        stacklevel: int = 1,
        *,
        event: str | None = None,
        event_occurrence_id: str | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> None:
        if event is not None and not EVENT_KEY_PATTERN.fullmatch(event):
            raise ValueError("event deve seguir ^[a-z0-9][a-z0-9_-]{2,79}$")
        if event_occurrence_id is not None and len(event_occurrence_id) > 200:
            raise ValueError("event_occurrence_id deve ter no máximo 200 caracteres")
        values = dict(extra or {})
        values["loghill_event"] = event
        values["loghill_event_occurrence_id"] = event_occurrence_id
        values["loghill_metadata"] = dict(metadata or {})
        super()._log(
            level,
            msg,
            args,
            exc_info=exc_info,
            extra=values,
            stack_info=stack_info,
            stacklevel=stacklevel + 1,
        )


class LogHillHandler(logging.Handler):
    """Encaminha registros do ``logging`` para um sender já cadastrado."""

    def __init__(
        self,
        sender_key: str,
        base_url: str = "http://localhost:8080",
        healthcheck_interval: float = 60.0,
        timeout: float = 10.0,
    ) -> None:
        super().__init__()
        if not sender_key.startswith("snd_"):
            raise ValueError("sender_key deve ser a chave exibida pelo LogHill")
        self.base_url = base_url.rstrip("/")
        self.sender_id = ""
        self._sender_key = sender_key
        self.healthcheck_interval = healthcheck_interval
        self.timeout = timeout
        self.instance_id = ""
        initialized = self._post(
            "/api/v1/instances/init",
            {},
        )
        self.instance_id = str(initialized["instance_id"])
        self.sender_id = str(initialized["sender"])
        self._stop_event = threading.Event()
        self._health_thread = threading.Thread(
            target=self._healthcheck_loop,
            name=f"loghill-health-{self.sender_id}",
            daemon=True,
        )
        self._health_thread.start()

    def emit(self, record: logging.LogRecord) -> None:
        try:
            metadata: dict[str, Any] = {
                "logger": record.name,
                "module": record.module,
                "function": record.funcName,
                "line": record.lineno,
                "thread": record.threadName,
            }
            custom_metadata = getattr(record, "loghill_metadata", None)
            if isinstance(custom_metadata, dict):
                metadata.update(custom_metadata)
            if record.exc_info:
                metadata["exception"] = logging.Formatter().formatException(
                    record.exc_info
                )
            payload: dict[str, Any] = {
                "sender": self.sender_id,
                "severity": self._severity(record.levelno),
                "message": record.getMessage(),
                "timestamp": datetime.fromtimestamp(
                    record.created, tz=timezone.utc
                ).isoformat(),
                "metadata": metadata,
            }
            event = getattr(record, "loghill_event", None)
            occurrence_id = getattr(record, "loghill_event_occurrence_id", None)
            if event:
                payload["event"] = event
            if occurrence_id:
                payload["event_occurrence_id"] = occurrence_id
            self._post(
                "/api/v1/logs",
                payload,
            )
        except Exception:
            self.handleError(record)

    def close(self) -> None:
        self._stop_event.set()
        if self._health_thread.is_alive() and threading.current_thread() is not self._health_thread:
            self._health_thread.join(timeout=min(self.timeout, 2.0))
        self._sender_key = ""
        super().close()

    def _healthcheck_loop(self) -> None:
        while not self._stop_event.wait(self.healthcheck_interval):
            try:
                self._post(
                    f"/api/v1/senders/{self.sender_id}/health",
                    {"status": "healthy", "details": {"client": "python-logging"}},
                )
            except (OSError, RuntimeError) as error:
                print(f"Falha no healthcheck do LogHill: {error}")

    def _post(self, path: str, payload: dict[str, Any]) -> dict[str, Any]:
        headers = {
            "Accept": "application/json",
            "Content-Type": "application/json",
            "X-Sender-Key": self._sender_key,
        }
        if self.instance_id:
            headers["X-Sender-Instance-ID"] = self.instance_id
        request = urllib.request.Request(
            f"{self.base_url}{path}",
            data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
            headers=headers,
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                body = response.read()
        except urllib.error.HTTPError as error:
            details = error.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"LogHill respondeu HTTP {error.code}: {details}") from error
        except urllib.error.URLError as error:
            raise RuntimeError(f"Não foi possível conectar ao LogHill: {error.reason}") from error
        return json.loads(body.decode("utf-8")) if body else {}

    @staticmethod
    def _severity(level: int) -> str:
        if level >= logging.CRITICAL:
            return "FATAL"
        if level >= logging.ERROR:
            return "ERROR"
        if level >= logging.WARNING:
            return "WARN"
        if level >= logging.INFO:
            return "INFO"
        if level >= logging.DEBUG:
            return "DEBUG"
        return "TRACE"


def create_logger(sender_key: str, base_url: str = "http://localhost:8080") -> LogHillLogger:
    log = LogHillLogger("loghill")
    log.setLevel(logging.DEBUG)
    log.propagate = False
    log.addHandler(LogHillHandler(sender_key, base_url))
    return log


def simulate_flow(log: LogHillLogger) -> None:
    while True:
        log.info(
            "teste 1",
            event="processamento_finalizado",
            metadata={"protocolo": "ABC-123"},
        )
        time.sleep(1)
        log.debug("teste 2 - detalhes do processamento")
        time.sleep(1)
        log.warning("teste 3 - processamento mais lento que o esperado, ECONNRESET")
        time.sleep(1)
        log.error("teste 4 - falha simulada")
        time.sleep(1)
        log.critical("teste 5 - erro fatal simulado")


if __name__ == "__main__":
    sender_key = "snd_ah_3x-SRvCMBhuvFHKdN175yQJ5tfIwL"
    logger = create_logger(sender_key, os.getenv("LOG_API_URL", "http://localhost:8080"))
    try:
        simulate_flow(logger)
        print("Logs enviados pelo sender identificado pela chave.")
    finally:
        for log_handler in logger.handlers[:]:
            log_handler.close()
            logger.removeHandler(log_handler)
