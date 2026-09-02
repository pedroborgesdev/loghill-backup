# ADR 0001: Persistência somente em JSON e JSONL

- Status: aceito
- Data: 2026-09-01

## Contexto

O LogHill é distribuído como um binário ou container autocontido e não deve
exigir PostgreSQL, Redis, RabbitMQ ou outro serviço de dados. O estado atual já
é persistido sob `DATA_DIR` em arquivos JSON e JSONL.

O roadmap exige tornar filas, idempotência e agendamentos recuperáveis sem
abandonar essa característica.

## Decisão

Toda persistência continuará baseada em arquivos:

- snapshots e configurações em JSON;
- logs e journals append-only em JSONL;
- alterações de snapshots por arquivo temporário, `Sync` e rename atômico;
- filas duráveis em snapshots JSON com gravação atômica;
- idempotência por índice persistido em arquivos;
- jobs em processamento protegidos por leases persistidos;
- recuperação no startup pela leitura dos snapshots e replay dos journals onde eles forem usados.

O contrato `repositories.SenderRepository` separa as regras de negócio da
implementação física, mas o produto terá apenas o driver de arquivos.

## Coordenação

Uma única instância gravadora continua sendo o modo padrão e recomendado.
Quando múltiplos processos forem habilitados, todos deverão apontar para o
mesmo `DATA_DIR` e o LogHill usará locks de arquivo para serializar alterações.

Réplicas com discos locais separados não compartilham estado e não são um modo
suportado. Compartilhamentos de rede só poderão ser declarados compatíveis se
oferecerem locks, criação exclusiva e rename atômico com as mesmas garantias do
filesystem local.

## Regras de segurança de dados

1. Nenhuma escrita sobrescreve diretamente um snapshot válido.
2. Journals aceitam somente registros completos terminados por newline.
3. Replay ignora apenas uma última linha comprovadamente truncada; corrupção no
   meio do arquivo impede o startup e exige reparo explícito.
4. Compactação mantém o arquivo anterior até o novo snapshot ser sincronizado.
5. Leases têm proprietário, início, expiração e contador de tentativas.
6. Operações idempotentes com a mesma chave e payload retornam o resultado já
   persistido; payload conflitante retorna conflito.
7. Backup consistente inclui todo o `DATA_DIR`.

## Consequências

### Positivas

- instalação sem serviços externos;
- backup e restauração por diretório;
- compatibilidade com o modelo atual;
- operação offline e baixo custo operacional.

### Limitações aceitas

- escalabilidade limitada pelo filesystem compartilhado;
- ausência de coordenação entre discos independentes;
- consultas históricas dependem de índices em memória reconstruídos no startup;
- compactações precisam ser controladas para não bloquear ingestão por períodos
  longos.

## Ordem de implementação

1. contrato de repositório e compatibilidade com o driver atual;
2. locks de arquivo e unidade atômica por sender;
3. outbox JSON com leases;
4. índice idempotente de ocorrências;
5. scheduler persistente;
6. novos tipos de action;
7. melhorias do editor e regex.
