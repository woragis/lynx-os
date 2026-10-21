import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { db } from "@/lib/server/db";
import { POST } from "@/app/api/telegram/webhook/route";

beforeAll(() => {
  process.env.TELEGRAM_WEBHOOK_SECRET = "webhook-secret-for-tests";
  process.env.TELEGRAM_ALLOWED_USER_IDS = "42";
});

afterAll(async () => db.$disconnect());

describe("POST /api/telegram/webhook", () => {
  it("rejeita segredo ausente ou incorreto antes de processar update", async () => {
    const spy = vi.spyOn(db.telegramConversation, "findUnique");
    const response = await POST(new NextRequest("http://localhost/api/telegram/webhook", {
      method: "POST", body: JSON.stringify({ message: { text: "/start" } }),
    }));
    expect(response.status).toBe(401);
    expect(await response.json()).toEqual({ ok: false });
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });

  it("aceita update autenticado sem executar comandos de usuário não permitido", async () => {
    const response = await POST(new NextRequest("http://localhost/api/telegram/webhook", {
      method: "POST",
      headers: { "x-telegram-bot-api-secret-token": "webhook-secret-for-tests", "content-type": "application/json" },
      body: JSON.stringify({ message: { text: "/start", from: { id: 999 }, chat: { id: 999, type: "private" } } }),
    }));
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ ok: true });
    expect(await db.telegramConversation.count()).toBe(0);
  });
});
