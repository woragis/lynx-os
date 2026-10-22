import "server-only";

import OpenAI from "openai";
import { z } from "zod";
import { randomUUID } from "node:crypto";
import { db } from "@/lib/server/db";
import { executeProvisioning, type BootstrapPayload } from "@/lib/server/provisioning";

const DraftSchema = z.object({
  integrationName: z.string().nullable(), accountName: z.string().nullable(), accountTaxId: z.string().nullable(), accountEmail: z.string().nullable(), accountAddress: z.string().nullable(),
  customerTradeName: z.string().nullable(), customerLegalName: z.string().nullable(), customerTaxId: z.string().nullable(), customerEmail: z.string().nullable(), customerPhone: z.string().nullable(), customerTaxRegime: z.string().nullable(), relationshipType: z.string().nullable(), customerStatus: z.string().nullable(),
  userName: z.string().nullable(), userEmail: z.string().nullable(), userPhone: z.string().nullable(),
});
type Draft = z.infer<typeof DraftSchema>;

const schema = {
  type: "object", additionalProperties: false,
  properties: Object.fromEntries(Object.keys(DraftSchema.shape).map((key) => [key, { type: ["string", "null"] }])) as Record<string, { type: string[] }>,
  required: Object.keys(DraftSchema.shape),
};

const fieldLabels: Record<keyof Draft, string> = {
  integrationName: "sistema (ex.: EasyBill)", accountName: "nome do escritório", accountTaxId: "CNPJ do escritório", accountEmail: "e-mail do escritório", accountAddress: "endereço do escritório",
  customerTradeName: "nome fantasia do cliente", customerLegalName: "", customerTaxId: "CNPJ ou CPF do cliente", customerEmail: "", customerPhone: "", customerTaxRegime: "regime tributário do cliente", relationshipType: "tipo de relação (Recorrente ou Avulso)", customerStatus: "status do cliente (Ativo ou Inativo)",
  userName: "nome do usuário responsável", userEmail: "e-mail de acesso", userPhone: "",
};
const requiredKeys: (keyof Draft)[] = ["integrationName", "accountName", "accountTaxId", "accountEmail", "accountAddress", "customerTradeName", "customerTaxId", "customerTaxRegime", "relationshipType", "customerStatus", "userName", "userEmail"];

const TelegramUpdateSchema = z.object({
  message: z.object({
    text: z.string().optional(),
    from: z.object({ id: z.number().int() }).optional(),
    chat: z.object({ id: z.number().int(), type: z.string() }).optional(),
  }).optional(),
});

function missing(draft: Draft) {
  return requiredKeys.filter((key) => !draft[key]?.trim()).map((key) => fieldLabels[key]);
}

function cleanDraft(incoming: Draft, previous?: Draft): Draft {
  const merged = Object.fromEntries(Object.keys(DraftSchema.shape).map((key) => [key, incoming[key as keyof Draft] ?? previous?.[key as keyof Draft] ?? null])) as Draft;
  const normalized = { ...merged };
  if (normalized.relationshipType) {
    const value = normalized.relationshipType.toLocaleLowerCase("pt-BR");
    normalized.relationshipType = value.startsWith("recorr") ? "Recorrente" : value.startsWith("avuls") ? "Avulso" : null;
  }
  if (normalized.customerStatus) {
    const value = normalized.customerStatus.toLocaleLowerCase("pt-BR");
    normalized.customerStatus = value.startsWith("ativ") ? "Ativo" : value.startsWith("inativ") ? "Inativo" : null;
  }
  return normalized;
}

async function extractDraft(text: string, previous: Draft | undefined, integrations: { name: string; slug: string }[]) {
  const apiKey = process.env.OPENAI_API_KEY;
  if (!apiKey) throw new Error("OPENAI_API_KEY não configurada.");
  const openai = new OpenAI({ apiKey });
  const response = await openai.responses.create({
    model: process.env.OPENAI_MODEL || "gpt-6-luna",
    reasoning: { effort: "low" },
    instructions: [
      "Extraia informações para um rascunho de provisionamento de SaaS Lynx.",
      "Retorne apenas os valores explicitamente informados; use null para desconhecidos. Nunca invente, complete por contexto ou copie CNPJ, e-mail ou telefone entre escritório, cliente e usuário, a menos que a pessoa diga que o dado é compartilhado.",
      "Mescle o rascunho anterior com a mensagem nova. A mensagem nova pode corrigir um valor anterior.",
      `Sistemas disponíveis: ${integrations.map((item) => `${item.name} (identificador ${item.slug})`).join(", ") || "nenhum"}. Para integrationName retorne o nome de um sistema disponível somente se houver correspondência clara.`,
      "relationshipType só pode ser Recorrente ou Avulso. customerStatus só pode ser Ativo ou Inativo. Não defina esses campos por conta própria.",
      "Dados da pessoa responsável são os do usuário inicial, não os do cliente.",
    ].join(" "),
    input: JSON.stringify({ previousDraft: previous ?? null, message: text }),
    text: { format: { type: "json_schema", name: "lynx_provisioning_draft", strict: true, schema } },
    max_output_tokens: 700,
    store: false,
  });
  return DraftSchema.parse(JSON.parse(response.output_text));
}

function payloadFromDraft(draft: Draft): BootstrapPayload {
  return {
    contractVersion: "1",
    operation: "bootstrap",
    input: {
      account: { name: draft.accountName!, taxId: draft.accountTaxId!, email: draft.accountEmail!, address: draft.accountAddress! },
      initialCustomer: { tradeName: draft.customerTradeName!, legalName: draft.customerLegalName ?? "", taxId: draft.customerTaxId!, email: draft.customerEmail ?? "", phone: draft.customerPhone ?? "", taxRegime: draft.customerTaxRegime!, relationshipType: draft.relationshipType!, status: draft.customerStatus! },
      initialUser: { name: draft.userName!, email: draft.userEmail!, phone: draft.userPhone ?? "" },
    },
  };
}

function formatPreview(draft: Draft, integrationName: string) {
  return [
    `Vou cadastrar no ${integrationName}:`,
    `• Escritório: ${draft.accountName} · ${draft.accountTaxId}`,
    `  E-mail: ${draft.accountEmail} · Endereço: ${draft.accountAddress}`,
    `• Cliente: ${draft.customerTradeName}${draft.customerLegalName ? ` (${draft.customerLegalName})` : ""} · ${draft.customerTaxId}`,
    ...(draft.customerEmail || draft.customerPhone ? [`  Contato: ${[draft.customerEmail, draft.customerPhone].filter(Boolean).join(" · ")}`] : []),
    `• Regime/tipo/status: ${draft.customerTaxRegime} · ${draft.relationshipType} · ${draft.customerStatus}`,
    `• Usuário owner: ${draft.userName} · ${draft.userEmail}${draft.userPhone ? ` · ${draft.userPhone}` : ""}`,
    "",
    "Responda CONFIRMAR para executar ou CANCELAR para descartar.",
  ].join("\n");
}

export async function processTelegramUpdate(input: unknown): Promise<string | null> {
  const parsed = TelegramUpdateSchema.safeParse(input);
  if (!parsed.success) return null;
  const update = parsed.data;
  const message = update.message;
  const text = message?.text?.trim();
  const fromId = message?.from?.id;
  const chatId = message?.chat?.id;
  if (!text || !fromId || !chatId || message?.chat?.type !== "private") return null;

  const linkMatch = text.match(/^\/link(?:@\w+)?\s+(\d{8})$/i);
  if (linkMatch) {
    const baseURL = process.env.LYNX_GO_INTERNAL_URL || process.env.LYNX_GO_API_URL;
    const internalToken = process.env.LYNX_TELEGRAM_INTERNAL_TOKEN;
    if (!baseURL || !internalToken) return "O vínculo Telegram não está configurado no backend.";
    try {
      const response = await fetch(`${baseURL.replace(/\/$/, "")}/api/v1/auth/telegram/link`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${internalToken}` },
        body: JSON.stringify({ telegramUserId: String(fromId), code: linkMatch[1] }),
        signal: AbortSignal.timeout(5_000),
        cache: "no-store",
      });
      if (response.ok) return "Conta Telegram vinculada com sucesso.";
      if (response.status === 409) return "Esta conta Telegram já está vinculada a outro administrador.";
      return "Código inválido ou expirado. Gere um novo código na área de segurança do Lynx OS.";
    } catch {
      return "Não consegui falar com o backend agora. Tente novamente em instantes.";
    }
  }

  const allowedIds = (process.env.TELEGRAM_ALLOWED_USER_IDS ?? "").split(",").map((value) => value.trim()).filter(Boolean);
  if (!allowedIds.includes(String(fromId))) return null;

  if (text === "/start") {
    return "Lynx OS pronto. Descreva o sistema e os dados do escritório, do cliente e da pessoa que terá acesso. Vou revisar tudo com você antes de cadastrar.\n\nUse /link CÓDIGO para vincular sua conta Lynx OS, ou /cancelar para apagar o rascunho.";
  }
  if (text.toLocaleLowerCase("pt-BR") === "/cancelar" || text.toLocaleLowerCase("pt-BR") === "cancelar") {
    await db.telegramConversation.deleteMany({ where: { chatId: String(chatId) } });
    return "Rascunho descartado.";
  }

  const conversation = await db.telegramConversation.findUnique({ where: { chatId: String(chatId) } });
  if (conversation && Date.now() - conversation.updatedAt.getTime() > 24 * 60 * 60 * 1000) {
    await db.telegramConversation.delete({ where: { chatId: String(chatId) } });
  }
  if (conversation && Date.now() - conversation.updatedAt.getTime() <= 24 * 60 * 60 * 1000 && conversation.state === "AWAITING_CONFIRMATION") {
    if (text.toLocaleUpperCase("pt-BR") !== "CONFIRMAR") {
      return "O rascunho está pronto. Responda CONFIRMAR para executar ou CANCELAR para descartar.";
    }
    const stored = JSON.parse(conversation.draftJson) as { integrationId: string; draft: Draft; idempotencyKey: string };
    const payload = payloadFromDraft(stored.draft);
    try {
      const outcome = await executeProvisioning(stored.integrationId, payload, stored.idempotencyKey);
      if (outcome.status === "RETRYABLE") {
        return "O SaaS respondeu com uma falha temporária. Responda CONFIRMAR para tentar de novo com a mesma chave segura, ou CANCELAR para descartar.";
      }
      await db.telegramConversation.deleteMany({ where: { chatId: String(chatId) } });
      return `Solicitação enviada. Estado: ${outcome.status}.\n${JSON.stringify(outcome.body).slice(0, 2800)}`;
    } catch {
      return "Não consegui concluir a chamada. O mesmo pedido pode ser enviado novamente com segurança; confira o painel para o resultado.";
    }
  }

  const integrations = await db.integration.findMany({ where: { active: true }, select: { id: true, name: true, slug: true } });
  const previous = conversation && Date.now() - conversation.updatedAt.getTime() <= 24 * 60 * 60 * 1000 ? DraftSchema.parse((JSON.parse(conversation.draftJson) as { draft: Draft }).draft) : undefined;
  let draft: Draft;
  try {
    draft = cleanDraft(await extractDraft(text, previous, integrations), previous);
  } catch {
    return "Não consegui organizar os dados agora. Tente descrever o cadastro em frases simples ou continue informando os campos que faltam.";
  }
  const integration = integrations.find((item) => item.slug === draft.integrationName?.toLocaleLowerCase("pt-BR") || item.name.toLocaleLowerCase("pt-BR") === draft.integrationName?.toLocaleLowerCase("pt-BR"));
  const stillMissing = missing(draft);
  if (!integration) stillMissing.unshift("sistema conectado (" + integrations.map((item) => item.name).join(", ") + ")");
  if (stillMissing.length || !integration) {
    await db.telegramConversation.upsert({ where: { chatId: String(chatId) }, create: { chatId: String(chatId), state: "COLLECTING", draftJson: JSON.stringify({ draft }) }, update: { state: "COLLECTING", draftJson: JSON.stringify({ draft }) } });
    return `Ainda preciso de: ${stillMissing.join(", ")}.\n\nPode mandar os dados que faltam em uma mensagem. Use /cancelar para começar de novo.`;
  }
  const idempotencyKey = `tg-${randomUUID()}`;
  await db.telegramConversation.upsert({ where: { chatId: String(chatId) }, create: { chatId: String(chatId), state: "AWAITING_CONFIRMATION", draftJson: JSON.stringify({ integrationId: integration.id, draft, idempotencyKey }) }, update: { state: "AWAITING_CONFIRMATION", draftJson: JSON.stringify({ integrationId: integration.id, draft, idempotencyKey }) } });
  return formatPreview(draft, integration.name);
}
