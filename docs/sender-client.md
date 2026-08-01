# Cliente de logs do LogHill

O cliente não cria senders. Antes do deploy, um administrador deve abrir o dashboard ou `/senders`, selecionar **Novo sender** e copiar o ID e a chave exibida uma única vez.

Guarde a chave em um cofre de secrets. Não a coloque em código-fonte, URL, query string, logs, `localStorage` ou arquivos versionados.

## Variáveis de ambiente

```env
LOG_API_URL=http://localhost:8080
LOG_SENDER_ID=automacao-financeira
LOG_SENDER_KEY=snd_chave_copiada_na_interface
```

## Requisições

Logs usam `POST /api/v1/logs`; healthchecks usam `POST /api/v1/senders/{sender}/health`. Ambos exigem a mesma chave em `X-Sender-Key`.

O campo opcional `event` chama uma configuração explícita de **Eventos**. Consulte [events.md](events.md) para matching, templates e limites.

```bash
curl -X POST "$LOG_API_URL/api/v1/logs" \
  -H "Content-Type: application/json" \
  -H "X-Sender-Key: $LOG_SENDER_KEY" \
  -d '{"sender":"automacao-financeira","severity":"ERROR","message":"Falha ao processar boleto"}'
```

## Go

```go
type LogClient struct {
    BaseURL   string
    SenderID  string
    SenderKey string
    Client    *http.Client
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
    response, err := client.Client.Do(req)
    if err != nil { return err }
    defer response.Body.Close()
    if response.StatusCode >= 300 { return fmt.Errorf("LogHill HTTP %d", response.StatusCode) }
    return nil
}
```

## Python

O arquivo [`examples/python_log_client.py`](../examples/python_log_client.py) fornece `LogHillHandler`, integrado ao módulo `logging`, e um fluxo com `log.info("teste 1")`. Ele lê as três variáveis acima e também mantém o sender online com healthchecks autenticados.

O mesmo cliente aceita evento e metadata diretamente:

```python
log.info(
    "Processamento concluído",
    event="processamento_finalizado",
    metadata={"protocolo": "ABC-123"},
)
```

## Rotação e reativação

Ao gerar uma nova chave, a anterior deixa de funcionar imediatamente. Atualize `LOG_SENDER_KEY` no cofre e reinicie/recarregue o cliente. Reativar um sender revogado também gera uma chave nova; a chave revogada nunca é restaurada.

Senders criados antes da autenticação por chave mantêm o ID e o diretório originais. Antes de reconectar um cliente legado, use **Gerar nova chave** na interface e configure a credencial gerada; nenhum ID legado é renomeado automaticamente.

Respostas `401 INVALID_SENDER_KEY` não revelam se o sender existe ou se a chave pertence a outro sender.
