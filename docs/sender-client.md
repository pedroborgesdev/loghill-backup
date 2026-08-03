# Cliente de logs do LogHill

O cliente não cria senders. Antes do deploy, um administrador deve abrir o dashboard ou `/senders`, selecionar **Novo sender** e copiar a chave exibida uma única vez. O backend identifica o sender pela própria chave durante o handshake.

Guarde a chave em um cofre de secrets. Não a coloque em código-fonte, URL, query string, logs, `localStorage` ou arquivos versionados.

## Variáveis de ambiente

```env
LOGHILL_API_URL=http://localhost:8080
LOGHILL_SENDER_KEY=snd_chave_copiada_na_interface
```

O cliente Python também aceita `LOGHILL_SENDER_ID` como alias da chave, além dos nomes legados `LOG_API_URL` e `LOG_SENDER_KEY`.

## Requisições

Cada processo cliente deve começar chamando `POST /api/v1/instances/init`. A resposta contém o sender identificado pela chave e um `instance_id` único para aquela execução. Logs e healthchecks enviam esse valor em `X-Sender-Instance-ID`, além da chave normal em `X-Sender-Key`.

O ID da instância existe exclusivamente para separar os consoles de logs de processos simultâneos. Alertas, eventos e regras de monitoramento continuam associados ao sender e não à instância.

```bash
curl -X POST "$LOGHILL_API_URL/api/v1/instances/init" \
  -H "Content-Type: application/json" \
  -H "X-Sender-Key: $LOGHILL_SENDER_KEY" \
  -d '{}'
```

Resposta:

```json
{"sender":"automacao-financeira","instance_id":"ins_0123456789abcdef0123456789abcdef","initialized_at":"2026-08-03T12:00:00Z"}
```

Clientes antigos sem handshake continuam aceitos, mas somente inicializações com handshake possuem isolamento físico por processo.

O campo opcional `event` chama uma configuração explícita de **Eventos**. Consulte [events.md](events.md) para matching, templates e limites.

```bash
curl -X POST "$LOGHILL_API_URL/api/v1/logs" \
  -H "Content-Type: application/json" \
  -H "X-Sender-Key: $LOGHILL_SENDER_KEY" \
  -H "X-Sender-Instance-ID: $LOG_INSTANCE_ID" \
  -d '{"sender":"automacao-financeira","severity":"ERROR","message":"Falha ao processar boleto"}'
```

## Go

```go
type LogClient struct {
    BaseURL   string
    SenderID  string
    SenderKey string
    InstanceID string
    Client    *http.Client
}

func (client *LogClient) Init(ctx context.Context) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        client.BaseURL+"/api/v1/instances/init", bytes.NewReader([]byte(`{}`)))
    if err != nil { return err }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Sender-Key", client.SenderKey)
    response, err := client.Client.Do(req)
    if err != nil { return err }
    defer response.Body.Close()
    var result struct { Sender string `json:"sender"`; InstanceID string `json:"instance_id"` }
    if err = json.NewDecoder(response.Body).Decode(&result); err != nil { return err }
    client.InstanceID = result.InstanceID
    client.SenderID = result.Sender
    return nil
}

func (client *LogClient) Send(ctx context.Context, severity, message string) error {
    payload := map[string]any{
        "sender": client.SenderID, "severity": severity, "message": message,
    }
    body, err := json.Marshal(payload)
    if err != nil { return err }

    req, err := http.NewRequestWithContext(
        ctx, http.MethodPost, client.BaseURL+"/api/v1/logs", bytes.NewReader(body),
    )
    if err != nil { return err }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Sender-Key", client.SenderKey)
    req.Header.Set("X-Sender-Instance-ID", client.InstanceID)
    response, err := client.Client.Do(req)
    if err != nil { return err }
    defer response.Body.Close()
    if response.StatusCode >= 300 { return fmt.Errorf("LogHill HTTP %d", response.StatusCode) }
    return nil
}
```

## Python

- [`examples/python_log_client.py`](../examples/python_log_client.py) — classe `LogHill` (lê `.env`, handshake, healthcheck, integração com `logging`)
- [`examples/simulate_logs.py`](../examples/simulate_logs.py) — script de exemplo que envia logs em loop

```python
from python_log_client import LogHill

with LogHill.from_env() as client:
    log = client.logger()
    log.info(
        "Processamento concluído",
        event="processamento_finalizado",
        metadata={"protocolo": "ABC-123"},
    )
```

Também dá para enviar sem o módulo `logging`:

```python
client = LogHill.from_env()
client.send("Falha ao processar boleto", severity="ERROR")
client.close()
```

Para ver o fluxo completo:

```bash
python examples/simulate_logs.py
```

## Rotação e reativação

Ao gerar uma nova chave, a anterior deixa de funcionar imediatamente. Atualize `LOGHILL_SENDER_KEY` no cofre e reinicie/recarregue o cliente. Reativar um sender revogado também gera uma chave nova; a chave revogada nunca é restaurada.

Senders criados antes da autenticação por chave mantêm o ID e o diretório originais. Antes de reconectar um cliente legado, use **Gerar nova chave** na interface e configure a credencial gerada; nenhum ID legado é renomeado automaticamente.

Respostas `401 INVALID_SENDER_KEY` não revelam se o sender existe ou se a chave pertence a outro sender.
