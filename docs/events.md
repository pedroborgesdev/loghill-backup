# Eventos do LogHill

Eventos executam uma ação somente quando o cliente informa explicitamente uma chave no log. Nesta versão, a única ação é o envio de e-mail pelo mesmo Outlook, fila, workers, retry e template base usados pelos alertas.

## Criar pela interface

1. Configure o Outlook em **Configurações → E-mail**. Sem provider, o evento só pode ser salvo inativo.
2. Abra **Eventos** e clique em **Novo evento**.
3. Informe nome e uma chave estável, como `processamento_finalizado`.
4. Selecione de 1 a 100 senders, de 1 a 20 destinatários e configure assunto e mensagem.
5. Revise, ative e salve.

A chave aceita de 3 a 80 caracteres no formato `^[a-z0-9][a-z0-9_-]{2,79}$` e é imutável após a criação.

## Chamar pelo cliente

```json
{
  "sender": "automacao-financeira",
  "severity": "INFO",
  "message": "Mensagem enviada com sucesso",
  "event": "envia_email_sucesso",
  "metadata": {
    "destinatario": "cliente@empresa.com",
    "protocolo": "ABC-123"
  }
}
```

O matching é `evento ativo + chave exata + sender associado`. A severity não participa. Um mesmo log pode disparar um alerta e um evento, gerando duas mensagens distintas. Evento ausente, desconhecido, inativo ou não associado não rejeita o log.

Python, usando o cliente de exemplo:

```python
log.info(
    "Mensagem enviada com sucesso",
    event="envia_email_sucesso",
    metadata={
        "destinatario": "cliente@empresa.com",
        "protocolo": "ABC-123",
    },
)
```

Go:

```go
type LogOptions struct {
    Event             string
    EventOccurrenceID string
    Metadata          map[string]any
}

err := logClient.Info(ctx, "Mensagem enviada com sucesso", LogOptions{
    Event: "envia_email_sucesso",
    Metadata: map[string]any{"protocolo": "ABC-123"},
})
```

## Templates

Variáveis disponíveis:

- `{{event.key}}`, `{{event.name}}`
- `{{sender.id}}`, `{{sender.name}}`, `{{sender.status}}`
- `{{log.message}}`, `{{log.severity}}`, `{{log.timestamp}}`
- `{{metadata.chave}}`, substituindo `chave` por uma propriedade do objeto `metadata`
- `{{app.public_url}}`

Variáveis ausentes renderizam vazio. Assunto, mensagem, dados do log e metadata são tratados como texto e escapados no HTML. Não há HTML livre, funções, loops, arquivos, variáveis de ambiente ou templates externos.

## Testar

No menu de ações ou no drawer do evento, clique em **Enviar teste** e informe um destinatário. A API usa dados fictícios, enfileira a mensagem e não incrementa as execuções reais:

```http
POST /api/v1/events/{eventID}/test
Content-Type: application/json

{"recipient":"usuario@empresa.com"}
```

## Persistência e execução

As definições ficam em `data/events.json`, gravadas com arquivo temporário, `Sync`, fechamento e rename atômico. Na inicialização, duplicidades e registros inválidos impedem uma restauração silenciosamente corrompida. Um índice `sender_id → event_key → event_ids` é reconstruído e atualizado no CRUD.

O log é persistido e publicado no SSE antes do matching. O endpoint apenas tenta enfileirar notificações e responde sem aguardar o Outlook. Fila cheia ou falha de entrega atualiza o resultado sanitizado do evento, sem apagar ou rejeitar o log.

## Limitações da primeira versão

- Apenas a ação `email` e o provider Outlook.
- A fila é mantida em memória e tarefas pendentes não sobrevivem ao reinício.
- `event_occurrence_id` já é persistido, mas a deduplicação ainda não está ativa; requisições repetidas podem gerar novos envios.
- Não há webhook, SMS, Teams, Slack, comandos ou HTTP genérico.
