# Monitoramento

O Monitoramento permite montar regras low-code sobre logs recebidos. A fonte de verdade é uma árvore de expressão; o texto exibido na interface é apenas um resumo.

## Fluxo

1. Abra **Monitoramento** na sidebar e selecione **Nova regra**. O construtor abre em um modal largo sem desmontar a listagem.
2. Informe nome, descrição, estado e ao menos um sender.
3. Arraste blocos da biblioteca para o canvas vertical — ou clique neles — e combine condições em grupos **AND**, **OR** e **NOT**.
4. Adicione uma ou mais ações de evento ou e-mail.
5. Crie a regra completa ou salve como rascunho. Em edições, use **Testar** para avaliar um contexto fictício sem executar ações.

Rascunhos usam `status: "draft"`, nunca são indexados para execução e podem preservar blocos incompletos. O editor mantém apenas ordem, tipo, operador, negação e configuração; nenhuma coordenada visual é persistida.

### Grupos aninhados

Abra o menu de uma condição e escolha **Group with previous** para agrupar dois blocos adjacentes. Selecione o cartão do grupo para escolher se todos os filhos devem corresponder (`AND`), se basta um (`OR`) e se o resultado inteiro deve ser negado (`NOT`). Um grupo pode ser combinado novamente com o bloco anterior, formando até cinco níveis. **Dissolve group** devolve os filhos ao nível pai sem removê-los.

O canvas renderiza a mesma árvore enviada no campo `expression`: abrir e salvar uma regra existente preserva IDs, operadores, negações e subgrupos. Os conectores escritos nos nós são mantidos por compatibilidade, mas a combinação efetiva é definida pelo `operator` de cada grupo, assim como no avaliador do backend.

Tipos iniciais de condição: evento, alerta, mensagem, severity, metadata, horário, dia da semana e data. Ações iniciais: disparar evento existente e enviar e-mail pelos providers e workers já usados por alertas e eventos.

Condições de mensagem aceitam comparação por conteúdo, igualdade, prefixo, sufixo e expressões regulares RE2 pelos operadores `matches_regex` e `not_matches_regex`. O padrão é validado ao salvar e limitado a 500 bytes. O motor RE2 não utiliza backtracking, evitando expressões com tempo de execução exponencial; recursos como lookaround e backreferences não fazem parte da sintaxe.

## Persistência

- `data/monitoring-rules.json`: regras, métricas e último estado, escrito com arquivo temporário, flush e rename.
- `data/monitoring-pending.json`: avaliações futuras de ausência e agenda temporal por regra/sender; é reaberto no startup.
- `data/monitoring-executions.jsonl`: histórico append-only de avaliações e ações.

As regras ativas são indexadas em memória por sender e também por referências de evento e alerta. O índice é reconstruído ao iniciar e depois de cada alteração.

## Ausências e ciclos

Uma condição negativa de evento com `window_minutes` cria uma pendência. A chegada do evento esperado antes do prazo cancela a pendência; o scheduler processa as restantes ao vencer. Não são usados timers isolados por regra.

## Regras exclusivamente temporais

Horário, dia da semana, data e **Wait Until** também podem iniciar uma regra sem depender da chegada de um log. Ao colocar um desses blocos na primeira posição, o editor o trata como gatilho agendado.

O scheduler cria uma avaliação persistida para cada combinação de regra e sender. A expressão é verificada por minuto e a ação roda somente na transição de “não corresponde” para “corresponde”; assim, uma faixa como `12:00–17:00` executa uma vez ao entrar na faixa, não uma vez por minuto. O estado e o próximo horário ficam em `monitoring-pending.json`, portanto reinícios não recriam execuções já confirmadas.

`Wait Until` usa o timezone IANA configurado no bloco e corresponde apenas ao minuto exato do dia da semana selecionado. Regras alteradas têm seu estado temporal reinicializado, enquanto regras desativadas ou removidas têm suas agendas descartadas.

A validação bloqueia ciclos diretos entre o evento que inicia a regra e a ação de evento. O runtime também limita a profundidade encadeada a dez ações e propaga um `correlation_id`.

## Limites da primeira versão

- O teste da interface usa um contexto fictício seguro e não executa ações. A API exige confirmação administrativa explícita para execução real.
