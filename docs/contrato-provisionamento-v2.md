# Contrato Lynx de provisionamento v2

## Transporte e negociação

Um SaaS integrado publica uma única rota, `GET/POST /api/v1/provisioning`, sobre HTTPS. A autenticação usa `X-Lynx-API-Key`. O `GET` sem query continua retornando o contrato v1 durante a migração; `GET ?contractVersion=2` retorna capacidades v2. Clientes v2 declaram `contractVersion: "2"` no corpo do POST. O EasyBill mantém compatibilidade com o POST v1.

Cada POST mutável inclui `Idempotency-Key` (8–128 caracteres seguros). Repetir a chave com operação e corpo equivalentes devolve a resposta original. Reusar a chave para outro comando ou corpo responde HTTP 409 com `idempotency.conflict`. Listagem e leitura usam `provisioning:read` e não exigem chave.

Escopos: `provisioning:read` para capacidades, lista e detalhe; `provisioning:write` para bootstrap, criação, edição, desativação e reativação; `provisioning:purge` exclusivamente para exclusão permanente após o prazo. O principal é vinculado ao SaaS e nunca ao cliente final.

## Hierarquia neutra

- `account`: organização/tenant do cliente dentro do SaaS (no EasyBill, `Company`/escritório).
- `customer`: cliente atendido dentro de uma `account` (no EasyBill, `Client`).
- `user`: pessoa/vínculo de acesso dentro de uma `account` (no EasyBill, `CompanyUser` associado a `User`). O ciclo de vida do vínculo não desativa a identidade global da pessoa nem seus vínculos em outras contas.

Cada recurso devolve `id`, `resourceType`, `accountId` (nulo para account), `lifecycle`, `createdAt` e `updatedAt`, além dos campos declarados nas capacidades. Respostas não incluem senhas, tokens de convite ou segredos. Campos pessoais retornados devem ser limitados ao necessário para a operação solicitada.

## Capacidades

`GET ?contractVersion=2` responde:

```json
{
  "contractVersion": "2",
  "service": { "name": "EasyBill", "slug": "easybill" },
  "resources": {
    "account": { "operations": ["list", "get", "create", "update", "deactivate", "reactivate", "purge"], "fields": {}, "purge": { "mode": "delete", "cascades": ["customer", "user"] } },
    "customer": { "operations": ["list", "get", "create", "update", "deactivate", "reactivate", "purge"], "fields": {}, "purge": { "mode": "delete", "cascades": ["contracts", "revenueEvents", "visits", "partners"] } },
    "user": { "operations": ["list", "get", "create", "update", "deactivate", "reactivate", "purge"], "fields": {}, "purge": { "mode": "anonymize", "requiresSoleAccountMembership": true } }
  },
  "bootstrap": { "supported": true, "creates": ["account", "initialCustomer", "initialUser"] },
  "lifecycle": { "deactivation": "reversible", "purge": "after-grace-period", "minimumGracePeriodDays": 180 }
}
```

`fields` é por operação e declara `required`, `optional`, `readOnly`, `enum` e formatos. Um cliente só pergunta ou envia campos que o SaaS anuncia. `operations` lista apenas o que realmente funciona. `minimumGracePeriodDays` é um limite do SaaS; o Lynx OS pode impor um prazo maior, nunca menor. `purge.mode` declara `delete` ou `anonymize`; `purge.cascades` nomeia dependentes removidos junto e `requiresSoleAccountMembership` explicita a condição para apagar/anonymize uma identidade compartilhada.

## Envelope POST

```json
{
  "contractVersion": "2",
  "operation": "resource.list",
  "input": {
    "resourceType": "customer",
    "accountId": "opaque-id",
    "filters": { "lifecycle": "active", "search": "Acme" },
    "page": { "cursor": null, "limit": 25 }
  }
}
```

Operações definidas:

- `bootstrap`: cria `account`, `initialCustomer` e `initialUser` atomicamente, como v1.
- `resource.list`: lista paginada; `accountId` obrigatório para customer/user; `filters.lifecycle` aceita `active`, `hibernated` ou `all`.
- `resource.get`: `resourceType`, `resourceId` e `accountId` quando aplicável.
- `resource.create`: tipo, `accountId` para customer/user e `data` validado por capabilities.
- `resource.update`: tipo, ids, `changes` parcial limitado aos campos editáveis.
- `resource.deactivate`: hiberna o recurso agora, define `deactivatedAt` e `purgeAfter` segundo política; account desativada fica efetivamente indisponível com seus descendentes.
- `resource.reactivate`: cancela a purga antes do prazo e restaura apenas o estado alterado por esta desativação; não reativa descendentes que já estavam hibernados independentemente.
- `resource.purge`: ação irreversível declarada nas capacidades (`delete` ou `anonymize`). Só aceita recurso hibernado cujo `purgeAfter` já venceu. A resposta declara o modo e os dependentes afetados; cascatas nunca ficam implícitas. Requer scope `provisioning:purge`.

IDs são opacos para o Lynx OS. `customer` e `user` sempre são escopados à conta. Cursor e paginação não podem revelar outros tenants.

## Envelope de resposta e erros

Sucesso inclui `contractVersion`, `requestId`, `operation`, `status` (`completed`, `pending` quando há efeito assíncrono), `data` e `replayed`. Listagem inclui `items` e `page.nextCursor`.

Erro:

```json
{
  "error": {
    "code": "resource.not_found",
    "message": "Registro não encontrado",
    "fieldErrors": [],
    "requestId": "..."
  }
}
```

Códigos estáveis incluem `contract.unsupported_version`, `operation.unsupported`, `scope.forbidden`, `resource.not_found`, `resource.invalid_state`, `resource.purge_not_due`, `resource.has_active_children`, `idempotency.conflict`, `validation.invalid`. Respostas nunca incluem stack, SQL, chave de API, token de convite ou causa interna.

## Ciclo de vida

Padrão inicial: 180 dias, configurável no Lynx OS e nunca abaixo do mínimo anunciado pelo SaaS. Desativar não apaga nem altera dados cadastrais/financeiros: só bloqueia uso e marca o estado operacional. Uma restauração até `purgeAfter` cancela a purga. A execução definitiva é uma operação idempotente, auditada no Lynx OS e no SaaS. Falha remota deixa o pedido pendente para retry; não é apresentada como exclusão concluída.

Uma account hibernada impede login/operações daquela account, sem desligar a identidade global do usuário se ele tiver outros vínculos. Um cliente hibernado não aparece nas listas ativas nem aceita novas operações, mas seus registros permanecem íntegros durante a retenção. A exclusão permanente segue a política de dependências declarada pelo SaaS; por exemplo, no EasyBill apagar definitivamente um cliente remove também razão, contratos, visitas e sócios em cascata, e a interface deve mostrar isso antes da confirmação.

## Segurança operacional

- TLS obrigatório; segredo de API em header, nunca em URL ou body.
- Chaves distintas por integração e ambiente; comparação constante, scopes mínimos, rotação com janela curta e revogação imediata.
- Restrição de frequência e limites de paginação/tamanho de payload.
- Cada escrita guarda request id, actor/key id, operação, resource ids, resultado e chave de idempotência; não registra payload pessoal inteiro nem credenciais.
- Operações destrutivas verificam elegibilidade no SaaS mesmo quando o Lynx OS já conferiu o prazo.
- Para v1, `bootstrap` segue aceito sem alteração. Não há downgrade silencioso de operação v2 para v1.
