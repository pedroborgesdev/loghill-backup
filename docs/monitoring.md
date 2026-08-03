# Monitoramento

O Monitoramento permite montar regras low-code sobre logs recebidos. A fonte de verdade é uma árvore de expressão; o texto exibido na interface é apenas um resumo.

## Fluxo

1. Abra **Monitoramento** na sidebar e selecione **Nova regra**. O construtor abre em um modal largo sem desmontar a listagem.
2. Informe nome, descrição, estado e ao menos um sender.
3. Arraste blocos da biblioteca para o canvas vertical — ou clique neles — e combine condições com **E**, **OU** e **NÃO**.
4. Adicione uma ou mais ações de evento ou e-mail.
5. Crie a regra completa ou salve como rascunho. Em edições, use **Testar** para avaliar um contexto fictício sem executar ações.

Rascunhos usam `status: "draft"`, nunca são indexados para execução e podem preservar blocos incompletos. O editor mantém apenas ordem, tipo, operador, negação e configuração; nenhuma coordenada visual é persistida.

Tipos iniciais de condição: evento, alerta, mensagem, severity, metadata, horário, dia da semana e data. Ações iniciais: disparar evento existente e enviar e-mail pelos providers e workers já usados por alertas e eventos.

## Persistência

- `data/monitoring-rules.json`: regras, métricas e último estado, escrito com arquivo temporário, flush e rename.
- `data/monitoring-pending.json`: avaliações futuras de ausência; é reaberto no startup.
- `data/monitoring-executions.jsonl`: histórico append-only de avaliações e ações.

As regras ativas são indexadas em memória por sender e também por referências de evento e alerta. O índice é reconstruído ao iniciar e depois de cada alteração.

## Ausências e ciclos

Uma condição negativa de evento com `window_minutes` cria uma pendência. A chegada do evento esperado antes do prazo cancela a pendência; o scheduler processa as restantes ao vencer. Não são usados timers isolados por regra.

A validação bloqueia ciclos diretos entre o evento que inicia a regra e a ação de evento. O runtime também limita a profundidade encadeada a dez ações e propaga um `correlation_id`.

## Limites da primeira versão

- O builder visual usa um fluxo vertical no grupo raiz; o backend já aceita grupos aninhados até cinco níveis.
- Regex não foi exposta nesta versão; mensagem suporta contém, igualdade, prefixo e sufixo.
- Regras exclusivamente temporais não são agendadas: horário, dia e data são condições complementares.
- O teste da interface usa um contexto fictício seguro e não executa ações. A API exige confirmação administrativa explícita para execução real.
