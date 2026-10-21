import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { db } from "@/lib/server/db";
import { POST } from "@/app/api/internal/telegram/update/route";

beforeAll(() => {
  process.env.LYNX_TELEGRAM_INTERNAL_TOKEN = "internal-token-for-tests-at-least-32";
  process.env.TELEGRAM_ALLOWED_USER_IDS = "42";
});

afterAll(async () => db.$disconnect());

describe("POST /api/internal/telegram/update", () => {
  it("rejeita token interno ausente antes de processar update", async () => {
    const spy = vi.spyOn(db.telegramConversation, "findUnique");
    const response = await POST(new NextRequest("http://localhost/api/internal/telegram/update", {
      method: "POST", body: JSON.stringify({ message: { text: "/start" } }),
    }));
    expect(response.status).toBe(401);
    expect(await response.json()).toEqual({ error: "unauthorized" });
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });

  it("aceita update interno, mas ignora usuário que não está na allowlist", async () => {
    const response = await POST(new NextRequest("http://localhost/api/internal/telegram/update", {
      method: "POST",
      headers: { authorization: "Bearer internal-token-for-tests-at-least-32", "content-type": "application/json" },
      body: JSON.stringify({ message: { text: "/start", from: { id: 999 }, chat: { id: 999, type: "private" } } }),
    }));
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ reply: null });
    expect(await db.telegramConversation.count()).toBe(0);
  });
});
