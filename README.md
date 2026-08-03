# LogHill

Aplicação centralizada para receber, armazenar, consultar e acompanhar logs em tempo real. O backend Go serve a API, o stream SSE e o frontend React incorporado no mesmo executável.

## Arquitetura

O projeto separa responsabilidades em camadas:

- `internal/domain`: entidades, filtros e erros.
- `internal/repository`: persistência atômica de `sender.json` e arquivos JSON Lines.
- `internal/storage`: locks independentes por sender.
- `internal/service`: regras de criação, atividade, inatividade, expiração e stream.
- `internal/handler`: HTTP, validação, erros e fallback da SPA.
- `internal/scheduler`: rotina periódica de manutenção.
- `internal/alerts`: regras e persistência atômica de alertas.
- `internal/events`: CRUD, persistência atômica e índice de eventos por sender/chave.
- `internal/emailconfig`: configuração segura e criptografia opcional da credencial.
- `internal/emailprovider`: autenticação e envio pelo Microsoft Graph.
- `internal/notification`: matching, template e fila assíncrona de entrega.
- `frontend`: React, TypeScript, Vite e Tailwind.
- `web/dist`: build incorporado com `go:embed`.

Cada sender é armazenado em `data/senders/{sender}/`. Seus metadados ficam em `sender.json`, enquanto os logs são separados fisicamente em `instances/{instance-id}/logs.txt`; o `logs.txt` da raiz é mantido apenas para dados legados sem instância.

## Decisões técnicas

- **Limite de armazenamento:** o limite é configurável entre 0 e 10.000 em linhas ou MB e vale integralmente para cada instância. A escrita continua append-only e só compacta após ultrapassar o limite, usando uma margem interna de 5% para evitar reescrita a cada entrada.
- **Inatividade:** log ou healthcheck contam como atividade por padrão. Após cinco minutos, o sender fica `inactive` e preserva o volume configurado em linhas ou MB.
- **Expiração:** depois de sete dias inativo, os arquivos de logs das instâncias são removidos e `sender.json` permanece com status `expired`. Um sender expirado não pode ser reativado.
- **Concorrência:** cada sender possui um `sync.RWMutex`; senders distintos nunca disputam um lock global de escrita.
- **Persistência segura:** metadados e compactações usam arquivo temporário, `Sync`, fechamento e rename.
- **Configuração dinâmica:** `data/config.json` usa o mesmo fluxo atômico e um `RWMutex`. Alterações passam a valer no próximo log ou ciclo de manutenção, sem reinício.
- **Consultas:** os filtros são aplicados no backend. Sem banco, buscas textuais têm custo linear no tamanho do arquivo.
- **Tempo real:** cada subscriber SSE possui buffer limitado. Um consumidor lento perde eventos em vez de bloquear a ingestão.
- **Produção:** o Vite grava em `web/dist`; `embed` incorpora os assets ao binário Go. A imagem final não contém Node.js.

## Requisitos

- Go 1.24+
- Node.js 22+ e npm (somente desenvolvimento/build do frontend)
- Docker e Docker Compose (opcional)

## Desenvolvimento

Backend:

```bash
go mod tidy
go run ./cmd/server
```

Frontend, em outro terminal:

```bash
cd frontend
npm ci
npm run dev
```

O Vite encaminha `/api`, `/health` e `/ready` para `localhost:8080`.

## Build de produção

```bash
cd frontend
npm ci
npm run build
cd ..
go build -o log-theater ./cmd/server
./log-theater
```

A interface e a API ficam disponíveis em `http://localhost:8080`.

## Testes e lint

```bash
go test -race ./...
go vet ./...
cd frontend
npm run test:run
npm run lint
```

## Docker

```bash
docker compose up --build
```

O volume `./data:/app/data` preserva os logs. O Dockerfile tem estágios separados para frontend, backend e runtime sem privilégios.

## API

A especificação está em `docs/openapi.yaml` e a interface Swagger em `/docs`.
O fluxo completo de configuração dos clientes está em [`docs/sender-client.md`](docs/sender-client.md).

| Método | Caminho | Finalidade |
|---|---|---|
| `POST` | `/api/v1/senders` | Criar sender administrativamente e exibir a chave uma vez |
| `GET` | `/api/v1/senders/check-id` | Verificar disponibilidade do ID |
| `PUT/DELETE` | `/api/v1/senders/{sender}` | Editar informações ou excluir sender |
| `POST` | `/api/v1/senders/{sender}/rotate-key` | Gerar uma nova chave |
| `POST` | `/api/v1/senders/{sender}/revoke` | Revogar acesso |
| `POST` | `/api/v1/senders/{sender}/reactivate` | Reativar com nova chave |
| `POST` | `/api/v1/logs` | Receber log |
| `POST` | `/api/v1/senders/{sender}/health` | Registrar healthcheck |
| `GET` | `/api/v1/senders` | Listar senders |
| `GET` | `/api/v1/senders/{sender}` | Detalhar sender |
| `GET` | `/api/v1/senders/{sender}/logs` | Consultar logs |
| `GET` | `/api/v1/senders/{sender}/logs/stream` | Stream SSE |
| `GET` | `/api/v1/senders/{sender}/logs/download` | Exportar JSONL |
| `GET` | `/api/v1/dashboard/summary` | Métricas |
| `GET` | `/api/v1/settings` | Consultar limites atuais |
| `PUT` | `/api/v1/settings` | Persistir e aplicar novos limites |
| `GET/POST` | `/api/v1/alerts` | Listar ou criar alertas |
| `GET/PUT/DELETE` | `/api/v1/alerts/{alertID}` | Consultar, editar ou excluir alerta |
| `PATCH` | `/api/v1/alerts/{alertID}/status` | Ativar ou desativar alerta |
| `POST` | `/api/v1/alerts/{alertID}/test` | Enfileirar teste do alerta |
| `GET/POST` | `/api/v1/events` | Listar ou criar eventos |
| `GET/PUT/DELETE` | `/api/v1/events/{eventID}` | Consultar, editar ou excluir evento |
| `PATCH` | `/api/v1/events/{eventID}/status` | Ativar ou desativar evento |
| `POST` | `/api/v1/events/{eventID}/test` | Enfileirar teste do evento |
| `GET/PUT` | `/api/v1/settings/email` | Consultar ou configurar Outlook |
| `POST` | `/api/v1/settings/email/test-connection` | Validar autenticação O365 |
| `POST` | `/api/v1/settings/email/send-test` | Enviar e-mail de teste |
| `GET` | `/health` | Liveness |
| `GET` | `/ready` | Readiness |

Criar um sender:

```bash
curl -X POST http://localhost:8080/api/v1/senders \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $APP_PASSWORD" \
  -d '{"name":"Automação Teste","description":"Processamento de exemplo"}'
```

Enviar um log:

```bash
curl -X POST http://localhost:8080/api/v1/logs \
  -H "Content-Type: application/json" \
  -H "X-Sender-Key: $LOG_SENDER_KEY" \
  -d '{"sender":"automacao-teste","severity":"ERROR","message":"Falha no login","metadata":{"step":"login"}}'
```

Enviar um log que chama um evento explicitamente:

```bash
curl -X POST http://localhost:8080/api/v1/logs \
  -H "Content-Type: application/json" \
  -H "X-Sender-Key: $LOG_SENDER_KEY" \
  -d '{"sender":"automacao-teste","severity":"INFO","message":"Processamento concluído","event":"processamento_finalizado","metadata":{"protocolo":"ABC-123"}}'
```

Consultar:

```bash
curl "http://localhost:8080/api/v1/senders/automacao-teste/logs?severity=ERROR,WARN&search=login&page=1&page_size=100&order=desc"
```

## Configuração

A infraestrutura é configurada por variáveis de ambiente e não depende de arquivo `.env`. Os valores editáveis da interface são persistidos em `data/config.json`; esse arquivo é criado automaticamente com 10.000 linhas de limite máximo, preservação de 2.000 linhas, inatividade após 300 segundos e exclusão após 7 dias.

As unidades internas aceitas são `lines` e `mb`, considerando `1 MB = 1024 × 1024 bytes`. O valor `0` desativa o limite máximo; para preservação, `0` esvazia o arquivo quando o sender se torna inativo. Em MB, a leitura ocorre a partir do fim e somente entradas JSON Lines completas são mantidas. Quando ambas as opções usam a mesma unidade, a preservação não pode superar o limite máximo, exceto quando o máximo é `0`.

As rotas de configuração são administrativas e exigem sessão autenticada (ou `X-API-Key` com `APP_PASSWORD`) quando a autenticação está habilitada. Consulte [`.env.example`](.env.example) para as variáveis de infraestrutura. O arquivo `.env` na raiz é carregado automaticamente na subida do servidor.

As principais variáveis são:

| Variável | Padrão | Descrição |
|---|---:|---|
| `APP_PORT` | `8080` | Porta HTTP |
| `DATA_DIR` | `./data` | Diretório persistente |
| `MAX_LOG_LINES` | `100000` | Limite do arquivo |
| `LOG_COMPACT_TARGET_LINES` | `95000` | Alvo após exceder o limite |
| `INACTIVE_AFTER` | `5m` | Prazo de inatividade |
| `COMPACT_KEEP_LINES` | `2000` | Linhas mantidas ao inativar |
| `DELETE_AFTER` | `168h` | Prazo para expiração |
| `APP_PASSWORD` | _(vazio)_ | Senha da interface. Se definida, o login é exigido |
| `APP_AUTH_ENABLED` | _(auto)_ | Força auth on/off; por padrão segue `APP_PASSWORD` |
| `EMAIL_SETTINGS_ENCRYPTION_KEY` | auto (`DATA_DIR/email-encryption.key`) | Base64 de 32 bytes para secrets salvos pela UI. Se vazio/inválido, é gerada e persistida automaticamente |

Logs de ingestão e healthchecks de sender usam `X-Sender-Key`, gerada no cadastro administrativo. A chave completa não é persistida e só aparece na criação, rotação ou reativação. A interface autentica com cookie de sessão (`APP_PASSWORD`); clientes não-browser podem enviar `X-API-Key` com a mesma senha.

## Alertas por e-mail e Outlook

Os alertas são persistidos separadamente em `data/alerts.json`. Cada regra aceita `sender_ids` com um ou mais senders; o matching usa um índice em memória por sender e severity. Regras antigas com `sender_id` são migradas durante a leitura sem renomear senders ou diretórios existentes.

### Referência O365 analisada

A integração de referência está em `repo_exemplo/worker_busca_acordos_santander/worker/infra/email_client.py`, apoiada por `worker/core/config.py`, `worker/schemas/config.py` e `tests/test_email_client.py`. Ela usa `O365.Account((client_id, client_secret), auth_flow_type="credentials", tenant_id=...)`, com as variáveis `EMAIL_PROVIDER=o365`, `O365_TENANT_ID`, `O365_CLIENT_ID`, `O365_CLIENT_SECRET` e `EMAIL_FROM_ADDR`/`EMAIL_USER`. A biblioteca O365 obtém e renova o token do fluxo client credentials.

O cliente de exemplo é usado para ler a caixa postal durante autenticação de dois fatores e não possui método de envio, timeout de envio ou retry de entrega que pudesse ser importado diretamente. Por isso, esta aplicação Go preserva os nomes, o fluxo e o contrato de credenciais, mas implementa o transporte equivalente no próprio processo pelo Microsoft Graph, sem sidecar Python e sem duplicar autenticação dentro do projeto atual.

O envio usa Microsoft Graph com OAuth 2.0 client credentials e a permissão de aplicação `Mail.Send`. Configure um registro de aplicativo no Microsoft Entra ID, conceda consentimento administrativo a essa permissão e informe:

```env
EMAIL_PROVIDER=outlook
OUTLOOK_ENABLED=true
OUTLOOK_TENANT_ID=seu-tenant
OUTLOOK_CLIENT_ID=seu-client-id
OUTLOOK_CLIENT_SECRET=seu-secret
OUTLOOK_SENDER_EMAIL=logs@empresa.com
OUTLOOK_SENDER_NAME=LogHill
APP_PUBLIC_URL=https://logs.empresa.com
```

Também são aceitos os nomes legados do repositório de referência: `O365_TENANT_ID`, `O365_CLIENT_ID`, `O365_CLIENT_SECRET` e `EMAIL_FROM_ADDR`. Os nomes `OUTLOOK_*` têm prioridade. O access token permanece somente em memória e é renovado automaticamente antes de expirar.

Quando a credencial é informada pela interface, o secret é persistido com AES-256-GCM em `data/email-settings.json`. A chave de criptografia vem de `EMAIL_SETTINGS_ENCRYPTION_KEY` ou, se estiver vazia/inválida, é gerada automaticamente em `DATA_DIR/email-encryption.key`. A chave nunca é salva no arquivo de settings e a API devolve apenas `client_secret_configured`. Se a configuração vier do ambiente, a interface a identifica como gerenciada e impede alterações.

O endpoint de logs apenas tenta colocar a notificação em uma fila limitada e retorna sem aguardar o Outlook. Workers aplicam timeout e retry em background. A fila é somente em memória: tarefas pendentes não sobrevivem a um reinício. Tamanho, workers e retry são controlados por `EMAIL_ALERT_QUEUE_SIZE`, `EMAIL_ALERT_WORKERS`, `EMAIL_ALERT_SEND_TIMEOUT`, `EMAIL_ALERT_MAX_RETRIES` e `EMAIL_ALERT_RETRY_INTERVAL`.

Na interface, abra **Configurações > E-mail**, salve ou consulte a configuração, use **Testar conexão** para validar o token e **Enviar e-mail de teste** para validar a entrega. Depois, acesse **Alertas**, clique em **Novo alerta**, escolha sender, severidades e destinatários.

Se o Microsoft Graph responder HTTP 403, as credenciais foram aceitas, mas o aplicativo ou a mailbox não estão autorizados a enviar. Conceda a permissão de aplicação `Mail.Send` com consentimento administrativo e confira o escopo de Application RBAC/política de acesso do Exchange. Consulte [a solução de problemas](docs/alerts.md#solução-do-erro-http-403).

## Eventos explícitos

Eventos são persistidos em `data/events.json` e usam matching por sender e chave exata, sem considerar severity. O endpoint de logs persiste e publica o registro antes de enfileirar a ação, então falhas ou fila cheia não rejeitam o log. Alertas e eventos permanecem independentes e podem disparar juntos.

Abra **Eventos**, crie uma chave imutável, selecione senders e configure destinatários, assunto e mensagem. Consulte [a documentação de eventos](docs/events.md) para variáveis de template, exemplos de Python/Go, envio de teste e limitações da primeira versão.

## Estrutura

```text
cmd/server/             ponto de entrada
internal/config/        variáveis de ambiente
internal/domain/        modelos e filtros
internal/repository/    persistência em arquivos
internal/service/       regras de negócio e SSE
internal/handler/       API e SPA
internal/middleware/    segurança HTTP
internal/scheduler/     inatividade e expiração
internal/storage/       locks por sender
frontend/src/           aplicação React
web/dist/               assets incorporados
docs/openapi.yaml       contrato da API
```

## Limitações conhecidas

- Busca, métricas detalhadas por severidade e paginação exigem varredura linear dos arquivos; a solução prioriza simplicidade sem banco de dados.
- Subscribers SSE lentos podem perder eventos quando seu buffer fica cheio; o arquivo continua sendo a fonte durável.
- O endpoint de download respeita o limite máximo configurado de linhas do arquivo.
