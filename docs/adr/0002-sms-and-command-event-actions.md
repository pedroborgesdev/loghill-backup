# ADR 0002: ações SMS e comandos locais

- Status: aceito para implementação
- Data: 2026-09-02

## Contexto

Eventos já executam e-mail e webhook por uma outbox JSON durável. O roadmap inclui SMS e comandos locais, mas ambos introduzem novas fronteiras de confiança: credenciais de um provedor externo, números de telefone controlados pelo usuário e execução de processos no host.

## Decisão

### SMS

A primeira integração será **Twilio Programmable Messaging**, sem SDK adicional. O worker fará `POST` HTTPS na API REST oficial usando a biblioteca HTTP padrão. A configuração será exclusivamente por ambiente:

- `TWILIO_SMS_ENABLED` (padrão `false`);
- `TWILIO_ACCOUNT_SID`;
- `TWILIO_AUTH_TOKEN`;
- `TWILIO_FROM_NUMBER`, em E.164;
- `TWILIO_API_BASE_URL`, fixo em produção e substituível somente para testes do backend.

O JSON do evento armazenará somente `phone_numbers` e `sms_template`; credenciais nunca serão gravadas em `events.json`, na outbox, nas respostas da API ou nos logs. Destinatários devem estar em E.164, sem nomes, extensões ou números curtos. O texto renderizado terá no máximo 1.600 caracteres. Erros públicos serão sanitizados e não incluirão token, corpo do provedor ou números completos.

A ação terá `action_type: "sms"` e usará a mesma outbox, leases, retentativas e métricas das demais notificações. A entrega continua sendo pelo menos uma vez: uma interrupção após o aceite da Twilio e antes do `Complete` local pode duplicar o SMS.

### Comandos locais

A API nunca aceitará executável, caminho, shell, argumentos livres, diretório de trabalho ou variáveis de ambiente. Um evento armazenará apenas:

```json
{
  "action_type": "command",
  "command_id": "restart-payment-worker"
}
```

Os comandos permitidos serão definidos pelo operador em `DATA_DIR/action-commands.json`, sem endpoint de escrita:

```json
{
  "version": 1,
  "commands": [
    {
      "id": "restart-payment-worker",
      "executable": "/usr/local/bin/restart-payment-worker",
      "args": ["--source", "loghill"],
      "timeout_seconds": 15
    }
  ]
}
```

Política obrigatória do executor:

- abrir e validar o arquivo com o mesmo lock compartilhado e a mesma disciplina de leitura dos demais JSONs;
- exigir IDs únicos, executável absoluto e arquivo regular; rejeitar caminhos relativos;
- executar diretamente com `exec.CommandContext`, nunca por shell (`sh`, `bash`, `cmd`, PowerShell ou equivalentes);
- usar somente argumentos literais definidos na allowlist; nenhum campo do log será interpolado na linha de comando;
- enviar o evento renderizado como JSON pelo `stdin`, limitado ao tamanho global de payload;
- iniciar com ambiente mínimo e explícito (`PATH` não herdado); não repassar o ambiente completo do servidor;
- aplicar timeout entre 1 e 300 segundos e capturar no máximo 64 KiB combinados de stdout/stderr;
- registrar apenas status, duração, `command_id` e erro sanitizado; saída não será persistida por padrão;
- recusar executáveis que sejam shells ou interpretadores genéricos;
- desabilitar toda a ação por padrão por meio de `LOCAL_COMMANDS_ENABLED=false`.

Comandos também passam pela outbox. Como efeitos locais podem não ser idempotentes, cada programa permitido receberá no `stdin` o `event_occurrence_id` e deverá deduplicá-lo se o efeito exigir execução exatamente uma vez.

## Compatibilidade com réplicas

Todas as réplicas que consomem a mesma outbox devem ter a mesma versão da allowlist e os mesmos executáveis. O startup calculará um hash da configuração validada para facilitar diagnóstico. Operações com configuração divergente não têm garantia de comportamento uniforme.

## Consequências

- Não é necessário banco de dados nem nova dependência Go.
- SMS fica acoplado a um provedor inicial, mas uma interface interna permitirá outros provedores depois.
- A allowlist reduz injeção de comandos e escalada via API, mas executar processos continua sendo uma capacidade privilegiada e deve permanecer desabilitada em instalações que não a utilizam.
- SMS e comandos reutilizam a durabilidade e a política de retry atuais, inclusive a possibilidade documentada de entrega duplicada.
