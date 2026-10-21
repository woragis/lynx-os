import { timingSafeEqual } from "node:crypto";
import { processTelegramUpdate } from "@/lib/server/telegram";

function sameSecret(actual: string | null, expected: string) {
  if (!actual) return false;
  const a = Buffer.from(actual);
  const b = Buffer.from(expected);
  return a.length === b.length && timingSafeEqual(a, b);
}

export async function POST(request: Request) {
  const expected = process.env.LYNX_TELEGRAM_INTERNAL_TOKEN;
  if (!expected || !sameSecret(request.headers.get("authorization")?.replace(/^Bearer\s+/i, "") ?? null, expected)) {
    return Response.json({ error: "unauthorized" }, { status: 401 });
  }

  const contentLength = Number(request.headers.get("content-length") ?? 0);
  if (contentLength > 64_000) return Response.json({ error: "payload_too_large" }, { status: 413 });

  try {
    const update: unknown = await request.json();
    const reply = await processTelegramUpdate(update);
    return Response.json({ reply });
  } catch {
    return Response.json({ error: "telegram_update_failed" }, { status: 500 });
  }
}
