"""Logger Python resiliente para o LogHill, sem dependências externas.

Falhas internas do cliente nunca interrompem a aplicação nem exibem traceback.
Quando a API não puder ser usada, os logs continuam aparecendo no terminal.
"""

from __future__ import annotations

import atexit
import json
import logging
import os
import re
import socket
import sys
import threading
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Mapping


_EVENT_RE = re.compile(r"^[a-z0-9][a-z0-9_-]{2,79}$")
_KEY_RE = re.compile(r"^snd_[A-Za-z0-9_-]+$")
_URL_ENVS = ("LOGHILL_API_URL", "LOG_API_URL")
_KEY_ENVS = ("LOGHILL_SENDER_KEY", "LOGHILL_SENDER_ID", "LOG_SENDER_KEY")

_STARTUP_BANNER = r"""
 __            _____ _ _ _ 
|  |   ___ ___|  |  |_| | |
|  |__| . | . |     | | | |
|_____|___|_  |__|__|_|_|_|
          |___|             
""".strip("\n")


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
    sender_key: str | None,
    env_file: str | Path | None,
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
    key = sender_key or _env(*_KEY_ENVS)

    if not url:
        raise ValueError("LOGHILL_API_URL não foi configurada no ambiente ou no arquivo .env.")
    if not url.startswith(("http://", "https://")):
        raise ValueError("LOGHILL_API_URL deve começar com http:// ou https://.")
    if not key:
        raise ValueError("LOGHILL_SENDER_KEY não foi configurada no ambiente ou no arquivo .env.")
    if not _KEY_RE.fullmatch(key):
        raise ValueError("LOGHILL_SENDER_KEY é inválida: a chave deve começar com 'snd_'.")

    return url, key


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
    if error.code == 401 or code == "INVALID_SENDER_KEY":
        return (
            "a chave LOGHILL_SENDER_KEY é inválida ou não pertence a este sender. "
            "Confira a chave no arquivo .env."
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
        return f"ocorreu uma falha de rede ou do sistema operacional: {text}" if text else "ocorreu uma falha de rede ou do sistema operacional."

    text = str(error).strip()
    return text or f"ocorreu uma falha interna no cliente LogHill ({type(error).__name__})."


class LogHillLogger(logging.Logger):
    """Um único objeto para console, API e healthcheck do LogHill."""

    def __init__(
        self,
        name: str = "loghill",
        *,
        api_url: str | None = None,
        sender_key: str | None = None,
        env_file: str | Path | None = None,
        level: int = logging.DEBUG,
        console: bool = True,
        healthcheck_interval: float = 60.0,
        timeout: float = 10.0,
    ) -> None:
        super().__init__(name, level)
        self.api_url = ""
        self.sender_key = ""
        self.healthcheck_interval = healthcheck_interval
        self.timeout = timeout
        self.sender_id = ""
        self.instance_id = ""
        self.propagate = False

        self._closed = False
        self._remote_enabled = True
        self._disabled_reason = ""
        self._reported_errors: set[str] = set()
        self._lock = threading.Lock()
        self._stop = threading.Event()
        self._health_thread: threading.Thread | None = None

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
                self._report_error(f"não foi possível configurar a saída do terminal: {_friendly_error(error)}")

        try:
            self.api_url, self.sender_key = _config(api_url, sender_key, env_file)
            self._init_instance()
        except Exception as error:
            self._disable_remote(_friendly_error(error))

        try:
            atexit.register(self.close)
        except Exception:
            pass

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
                self._report_error("metadata inválida: informe um dicionário ou outro Mapping. O campo foi ignorado.")

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
        try:
            validation_error = _validation_error(event, event_occurrence_id)
            if validation_error:
                self._report_error(validation_error + " O campo inválido foi ignorado.")
                event = None
                event_occurrence_id = None

            if not self._remote_enabled:
                return {"sent": False, "reason": self._disabled_reason}

            self._init_instance()

            payload: dict[str, Any] = {
                "sender": self.sender_id,
                "severity": str(severity).upper(),
                "message": str(message),
                "timestamp": (timestamp or datetime.now(timezone.utc)).isoformat(),
                "metadata": dict(metadata or {}),
            }
            if event:
                payload["event"] = event
            if event_occurrence_id:
                payload["event_occurrence_id"] = event_occurrence_id

            return self._post("/api/v1/logs", payload)
        except Exception as error:
            reason = _friendly_error(error)
            self._report_error(f"log não enviado: {reason}")
            return {"sent": False, "reason": reason}

    def close(self) -> None:
        try:
            if self._closed:
                return
            self._closed = True
            self._stop.set()
            if (
                self._health_thread
                and self._health_thread.is_alive()
                and threading.current_thread() is not self._health_thread
            ):
                self._health_thread.join(timeout=min(float(self.timeout), 2.0))
        except Exception as error:
            self._report_error(f"não foi possível finalizar o logger corretamente: {_friendly_error(error)}")

    def _init_instance(self) -> None:
        if self.instance_id or not self._remote_enabled:
            return
        if self._closed:
            raise RuntimeError("o logger já foi fechado.")

        with self._lock:
            if self.instance_id:
                return

            data = self._post("/api/v1/instances/init", {})
            if not isinstance(data, dict):
                raise RuntimeError("a API retornou uma resposta inválida ao inicializar o logger.")

            instance_id = data.get("instance_id")
            sender_id = data.get("sender")
            if not instance_id or not sender_id:
                raise RuntimeError("a API não retornou instance_id e sender ao inicializar o logger.")

            self.instance_id = str(instance_id)
            self.sender_id = str(sender_id)
            self._health_thread = threading.Thread(
                target=self._health_loop,
                name=f"loghill-health-{self.sender_id}",
                daemon=True,
            )
            self._health_thread.start()
            self._report_started()

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
            self._report_error(f"log não enviado: {_friendly_error(error)}")

    def _health_loop(self) -> None:
        try:
            interval = max(float(self.healthcheck_interval), 1.0)
        except Exception:
            interval = 60.0
            self._report_error("healthcheck_interval inválido; usando 60 segundos.")

        while not self._stop.wait(interval):
            try:
                self._post(
                    f"/api/v1/senders/{self.sender_id}/health",
                    {"status": "healthy", "details": {"client": "python-logging"}},
                )
            except Exception as error:
                self._report_error(f"healthcheck não enviado: {_friendly_error(error)}")

    def _post(self, path: str, payload: Mapping[str, Any]) -> dict[str, Any]:
        try:
            headers = {
                "Accept": "application/json",
                "Content-Type": "application/json",
                "X-Sender-Key": self.sender_key,
            }
            if self.instance_id:
                headers["X-Sender-Instance-ID"] = self.instance_id

            try:
                encoded_payload = json.dumps(payload, ensure_ascii=False).encode("utf-8")
            except Exception as error:
                raise ValueError(f"não foi possível converter o log para JSON: {error}") from None

            request = urllib.request.Request(
                self.api_url + path,
                encoded_payload,
                headers,
                method="POST",
            )

            with urllib.request.urlopen(request, timeout=float(self.timeout)) as response:
                body = response.read()

            if not body:
                return {}

            decoded = body.decode("utf-8")
            parsed = json.loads(decoded)
            if not isinstance(parsed, dict):
                raise ValueError("a API retornou JSON, mas o conteúdo não é um objeto.")
            return parsed

        except Exception as error:
            # O "from None" impede que Python monte uma cadeia de exceções.
            raise RuntimeError(_friendly_error(error)) from None

    def _disable_remote(self, reason: str) -> None:
        self._remote_enabled = False
        self._disabled_reason = reason
        self._report_error(
            f"{reason} Os logs continuarão aparecendo no terminal, mas não serão enviados à API."
        )

    def _report_started(self) -> None:
        """Mostra o banner somente quando a API inicializar a instância com sucesso."""
        try:
            print(
                """
 __            _____ _ _ _ 
|  |   ___ ___|  |  |_| | |
|  |__| . | . |     | | | |
|_____|___|_  |__|__|_|_|_|
          |___|             
""",
                file=sys.stdout,
                flush=True,
            )
            print("Inicializado com sucesso!\n", file=sys.stdout, flush=True)
        except Exception:
            # A mensagem visual nunca pode interromper a aplicação.
            pass

    def _report_error(self, message: str) -> None:
        try:
            message = str(message).strip() or "ocorreu uma falha interna no cliente LogHill."
            if message in self._reported_errors:
                return
            self._reported_errors.add(message)
            print(f"[LogHill] {message}", file=sys.stderr)
        except Exception:
            # Nunca permite que o próprio tratamento de erro gere outro erro.
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


def create_logger(**kwargs: Any) -> LogHillLogger:
    """Retorna um logger pronto; falhas do LogHill nunca escapam como traceback."""
    try:
        return LogHillLogger(**kwargs)
    except Exception as error:
        # Proteção final inclusive para argumentos inválidos passados ao construtor.
        try:
            name = str(kwargs.get("name", "loghill"))
            level = kwargs.get("level", logging.DEBUG)
            logger = LogHillLogger(name=name, level=level, console=bool(kwargs.get("console", True)))
            logger._disable_remote(f"não foi possível criar o logger: {_friendly_error(error)}")
            return logger
        except Exception:
            # Último fallback: logger padrão apenas para terminal.
            logger = LogHillLogger.__new__(LogHillLogger)
            logging.Logger.__init__(logger, "loghill", logging.DEBUG)
            logger.propagate = False
            handler = logging.StreamHandler(sys.stdout)
            handler.setFormatter(logging.Formatter("%(asctime)s [%(levelname)-8s] %(message)s"))
            logger.addHandler(handler)
            logger.api_url = ""
            logger.sender_key = ""
            logger.healthcheck_interval = 60.0
            logger.timeout = 10.0
            logger.sender_id = ""
            logger.instance_id = ""
            logger._closed = False
            logger._remote_enabled = False
            logger._disabled_reason = "o cliente LogHill não pôde ser inicializado."
            logger._reported_errors = set()
            logger._lock = threading.Lock()
            logger._stop = threading.Event()
            logger._health_thread = None
            logger._report_error("o cliente LogHill não pôde ser inicializado. Usando somente o terminal.")
            return logger