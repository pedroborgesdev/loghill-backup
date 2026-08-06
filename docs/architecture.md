# Arquitetura da API

A API segue o fluxo obrigatorio:

```text
routes -> controllers -> services -> repositories
```

## Responsabilidades

- `internal/routes`: registra endpoints, grupos e middlewares. Nao valida dominio, nao constroi respostas e nao acessa dados.
- `internal/controllers`: interpreta e valida a requisicao HTTP, chama services e constroi a resposta HTTP.
- `internal/services`: concentra regras de negocio e coordenacao. Nao depende de `gin.Context`.
- `internal/repositories`: le e persiste dados. Nao decide regras de negocio.
- `internal/middlewares`: autenticacao, CORS, limites globais, logging, recuperacao de panics e erros HTTP nao tratados.
- `internal/dto`: contratos de entrada e saida HTTP que nao pertencem ao dominio.
- `internal/domain`: modelos e erros de dominio compartilhados pelas camadas.

## Dependencias

Uma camada pode depender apenas das camadas seguintes no fluxo. Dependencias concretas sao criadas em `cmd/server/main.go` e injetadas nos construtores. Controllers nao recebem repositories, e services nao recebem objetos HTTP.

Arquivos novos devem usar `snake_case` e indicar sua responsabilidade, por exemplo `sender_routes.go`, `sender_controller.go`, `sender_service.go`, `sender_repository.go` e `auth_middleware.go`.
