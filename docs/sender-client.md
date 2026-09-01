# Cliente de logs do LogHill

O cliente conecta pelo nome do sender e a API o cria automaticamente quando necessário. Configure apenas a URL da API e o nome desejado (ou seu identificador normalizado).

```env
LOGHILL_API_URL=http://localhost:8080
LOGHILL_SENDER_NAME=automacao-financeira
```

Ao usar `instrument(name="automacao-financeira")`, `LOGHILL_SENDER_NAME` é opcional: o nome do logger será usado. `LOGHILL_SENDER_ID` continua aceito como alias de migração.

## Inicialização da instância

Cada processo chama `POST /api/v1/instances/init` com `sender_name`. Se ainda não existir um sender com esse nome, a API o cria automaticamente; não é necessário cadastrá-lo previamente. A API retorna o ID canônico, um `instance_id` e um `instance_token` exclusivo da execução. Somente o hash do token é persistido pelo LogHill.

```bash
curl -X POST "$LOGHILL_API_URL/api/v1/instances/init" \
  -H "Content-Type: application/json" \
  -d '{"sender_name":"automacao-financeira"}'
```

```json
{
  "sender_id": "automacao-financeira",
  "instance_id": "ins_0123456789abcdef0123456789abcdef",
  "instance_token": "inst_credencial_da_execucao",
  "initialized_at": "2026-08-28T12:00:00Z"
}
```

O `sender_name` é usado somente nessa inicialização. Logs seguintes usam o `sender_id` devolvido, junto de `X-Sender-Instance-ID` e `X-Sender-Instance-Token`. O token deve ficar apenas na memória do processo e ser descartado ao encerrar.

```bash
curl -X POST "$LOGHILL_API_URL/api/v1/logs" \
  -H "Content-Type: application/json" \
  -H "X-Sender-Instance-ID: $LOGHILL_INSTANCE_ID" \
  -H "X-Sender-Instance-Token: $LOGHILL_INSTANCE_TOKEN" \
  -d '{"sender_id":"automacao-financeira","severity":"ERROR","message":"Falha ao processar boleto"}'
```

O ID da instância separa os consoles de processos simultâneos. Alertas, eventos e regras de monitoramento continuam associados ao sender.

Uma instância sem logs ou healthchecks passa a inativa após o prazo configurado. Ao terminar a retenção de inativos, a API remove permanentemente a instância e seus logs. Quando a última instância expira, o sender também é removido; uma conexão futura com o mesmo nome cria outro automaticamente.

## Python

```python
from loghill import instrument

log = instrument(name="automacao-financeira")

# Importe Uvicorn, FastAPI e a aplicação somente depois da instrumentação.
import uvicorn

log.info("Processamento concluído")
```

`instrument()` é idempotente: chamadas posteriores no mesmo processo, inclusive por `create_logger()`, reutilizam a mesma conexão e o mesmo `instance_id`.

Também é possível informar o nome explicitamente:

```python
log = instrument(
    name="worker",
    sender_name="automacao-financeira",
    api_url="http://localhost:8080",
)
```

O cliente mantém fila SQLite, healthcheck e captura os descritores reais `stdout`/`stderr`. Isso inclui `print`, handlers de logging, `os.write(1/2, ...)`, Uvicorn, bibliotecas nativas e subprocessos que herdem os descritores. Saídas brutas do terminal recebem severidade `UNDEFINED`. Cada item enfileirado guarda o `sender_id` e o `instance_id` que o originaram, mas nunca o token. Após cada handshake bem-sucedido, todos os registros persistidos de execuções anteriores são apagados antes do worker de envio começar; portanto, eles não são reenviados depois de reiniciar o processo.

## Compatibilidade

As rotas com `X-Sender-Key` continuam disponíveis para clientes antigos e para operações internas. Novas integrações não precisam receber, armazenar ou configurar essa chave.
