# Alertas de e-mail do LogMate

## Fluxo

1. `POST /api/v1/logs` valida e persiste o log.
2. O evento é publicado no SSE.
3. O matcher usa um índice em memória por sender e consulta regras ativas nas quais o ID está em `sender_ids`, além da severity.
4. Cada correspondência é persistida, sem aguardar o envio, na outbox limitada em `data/outbox/notifications.json`.
5. Um worker renderiza HTML e texto, autentica no Microsoft Graph e envia pelo endpoint `sendMail`.
6. O resultado sanitizado é persistido em `data/alerts.json`.

O envio nunca faz parte do resultado da ingestão. Se Outlook estiver indisponível ou a outbox estiver cheia, o log continua aceito. Pendências sobrevivem ao reinício; workers usam leases persistidos para recuperar tarefas interrompidas. A entrega é pelo menos uma vez, portanto uma queda entre o envio ao provider e a confirmação local pode produzir uma duplicidade.

## Outlook/O365

O provedor usa OAuth 2.0 client credentials. O token é solicitado com o escopo `https://graph.microsoft.com/.default`, mantido somente em memória e renovado antes de expirar. O aplicativo do Microsoft Entra ID precisa da permissão de aplicação `Mail.Send` com consentimento administrativo para a mailbox configurada.

```env
EMAIL_PROVIDER=outlook
OUTLOOK_ENABLED=true
OUTLOOK_TENANT_ID=
OUTLOOK_CLIENT_ID=
OUTLOOK_CLIENT_SECRET=
OUTLOOK_SENDER_EMAIL=logs@empresa.com
OUTLOOK_SENDER_NAME=LogMate
APP_PUBLIC_URL=https://logs.empresa.com
```

Os aliases `O365_TENANT_ID`, `O365_CLIENT_ID`, `O365_CLIENT_SECRET`, `EMAIL_FROM_ADDR` e `EMAIL_USER` mantêm compatibilidade com o repositório de referência.

Para salvar um novo secret pela interface, o servidor usa `EMAIL_SETTINGS_ENCRYPTION_KEY` (Base64 de 32 bytes) ou gera automaticamente `DATA_DIR/email-encryption.key` se a variável estiver vazia/inválida. O arquivo `data/email-settings.json` recebe apenas o ciphertext AES-256-GCM. A chave fica fora do arquivo e respostas HTTP nunca incluem secret ou tokens.

## Operação

- Abra **Configurações > E-mail**.
- Use **Testar conexão** para validar a obtenção do token sem enviar mensagem.
- Use **Enviar e-mail de teste** para validar a permissão e a mailbox.
- Abra **Alertas > Novo alerta**, escolha um ou mais senders não expirados/revogados, uma ou mais severidades e até 20 destinatários.
- Regras antigas com `sender_id` são migradas para `sender_ids` na leitura, sem renomear IDs ou diretórios legados.
- Um e-mail individual será produzido para cada log correspondente.

### Solução do erro HTTP 403

Uma conexão autenticada não garante permissão para enviar. Se o teste de envio retornar HTTP 403:

1. No Microsoft Entra ID, abra o registro do aplicativo e acesse **API permissions**.
2. Adicione **Microsoft Graph > Application permissions > Mail.Send**.
3. Selecione **Grant admin consent** para o tenant.
4. Confirme que o e-mail remetente possui uma mailbox no Exchange Online.
5. Se a organização usa Application RBAC ou uma política de acesso, inclua essa mailbox no escopo permitido ao aplicativo.

Depois da alteração, aguarde a propagação da Microsoft e execute **Testar conexão** novamente. O teste manual solicita um token novo para não reutilizar uma permissão antiga em cache.

Os endpoints estão descritos integralmente em `docs/openapi.yaml`. Quando `APP_PASSWORD` está definido (auth habilitada), os endpoints de alertas e configuração de e-mail exigem sessão autenticada ou `X-API-Key` com a mesma senha.

## Limitações da primeira versão

- Somente Outlook/Microsoft 365 está disponível; Gmail é apenas informativo na interface.
- Não há cooldown, agregação, janela temporal, agenda ou filtro por conteúdo/metadata.
- A outbox garante recuperação após reinício, mas não elimina a pequena janela de duplicidade entre o envio externo e a confirmação local.
- O status de entrega representa o resultado final do último processamento, não um histórico completo de tentativas.
