# Contrato Lynx de provisionamento v1

Cada SaaS integrado expõe a rota única `/api/v1/provisioning`:

- `GET`: capacidades e `contractVersion: "1"`;
- `POST`: operação `bootstrap`, com empresa (`account`), cliente inicial (`initialCustomer`) e usuário/vínculo inicial (`initialUser`).

Autenticação por `X-Lynx-API-Key` sobre HTTPS. A credencial cadastrada no Lynx OS reúne somente `provisioning:read` (GET) e `provisioning:write` (POST). A chamada de escrita inclui `Idempotency-Key`. Repetir a mesma chave e corpo deve retornar o resultado original; repetir a chave com outro corpo deve responder `409`.

O formato comum usa identificadores neutros (`account`, `initialCustomer`, `initialUser`). O `GET` descreve os campos obrigatórios, opções válidas e defaults explícitos de cada produto. O Lynx OS pergunta por valores obrigatórios ausentes e não infere decisões comerciais.

No EasyBill, `account` cria `Company` (o escritório/tenant), `initialCustomer` cria `Client` dentro dessa empresa e `initialUser` cria ou reutiliza `User`, cria `CompanyUser` com papel owner e envia convite. A pessoa define a própria senha pelo convite. O provisionamento não aceita honorário; o valor segue derivado do razão. `taxRegime`, `relationshipType` (`Recorrente`/`Avulso`) e `status` (`Ativo`/`Inativo`) são exigidos para o cliente.

Resposta de sucesso inclui `requestId`, `status`, os IDs/estados dos três recursos e o estado do convite. Erros usam `{ "code", "message", "fieldErrors", "requestId" }`. Nenhuma resposta inclui credenciais ou token de convite.

O contrato v1 cria o conjunto inicial; não fornece listagem ampla, edição, exclusão, senha arbitrária ou acesso de administração geral. A chave tem estado ativo/revogado e deve poder ser rotacionada com uma janela curta em que duas chaves coexistem.
