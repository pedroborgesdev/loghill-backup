"""Logger Python resiliente para o LogHill, sem dependências externas.

Características principais:
- nunca deixa falhas internas interromperem a aplicação principal;
- funciona somente como logger local quando LOGHILL_API_URL não está definida;
- envia logs em uma thread de segundo plano;
- mantém uma fila FIFO persistida em SQLite;
- bloqueia o fluxo principal durante a inicialização e suas tentativas de conexão;
- tenta novamente três vezes antes de informar que o LogHill está fora do ar;
- continua tentando restabelecer a conexão e reenviar os logs em ordem;
- mantém a saída normal dos logs no terminal.
"""

from __future__ import annotations

import atexit
import codecs
import hashlib
import io
import json
import logging
import os
import re
import socket
import sqlite3
import sys
import threading
import urllib.error
import urllib.request
from collections import deque
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Mapping


_EVENT_RE = re.compile(r"^[a-z0-9][a-z0-9_-]{2,79}$")
_URL_ENVS = ("LOGHILL_API_URL", "LOG_API_URL")
_NAME_ENVS = ("LOGHILL_SENDER_NAME", "LOGHILL_SENDER_ID")
_QUEUE_FILE_ENV = "LOGHILL_QUEUE_FILE"
_ANSI_ESCAPE_RE = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")

_STARTUP_BANNER = r"""
 __            _____ _ _ _ 
|  |   ___ ___|  |  |_| | |
|  |__| . | . |     | | | |
|_____|___|_  |__|__|_|_|_|
          |___|             
""".strip("\n")


class _CapturedStream:
    """Preserva a stream original e encaminha cada linha completa ao LogHill."""

    def __init__(self, stream: Any, source: str, callback: Any) -> None:
        self._stream = stream
        self._source = source
        self._callback = callback
        self._buffers: dict[int, str] = {}
        self._lock = threading.RLock()

    def write(self, value: object) -> int:
        text = str(value)
        written = self._stream.write(text)
        if not text:
            return int(written or 0)

        thread_id = threading.get_ident()
        with self._lock:
            buffered = self._buffers.get(thread_id, "") + text
            parts = buffered.split("\n")
            self._buffers[thread_id] = parts.pop()
        for line in parts:
            self._emit(line.removesuffix("\r"))
        return int(written if written is not None else len(text))

    def flush(self) -> None:
        self._stream.flush()
        thread_id = threading.get_ident()
        with self._lock:
            pending = self._buffers.pop(thread_id, "")
        self._emit(pending.removesuffix("\r"))

    def drain(self) -> None:
        with self._lock:
            pending = list(self._buffers.values())
            self._buffers.clear()
        for line in pending:
            self._emit(line.removesuffix("\r"))

    def _emit(self, line: str) -> None:
        if not line.strip():
            return
        try:
            self._callback(line, self._source)
        except Exception:
            pass

    def __getattr__(self, name: str) -> Any:
        return getattr(self._stream, name)


class _CapturedFileDescriptor:
    """Faz tee do descritor real do processo sem depender do framework de logs."""

    def __init__(self, stream: Any, source: str, callback: Any) -> None:
        self._stream = stream
        self._source = source
        self._callback = callback
        self._fd = int(stream.fileno())
        self._saved_fd = -1
        self._read_fd = -1
        self._thread: threading.Thread | None = None
        self.bypass_stream: Any = None
        self.process_stream: Any = None

        try:
            stream.flush()
            self._saved_fd = os.dup(self._fd)
            read_fd, write_fd = os.pipe()
            self._read_fd = read_fd
            try:
                os.dup2(write_fd, self._fd)
            finally:
                os.close(write_fd)

            encoding = getattr(stream, "encoding", None) or "utf-8"
            errors = getattr(stream, "errors", None) or "replace"
            bypass_fd = os.dup(self._saved_fd)
            try:
                if os.name == "nt":
                    import msvcrt

                    msvcrt.setmode(bypass_fd, os.O_BINARY)
                self.bypass_stream = os.fdopen(
                    bypass_fd,
                    "w",
                    buffering=1,
                    encoding=encoding,
                    errors=errors,
                    closefd=True,
                )
            except Exception:
                os.close(bypass_fd)
                raise
            process_fd = os.dup(self._fd)
            try:
                if os.name == "nt":
                    import msvcrt

                    msvcrt.setmode(process_fd, os.O_BINARY)
                self.process_stream = os.fdopen(
                    process_fd,
                    "w",
                    buffering=1,
                    encoding=encoding,
                    errors=errors,
                    closefd=True,
                )
            except Exception:
                os.close(process_fd)
                raise
            self._thread = threading.Thread(
                target=self._read_loop,
                name=f"loghill-terminal-{source}",
                daemon=True,
            )
            self._thread.start()
        except Exception:
            if self._saved_fd >= 0:
                try:
                    os.dup2(self._saved_fd, self._fd)
                except Exception:
                    pass
                try:
                    os.close(self._saved_fd)
                except Exception:
                    pass
                self._saved_fd = -1
            if self._read_fd >= 0:
                try:
                    os.close(self._read_fd)
                except Exception:
                    pass
                self._read_fd = -1
            try:
                if self.process_stream is not None:
                    self.process_stream.close()
            except Exception:
                pass
            self.process_stream = None
            self.close_bypass()
            raise

    def stop(self) -> None:
        if self._saved_fd < 0:
            return
        try:
            if self.process_stream is not None:
                self.process_stream.flush()
                self.process_stream.close()
        except Exception:
            pass
        self.process_stream = None
        os.dup2(self._saved_fd, self._fd)
        if self._thread and self._thread is not threading.current_thread():
            self._thread.join(timeout=2.0)
        os.close(self._saved_fd)
        self._saved_fd = -1

    def close_bypass(self) -> None:
        try:
            if self.bypass_stream is not None:
                self.bypass_stream.close()
        except Exception:
            pass
        self.bypass_stream = None

    def _read_loop(self) -> None:
        encoding = getattr(self._stream, "encoding", None) or "utf-8"
        errors = getattr(self._stream, "errors", None) or "replace"
        decoder = codecs.getincrementaldecoder(encoding)(errors=errors)
        buffered = ""
        try:
            while True:
                chunk = os.read(self._read_fd, 8192)
                if not chunk:
                    break
                text = decoder.decode(chunk)
                if self.bypass_stream is not None:
                    self.bypass_stream.write(text)
                    self.bypass_stream.flush()
                buffered += text
                parts = buffered.split("\n")
                buffered = parts.pop()
                for line in parts:
                    self._emit(line.removesuffix("\r"))
            buffered += decoder.decode(b"", final=True)
            self._emit(buffered.removesuffix("\r"))
        except Exception:
            pass
        finally:
            if self._read_fd >= 0:
                try:
                    os.close(self._read_fd)
                except Exception:
                    pass
                self._read_fd = -1

    def _emit(self, line: str) -> None:
        if not line.strip():
            return
        try:
            self._callback(line, self._source)
        except Exception:
            pass


class _RequestFailure(RuntimeError):
    """Falha controlada de comunicação com a API."""

    def __init__(self, message: str, *, retryable: bool) -> None:
        super().__init__(message)
        self.retryable = retryable


def _env(*names: str) -> str:
    return next((value for name in names if (value := os.getenv(name, "").strip())), "")


def _load_env(path: Path) -> bool:
    if not path.is_file():
        return False

    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except Exception as error:
        raise RuntimeError(f"não foi possível ler o arquivo .env: {error}") from None

    for raw in lines:
        line = raw.strip().removeprefix("export ").strip()
        if not line or line.startswith("#") or "=" not in line:
            continue

        key, value = map(str.strip, line.split("=", 1))
        if len(value) > 1 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        if key:
            os.environ.setdefault(key, value)

    return True


def _config(
    api_url: str | None,
    sender_name: str | None,
    env_file: str | Path | None,
    default_sender_name: str,
) -> tuple[str, str]:
    base = Path(__file__).resolve().parent
    candidates = (
        [Path(env_file)]
        if env_file
        else [Path.cwd() / ".env", base / ".env", base.parent / ".env"]
    )

    for path in candidates:
        if path.is_file():
            _load_env(path)
            break

    url = (api_url or _env(*_URL_ENVS)).rstrip("/")
    configured_name = sender_name or _env(*_NAME_ENVS) or default_sender_name

    if not url:
        return "", ""
    if not url.startswith(("http://", "https://")):
        raise ValueError("LOGHILL_API_URL deve começar com http:// ou https://.")
    configured_name = " ".join(str(configured_name).split())
    if not configured_name or len(configured_name) > 80:
        raise ValueError("LOGHILL_SENDER_NAME deve possuir entre 1 e 80 caracteres.")

    return url, configured_name


def _validation_error(event: str | None, occurrence_id: str | None) -> str | None:
    if event is not None and not _EVENT_RE.fullmatch(event):
        return "event inválido: use apenas letras minúsculas, números, '_' ou '-', com 3 a 80 caracteres."
    if occurrence_id is not None and len(occurrence_id) > 200:
        return "event_occurrence_id inválido: o limite é de 200 caracteres."
    return None


def _http_error_message(error: urllib.error.HTTPError) -> str:
    """Converte respostas HTTP da API em mensagens curtas e compreensíveis."""
    body = ""
    try:
        body = error.read().decode("utf-8", errors="replace")
    except Exception:
        pass

    code = ""
    api_message = ""
    try:
        parsed = json.loads(body)
        details = parsed.get("error", parsed) if isinstance(parsed, dict) else {}
        if isinstance(details, dict):
            code = str(details.get("code", ""))
            api_message = str(details.get("message", ""))
    except Exception:
        pass

    if error.code == 400:
        return api_message or "a API rejeitou os dados enviados. Confira os campos do log."
    if error.code == 401 or code in {"INVALID_SENDER_KEY", "INVALID_INSTANCE_TOKEN"}:
        return (
            "a credencial da instância foi recusada. "
            "Confira LOGHILL_SENDER_NAME e reinicie a aplicação para criar uma nova instância."
        )
    if error.code == 403:
        return "a chave informada não tem permissão para executar esta operação."
    if error.code == 404:
        return "a rota da API não foi encontrada. Confira LOGHILL_API_URL e a versão da API."
    if error.code == 408:
        return "a API demorou demais para processar a requisição."
    if error.code == 409:
        return api_message or "a API encontrou um conflito ao processar o log."
    if error.code == 413:
        return "o log é grande demais para ser enviado. Reduza a mensagem ou os metadados."
    if error.code == 422:
        return api_message or "a API não conseguiu validar os dados enviados."
    if error.code == 429:
        return "a API recebeu requisições demais e limitou temporariamente os envios."
    if error.code >= 500:
        return "o servidor do LogHill está indisponível ou apresentou uma falha interna."
    if api_message:
        return api_message
    return f"a API recusou a requisição com o status HTTP {error.code}."


def _friendly_error(error: Exception) -> str:
    """Traduz qualquer falha interna conhecida para uma mensagem legível."""
    if isinstance(error, _RequestFailure):
        return str(error)

    if isinstance(error, urllib.error.HTTPError):
        return _http_error_message(error)

    if isinstance(error, urllib.error.URLError):
        reason = error.reason
        if isinstance(reason, socket.timeout):
            return "a conexão com a API excedeu o tempo limite."
        if isinstance(reason, ConnectionRefusedError):
            return "a conexão com a API foi recusada. Confira se o servidor está ativo e a porta está correta."
        if isinstance(reason, OSError):
            return _friendly_error(reason)
        return "não foi possível conectar à API. Confira LOGHILL_API_URL, a rede e o servidor."

    if isinstance(error, (TimeoutError, socket.timeout)):
        return "a conexão com a API excedeu o tempo limite."
    if isinstance(error, ConnectionRefusedError):
        return "a conexão com a API foi recusada. Confira se o servidor está ativo e a porta está correta."
    if isinstance(error, ConnectionResetError):
        return "a conexão com a API foi encerrada pelo servidor antes de a resposta ser concluída."
    if isinstance(error, ConnectionAbortedError):
        return "a conexão com a API foi cancelada antes de a requisição terminar."
    if isinstance(error, BrokenPipeError):
        return "a conexão com a API foi interrompida durante o envio."
    if isinstance(error, socket.gaierror):
        return "não foi possível localizar o endereço da API. Confira o domínio em LOGHILL_API_URL."
    if isinstance(error, json.JSONDecodeError):
        return "a API retornou uma resposta que não é um JSON válido."
    if isinstance(error, UnicodeError):
        return "a resposta da API contém texto em uma codificação inválida."
    if isinstance(error, (TypeError, ValueError)):
        return str(error) or "os dados informados ao logger são inválidos."
    if isinstance(error, KeyError):
        field = str(error).strip("'")
        return f"a resposta da API não contém o campo obrigatório '{field}'."
    if isinstance(error, OSError):
        text = str(error).strip()
        return (
            f"ocorreu uma falha de rede ou do sistema operacional: {text}"
            if text
            else "ocorreu uma falha de rede ou do sistema operacional."
        )

    text = str(error).strip()
    return text or f"ocorreu uma falha interna no cliente LogHill ({type(error).__name__})."


def _is_retryable_http_status(status: int) -> bool:
    return status in {408, 425, 429} or status >= 500


class LogHillLogger(logging.Logger):
    """Logger local com integração remota opcional ao LogHill."""

    def __init__(
        self,
        name: str = "loghill",
        *,
        api_url: str | None = None,
        sender_name: str | None = None,
        env_file: str | Path | None = None,
        level: int = logging.DEBUG,
        console: bool = True,
        healthcheck_interval: float = 60.0,
        timeout: float = 10.0,
        retry_attempts: int = 3,
        retry_interval: float = 5.0,
        queue_file: str | Path | None = None,
        capture_system_logs: bool = True,
        shutdown_flush_timeout: float = 2.0,
    ) -> None:
        super().__init__(name, level)
        self.api_url = ""
        self.sender_name = ""
        self.healthcheck_interval = healthcheck_interval
        self.timeout = timeout
        self.retry_attempts = max(int(retry_attempts), 0)
        self.retry_interval = max(float(retry_interval), 0.1)
        self.shutdown_flush_timeout = max(float(shutdown_flush_timeout), 0.0)
        self.sender_id = ""
        self.instance_id = ""
        self.instance_token = ""
        self.propagate = False

        self._closed = False
        self._remote_enabled = True
        self._disabled_reason = ""
        self._reported_errors: set[str] = set()
        self._report_lock = threading.Lock()
        self._instance_lock = threading.Lock()
        self._queue_lock = threading.Lock()
        self._stop = threading.Event()
        self._queue_event = threading.Event()
        self._queue_drained = threading.Event()
        self._queue_drained.set()
        self._health_thread: threading.Thread | None = None
        self._worker_thread: threading.Thread | None = None
        self._previous_sys_excepthook = None
        self._previous_threading_excepthook = None
        self._sys_excepthook = None
        self._threading_excepthook = None
        self._queue_path: Path | None = None
        self._persistent_queue_enabled = False
        self._memory_queue: deque[dict[str, Any]] = deque()
        self._capture_system_logs = bool(capture_system_logs)
        self._original_stdout = sys.stdout
        self._original_stderr = sys.stderr
        self._stdout_capture: _CapturedStream | None = None
        self._stderr_capture: _CapturedStream | None = None
        self._stdout_fd_capture: _CapturedFileDescriptor | None = None
        self._stderr_fd_capture: _CapturedFileDescriptor | None = None
        self._terminal_stdout = self._original_stdout
        self._terminal_stderr = self._original_stderr
        self._retargeted_stream_handlers: list[tuple[logging.StreamHandler, Any]] = []

        if console:
            try:
                handler = logging.StreamHandler(sys.stdout)
                handler.setLevel(level)
                handler.setFormatter(
                    logging.Formatter(
                        "%(asctime)s [%(levelname)-8s] %(message)s",
                        "%Y-%m-%d %H:%M:%S",
                    )
                )
                self.addHandler(handler)
            except Exception as error:
                self._report_error(
                    f"não foi possível configurar a saída do terminal: {_friendly_error(error)}"
                )

        try:
            self.api_url, self.sender_name = _config(api_url, sender_name, env_file, name)
            configured_shutdown_timeout = _env("LOGHILL_SHUTDOWN_TIMEOUT_SECONDS")
            if configured_shutdown_timeout:
                self.shutdown_flush_timeout = max(
                    float(configured_shutdown_timeout), 0.0
                )
            if not self.api_url:
                self._remote_enabled = False
                self._disabled_reason = "LOGHILL_API_URL não configurada; modo local ativo."
            else:
                self._queue_path = self._resolve_queue_path(queue_file)
                self._prepare_queue_storage()

                # A inicialização é síncrona: instrument() só devolve o controle
                # após conectar ou esgotar todas as tentativas iniciais.
                initialized = self._initialize_blocking()

                # O worker só é iniciado quando o init teve sucesso. Se todas as
                # tentativas iniciais falharem, não começa uma segunda sequência
                # de tentativas em segundo plano nesta execução.
                if initialized and self._remote_enabled:
                    self._start_worker()
        except Exception as error:
            self._disable_remote(_friendly_error(error))

        try:
            atexit.register(self.close)
        except Exception:
            pass

        if self._remote_enabled:
            self._install_exception_hooks()
            self._install_system_log_capture()

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
        metadata: Mapping[str, Any] | None = None,
    ) -> None:
        try:
            validation_error = _validation_error(event, event_occurrence_id)
            if validation_error:
                self._report_error(validation_error + " O campo inválido foi ignorado.")
                event = None
                event_occurrence_id = None

            try:
                safe_metadata = dict(metadata or {})
            except Exception:
                safe_metadata = {}
                self._report_error(
                    "metadata inválida: informe um dicionário ou outro Mapping. O campo foi ignorado."
                )

            super()._log(
                level,
                msg,
                args,
                exc_info=exc_info,
                extra={
                    **(extra or {}),
                    "loghill_event": event,
                    "loghill_occurrence_id": event_occurrence_id,
                    "loghill_metadata": safe_metadata,
                },
                stack_info=stack_info,
                stacklevel=stacklevel + 1,
            )
        except Exception as error:
            self._report_error(f"não foi possível registrar a mensagem: {_friendly_error(error)}")

    def handle(self, record: logging.LogRecord) -> None:
        try:
            if self.disabled or not self.filter(record):
                return
            if self.handlers or self.propagate:
                self.callHandlers(record)
            if self._remote_enabled:
                self._send_record(record)
        except Exception as error:
            self._report_error(f"não foi possível processar o log: {_friendly_error(error)}")

    def send(
        self,
        message: str,
        *,
        severity: str = "INFO",
        event: str | None = None,
        event_occurrence_id: str | None = None,
        metadata: Mapping[str, Any] | None = None,
        timestamp: datetime | None = None,
    ) -> dict[str, Any]:
        """Registra localmente ou enfileira para a API quando ela está configurada."""
        try:
            validation_error = _validation_error(event, event_occurrence_id)
            if validation_error:
                self._report_error(validation_error + " O campo inválido foi ignorado.")
                event = None
                event_occurrence_id = None

            if not self._remote_enabled:
                severity_name = str(severity).upper()
                local_level = {
                    "TRACE": logging.DEBUG,
                    "DEBUG": logging.DEBUG,
                    "INFO": logging.INFO,
                    "WARN": logging.WARNING,
                    "WARNING": logging.WARNING,
                    "ERROR": logging.ERROR,
                    "FATAL": logging.CRITICAL,
                    "CRITICAL": logging.CRITICAL,
                }.get(severity_name, logging.INFO)
                self.log(local_level, str(message))
                return {"sent": False, "queued": False, "local": True}

            try:
                safe_metadata = dict(metadata or {})
            except Exception:
                safe_metadata = {}
                self._report_error(
                    "metadata inválida: informe um dicionário ou outro Mapping. O campo foi ignorado."
                )

            payload: dict[str, Any] = {
                "severity": str(severity).upper(),
                "message": str(message),
                "timestamp": (timestamp or datetime.now(timezone.utc)).isoformat(),
                "metadata": safe_metadata,
            }
            if event:
                payload["event"] = event
            if event_occurrence_id:
                payload["event_occurrence_id"] = event_occurrence_id

            queue_id = self._enqueue_payload(payload)
            return {
                "sent": False,
                "queued": True,
                "queue_id": queue_id,
            }
        except Exception as error:
            reason = _friendly_error(error)
            self._report_error(f"não foi possível enfileirar o log: {reason}")
            return {"sent": False, "queued": False, "reason": reason}

    def close(self) -> None:
        """Finaliza as threads; registros ainda não enviados permanecem no SQLite."""
        try:
            if self._closed:
                return
            self._restore_system_log_capture()
            if threading.current_thread() is not self._worker_thread:
                self.flush(self.shutdown_flush_timeout)
            self._closed = True
            self._restore_exception_hooks()
            self._stop.set()
            self._queue_event.set()

            current = threading.current_thread()
            for thread in (self._worker_thread, self._health_thread):
                if thread and thread.is_alive() and current is not thread:
                    thread.join(timeout=min(max(float(self.timeout), 0.1), 2.0))
        except Exception as error:
            self._report_error(
                f"não foi possível finalizar o logger corretamente: {_friendly_error(error)}"
            )

    def _install_system_log_capture(self) -> None:
        """Captura os descritores reais stdout/stderr usados por todo o processo."""
        if (
            not self._capture_system_logs
            or self._stdout_capture is not None
            or self._stdout_fd_capture is not None
        ):
            return

        try:
            self._stdout_fd_capture = _CapturedFileDescriptor(
                self._original_stdout, "stdout", self._capture_system_line
            )
            self._terminal_stdout = self._stdout_fd_capture.bypass_stream
            self._stderr_fd_capture = _CapturedFileDescriptor(
                self._original_stderr, "stderr", self._capture_system_line
            )
            self._terminal_stderr = self._stderr_fd_capture.bypass_stream
            sys.stdout = self._stdout_fd_capture.process_stream
            sys.stderr = self._stderr_fd_capture.process_stream

            loggers: list[logging.Logger] = [logging.getLogger(), self]
            loggers.extend(
                logger
                for logger in logging.root.manager.loggerDict.values()
                if isinstance(logger, logging.Logger)
            )
            visited: set[int] = set()
            for logger in loggers:
                for handler in logger.handlers:
                    if id(handler) in visited or not isinstance(handler, logging.StreamHandler):
                        continue
                    visited.add(id(handler))
                    stream = getattr(handler, "stream", None)
                    replacement = None
                    if stream is self._original_stdout or stream is sys.__stdout__:
                        replacement = (
                            self._terminal_stdout
                            if logger is self
                            else self._stdout_fd_capture.process_stream
                        )
                    elif stream is self._original_stderr or stream is sys.__stderr__:
                        replacement = (
                            self._terminal_stderr
                            if logger is self
                            else self._stderr_fd_capture.process_stream
                        )
                    if replacement is not None:
                        handler.setStream(replacement)
                        self._retargeted_stream_handlers.append((handler, stream))
            return
        except (AttributeError, io.UnsupportedOperation, OSError, ValueError):
            self._restore_system_log_capture(flush_pending=False)

        try:
            # Fallback para consoles virtuais que nao expoem descritores de arquivo.
            stdout_capture = _CapturedStream(
                self._original_stdout, "stdout", self._capture_system_line
            )
            stderr_capture = _CapturedStream(
                self._original_stderr, "stderr", self._capture_system_line
            )
            self._stdout_capture = stdout_capture
            self._stderr_capture = stderr_capture
            sys.stdout = stdout_capture
            sys.stderr = stderr_capture

        except Exception as error:
            self._restore_system_log_capture(flush_pending=False)
            self._report_error(
                f"nao foi possivel capturar stdout/stderr: {_friendly_error(error)}"
            )

    def _restore_system_log_capture(self, *, flush_pending: bool = True) -> None:
        stdout_capture = self._stdout_capture
        stderr_capture = self._stderr_capture
        stdout_fd_capture = self._stdout_fd_capture
        stderr_fd_capture = self._stderr_fd_capture
        if (
            stdout_capture is None
            and stderr_capture is None
            and stdout_fd_capture is None
            and stderr_fd_capture is None
        ):
            return

        if flush_pending:
            if stdout_capture is not None:
                stdout_capture.drain()
            if stderr_capture is not None:
                stderr_capture.drain()

        for handler, original in self._retargeted_stream_handlers:
            try:
                stream = getattr(handler, "stream", None)
                if (
                    stream is stdout_capture
                    or stream is stderr_capture
                    or stream is self._terminal_stdout
                    or stream is self._terminal_stderr
                ):
                    handler.setStream(original)
            except Exception:
                pass
        self._retargeted_stream_handlers.clear()

        if (
            stdout_fd_capture is not None
            and sys.stdout is stdout_fd_capture.process_stream
        ):
            sys.stdout = self._original_stdout
        if (
            stderr_fd_capture is not None
            and sys.stderr is stderr_fd_capture.process_stream
        ):
            sys.stderr = self._original_stderr
        if stdout_fd_capture is not None:
            stdout_fd_capture.stop()
        if stderr_fd_capture is not None:
            stderr_fd_capture.stop()

        if stdout_capture is not None and sys.stdout is stdout_capture:
            sys.stdout = self._original_stdout
        if stderr_capture is not None and sys.stderr is stderr_capture:
            sys.stderr = self._original_stderr
        self._stdout_capture = None
        self._stderr_capture = None
        self._stdout_fd_capture = None
        self._stderr_fd_capture = None
        self._terminal_stdout = self._original_stdout
        self._terminal_stderr = self._original_stderr
        if stdout_fd_capture is not None:
            stdout_fd_capture.close_bypass()
        if stderr_fd_capture is not None:
            stderr_fd_capture.close_bypass()

    def _capture_system_line(self, message: str, source: str) -> None:
        if self._closed or not self._remote_enabled:
            return
        message = _ANSI_ESCAPE_RE.sub("", message)
        self.send(
            message,
            severity="UNDEFINED",
            metadata={"captured": True, "source": source},
        )

    def _install_exception_hooks(self) -> None:
        """Captura tracebacks nao tratados sem substituir a impressao padrao."""
        try:
            self._previous_sys_excepthook = sys.excepthook
            self._sys_excepthook = self._handle_unhandled_exception
            sys.excepthook = self._sys_excepthook

            if hasattr(threading, "excepthook"):
                self._previous_threading_excepthook = threading.excepthook
                self._threading_excepthook = self._handle_thread_exception
                threading.excepthook = self._threading_excepthook
        except Exception as error:
            self._report_error(
                f"nao foi possivel capturar tracebacks nao tratados: {_friendly_error(error)}"
            )

    def _restore_exception_hooks(self) -> None:
        try:
            if self._sys_excepthook is not None and sys.excepthook is self._sys_excepthook:
                sys.excepthook = self._previous_sys_excepthook or sys.__excepthook__

            if (
                self._threading_excepthook is not None
                and hasattr(threading, "excepthook")
                and threading.excepthook is self._threading_excepthook
            ):
                threading.excepthook = (
                    self._previous_threading_excepthook or threading.__excepthook__
                )
        except Exception:
            pass

    def _handle_unhandled_exception(
        self,
        exc_type: type[BaseException],
        exc_value: BaseException,
        exc_traceback: Any,
    ) -> None:
        try:
            self._send_unhandled_exception(exc_type, exc_value, exc_traceback, thread_name=None)
        finally:
            previous = self._previous_sys_excepthook or sys.__excepthook__
            previous(exc_type, exc_value, exc_traceback)

    def _handle_thread_exception(self, args: threading.ExceptHookArgs) -> None:
        try:
            self._send_unhandled_exception(
                args.exc_type,
                args.exc_value,
                args.exc_traceback,
                thread_name=getattr(args.thread, "name", None),
            )
        finally:
            previous = self._previous_threading_excepthook or threading.__excepthook__
            previous(args)

    def _send_unhandled_exception(
        self,
        exc_type: type[BaseException] | None,
        exc_value: BaseException | None,
        exc_traceback: Any,
        *,
        thread_name: str | None,
    ) -> None:
        try:
            if exc_type is None or exc_value is None:
                return
            if issubclass(exc_type, (KeyboardInterrupt, SystemExit)):
                return

            message = logging.Formatter().formatException(
                (exc_type, exc_value, exc_traceback)
            )
            self.send(message, severity="ERROR")
        except Exception as error:
            self._report_error(
                f"nao foi possivel enviar traceback nao tratado: {_friendly_error(error)}"
            )

    def pending_count(self) -> int:
        """Retorna quantos logs ainda aguardam envio."""
        total = 0
        try:
            with self._queue_lock:
                total += len(self._memory_queue)
                if self._persistent_queue_enabled and self._queue_path:
                    with self._queue_connection() as connection:
                        row = connection.execute("SELECT COUNT(*) FROM log_queue").fetchone()
                        total += int(row[0]) if row else 0
        except Exception as error:
            self._report_error(f"não foi possível consultar a fila: {_friendly_error(error)}")
        return total

    def flush(self, timeout: float | None = None) -> bool:
        """Aguarda a fila esvaziar sem ultrapassar o prazo informado."""
        if self.pending_count() == 0:
            self._queue_drained.set()
            return True
        worker = self._worker_thread
        if not self._remote_enabled or worker is None or not worker.is_alive():
            return False
        self._queue_event.set()
        wait_timeout = self.shutdown_flush_timeout if timeout is None else timeout
        return self._queue_drained.wait(timeout=max(float(wait_timeout), 0.0))

    def _resolve_queue_path(self, queue_file: str | Path | None) -> Path:
        configured = queue_file or os.getenv(_QUEUE_FILE_ENV, "").strip()
        if configured:
            return Path(configured).expanduser().resolve()

        queue_identity = f"{self.api_url}|{self.sender_name}"
        key_hash = hashlib.sha256(queue_identity.encode("utf-8")).hexdigest()[:12]
        return Path.home() / ".loghill" / f"queue-{key_hash}.sqlite3"

    def _prepare_queue_storage(self) -> None:
        if not self._queue_path:
            return

        try:
            self._queue_path.parent.mkdir(parents=True, exist_ok=True)
            with self._queue_connection() as connection:
                connection.execute("PRAGMA journal_mode=WAL")
                connection.execute("PRAGMA synchronous=FULL")
                connection.execute(
                    """
                    CREATE TABLE IF NOT EXISTS log_queue (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        payload TEXT NOT NULL,
                        created_at TEXT NOT NULL
                    )
                    """
                )
                connection.commit()
                pending = connection.execute(
                    "SELECT 1 FROM log_queue LIMIT 1"
                ).fetchone()
                if pending:
                    self._queue_drained.clear()
                else:
                    self._queue_drained.set()
            self._persistent_queue_enabled = True
        except Exception as error:
            self._persistent_queue_enabled = False
            self._report_error(
                "não foi possível criar a fila persistente; usando uma fila temporária em memória: "
                f"{_friendly_error(error)}"
            )

    def _queue_connection(self) -> sqlite3.Connection:
        if not self._queue_path:
            raise RuntimeError("o caminho da fila persistente não foi configurado.")
        return sqlite3.connect(str(self._queue_path), timeout=5.0)

    def _enqueue_payload(self, payload: Mapping[str, Any]) -> int | None:
        queued_payload = dict(payload)
        if self.sender_id and self.instance_id:
            queued_payload["_loghill_sender_id"] = self.sender_id
            queued_payload["_loghill_instance_id"] = self.instance_id
        serialized = json.dumps(queued_payload, ensure_ascii=False, default=str)

        with self._queue_lock:
            self._queue_drained.clear()
            if self._persistent_queue_enabled:
                try:
                    with self._queue_connection() as connection:
                        cursor = connection.execute(
                            "INSERT INTO log_queue (payload, created_at) VALUES (?, ?)",
                            (serialized, datetime.now(timezone.utc).isoformat()),
                        )
                        connection.commit()
                        queue_id = int(cursor.lastrowid)
                    self._queue_event.set()
                    return queue_id
                except Exception as error:
                    self._report_error(
                        "falha ao gravar na fila persistente; o log ficará temporariamente em memória: "
                        f"{_friendly_error(error)}"
                    )

            self._memory_queue.append(queued_payload)
            self._queue_event.set()
            return None

    def _peek_payload(self) -> tuple[str, int | None, dict[str, Any]] | None:
        with self._queue_lock:
            if self._persistent_queue_enabled:
                try:
                    while True:
                        with self._queue_connection() as connection:
                            row = connection.execute(
                                "SELECT id, payload FROM log_queue ORDER BY id ASC LIMIT 1"
                            ).fetchone()

                        if not row:
                            break

                        queue_id = int(row[0])
                        try:
                            payload = json.loads(str(row[1]))
                            if not isinstance(payload, dict):
                                raise ValueError("o conteúdo da fila não é um objeto JSON.")
                            return "disk", queue_id, payload
                        except Exception as error:
                            self._delete_disk_payload(queue_id)
                            self._report_error(
                                f"um registro inválido foi removido da fila persistente: {_friendly_error(error)}"
                            )
                except Exception as error:
                    self._report_error(
                        f"não foi possível ler a fila persistente: {_friendly_error(error)}"
                    )

            if self._memory_queue:
                return "memory", None, dict(self._memory_queue[0])

        return None

    def _ack_payload(self, source: str, queue_id: int | None) -> None:
        with self._queue_lock:
            if source == "disk" and queue_id is not None:
                self._delete_disk_payload(queue_id)
            elif source == "memory" and self._memory_queue:
                self._memory_queue.popleft()
            self._update_queue_drained_locked()

    def _clear_persistent_queue(self) -> int:
        """Remove todos os registros SQLite ao inicializar uma execução."""
        if not self._persistent_queue_enabled:
            return 0

        connection = None
        with self._queue_lock:
            try:
                connection = self._queue_connection()
                row = connection.execute("SELECT COUNT(*) FROM log_queue").fetchone()
                removed = int(row[0] if row else 0)
                connection.execute("DELETE FROM log_queue")
                connection.commit()
                if self._memory_queue:
                    self._queue_drained.clear()
                else:
                    self._queue_drained.set()
                return removed
            except Exception as error:
                self._persistent_queue_enabled = False
                if self._memory_queue:
                    self._queue_drained.clear()
                else:
                    self._queue_drained.set()
                self._report_error(
                    "não foi possível limpar a fila persistente durante a inicialização: "
                    f"{_friendly_error(error)}. A fila SQLite não será usada nesta execução."
                )
                return 0
            finally:
                if connection is not None:
                    connection.close()

    def _update_queue_drained_locked(self) -> None:
        if self._memory_queue:
            self._queue_drained.clear()
            return
        if self._persistent_queue_enabled:
            try:
                with self._queue_connection() as connection:
                    pending = connection.execute(
                        "SELECT 1 FROM log_queue LIMIT 1"
                    ).fetchone()
                if pending:
                    self._queue_drained.clear()
                    return
            except Exception:
                self._queue_drained.clear()
                return
        self._queue_drained.set()

    def _delete_disk_payload(self, queue_id: int) -> None:
        with self._queue_connection() as connection:
            connection.execute("DELETE FROM log_queue WHERE id = ?", (queue_id,))
            connection.commit()

    def _initialize_blocking(self) -> bool:
        """Inicializa o logger bloqueando somente durante as tentativas iniciais.

        A primeira conexão é feita imediatamente. Se falhar por um motivo
        temporário, executa ``retry_attempts`` novas tentativas, respeitando
        ``retry_interval``. Depois disso, libera a aplicação sem iniciar o
        worker remoto; uma nova conexão será tentada na próxima execução.
        """
        retries_done = 0

        while not self._stop.is_set() and self._remote_enabled:
            try:
                self._initialize_instance()
                return True

            except _RequestFailure as error:
                if not error.retryable:
                    self._handle_permanent_request_failure(error)
                    return False

                if retries_done >= self.retry_attempts:
                    self._report_status(
                        "LogHill está fora do ar após as tentativas iniciais. "
                        "A aplicação será liberada e não haverá novas tentativas nesta execução."
                    )
                    return False

                retries_done += 1
                self._report_status(
                    "Falha ao inicializar a conexão com a API. "
                    f"Nova tentativa ({retries_done}/{self.retry_attempts}): {error}"
                )

                if self._stop.wait(self.retry_interval):
                    return False

            except Exception as error:
                self._report_error(
                    "falha interna durante a inicialização do LogHill: "
                    f"{_friendly_error(error)}"
                )
                return False

        return False

    def _start_worker(self) -> None:
        if self._worker_thread and self._worker_thread.is_alive():
            return

        self._worker_thread = threading.Thread(
            target=self._worker_loop,
            name=f"loghill-sender-{self.name}",
            daemon=True,
        )
        self._worker_thread.start()

    def _worker_loop(self) -> None:
        consecutive_failures = 0
        queue_notice_shown = False
        offline_announced = False

        while not self._stop.is_set() and self._remote_enabled:
            try:
                if not self.instance_id:
                    self._initialize_instance()
                    consecutive_failures = 0
                    if offline_announced:
                        self._report_status(
                            f"Conexão restabelecida. Reenviando {self.pending_count()} log(s) pendente(s) em ordem."
                        )
                    queue_notice_shown = False
                    offline_announced = False

                queued = self._peek_payload()
                if queued is None:
                    self._queue_event.clear()
                    self._queue_event.wait(timeout=0.5)
                    continue

                source, queue_id, payload = queued
                api_payload = dict(payload)
                origin_sender_id = str(
                    api_payload.pop("_loghill_sender_id", "") or self.sender_id
                )
                origin_instance_id = str(
                    api_payload.pop("_loghill_instance_id", "") or self.instance_id
                )
                if origin_sender_id != self.sender_id:
                    self._ack_payload(source, queue_id)
                    self._report_status(
                        "Um registro pendente de outro sender foi removido da fila."
                    )
                    continue
                api_payload["sender_id"] = origin_sender_id

                self._post(
                    "/api/v1/logs",
                    api_payload,
                    origin_instance_id=origin_instance_id,
                )
                self._ack_payload(source, queue_id)

                if consecutive_failures or offline_announced:
                    self._report_status(
                        f"Conexão restabelecida. Reenviando {self.pending_count()} log(s) pendente(s) em ordem."
                    )
                consecutive_failures = 0
                queue_notice_shown = False
                offline_announced = False

            except _RequestFailure as error:
                if not error.retryable:
                    self._handle_permanent_request_failure(error)
                    return

                consecutive_failures += 1

                if not queue_notice_shown:
                    self._report_status(
                        "Falha na conexão com a API. Os logs estão sendo enfileirados para reenvio automático."
                    )
                    queue_notice_shown = True

                if consecutive_failures <= self.retry_attempts:
                    self._report_status(
                        "Nova tentativa de conexão "
                        f"({consecutive_failures}/{self.retry_attempts}): {error}"
                    )
                elif not offline_announced:
                    self._report_status(
                        "LogHill está fora do ar. Os logs continuarão enfileirados e serão reenviados automaticamente."
                    )
                    offline_announced = True
                    # Após uma indisponibilidade prolongada, força uma nova instância.
                    # Isso também cobre reinícios da API que invalidem a instância anterior.
                    self.instance_id = ""
                    self.sender_id = ""
                    self.instance_token = ""

                self._stop.wait(self.retry_interval)

            except Exception as error:
                self._report_error(
                    f"falha interna no worker de envio: {_friendly_error(error)}"
                )
                self._stop.wait(self.retry_interval)

    def _handle_permanent_request_failure(self, error: _RequestFailure) -> None:
        self._disable_remote(str(error))

    def _initialize_instance(self) -> None:
        if self.instance_id:
            return
        if self._closed:
            raise RuntimeError("o logger já foi fechado.")

        with self._instance_lock:
            if self.instance_id:
                return

            data = self._post(
                "/api/v1/instances/init",
                {"sender_name": self.sender_name},
            )
            if not isinstance(data, dict):
                raise _RequestFailure(
                    "a API retornou uma resposta inválida ao inicializar o logger.",
                    retryable=False,
                )

            instance_id = data.get("instance_id")
            sender_id = data.get("sender_id") or data.get("sender")
            instance_token = data.get("instance_token")
            if not instance_id or not sender_id or not instance_token:
                raise _RequestFailure(
                    "a API não retornou sender_id, instance_id e instance_token ao inicializar o logger.",
                    retryable=False,
                )

            self.instance_id = str(instance_id)
            self.sender_id = str(sender_id)
            self.instance_token = str(instance_token)
            discarded = self._clear_persistent_queue()
            if discarded:
                self._report_status(
                    f"{discarded} registro(s) persistido(s) foram removidos da fila ao iniciar."
                )
            self._start_health_thread()
            self._report_started()

    def _start_health_thread(self) -> None:
        if self._health_thread and self._health_thread.is_alive():
            return

        self._health_thread = threading.Thread(
            target=self._health_loop,
            name=f"loghill-health-{self.sender_id}",
            daemon=True,
        )
        self._health_thread.start()

    def _send_record(self, record: logging.LogRecord) -> None:
        try:
            custom_metadata = getattr(record, "loghill_metadata", {})
            if not isinstance(custom_metadata, Mapping):
                custom_metadata = {}

            metadata = {
                "logger": record.name,
                "module": record.module,
                "function": record.funcName,
                "line": record.lineno,
                "thread": record.threadName,
                **dict(custom_metadata),
            }
            if record.exc_info:
                try:
                    metadata["exception"] = logging.Formatter().formatException(record.exc_info)
                except Exception:
                    metadata["exception"] = "não foi possível formatar a exceção."

            self.send(
                record.getMessage(),
                severity=self._severity(record.levelno),
                event=getattr(record, "loghill_event", None),
                event_occurrence_id=getattr(record, "loghill_occurrence_id", None),
                metadata=metadata,
                timestamp=datetime.fromtimestamp(record.created, timezone.utc),
            )
        except Exception as error:
            self._report_error(f"log não enfileirado: {_friendly_error(error)}")

    def _health_loop(self) -> None:
        try:
            interval = max(float(self.healthcheck_interval), 1.0)
        except Exception:
            interval = 60.0
            self._report_error("healthcheck_interval inválido; usando 60 segundos.")

        while not self._stop.wait(interval):
            try:
                if not self.instance_id or not self.sender_id:
                    continue
                self._post(
                    f"/api/v1/senders/{self.sender_id}/health",
                    {"status": "healthy", "details": {"client": "python-logging"}},
                )
            except _RequestFailure as error:
                if not error.retryable:
                    self._report_error(f"healthcheck rejeitado: {error}")
                else:
                    self._report_error(
                        "healthcheck não enviado; o worker continuará tentando restabelecer a conexão: "
                        f"{error}"
                    )
            except Exception as error:
                self._report_error(f"healthcheck não enviado: {_friendly_error(error)}")

    def _post(
        self,
        path: str,
        payload: Mapping[str, Any],
        *,
        origin_instance_id: str = "",
    ) -> dict[str, Any]:
        headers = {
            "Accept": "application/json",
            "Content-Type": "application/json",
        }
        if self.instance_id:
            headers["X-Sender-Instance-ID"] = self.instance_id
            headers["X-Sender-Instance-Token"] = self.instance_token
        if origin_instance_id and origin_instance_id != self.instance_id:
            headers["X-LogHill-Origin-Instance-ID"] = origin_instance_id

        try:
            encoded_payload = json.dumps(
                payload,
                ensure_ascii=False,
                default=str,
            ).encode("utf-8")
        except Exception as error:
            raise _RequestFailure(
                f"não foi possível converter o log para JSON: {error}",
                retryable=False,
            ) from None

        request = urllib.request.Request(
            self.api_url + path,
            encoded_payload,
            headers,
            method="POST",
        )

        try:
            with urllib.request.urlopen(request, timeout=float(self.timeout)) as response:
                body = response.read()
        except urllib.error.HTTPError as error:
            raise _RequestFailure(
                _http_error_message(error),
                retryable=_is_retryable_http_status(error.code),
            ) from None
        except urllib.error.URLError as error:
            raise _RequestFailure(_friendly_error(error), retryable=True) from None
        except (
            TimeoutError,
            socket.timeout,
            ConnectionRefusedError,
            ConnectionResetError,
            ConnectionAbortedError,
            BrokenPipeError,
            socket.gaierror,
            OSError,
        ) as error:
            raise _RequestFailure(_friendly_error(error), retryable=True) from None
        except Exception as error:
            raise _RequestFailure(_friendly_error(error), retryable=False) from None

        if not body:
            return {}

        try:
            decoded = body.decode("utf-8")
            parsed = json.loads(decoded)
        except Exception as error:
            raise _RequestFailure(_friendly_error(error), retryable=False) from None

        if not isinstance(parsed, dict):
            raise _RequestFailure(
                "a API retornou JSON, mas o conteúdo não é um objeto.",
                retryable=False,
            )
        return parsed

    def _disable_remote(self, reason: str) -> None:
        self._remote_enabled = False
        self._restore_system_log_capture(flush_pending=False)
        self._disabled_reason = reason
        self._stop.set()
        self._queue_event.set()
        self._report_error(
            f"{reason} Os logs continuarão aparecendo no terminal. "
            "Os registros já persistidos continuarão guardados para a próxima inicialização."
        )

    def _report_started(self) -> None:
        """Mostra o banner somente quando a API inicializar a instância com sucesso."""
        try:
            print(_STARTUP_BANNER, file=self._terminal_stdout, flush=True)
            print("Inicializado com sucesso!\n", file=self._terminal_stdout, flush=True)
        except Exception:
            pass

    def _report_status(self, message: str) -> None:
        """Exibe mudanças de estado, permitindo que ocorram novamente no futuro."""
        try:
            print(f"[LogHill] {str(message).strip()}", file=self._terminal_stderr, flush=True)
        except Exception:
            pass

    def _report_error(self, message: str) -> None:
        try:
            message = str(message).strip() or "ocorreu uma falha interna no cliente LogHill."
            with self._report_lock:
                if message in self._reported_errors:
                    return
                self._reported_errors.add(message)
            print(f"[LogHill] {message}", file=self._terminal_stderr, flush=True)
        except Exception:
            pass

    @staticmethod
    def _severity(level: int) -> str:
        try:
            for minimum, name in (
                (logging.CRITICAL, "FATAL"),
                (logging.ERROR, "ERROR"),
                (logging.WARNING, "WARN"),
                (logging.INFO, "INFO"),
                (logging.DEBUG, "DEBUG"),
            ):
                if level >= minimum:
                    return name
        except Exception:
            pass
        return "TRACE"


def _build_logger(**kwargs: Any) -> LogHillLogger:
    """Retorna um logger pronto; falhas do LogHill nunca escapam como traceback."""
    try:
        return LogHillLogger(**kwargs)
    except Exception as error:
        try:
            name = str(kwargs.get("name", "loghill"))
            level = kwargs.get("level", logging.DEBUG)
            logger = LogHillLogger(
                name=name,
                level=level,
                console=bool(kwargs.get("console", True)),
            )
            logger._disable_remote(f"não foi possível criar o logger: {_friendly_error(error)}")
            return logger
        except Exception:
            logger = LogHillLogger.__new__(LogHillLogger)
            logging.Logger.__init__(logger, "loghill", logging.DEBUG)
            logger.propagate = False
            handler = logging.StreamHandler(sys.stdout)
            handler.setFormatter(
                logging.Formatter("%(asctime)s [%(levelname)-8s] %(message)s")
            )
            logger.addHandler(handler)
            logger.api_url = ""
            logger.sender_name = ""
            logger.healthcheck_interval = 60.0
            logger.timeout = 10.0
            logger.retry_attempts = 3
            logger.retry_interval = 5.0
            logger.shutdown_flush_timeout = 2.0
            logger.sender_id = ""
            logger.instance_id = ""
            logger.instance_token = ""
            logger._closed = False
            logger._remote_enabled = False
            logger._disabled_reason = "o cliente LogHill não pôde ser inicializado."
            logger._reported_errors = set()
            logger._report_lock = threading.Lock()
            logger._instance_lock = threading.Lock()
            logger._queue_lock = threading.Lock()
            logger._stop = threading.Event()
            logger._queue_event = threading.Event()
            logger._queue_drained = threading.Event()
            logger._queue_drained.set()
            logger._health_thread = None
            logger._worker_thread = None
            logger._previous_sys_excepthook = None
            logger._previous_threading_excepthook = None
            logger._sys_excepthook = None
            logger._threading_excepthook = None
            logger._queue_path = None
            logger._persistent_queue_enabled = False
            logger._memory_queue = deque()
            logger._capture_system_logs = False
            logger._original_stdout = sys.stdout
            logger._original_stderr = sys.stderr
            logger._stdout_capture = None
            logger._stderr_capture = None
            logger._stdout_fd_capture = None
            logger._stderr_fd_capture = None
            logger._terminal_stdout = logger._original_stdout
            logger._terminal_stderr = logger._original_stderr
            logger._retargeted_stream_handlers = []
            logger._report_error(
                "o cliente LogHill não pôde ser inicializado. Usando somente o terminal."
            )
            return logger


_instrument_lock = threading.RLock()
_instrumented_logger: LogHillLogger | None = None
_instrumented_pid: int | None = None


def instrument(**kwargs: Any) -> LogHillLogger:
    """Instrumenta stdout/stderr uma única vez por processo e retorna o logger."""
    global _instrumented_logger, _instrumented_pid

    process_id = os.getpid()
    with _instrument_lock:
        current = _instrumented_logger
        if (
            current is not None
            and _instrumented_pid == process_id
            and not current._closed
        ):
            return current

        if current is not None and _instrumented_pid != process_id:
            # Depois de um fork, as threads do processo pai não existem no filho.
            # Restaura os descritores herdados antes de criar a instrumentação local.
            try:
                current._restore_system_log_capture(flush_pending=False)
                current._closed = True
                current._stop.set()
                current._queue_event.set()
            except Exception:
                pass

        logger = _build_logger(**kwargs)
        _instrumented_logger = logger
        _instrumented_pid = process_id
        return logger


def create_logger(**kwargs: Any) -> LogHillLogger:
    """Alias compatível de :func:`instrument`, com uma instância por processo."""
    return instrument(**kwargs)
