# Instalação do LogMate

Este guia apresenta duas formas de executar o LogMate:

1. [Binário nativo](#1-binário-nativo), compilado a partir do código-fonte;
2. [Docker Compose](#2-docker-compose), construindo localmente ou usando a imagem publicada no GHCR.

Nos dois casos, a aplicação utiliza a porta `8080` por padrão e precisa de armazenamento persistente para `DATA_DIR`.

## Requisitos gerais

- Porta TCP `8080` disponível, ou outra porta configurada em `APP_PORT`;
- Um diretório persistente para logs e configurações;
- `APP_PASSWORD` definido quando o serviço estiver acessível por rede;
- URL pública correta em `APP_PUBLIC_URL` para links enviados por alertas e eventos.

Crie um arquivo `.env` a partir do exemplo:

```bash
cp .env.example .env
```

No PowerShell:

```powershell
Copy-Item .env.example .env
```

Configuração mínima recomendada:

```env
APP_HOST=0.0.0.0
APP_PORT=8080
APP_PUBLIC_URL=http://localhost:8080
DATA_DIR=./data
APP_PASSWORD=troque-por-uma-senha-forte
TZ=America/Sao_Paulo
```

Não versione o arquivo `.env`.

## 1. Binário nativo

### 1.1 Requisitos para compilação

- Go 1.24 ou superior;
- Node.js 22 ou superior;
- npm;
- Git, caso o código seja obtido por clone.

O frontend precisa ser compilado antes do backend porque os arquivos de `web/dist` são incorporados ao executável com `go:embed`.

### 1.2 Obter o código

```bash
git clone https://github.com/pedroborgesdev/logmate-backup.git
cd logmate-backup
```

### 1.3 Compilar no Linux

```bash
cd frontend
npm ci
npm run build
cd ..

go test ./...
go build -trimpath -ldflags="-s -w" -o logmate ./cmd/server
```

### 1.4 Compilar no Windows

```powershell
Set-Location frontend
npm ci
npm run build
Set-Location ..

go test ./...
go build -trimpath -ldflags="-s -w" -o logmate.exe ./cmd/server
```

### 1.5 Executar

Linux:

```bash
mkdir -p data
./logmate
```

Windows:

```powershell
New-Item -ItemType Directory -Force data | Out-Null
./logmate.exe
```

O servidor carrega `.env` do diretório de trabalho e fica disponível em:

- Interface: <http://localhost:8080>
- Swagger: <http://localhost:8080/docs>
- Liveness: <http://localhost:8080/health>
- Readiness: <http://localhost:8080/ready>

### 1.6 Executar como serviço systemd

Crie um usuário dedicado e prepare o diretório:

```bash
sudo useradd --system --home /opt/logmate --shell /usr/sbin/nologin logmate
sudo mkdir -p /opt/logmate/data
sudo cp logmate /opt/logmate/logmate
sudo cp .env /opt/logmate/.env
sudo chown -R logmate:logmate /opt/logmate
sudo chmod 750 /opt/logmate/logmate
sudo chmod 600 /opt/logmate/.env
```

Crie `/etc/systemd/system/logmate.service`:

```ini
[Unit]
Description=LogMate Observability Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=logmate
Group=logmate
WorkingDirectory=/opt/logmate
ExecStart=/opt/logmate/logmate
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

Ative o serviço:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now logmate
sudo systemctl status logmate
```

Logs do serviço:

```bash
journalctl -u logmate -f
```

### 1.7 Atualizar o binário

Compile a nova versão, pare o serviço, substitua o executável e inicie novamente:

```bash
sudo systemctl stop logmate
sudo cp logmate /opt/logmate/logmate
sudo chown logmate:logmate /opt/logmate/logmate
sudo chmod 750 /opt/logmate/logmate
sudo systemctl start logmate
```

O diretório `/opt/logmate/data` não deve ser removido durante a atualização.

## 2. Docker Compose

### 2.1 Requisitos

- Docker Engine ou Docker Desktop;
- Docker Compose v2, disponível pelo comando `docker compose`.

### 2.2 Construir a imagem localmente

O repositório contém [`Dockerfile`](./Dockerfile) e [`docker-compose.yml`](./docker-compose.yml).

```bash
git clone https://github.com/pedroborgesdev/logmate-backup.git
cd logmate-backup
docker compose up -d --build
```

O compose padrão:

- constrói frontend e backend;
- publica `8080:8080`;
- persiste `/app/data` em `./data`;
- reinicia o container automaticamente;
- verifica `/health` a cada 30 segundos.

Para habilitar autenticação no compose padrão, defina `APP_PASSWORD` na seção `environment` ou use um arquivo de override que não seja versionado:

```yaml
# docker-compose.override.yml
services:
  logmate:
    environment:
      APP_PASSWORD: ${APP_PASSWORD}
      APP_PUBLIC_URL: ${APP_PUBLIC_URL:-http://localhost:8080}
```

Defina as variáveis no shell antes de subir:

```bash
export APP_PASSWORD='troque-por-uma-senha-forte'
export APP_PUBLIC_URL='http://localhost:8080'
docker compose up -d --build
```

PowerShell:

```powershell
$env:APP_PASSWORD = 'troque-por-uma-senha-forte'
$env:APP_PUBLIC_URL = 'http://localhost:8080'
docker compose up -d --build
```

### 2.3 Usar a imagem publicada no GHCR

A imagem publicada pela GitHub Action é:

```text
ghcr.io/pedroborgesdev/logmate-backup:latest
```

Crie um diretório de implantação contendo o seguinte `compose.yml`:

```yaml
services:
  logmate:
    image: ghcr.io/pedroborgesdev/logmate-backup:latest
    container_name: logmate
    restart: unless-stopped
    ports:
      - "8080:8080"
    env_file:
      - .env
    environment:
      APP_HOST: 0.0.0.0
      APP_PORT: "8080"
      DATA_DIR: /app/data
    volumes:
      - logmate-data:/app/data
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/health"]
      interval: 30s
      timeout: 3s
      start_period: 5s
      retries: 3

volumes:
  logmate-data:
```

Crie `.env` ao lado do compose:

```env
APP_PUBLIC_URL=http://localhost:8080
APP_PASSWORD=troque-por-uma-senha-forte
TZ=America/Sao_Paulo
```

Se a imagem estiver privada, autentique no GHCR antes do pull:

```bash
docker login ghcr.io -u pedroborgesdev
```

Use um GitHub Personal Access Token com permissão `read:packages` como senha.

Suba o serviço:

```bash
docker compose pull
docker compose up -d
```

### 2.4 Operação diária

Estado dos containers:

```bash
docker compose ps
```

Logs:

```bash
docker compose logs -f logmate
```

Reiniciar:

```bash
docker compose restart logmate
```

Atualizar para a imagem mais recente:

```bash
docker compose pull
docker compose up -d
```

Parar sem apagar dados:

```bash
docker compose down
```

Não use `docker compose down -v` em produção: a opção `-v` remove o volume nomeado e todos os dados persistidos.

### 2.5 Backup e restauração

Com bind mount `./data`, pare o serviço e copie o diretório:

```bash
docker compose stop
tar -czf logmate-data-backup.tar.gz data/
docker compose start
```

Com volume nomeado, identifique o volume:

```bash
docker volume ls
docker volume inspect logmate-data
```

O backup deve incluir todo o conteúdo de `/app/data`, especialmente:

- `senders/` e arquivos de logs;
- `config.json`;
- regras de alertas, eventos e monitoramento;
- histórico de execuções;
- `email-encryption.key` e configurações criptografadas de e-mail.

### 2.6 Aplicações clientes em Docker

Dentro de um container, `localhost` aponta para o próprio container. Se o cliente e o LogMate estiverem no mesmo compose, use o nome do serviço:

```env
LOGMATE_API_URL=http://logmate:8080
```

Se o LogMate estiver exposto por domínio:

```env
LOGMATE_API_URL=https://logmate.exemplo.com
```

## 3. Validação da instalação

Confira o healthcheck:

```bash
curl http://localhost:8080/health
```

Resposta esperada:

```json
{
  "status": "healthy",
  "time": "2026-08-31T12:00:00Z",
  "uptime_seconds": 42,
  "senders": {
    "total": 0,
    "never_connected": 0,
    "online": 0,
    "inactive": 0,
    "expired": 0,
    "revoked": 0
  },
  "storage": {
    "writable": true,
    "path": "./data"
  }
}
```

Confira a prontidão:

```bash
curl http://localhost:8080/ready
```

Abra <http://localhost:8080> e autentique com `APP_PASSWORD`, caso configurada.

## 4. Solução de problemas

### Porta já utilizada

Altere apenas a porta publicada, mantendo a porta interna `8080`:

```yaml
ports:
  - "9090:8080"
```

Nesse caso, use `APP_PUBLIC_URL=http://localhost:9090`.

### Container sem permissão para gravar

A imagem executa como usuário não-root. Em bind mounts Linux, garanta que o diretório de dados possa ser escrito pelo usuário do container. Volumes nomeados evitam a maior parte dos problemas de ownership.

### Interface abre, mas não mantém dados

Confirme que `DATA_DIR=/app/data` e que `/app/data` está associado a um volume ou bind mount persistente.

### Imagem não encontrada ou acesso negado

- Confirme que o workflow **Publish container image** terminou com sucesso;
- confira a existência da tag no GitHub Packages;
- execute `docker login ghcr.io` quando o pacote for privado;
- confira se o token possui `read:packages`.

### Aplicação cliente não conecta

- Fora de containers, use `http://localhost:8080` apenas quando o LogMate estiver na mesma máquina;
- entre serviços do Compose, use `http://logmate:8080`;
- em Kubernetes, use o DNS do Service;
- confira firewall, TLS, proxy e CORS conforme o ambiente.

## 5. Próximos passos

- Configure o primeiro cliente com [docs/sender-client.md](./docs/sender-client.md);
- configure alertas em [docs/alerts.md](./docs/alerts.md);
- consulte o contrato em [docs/openapi.yaml](./docs/openapi.yaml);
- revise todas as variáveis em [`.env.example`](./.env.example).
