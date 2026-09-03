# Eventos do LogMate

Eventos executam uma ação somente quando o cliente informa explicitamente uma chave no log. As ações disponíveis são monitoramento sem entrega, e-mail, webhook HTTPS e requisição HTTP configurável. As entregas externas compartilham a mesma outbox durável, workers, timeout e política de retry.

## Criar pela interface

1. Configure o Outlook em **Configurações → E-mail**. Sem provider, o evento só pode ser salvo inativo.
2. Abra **Eventos** e clique em **Novo evento**.
3. Informe nome e uma chave estável, como `processamento_finalizado`.
4. Selecione de 1 a 100 senders e configure os campos específicos da ação escolhida.
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
    EventOccurrenceID: "protocolo-ABC-123-finalizado",
    Metadata: map[string]any{"protocolo": "ABC-123"},
})
```

## Idempotência

Use um `event_occurrence_id` estável em toda tentativa de envio do mesmo evento. A chave é escopada ao sender e fica persistida em `data/senders/{sender_id}/event-occurrences.json` junto do fingerprint e do resultado original.

- Mesmo ID e mesmo payload: responde `202` com o `received_at` original, sem gravar outro log, publicar outro SSE ou disparar novamente eventos e monitoramentos.
- Mesmo ID e payload diferente: responde `409 EVENT_OCCURRENCE_CONFLICT`.
- ID diferente ou ausente: cria uma nova ocorrência normalmente.

O fingerprint considera severity normalizada, mensagem, evento, instância de origem, timestamp informado e metadata. A ordem das propriedades de metadata não altera o resultado.

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

O log é persistido e publicado no SSE antes do matching. O endpoint persiste a notificação na outbox e responde sem aguardar o Outlook. Outbox cheia ou falha de entrega atualiza o resultado sanitizado do evento, sem apagar ou rejeitar o log. As pendências ficam em `data/outbox/notifications.json`; leases permitem retomá-las depois de reinício ou interrupção de um worker.

### Webhook

Defina `action_type: "webhook"` e `webhook_url` para receber um `POST` com `event_occurrence_id`, definição do evento, sender, log e horário de entrega. A URL deve usar HTTPS, não pode conter usuário/senha e é validada novamente no envio. O cliente bloqueia redirects e destinos loopback, privados, link-local ou multicast, inclusive depois da resolução DNS.

O endpoint deve responder com qualquer status `2xx`. Falhas de rede e respostas fora dessa faixa seguem a política normal de retry e registram apenas um erro sanitizado, sem expor a URL nos logs.

### Requisição HTTP

Defina `action_type: "http"` e `http_request` com `method`, `url`, `headers`, `cookies` e `body`. São aceitos `GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE`, `CONNECT`, `OPTIONS` e `TRACE`. A URL precisa ser HTTPS pública; redirects e destinos privados continuam bloqueados.

O evento apenas enfileira a chamada. O worker envia a requisição e fecha a resposta assim que recebe os cabeçalhos: nenhum corpo de resposta é baixado ou interpretado e o status retornado não altera o resultado da entrega. Body, valores de headers e valores de cookies aceitam as mesmas variáveis dos templates de evento.

Headers e cookies são persistidos em `data/events.json` e na outbox enquanto a chamada estiver pendente. Use permissões restritas no `DATA_DIR`, prefira credenciais de escopo mínimo e faça rotação periódica.

## Limitações da primeira versão

- Teams, Slack e comandos locais ainda não estão disponíveis.
- A outbox oferece entrega pelo menos uma vez; uma queda depois do envio externo e antes da confirmação local pode duplicar a mensagem.
