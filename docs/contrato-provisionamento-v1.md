# Contrato Lynx de provisionamento v1

## Transporte

Cada SaaS integrado publica uma rota única sobre HTTPS: `GET` consulta capacidades e `POST` executa operações em `/api/v1/provisioning`. A autenticação usa `X-Lynx-API-Key`. O contrato não tem negociação por query string nem versões diferentes por operação: requisições e respostas carregam `contractVersion: "1"`.

Toda operação usa o mesmo envelope:

```json
{
  "contractVersion": "1",
  "operation": "resource.list",
  "input": {
    "resourceType": "customer",
    "accountId": "id-opaco",
    "filters": { "lifecycle": "active", "search": "Acme" },
    "page": { "cursor": null, "limit": 25 }
  }
}
```

## Capacidades e recursos

`GET /api/v1/provisioning` requer `provisioning:read` e anuncia serviço, operações, recursos, campos editáveis/obrigatórios, enums e ciclo de vida. A interface só coleta campos e oferece ações publicadas pelo SaaS.

- `account`: organização ou tenant do cliente dentro do SaaS;
- `customer`: cliente atendido dentro de uma conta;
- `user`: pessoa ou vínculo de acesso dentro de uma conta.

IDs são opacos. `customer` e `user` são sempre escopados pela conta. As respostas não incluem senha ou token de convite.

Operações comuns: `resource.list`, `resource.get`, `resource.update`, `resource.deactivate`, `resource.reactivate` e `resource.purge`. Cada SaaS anuncia somente as operações implementadas. Criação inicial usa `bootstrap`, que cria `account`, `initialCustomer` e `initialUser` atomicamente. Criação isolada de cliente/usuário será anunciada quando o SaaS a suportar.

No EasyBill, a hibernação é reversível e a purga respeita a retenção mínima anunciada pelo serviço. A purga informa se apaga ou anonimiza e quais dependentes são afetados.

## Idempotência e escopos

Toda escrita inclui `Idempotency-Key` com 8 a 128 caracteres seguros. Repetir a mesma chave e operação devolve o resultado original; reusar a chave com payload diferente retorna `409 idempotency.conflict`. Leituras não exigem chave.

- `provisioning:read`: capacidades, lista e detalhe;
- `provisioning:write`: bootstrap, edição, hibernação e reativação;
- `provisioning:purge`: purga definitiva após a elegibilidade confirmada pelo SaaS.

A chave de cada integração é distinta, revogável e enviada apenas no header. Segredos ficam fora do banco da aplicação SaaS; registros de idempotência guardam hashes, IDs e resultado mínimo, nunca o segredo ou o payload pessoal completo.

## Respostas

Sucesso segue o envelope:

```json
{
  "contractVersion": "1",
  "requestId": "id-da-requisicao",
  "operation": "resource.list",
  "status": "completed",
  "data": { "items": [], "page": { "nextCursor": null } },
  "replayed": false
}
```

`bootstrap` coloca recursos criados e estado do convite dentro de `data`. Erros seguem:

```json
{
  "error": {
    "code": "resource.not_found",
    "message": "Registro não encontrado.",
    "fieldErrors": [],
    "requestId": "id-da-requisicao"
  }
}
```

Nenhuma resposta inclui credenciais, token de convite, SQL ou stack trace. A escrita remota é sempre revalidada pelo próprio SaaS, inclusive isolamento de conta, elegibilidade de purga e regras financeiras.
