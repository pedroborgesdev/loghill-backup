# LogHill — documentação end-to-end

Este documento descreve o estado atual do LogHill, da instalação à operação, incluindo arquitetura, persistência, API, interface, padrões de design, segurança, alertas, eventos, build, testes, backup e transferência para outro computador.

> Estado documentado: 31 de julho de 2026. O nome visual do produto é **LogHill**. O módulo Go e alguns nomes históricos de binário/serviço ainda usam `logtheater` ou `log-theater`; isso é apenas nomenclatura interna e não altera o funcionamento.

## 1. O que é o LogHill

O LogHill é uma aplicação centralizada de observabilidade para:

- cadastrar aplicações produtoras de logs, chamadas de **senders**;
- autenticar cada sender com uma chave exclusiva;
- receber logs estruturados por HTTP;
- armazenar os registros localmente em JSON Lines, sem banco de dados;
- consultar, filtrar, paginar, exportar e acompanhar logs em tempo real por SSE;
- controlar inatividade, compactação e expiração dos senders;
- enviar alertas por severidade via Microsoft 365/Outlook;
- executar eventos explícitos informados pelo cliente e enviar e-mails personalizados;
- administrar tudo por uma SPA React responsiva servida pelo próprio backend Go.

Em produção, frontend, API e stream SSE são entregues pelo mesmo processo e pela mesma origem HTTP.

## 2. Checklist rápido para transferir o projeto

### 2.1 Antes de copiar no computador antigo

1. Pare o servidor com `Ctrl+C` e aguarde o encerramento. Isso permite finalizar as gravações e os workers de e-mail.
2. Copie o projeto inteiro, com atenção especial a:
   - `data/` — senders, logs, limites, alertas, eventos e configuração de e-mail;
   - `.env`, caso ele seja usado como referência local;
   - as variáveis de ambiente configuradas no sistema, serviço, CI ou Docker;
   - `docs/openapi.yaml` — necessário para o Swagger em runtime;
   - `web/dist/` — frontend que será incorporado pelo próximo build Go;
   - `frontend/` — necessário para alterar e reconstruir a interface.
3. Guarde separadamente os segredos:
   - `ADMIN_API_KEY`;
   - `EMAIL_SETTINGS_ENCRYPTION_KEY`;
   - credenciais `OUTLOOK_*`, se forem fornecidas pelo ambiente;
   - chaves `X-Sender-Key` configuradas nas aplicações clientes.
4. Não é necessário copiar `frontend/node_modules/` nem `.cache/`; ambos podem ser recriados.

### 2.2 Pontos críticos da transferência

- O Go **não carrega `.env` automaticamente**. O arquivo é ignorado pelo Git e serve apenas como referência, a menos que um shell, gerenciador de serviço ou Docker o injete no processo.
- Se `data/email-settings.json` contém um `client_secret_encrypted`, a nova máquina precisa receber exatamente a mesma `EMAIL_SETTINGS_ENCRYPTION_KEY`. Sem ela, o servidor falha ao descriptografar a credencial na inicialização.
- O servidor guarda apenas o hash das chaves dos senders. A chave completa aparece somente na criação, rotação ou reativação. Preserve a configuração dos clientes; o servidor não consegue recuperar uma chave perdida.
- Se `data/` for preservado, as chaves que já estão nos clientes continuam válidas, pois os hashes também são preservados.
- Ao mudar host, domínio ou porta, atualize `APP_PUBLIC_URL` e `LOG_API_URL` nos clientes.
- Execute o binário a partir da raiz do projeto ou disponibilize `./docs/openapi.yaml` relativamente ao diretório de trabalho. A interface React é incorporada no binário, mas a especificação OpenAPI é lida do disco.
- Faça o backup com o processo parado. Os arquivos de metadados são atômicos, mas `logs.txt` recebe append continuamente enquanto o servidor está ativo.

### 2.3 Subir no computador novo — Windows/PowerShell

Requisitos: Go 1.24 ou superior e, para reconstruir o frontend, Node.js 22 ou superior.

```powershell
cd C:\caminho\para\LogTheater

# Somente se quiser reconstruir a interface
cd frontend
npm ci
npm run build
cd ..

# Validar e executar diretamente
go test ./...
go run .\cmd\server\
```

Acesse:

- aplicação: `http://localhost:8080`;
- Swagger UI: `http://localhost:8080/docs`;
- OpenAPI bruto: `http://localhost:8080/openapi.yaml`;
- liveness: `http://localhost:8080/health`;
- readiness: `http://localhost:8080/ready`.

Para gerar um executável:

```powershell
go build -o loghill.exe .\cmd\server\
.\loghill.exe
```

As variáveis precisam estar no ambiente do processo. Exemplo:

```powershell
$env:APP_PORT = "8080"
$env:DATA_DIR = ".\data"
$env:APP_PUBLIC_URL = "http://localhost:8080"
$env:ADMIN_AUTH_ENABLED = "true"
$env:ADMIN_API_KEY = "uma-chave-administrativa-forte"
.\loghill.exe
```

### 2.4 Subir no computador novo — Linux/macOS

```bash
cd /caminho/para/LogTheater
cd frontend && npm ci && npm run build && cd ..
go test ./...
go build -o loghill ./cmd/server
./loghill
```

Exemplo de ambiente:

```bash
export APP_PORT=8080
export DATA_DIR=./data
export APP_PUBLIC_URL=http://localhost:8080
./loghill
```

Garanta permissão de leitura e escrita no diretório definido por `DATA_DIR`.

### 2.5 Subir com Docker

```bash
docker compose up --build
```

O compose atual:

- publica `8080:8080`;
- monta `./data:/app/data`;
- reinicia o serviço com `unless-stopped`;
- executa healthcheck em `/health`;
- usa usuário não privilegiado na imagem final.

O `docker-compose.yml` atual injeta apenas `APP_PORT`, `DATA_DIR`, `APP_PUBLIC_URL` e `TZ`. As demais entradas de um `.env` não entram automaticamente no container só por existirem. Para usar autenticação, Outlook ou outros ajustes, declare as variáveis em `environment` ou adicione conscientemente um `env_file` ao serviço. Antes de usar `.env.example` como `env_file`, substitua ou remova o placeholder inválido de `EMAIL_SETTINGS_ENCRYPTION_KEY`.

## 3. Arquitetura

### 3.1 Visão geral

```text
Cliente do sender
  ├─ POST /api/v1/logs ───────────────┐
  └─ POST /senders/{id}/health ──────┤
                                      v
                             Gin / middlewares
                                      |
                                      v
                              service.Service
                ┌─────────────────────┼─────────────────────┐
                v                     v                     v
       repositório em arquivos     Hub SSE        matching de notificações
                |                                           |
                v                                           v
       data/senders/{id}/                         fila compartilhada em memória
                                                     |              |
                                                     v              v
                                                  Alertas         Eventos
                                                     \              /
                                                      Microsoft Graph

Navegador React <── mesma API e mesmo processo Go ──> arquivos incorporados
```

### 3.2 Camadas e diretórios

| Caminho | Responsabilidade |
|---|---|
| `cmd/server/` | bootstrap, composição das dependências, servidor e shutdown |
| `internal/config/` | leitura e validação das variáveis de ambiente |
| `internal/domain/` | entidades, filtros, severidades, status e erros de domínio |
| `internal/repository/` | persistência de senders e logs em arquivos |
| `internal/storage/` | gerenciador de `sync.RWMutex` por sender |
| `internal/settings/` | persistência atômica dos limites editáveis |
| `internal/service/` | cadastro, autenticação, ingestão, consultas, lifecycle e SSE |
| `internal/scheduler/` | ciclo periódico de inatividade e expiração |
| `internal/alerts/` | CRUD, validação, índice e estado dos alertas |
| `internal/events/` | CRUD, validação, templates e índice dos eventos |
| `internal/emailconfig/` | configuração segura e criptografia do client secret |
| `internal/emailprovider/` | OAuth client credentials e envio pelo Microsoft Graph |
| `internal/notification/` | runtime, template, fila, workers, retry e gravação de resultado |
| `internal/handler/` | rotas HTTP, schemas, erros, Swagger e fallback da SPA |
| `internal/middleware/` | request ID, headers de segurança, CORS, rate limit, body limit e chave admin |
| `frontend/src/` | aplicação React/TypeScript |
| `frontend/public/` | assets públicos usados pelo Vite, inclusive `loghill.png` |
| `web/dist/` | build do frontend incorporado com `go:embed` |
| `docs/openapi.yaml` | contrato OpenAPI servido em runtime |
| `examples/python_log_client.py` | cliente Python de exemplo integrado ao `logging` |
| `data/` | estado persistente da instância |

### 3.3 Inicialização do processo

O `cmd/server/main.go` executa, nesta ordem:

1. carrega e valida as variáveis de ambiente;
2. cria/abre o repositório de senders;
3. abre `config.json`, `email-settings.json`, `alerts.json` e `events.json`;
4. constrói serviços e índices em memória;
5. cria o provider Outlook e a fila compartilhada de notificações;
6. conecta os runtimes de alertas e eventos à ingestão;
7. repara arquivos temporários/counters e aplica o lifecycle pendente;
8. inicia o scheduler;
9. carrega os assets incorporados da SPA;
10. inicia o servidor HTTP;
11. ao receber `SIGINT` ou `SIGTERM`, encerra HTTP, dispatcher e Hub SSE dentro de `SHUTDOWN_TIMEOUT`.

Arquivos persistidos inválidos ou uma credencial criptografada que não possa ser aberta interrompem o startup. O comportamento é fail-fast para evitar operar silenciosamente sobre estado corrompido.

## 4. Persistência e consistência

### 4.1 Layout do diretório de dados

```text
data/
├─ config.json
├─ alerts.json
├─ events.json
├─ email-settings.json
└─ senders/
   └─ {sender-id}/
      ├─ sender.json
      └─ logs.txt
```

- `sender.json` guarda nome, descrição, lifecycle, contadores, prefixo e hash da chave.
- `logs.txt`, apesar da extensão, é um arquivo **JSON Lines/NDJSON**: um objeto JSON completo por linha.
- `config.json` guarda os limites dinâmicos de retenção.
- `alerts.json` e `events.json` guardam definições e o último estado de entrega.
- `email-settings.json` guarda a configuração global de e-mail; o secret salvo pela interface fica criptografado.

### 4.2 Estratégia de escrita

- Logs usam append-only e `Sync` antes do retorno.
- Metadados e configurações usam arquivo `.tmp`, `Sync`, fechamento e `Rename`.
- Há locks independentes por sender; gravações em senders diferentes não disputam um lock global.
- Alertas, eventos, configurações e índices possuem sincronização própria.
- O startup repara resíduos temporários e recalcula contadores do sender quando necessário.

Não edite os arquivos de `data/` com o servidor ativo. Para correção manual, pare o processo, faça backup e preserve JSON válido.

### 4.3 Sem banco de dados

A decisão de usar arquivos reduz dependências e simplifica transporte/backup, mas implica:

- busca textual e filtros percorrem o arquivo do sender;
- métricas e paginação podem ter custo linear;
- não há transações distribuídas entre arquivos;
- o projeto é mais adequado a uma instância única compartilhando um filesystem local;
- não execute dois processos LogHill gravando no mesmo `DATA_DIR`.

## 5. Configuração

### 5.1 Regras gerais

- Apenas variáveis de ambiente são lidas pelo backend.
- Valores de duração seguem `time.ParseDuration`: `30s`, `5m`, `168h`.
- `DATA_DIR` e o caminho do OpenAPI são relativos ao diretório de trabalho quando não forem absolutos.
- Valores inválidos de inteiros, booleanos ou durações normalmente voltam ao padrão; combinações estruturais inválidas interrompem o startup.
- Os limites alterados na interface ficam em `data/config.json` e entram em vigor sem reinício.

### 5.2 Servidor, armazenamento e lifecycle

| Variável | Padrão | Uso atual |
|---|---:|---|
| `APP_HOST` | `0.0.0.0` | interface de rede do servidor |
| `APP_PORT` | `8080` | porta HTTP |
| `DATA_DIR` | `./data` | raiz persistente |
| `INACTIVE_AFTER` | `5m` | tempo sem atividade para marcar `inactive` |
| `DELETE_AFTER` | `168h` | tempo inativo até remover logs e marcar `expired` |
| `CLEANUP_INTERVAL` | `1m` | frequência do scheduler |
| `MAX_BODY_SIZE` | `1048576` | limite do body HTTP em bytes |
| `MAX_MESSAGE_SIZE` | `262144` | limite da mensagem em bytes |
| `MAX_METADATA_SIZE` | `262144` | limite do JSON de metadata em bytes |
| `MAX_PAGE_SIZE` | `1000` | teto da paginação da API |
| `MAX_LOG_LINES` | `100000` | teto usado pelo endpoint de exportação; a retenção ativa vem de `config.json` |
| `LOG_COUNTS_AS_ACTIVITY` | `true` | log renova atividade; o primeiro log conecta o sender de qualquer forma |
| `SHUTDOWN_TIMEOUT` | `10s` | prazo do encerramento gracioso |
| `APP_PUBLIC_URL` | `http://localhost:8080` | links e logo nos e-mails; deve ser URL HTTP(S) absoluta |
| `TZ` | `America/Sao_Paulo` no exemplo | timezone do processo/container; não é lido diretamente por `config.Load` |

### 5.3 SSE, CORS, rate limit e autenticação

| Variável | Padrão | Uso atual |
|---|---:|---|
| `SSE_HEARTBEAT_INTERVAL` | `20s` | heartbeat dos streams |
| `SSE_CLIENT_BUFFER` | `100` | buffer por subscriber no backend |
| `SSE_MAX_CLIENTS_PER_SENDER` | `100` | streams simultâneos por sender |
| `ADMIN_AUTH_ENABLED` | `false` | protege todas as rotas administrativas `/api/v1` |
| `ADMIN_API_KEY` | vazio | valor esperado em `X-API-Key` quando a proteção está ativa |
| `CORS_ENABLED` | `false` | habilita CORS explícito |
| `CORS_ALLOWED_ORIGINS` | vazio | origens exatas separadas por vírgula |
| `RATE_LIMIT_ENABLED` | `false` | liga limitador em memória por IP |
| `RATE_LIMIT_REQUESTS` | `120` | requisições permitidas por janela |
| `RATE_LIMIT_WINDOW` | `1m` | tamanho da janela |

O rate limiter é local ao processo, reinicia com a aplicação e atua globalmente, inclusive sobre health, documentação e assets.

### 5.4 Outlook e dispatcher

| Variável | Padrão | Uso atual |
|---|---:|---|
| `EMAIL_PROVIDER` | `outlook` | único provider disponível; `o365` é normalizado para `outlook` |
| `OUTLOOK_ENABLED` | `false` | habilita entregas |
| `OUTLOOK_TENANT_ID` | vazio | tenant do Entra ID |
| `OUTLOOK_CLIENT_ID` | vazio | application/client ID |
| `OUTLOOK_CLIENT_SECRET` | vazio | credencial do aplicativo |
| `OUTLOOK_SENDER_EMAIL` | vazio | mailbox remetente |
| `OUTLOOK_SENDER_NAME` | `LogHill` | nome amigável do remetente |
| `EMAIL_SETTINGS_ENCRYPTION_KEY` | chave de desenvolvimento no código | Base64 de exatamente 32 bytes para AES-256-GCM; substitua em produção |
| `EMAIL_ALERT_QUEUE_SIZE` | `1000` | capacidade da fila compartilhada |
| `EMAIL_ALERT_WORKERS` | `2` | workers simultâneos |
| `EMAIL_ALERT_SEND_TIMEOUT` | `30s` | timeout de token/envio por tentativa |
| `EMAIL_ALERT_MAX_RETRIES` | `3` | retries depois da primeira tentativa |
| `EMAIL_ALERT_RETRY_INTERVAL` | `5s` | espera entre tentativas |

Aliases legados aceitos: `O365_TENANT_ID`, `O365_CLIENT_ID`, `O365_CLIENT_SECRET`, `EMAIL_FROM_ADDR` e `EMAIL_USER`. Os nomes `OUTLOOK_*` têm prioridade.

Se qualquer variável gerenciada do Outlook estiver presente, a interface considera a configuração administrada pelo ambiente e bloqueia sua edição. O valor do secret nunca volta pela API.

Para gerar uma chave de criptografia segura com PowerShell:

```powershell
$bytes = New-Object byte[] 32
[System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
[Convert]::ToBase64String($bytes)
```

Guarde o resultado num cofre de secrets. Não use o valor público de desenvolvimento em produção.

### 5.5 Variáveis legadas ou reservadas

Estas entradas existem em `.env.example` ou em `Config`, mas não controlam hoje o fluxo principal indicado pelo nome:

| Variável | Situação atual |
|---|---|
| `LOG_COMPACT_TARGET_LINES` | validada junto de `MAX_LOG_LINES`, porém a retenção dinâmica usa margem interna de 5% |
| `COMPACT_KEEP_LINES` | carregada, porém a preservação ativa vem de `data/config.json` |
| `HEALTHCHECK_INTERVAL` | carregada, mas o servidor não agenda healthchecks do cliente; cada cliente escolhe seu intervalo |
| `API_KEY_ENABLED` / `API_KEY` | legado; não autentica ingestão nem substitui `ADMIN_AUTH_ENABLED` |
| `LOG_LEVEL` | presente no exemplo, mas o bootstrap atual não configura nível de `slog` por ela |
| `X-Admin-API-Key` | permitido na lista CORS, mas a autenticação atual lê somente `X-API-Key` |

Não baseie uma implantação nova nessas entradas sem primeiro atualizar a implementação.

## 6. Segurança e autenticação

### 6.1 Dois contextos de chave

1. **Chave do sender**: `X-Sender-Key` em logs e healthchecks. É exclusiva por sender e sempre exigida.
2. **Chave administrativa**: `X-API-Key` nas rotas de consulta/configuração, somente quando `ADMIN_AUTH_ENABLED=true`.

A chave do sender tem prefixo `snd_`, é gerada com aleatoriedade criptográfica e só é exibida uma vez. O servidor persiste SHA-256 e um prefixo para identificação, nunca a chave completa. Comparações são feitas em tempo constante.

Rotacionar ou reativar gera uma nova chave e invalida a anterior imediatamente. Revogar remove hash/prefixo e impede logs/healthchecks.

### 6.2 Interface com autenticação administrativa

O frontend adiciona a chave administrativa aos requests `fetch` quando encontra `admin_api_key` em `sessionStorage`:

```javascript
sessionStorage.setItem("admin_api_key", "SUA_CHAVE")
location.reload()
```

O projeto ainda não possui uma tela de login. `sessionStorage` é por aba/sessão e não deve ser usado como cofre permanente.

Limitação importante: o stream usa `EventSource`, que não permite header customizado nativamente, e a exportação atual usa um link `<a>` direto. Com `ADMIN_AUTH_ENABLED=true`, as consultas feitas por `fetch` recebem `X-API-Key`, mas SSE e download pela interface não conseguem enviá-lo e podem retornar `401`. Para uma implantação autenticada completa, use um proxy que injete a identidade com segurança ou altere o transporte/autenticação desses dois fluxos. Não coloque a chave em query string.

### 6.3 Superfície pública

Mesmo com autenticação administrativa ativa, continuam públicos:

- `/health` e `/ready`;
- `/docs`, assets do Swagger e `/openapi.yaml`;
- assets e HTML da SPA;
- ingestão e healthcheck, protegidos pela chave própria do sender.

A aplicação não é um sistema multiusuário de identidade/autorização. Em exposição externa, use HTTPS, firewall/reverse proxy e controle de acesso adicional.

### 6.4 Headers e erros

O middleware adiciona CSP, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY` e `Referrer-Policy: no-referrer`. Cada request recebe `X-Request-ID`; o cliente também pode fornecê-lo.

Erros seguem o formato:

```json
{
  "error": {
    "code": "INVALID_SENDER_KEY",
    "message": "A chave informada não é válida para este sender.",
    "field": "campo_quando_aplicável",
    "request_id": "req_..."
  }
}
```

## 7. Senders

### 7.1 Cadastro

O administrador informa nome e descrição. O ID é derivado do nome:

- minúsculas;
- acentos removidos;
- grupos de caracteres inválidos substituídos por `-`;
- máximo de 63 caracteres;
- o ID é imutável depois da criação;
- nome visível aceita de 3 a 80 caracteres;
- descrição aceita até 250 caracteres.

É possível ter senders com IDs diferentes e o mesmo nome visível. No dashboard eles são agrupados pelo nome sem acento de caixa; clicar no grupo abre uma segunda etapa para escolher a instância. Toda a linha é clicável e acessível por teclado.

### 7.2 Status e lifecycle

| Status | Significado |
|---|---|
| `never_connected` | cadastrado, mas ainda sem log/healthcheck válido |
| `online` | atividade recebida dentro da janela |
| `inactive` | excedeu `INACTIVE_AFTER`; histórico já foi compactado |
| `expired` | excedeu `DELETE_AFTER`; `logs.txt` foi removido |
| `revoked` | acesso manualmente revogado |
| `archived` | tipo previsto no domínio/UI, sem transição automática atual |

Fluxo padrão:

```text
never_connected ── primeiro log/health ──> online
online ── sem atividade por INACTIVE_AFTER ──> inactive
inactive ── após DELETE_AFTER ──> expired

online/inactive/never_connected ── revogar ──> revoked
revoked ── reativar + nova chave ──> never_connected
```

Um sender expirado não pode ser reativado ou rotacionado; cadastre outro sender. `sender.json` permanece como registro histórico, mas os logs são removidos.

### 7.3 Edição e exclusão

- Editar o nome não muda o ID nem o diretório.
- Nomes armazenados nos alertas são sincronizados após edição.
- Antes da exclusão, a API informa quantos alertas e eventos dependem do sender.
- A exclusão exige confirmação para removê-lo dessas dependências.
- Regras que ficarem sem sender são desativadas pelo store correspondente.
- A exclusão final remove o diretório do sender e seus logs.

## 8. Ingestão de logs

### 8.1 Contrato

```http
POST /api/v1/logs
Content-Type: application/json
X-Sender-Key: snd_...
```

```json
{
  "sender": "automacao-financeira",
  "severity": "ERROR",
  "message": "Falha ao processar boleto",
  "timestamp": "2026-07-31T18:00:00Z",
  "event": "processamento_falhou",
  "event_occurrence_id": "protocolo-123",
  "metadata": {
    "etapa": "consulta",
    "tentativa": 2
  }
}
```

Obrigatórios: `sender`, `severity` e `message`. Severidades aceitas, sem diferença entre maiúsculas/minúsculas na entrada: `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL`.

Regras relevantes:

- mensagem vazia ou acima de `MAX_MESSAGE_SIZE` é rejeitada;
- metadata precisa serializar e respeitar `MAX_METADATA_SIZE`;
- body respeita `MAX_BODY_SIZE`;
- `event` precisa corresponder a `^[a-z0-9][a-z0-9_-]{2,79}$`;
- `event_occurrence_id` aceita até 200 caracteres e rejeita CR, LF e NUL;
- timestamp é opcional; sem ele, o servidor usa o horário de recebimento.

Após autenticar e validar, o servidor:

1. persiste o JSONL;
2. aplica o limite dinâmico do sender;
3. atualiza counters/lifecycle;
4. publica no SSE;
5. avalia alertas por severidade;
6. avalia eventos explícitos;
7. responde `202 Accepted` sem esperar e-mail.

Falha ou fila cheia de e-mail não desfaz o log. Um evento desconhecido também não rejeita a ingestão.

### 8.2 Healthcheck

```http
POST /api/v1/senders/{sender}/health
Content-Type: application/json
X-Sender-Key: snd_...
```

O body pode ser vazio ou um objeto JSON. O conteúdo não é persistido atualmente; a chamada autentica o sender e atualiza status, última atividade e último healthcheck.

O backend não chama os clientes. Cada aplicação deve enviar seu próprio healthcheck, normalmente em intervalo menor que `INACTIVE_AFTER`.

### 8.3 Cliente Python incluído

`examples/python_log_client.py` contém:

- `LogHillLogger`, que aceita `event=`, `event_occurrence_id=` e `metadata=`;
- `LogHillHandler`, compatível com o módulo padrão `logging`;
- mapeamento dos níveis Python para as severidades do LogHill;
- thread de healthcheck;
- captura de módulo, função, linha, thread e exceção;
- fluxo de simulação com `log.info("teste 1")` e outros níveis.

Uso recomendado:

```env
LOG_API_URL=http://localhost:8080
LOG_SENDER_ID=automacao-financeira
LOG_SENDER_KEY=snd_chave_copiada_no_cadastro
```

```python
log.info(
    "Processamento concluído",
    event="processamento_finalizado",
    metadata={"protocolo": "ABC-123"},
)
```

Observação de segurança: o bloco `__main__` atual do exemplo contém valores de desenvolvimento no código. Antes de distribuir ou usar em produção, altere-o para ler `LOG_SENDER_ID` e `LOG_SENDER_KEY` do ambiente e rotacione qualquer chave que tenha sido exposta no repositório.

## 9. Consulta, paginação, stream e console de logs

### 9.1 Consulta HTTP

```http
GET /api/v1/senders/{sender}/logs
```

Filtros:

| Query | Exemplo | Efeito |
|---|---|---|
| `severity` | `ERROR,WARN` | uma ou mais severidades |
| `search` | `login` | busca em mensagem, evento e metadata serializada |
| `start_date` | RFC3339 | início inclusivo |
| `end_date` | RFC3339 | fim inclusivo |
| `event` | `with` / `without` | presença ou ausência de evento |
| `event_key` | `processamento_finalizado` | chave exata |
| `page` | `2` | página, começando em 1 |
| `page_size` | `100` | limitada por `MAX_PAGE_SIZE` |
| `order` | `desc` / `asc` | recentes ou antigos primeiro |

Exemplo:

```bash
curl "http://localhost:8080/api/v1/senders/automacao-financeira/logs?severity=ERROR,WARN&search=login&page=1&page_size=100&order=desc"
```

### 9.2 Exportação

`GET /api/v1/senders/{sender}/logs/download` aceita os mesmos filtros e `format=jsonl|txt`. Atualmente ambos os formatos retornam um objeto JSON por linha com content type NDJSON; a extensão muda, mas não há renderização textual diferente.

### 9.3 SSE

`GET /api/v1/senders/{sender}/logs/stream` emite:

- `status` ao conectar;
- `log` para cada novo registro aceito;
- `heartbeat` no intervalo configurado.

O stream filtra severidade e filtros de evento. Cada subscriber possui buffer limitado; um navegador lento pode perder eventos ao vivo para não bloquear a ingestão. O arquivo continua sendo a fonte durável e pode ser recarregado pela API.

### 9.4 Comportamento da interface de logs

- O app ocupa `100dvh`; não há scrollbar global. O painel de logs tem scroll próprio.
- O padrão é ordem decrescente, com registros mais recentes no topo.
- A densidade pode ser **Compacta** ou **Confortável** e é salva em `localStorage`.
- O botão **Follow** controla somente o acompanhamento visual.
- Com Follow ativo, a janela acompanha os registros recentes.
- Qualquer scroll/roda/touch do usuário desativa Follow.
- Follow desativado **não pausa nem impede novos logs**. A lista continua sendo atualizada, mas preserva a âncora visual para não saltar ou piscar.
- Voltar manualmente ao topo não reativa Follow; o usuário precisa clicar no botão.
- O viewer usa virtualização e âncora baseada na primeira linha visível para manter o viewport estático.
- Novos logs são aplicados em lotes de 150 ms, reduzindo rerenders e flicker.

**Pausar** é diferente de **Follow**:

- Pausar congela a aplicação visual dos eventos SSE e mantém até 1.000 registros pendentes no navegador.
- O contador do botão mostra pendências.
- Ao retomar, os registros pendentes são aplicados em lote.
- Se mais de 1.000 eventos chegarem durante uma pausa longa, os mais antigos podem sair da fila do navegador; eles continuam no arquivo do servidor.
- Busca textual e navegação/tamanho da página exigem uma lista estável. Se o usuário tentar usá-las ao vivo, um diálogo oferece **Pausar e continuar**.
- Filtros por severidade podem continuar ligados ao stream.
- Inserção ao vivo é combinada apenas na página 1, ordem `desc`, sem busca/data/filtro de evento. O número de linhas visíveis nunca excede o seletor de página; o excedente é contabilizado nas páginas seguintes.

O botão de limpar filtros foi removido por decisão de produto; cada controle é alterado individualmente e a busca possui apenas o `X` próprio quando há texto.

## 10. Limites, compactação, inatividade e expiração

### 10.1 Configuração dinâmica

Em **Configurações**, as categorias são:

- **Geral**: resumo dos valores aplicados;
- **Armazenamento de logs**: limite por sender ativo;
- **Inatividade**: volume preservado ao inativar;
- **E-mail**: provider Outlook.

Os limites usam controles próprios do tema, com steppers e listbox de unidade. Unidades:

- `lines` — quantidade de entradas completas;
- `mb` — `1 MB = 1024 × 1024` bytes, preservando somente linhas JSON completas.

Regras:

- valores novos entre 0 e 10.000;
- `log_limit = 0` desativa a retenção automática do sender ativo;
- `inactive_preservation = 0` esvazia os logs ao inativar;
- se as unidades forem iguais e o limite máximo não for zero, a preservação não pode ser maior que o máximo;
- valores antigos acima de 10.000 podem ser carregados para correção, mas não salvos novamente sem ajuste;
- alterações passam a valer no próximo log ou ciclo de manutenção, sem reinício.

### 10.2 Margem de compactação

Ao ultrapassar o limite, o repositório reduz para aproximadamente 95% do valor, removendo os registros mais antigos. Essa margem de 5% evita reescrever o arquivo a cada novo log. Para limites muito pequenos, a margem mínima é uma unidade.

### 10.3 Inatividade

O scheduler executa a cada `CLEANUP_INTERVAL`. Um sender `online` sem atividade por mais de `INACTIVE_AFTER`:

1. compacta `logs.txt` para `inactive_preservation`;
2. vira `inactive`;
3. recebe `inactive_at`, `compacted_at` e `expires_at`.

Um healthcheck autenticado antes da expiração volta o sender para `online` e remove a data de expiração. Um novo log faz o mesmo quando `LOG_COUNTS_AS_ACTIVITY=true`, que é o padrão.

Depois de `DELETE_AFTER`, o arquivo de logs é excluído, counters viram zero e o sender passa a `expired`.

## 11. Alertas por severidade

### 11.1 Conceito e matching

Um alerta relaciona:

- nome;
- de um a vários senders;
- uma ou mais severidades;
- de 1 a 20 destinatários;
- provider `outlook`;
- status ativo/inativo.

O índice em memória é `sender_id -> IDs de alertas`. Para cada log, o runtime seleciona regras ativas que contenham o sender e a severidade. Não há hoje cooldown, agregação, agenda, janela temporal ou filtro de conteúdo/metadata.

### 11.2 Interface

A tela **Alertas** oferece:

- métricas de total, ativos e falhas;
- busca por nome, sender ou destinatário;
- paginação sem apagar a tabela anterior durante refresh;
- criação/edição em diálogo;
- seleção de múltiplos senders e severidades;
- ativação/desativação;
- detalhes e último resultado;
- envio de teste;
- exclusão confirmada.

Sem Outlook pronto, a regra pode ser salva inativa, mas não ativada.

### 11.3 Persistência e compatibilidade

Os alertas ficam em `data/alerts.json`. Regras antigas que usavam `sender_id`/`sender_name` são migradas em memória para `sender_ids`/`sender_names` e persistidas no novo formato na próxima escrita.

O estado mantém última tentativa, status `pending|sent|failed`, erro sanitizado e contadores de sucesso, falha e teste. Não é um histórico completo de todas as tentativas.

## 12. Eventos explícitos

### 12.1 Diferença para alertas

Alertas são acionados implicitamente por sender + severidade. Eventos só são acionados quando o payload do log informa `event` com uma chave exata.

Matching:

```text
evento ativo + chave exata + sender associado
```

A severidade não participa. Um log pode disparar simultaneamente um alerta e um evento, produzindo duas mensagens independentes.

### 12.2 Definição

Cada evento possui:

- ID interno com prefixo `evt_`;
- nome de 3 a 100 caracteres;
- chave única e imutável, de 3 a 80 caracteres;
- de 1 a 100 senders;
- ação `email`;
- de 1 a 20 destinatários;
- assunto de até 200 caracteres, sem CR/LF;
- mensagem de até 10.000 caracteres;
- status ativo/inativo;
- timestamps e counters de disparo/entrega/teste.

Senders expirados ou revogados não podem ser associados. A tela verifica disponibilidade da chave antes do cadastro.

### 12.3 Templates

Variáveis permitidas:

- `{{event.key}}`, `{{event.name}}`;
- `{{sender.id}}`, `{{sender.name}}`, `{{sender.status}}`;
- `{{log.message}}`, `{{log.severity}}`, `{{log.timestamp}}`;
- `{{metadata.chave}}`, para chaves simples de metadata;
- `{{app.public_url}}`.

Placeholders desconhecidos são rejeitados. Valor ausente de metadata vira vazio. Conteúdo do template e dados do log são tratados como texto e escapados no HTML; não há HTML livre, execução de funções, loops, arquivos, ambiente ou template externo.

### 12.4 Interface e teste

A tela **Eventos** contém:

- métricas de total, ativos, disparos e falhas nas últimas 24 horas;
- busca, filtro por sender/status e paginação;
- wizard de criação/edição com prévia do e-mail;
- drawer de detalhes;
- cópia da chave;
- ativação/desativação e exclusão;
- envio de teste para um destinatário informado.

O teste usa dados fictícios, passa pela mesma fila e incrementa apenas `test_delivery_count`, não `trigger_count`.

### 12.5 Idempotência

`event_occurrence_id` é validado e persistido, mas a deduplicação ainda não está implementada. Repetir a mesma requisição pode gerar novos e-mails. O cliente deve evitar retries cegos de eventos não idempotentes ou implementar controle próprio até que o servidor ganhe uma store de ocorrências.

## 13. Outlook, templates de e-mail e fila

### 13.1 Configuração no Microsoft 365

O aplicativo no Microsoft Entra ID precisa:

1. fluxo OAuth 2.0 **client credentials**;
2. permissão Microsoft Graph **Application** `Mail.Send`;
3. consentimento administrativo;
4. mailbox do Exchange Online para `OUTLOOK_SENDER_EMAIL`;
5. inclusão dessa mailbox no escopo, se a organização usa Application RBAC ou política de acesso.

O token usa `https://graph.microsoft.com/.default`, permanece somente em memória e é renovado antes de expirar. Mudança de credencial invalida o cache por assinatura criptográfica.

### 13.2 Configuração pela interface

Abra **Configurações > E-mail**. É possível:

- habilitar/desabilitar Outlook;
- cadastrar tenant, client ID, client secret, remetente e nome;
- testar a conexão, obtendo token sem enviar mensagem;
- enviar um e-mail real de teste.

O teste manual invalida o token em cache para refletir alterações recentes de consentimento. A API nunca retorna client secret ou access token; responde apenas se a credencial está configurada.

### 13.3 Criptografia

Secrets salvos pela interface usam AES-256-GCM com nonce aleatório e ficam Base64 em `email-settings.json`. A chave de 32 bytes fica apenas no ambiente. Se a chave mudar, o ciphertext anterior deixa de ser legível e o startup falha de propósito.

### 13.4 Dispatcher

Alertas e eventos compartilham uma única fila limitada:

- a ingestão apenas tenta enfileirar e não espera o Outlook;
- workers renderizam texto e HTML e enviam em background;
- cada tentativa possui timeout;
- há retry configurável;
- falhas são sanitizadas e limitadas antes de persistir/logar;
- fila cheia registra falha da notificação, mas preserva o log;
- a fila é volátil: notificações pendentes são perdidas se o processo parar abruptamente.

O template HTML segue o tema escuro/âmbar do LogHill, é compatível com Outlook, centralizado, inclui versão text/plain, contexto do sender/log, CTA e logo baseada em `APP_PUBLIC_URL/loghill.png`. Assuntos não usam colchetes.

### 13.5 HTTP 403

Se **Testar conexão** funcionar, mas o envio retornar 403, as credenciais foram aceitas e a autorização de envio falhou. Verifique `Mail.Send` como permissão de aplicação, admin consent, existência da mailbox e escopo do Exchange. A mensagem mostrada ao usuário diferencia autenticação de autorização.

## 14. Interface e sistema de design

### 14.1 Rotas

| Rota | Tela |
|---|---|
| `/` | dashboard |
| `/senders` | inventário de senders |
| `/senders/:sender` | detalhes, lifecycle e console de logs |
| `/alerts` | alertas de e-mail |
| `/events` | eventos explícitos |
| `/status` | saúde da API e armazenamento |
| qualquer outra | página não encontrada |

### 14.2 Layout

- AppShell persistente com sidebar, header, status do backend e refresh global.
- Sidebar expansível no desktop e drawer no mobile.
- Logo LogHill ampliada e sem fundo artificial.
- Itens verticais ocupam toda a largura útil da sidebar, respeitando margens.
- Backend é verificado a cada 30 segundos.
- Estado SSE aparece no header da tela de logs.
- Conteúdo principal é limitado à viewport e usa scroll interno; o documento não cria scrollbar global.

No Dashboard, os cards resumem senders, status e volume; a tabela oferece busca, filtro de status, paginação, criação e ações de acesso. A atualização pode ser manual ou automática a cada 15, 30 ou 60 segundos, e a escolha fica no `localStorage`. A rota `/senders` reutiliza o inventário sem os cards da visão geral.

### 14.3 Tema

Padrões estabelecidos:

- fundo principal `#09090b`;
- sidebar `#111113`;
- painéis `#161618`;
- bordas na escala zinc, normalmente `zinc-800`/`zinc-700`;
- texto principal zinc claro, texto secundário zinc 500/600;
- verde para online/sucesso, âmbar para atenção/eventos, vermelho/rosa para falha/revogação, ciano como destaque contextual;
- fonte de interface Inter/system e fonte mono JetBrains Mono/system;
- botões com texto explicitamente claro para evitar herdar preto;
- foco com outline/ring branco fino e consistente com listboxes;
- animações curtas e desativadas com `prefers-reduced-motion`.

### 14.4 Componentes

Use primeiro os componentes compartilhados:

- `Button`, `IconButton`, `Input`, `SearchInput`;
- `Panel`, `MetricCard`, `StatusBadge`, `StatusIndicator`;
- `Pagination`, `ConfirmDialog`, `Tooltip`;
- `Listbox`, `DateTimePicker` e number input temático.

Não introduza `<select>`, spinner nativo de `input[type=number]`, datepicker puro ou botões isolados com estilo próprio quando o componente temático já cobre o caso.

Listboxes, menus, tooltips e diálogos usam portal e z-index coordenado para não ficarem atrás de painéis. Tooltips:

- fecham em clique, blur, Escape, scroll e resize;
- apenas um fica aberto;
- são limitados à viewport;
- quebram texto longo sem gerar scrollbar horizontal.

Diálogos mantêm foco, respondem a Escape, restauram foco ao gatilho e pedem confirmação ao fechar com alterações não salvas.

### 14.5 Responsividade e acessibilidade

- tabelas possuem apresentação alternativa em cards no mobile;
- linhas clicáveis também aceitam Enter/Espaço;
- botões de ícone têm `aria-label`;
- seleção/status usa `aria-current`, `aria-pressed`, roles e regiões de status;
- foco é visível;
- loading inicial usa skeleton; refresh mantém o conteúdo anterior para evitar flicker;
- overlays não devem causar overflow horizontal no documento.

### 14.6 Logo

O asset usado pelo frontend é `frontend/public/loghill.png`; o build o copia para `web/dist/loghill.png`. O arquivo `loghill.png` na raiz é a cópia de referência, mas o Vite não o copia automaticamente. Ao substituir a marca:

1. atualize `frontend/public/loghill.png`;
2. opcionalmente mantenha a cópia da raiz sincronizada;
3. execute `npm run build`;
4. reconstrua o binário Go.

## 15. API administrativa

O contrato completo e schemas estão em `docs/openapi.yaml`. Resumo das rotas:

| Método | Caminho | Função |
|---|---|---|
| `GET` | `/health` | liveness, uptime, senders e storage |
| `GET` | `/ready` | readiness do scheduler |
| `POST` | `/api/v1/logs` | ingerir log com chave de sender |
| `GET/POST` | `/api/v1/senders` | listar/criar senders |
| `GET` | `/api/v1/senders/check-id` | normalizar e verificar ID |
| `GET/PUT/DELETE` | `/api/v1/senders/{sender}` | consultar/editar/excluir |
| `GET` | `/api/v1/senders/{sender}/dependencies` | contar alertas/eventos dependentes |
| `POST` | `/api/v1/senders/{sender}/rotate-key` | rotacionar chave |
| `POST` | `/api/v1/senders/{sender}/revoke` | revogar acesso |
| `POST` | `/api/v1/senders/{sender}/reactivate` | reativar revogado com nova chave |
| `POST` | `/api/v1/senders/{sender}/health` | registrar atividade |
| `GET` | `/api/v1/senders/{sender}/logs` | consultar logs |
| `GET` | `/api/v1/senders/{sender}/logs/stream` | SSE |
| `GET` | `/api/v1/senders/{sender}/logs/download` | exportar NDJSON |
| `GET` | `/api/v1/dashboard/summary` | resumo global |
| `GET/PUT` | `/api/v1/settings` | limites dinâmicos |
| `GET/POST` | `/api/v1/alerts` | listar/criar alertas |
| `GET/PUT/DELETE` | `/api/v1/alerts/{alertID}` | detalhar/editar/excluir alerta |
| `PATCH` | `/api/v1/alerts/{alertID}/status` | ativar/desativar alerta |
| `POST` | `/api/v1/alerts/{alertID}/test` | teste do alerta |
| `GET/POST` | `/api/v1/events` | listar/criar eventos |
| `GET` | `/api/v1/events/check-key` | disponibilidade da chave |
| `GET/PUT/DELETE` | `/api/v1/events/{eventID}` | detalhar/editar/excluir evento |
| `PATCH` | `/api/v1/events/{eventID}/status` | ativar/desativar evento |
| `POST` | `/api/v1/events/{eventID}/test` | teste do evento |
| `GET/PUT` | `/api/v1/settings/email` | consultar/configurar provider |
| `POST` | `/api/v1/settings/email/test-connection` | validar token/permissão |
| `POST` | `/api/v1/settings/email/send-test` | enviar teste real |

### 15.1 Swagger/OpenAPI

- Swagger UI: `/docs` redireciona para `/docs/index.html`.
- Especificação: `/openapi.yaml`.
- O arquivo precisa começar com um campo de versão válido, atualmente `openapi: 3.x.x`.
- Se a UI estiver vazia, verifique no DevTools/Network se `/openapi.yaml` responde YAML em vez de HTML/404.
- Inicie pela raiz do projeto; `c.File("./docs/openapi.yaml")` usa o diretório de trabalho.
- O Swagger preserva autorizações informadas na interface durante a sessão.

## 16. Desenvolvimento

### 16.1 Backend

```bash
go mod tidy
go run ./cmd/server
```

O backend usa Go 1.24, Gin e biblioteca padrão para HTTP, filesystem, criptografia e concorrência.

### 16.2 Frontend com hot reload

Em outro terminal:

```bash
cd frontend
npm ci
npm run dev
```

O Vite faz proxy de `/api`, `/health` e `/ready` para `http://localhost:8080`. O `/docs` e o SSE funcionam melhor acessando o backend diretamente ou mantendo a mesma origem esperada.

Stack principal:

- React 19;
- TypeScript 5.8;
- Vite 7;
- Tailwind CSS 3;
- React Router 7;
- TanStack React Virtual;
- Lucide React;
- Vitest e Testing Library;
- ESLint 9.

### 16.3 Build de produção

```bash
cd frontend
npm ci
npm run build
cd ..
go build -o loghill ./cmd/server
```

`npm run build` primeiro executa `tsc --noEmit` e depois grava os assets em `web/dist`. O pacote `web` incorpora esse diretório no binário. Se alterar React/CSS e executar apenas `go build`, o binário conterá o último `web/dist`, não necessariamente o código fonte mais recente.

### 16.4 Makefile

```bash
make backend   # go run
make frontend  # vite
make build     # npm ci + frontend build + go build
make test      # backend race + frontend tests
make lint      # go vet + eslint
make docker    # docker compose up --build
```

No Windows sem `make`, execute os comandos equivalentes diretamente.

## 17. Testes e validação antes de entregar

Backend:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Frontend:

```bash
cd frontend
npm ci
npm run test:run
npm run lint
npm run build
```

Checklist funcional:

1. `/health` retorna `200` e storage `writable: true`.
2. `/ready` retorna `200`.
3. `/openapi.yaml` abre como YAML e `/docs` renderiza.
4. Dashboard lista e agrupa senders.
5. Criação mostra chave apenas uma vez.
6. Um log autenticado retorna `202` e aparece na consulta/SSE.
7. Scroll desativa Follow sem flicker e sem parar novos logs.
8. Busca/paginação solicitam pausa e mantêm a página escolhida.
9. Limite de linhas é respeitado no live viewer e no arquivo após compactação.
10. Configurações persistem após reinício.
11. Teste de conexão do Outlook e teste de envio apresentam mensagens distintas.
12. Alerta dispara apenas para sender + severity configurados.
13. Evento dispara apenas para sender + chave configurados.
14. Exclusão de sender trata dependências.

## 18. Deploy e operação

### 18.1 Binário

O binário contém a SPA, mas precisa de:

- ambiente configurado;
- diretório `DATA_DIR` gravável;
- `docs/openapi.yaml` no caminho relativo esperado, caso Swagger seja necessário;
- acesso de saída HTTPS para `login.microsoftonline.com` e `graph.microsoft.com`, se usar e-mail.

Para um serviço de sistema, defina explicitamente `WorkingDirectory` como a raiz de implantação e injete as variáveis no próprio serviço.

### 18.2 Reverse proxy

Ao usar Nginx, IIS, Traefik ou equivalente:

- preserve conexões longas e desative buffering em `/logs/stream`;
- use HTTPS;
- encaminhe `X-Request-ID` se houver tracing externo;
- ajuste timeout do proxy para SSE;
- preserve `APP_PUBLIC_URL` com a URL pública real;
- restrinja `/docs` e as rotas administrativas conforme a política da organização.

### 18.3 Monitoramento

- Use `/health` para liveness.
- Use `/ready` para readiness.
- Observe stdout/stderr do processo; o backend usa `slog` e o logger do Gin.
- Monitore espaço do `DATA_DIR`, principalmente se `log_limit=0`.
- Monitore falhas de entrega nas telas de Alertas/Eventos.
- A resposta `healthy` confirma que a consulta do repositório funcionou, mas o campo `storage.writable` atual é declarativo e não executa uma gravação de prova a cada chamada.

## 19. Backup, restauração e recuperação

### 19.1 Backup consistente

1. Pare o serviço graciosamente.
2. Copie `data/` integralmente.
3. Copie `EMAIL_SETTINGS_ENCRYPTION_KEY` e demais segredos por canal separado.
4. Registre versão do código/binário e variáveis de ambiente.
5. Opcionalmente gere checksum do backup.
6. Reinicie o serviço e valide `/ready`.

### 19.2 Restauração

1. Instale a mesma versão ou uma versão compatível do código.
2. Restaure `data/` antes de iniciar.
3. Restaure a chave de criptografia original.
4. Configure `DATA_DIR` apontando para a cópia.
5. Inicie a partir da raiz.
6. Confirme startup sem erros de decode/decrypt.
7. Valide senders, counters, alertas, eventos e e-mail.

### 19.3 Recuperação de chave de sender

Não existe recuperação. Opções:

- se o sender ainda está válido: use **Nova chave** e atualize o cliente;
- se está revogado: use **Reativar**, copie a nova chave e atualize o cliente;
- se está expirado: crie um sender novo.

## 20. Solução de problemas

### Swagger vazio ou “definition does not specify a valid version”

- Abra `/openapi.yaml` diretamente.
- Se aparecer HTML, o caminho está caindo no fallback da SPA.
- Confirme que `docs/openapi.yaml` existe no diretório de trabalho.
- Confirme o campo `openapi` no início do arquivo.
- Execute `go run ./cmd/server` a partir da raiz.

### Servidor inicia, mas a interface mostra backend offline

- Teste `/health` no mesmo host/porta.
- No Vite, confirme backend em `localhost:8080` ou configure `VITE_API_BASE_URL`.
- Verifique firewall, proxy e mixed content HTTP/HTTPS.

### API retorna 401 nas telas

- Se `ADMIN_AUTH_ENABLED=true`, salve a chave em `sessionStorage` ou envie `X-API-Key` no cliente HTTP.
- Confirme que está usando `ADMIN_API_KEY`, não a chave do sender.
- O SSE da SPA possui a limitação de header descrita na seção de segurança.

### Logs retornam 401

- Confirme `sender` e `X-Sender-Key` correspondentes.
- Chave rotacionada deixa de funcionar imediatamente.
- Chave revogada ou de outro sender não é aceita.

### Sender fica inativo

- Envie healthchecks em intervalo menor que `INACTIVE_AFTER`.
- Confirme relógio/timezone da máquina.
- Verifique se logs contam como atividade em `LOG_COUNTS_AS_ACTIVITY`.

### Interface de logs pisca ou muda de posição

- Verifique se alterações mantiveram a âncora do `LogViewer`.
- Follow desligado precisa continuar recebendo dados sem chamar `scrollTo`.
- Não remonte o viewer ou troque chaves de linha a cada batch.
- Preserve `ui_id` e a virtualização.

### E-mail não habilita

- Complete tenant, client ID, secret e remetente.
- Se a configuração é gerenciada pelo ambiente, edite o ambiente, não a UI.
- Se o secret vem da UI, configure uma chave Base64 válida de 32 bytes.

### Outlook 403

- Revise `Mail.Send` Application + admin consent.
- Confirme mailbox e escopo de Application RBAC/Exchange.
- Execute novamente o teste de conexão e depois o envio real.

### Startup falha ao descriptografar

- A `EMAIL_SETTINGS_ENCRYPTION_KEY` mudou ou não foi injetada.
- Restaure a chave original. Não apague o arquivo antes de fazer backup.
- Se a chave foi definitivamente perdida, a credencial não é recuperável; com o processo parado e backup feito, será necessário remover/substituir a configuração criptografada e cadastrar um novo secret.

### Porta em uso

No PowerShell:

```powershell
Get-NetTCPConnection -LocalPort 8080
```

Altere `APP_PORT` ou encerre conscientemente o processo conflitante.

## 21. Padrões para evolução do código

Ao adicionar uma feature:

1. modele contratos em `internal/domain`;
2. mantenha persistência encapsulada e atômica;
3. coloque regra de negócio no service, não no handler;
4. use erros tipados e resposta HTTP padrão com request ID;
5. mantenha índices derivados reconstruíveis a partir do store;
6. não bloqueie ingestão em integração externa;
7. não registre secrets, tokens ou chaves completas;
8. atualize `docs/openapi.yaml`, testes backend e frontend;
9. reutilize componentes visuais compartilhados;
10. preserve acessibilidade, responsividade, estado durante refresh e ausência de flicker;
11. execute testes, lint, typecheck e build;
12. atualize este documento quando o comportamento operacional mudar.

Para novas notificações, prefira estender o dispatcher compartilhado em vez de criar goroutines ou filas independentes. Para novos providers, mantenha credenciais globais, interface `emailprovider.Provider`, erros sanitizados e secrets fora das respostas.

## 22. Limitações conhecidas

- Persistência local sem banco e sem suporte a múltiplas réplicas escrevendo no mesmo volume.
- Consultas de logs fazem varredura linear.
- SSE pode descartar eventos para clientes lentos; recarregar pela API recupera o estado durável.
- Fila de e-mail não é durável e perde pendências em reinício abrupto.
- Apenas Microsoft 365/Outlook está disponível; Gmail é apenas informativo na UI.
- Alertas não possuem cooldown, agrupamento, agenda ou filtros de conteúdo.
- Eventos suportam somente ação de e-mail.
- `event_occurrence_id` ainda não deduplica.
- O último status de entrega não é um histórico completo.
- Autenticação administrativa não tem tela de login e não cobre o header do `EventSource` da SPA.
- O OpenAPI não é incorporado no binário; depende do arquivo no diretório de trabalho.
- `format=txt` da exportação ainda produz NDJSON.
- Algumas variáveis antigas continuam no exemplo/configuração sem controlar o fluxo atual.
- O status `archived` está modelado, mas não possui transição funcional automática.
- Os campos de dashboard `last_24_hours`, `errors_last_24_hours` e `fatal_last_24_hours` ainda são devolvidos como zero; somente o total de linhas e os counters de status são calculados atualmente.
- O cliente Python de exemplo deve ser ajustado para remover valores de desenvolvimento hardcoded antes de uso real.

## 23. Referências internas

- Contrato HTTP: [`docs/openapi.yaml`](docs/openapi.yaml)
- Cliente de sender: [`docs/sender-client.md`](docs/sender-client.md)
- Alertas: [`docs/alerts.md`](docs/alerts.md)
- Eventos: [`docs/events.md`](docs/events.md)
- Cliente Python: [`examples/python_log_client.py`](examples/python_log_client.py)
- Configuração de exemplo: [`.env.example`](.env.example)
- Build Docker: [`Dockerfile`](Dockerfile) e [`docker-compose.yml`](docker-compose.yml)

## 24. Critério de transferência concluída

A migração pode ser considerada concluída quando:

- o processo inicia sem erro;
- `/health`, `/ready`, `/openapi.yaml` e `/docs` funcionam;
- a SPA carrega com logo e estilos;
- senders, logs, alertas, eventos e configurações anteriores estão presentes;
- um cliente existente autentica com sua chave anterior;
- um novo log aparece pela API e no console ao vivo;
- compactação/lifecycle usam os valores esperados;
- a configuração de Outlook é legível;
- teste de conexão e envio funcionam, se a feature estiver habilitada;
- `data/` permanece gravável após reinício;
- os segredos não ficaram em repositório, logs ou arquivos públicos.
