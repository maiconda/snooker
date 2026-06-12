# ADR 0007: Isolamento do Auth Service na raiz

## Status

Aceito

## Contexto

O projeto deve evoluir com modularizacao rigida orientada a microservicos. A organizacao anterior separava o auth apenas dentro de `internal/`, o que ainda mantinha a aplicacao com formato de monolito modular.

## Decisao

O modulo de autenticacao passa a ser um servico de raiz em `auth/`, com:

- entrypoint proprio em `auth/cmd/auth`;
- codigo interno privado em `auth/internal`;
- Dockerfile proprio em `auth/Dockerfile`;
- container dedicado `snooker-auth`;
- banco dedicado `snooker-auth-postgres`.

O servico de auth passa a possuir somente dados e endpoints de autenticacao. Modulos como profile, storage, matchmaking, lobby e game devem nascer como pastas irmas na raiz, com seus proprios containers e ownership de dados.

## Consequencias

- O root do repositorio vira camada de orquestracao, documentacao e infraestrutura.
- `auth/internal/*` nao deve ser importado por outros servicos.
- Outros servicos devem integrar com auth via API publica ou eventos, nao por acesso direto a codigo interno ou banco.
- O compose passa a expressar a fronteira operacional do auth.
