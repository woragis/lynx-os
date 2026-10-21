import { timingSafeEqual } from "node:crypto";
import { NextRequest, NextResponse } from "next/server";
import { processTelegramUpdate } from "@/lib/server/telegram";

function sameSecret(actual: string | null, expected: string) {
  if (!actual) return false;
  const a = Buffer.from(actual);
  const b = Buffer.from(expected);
  return a.length === b.length && timingSafeEqual(a, b);
}

export async function POST(request: NextRequest) {
  const expected = process.env.TELEGRAM_WEBHOOK_SECRET;
  if (!expected || !sameSecret(request.headers.get("x-telegram-bot-api-secret-token"), expected)) {
    return NextResponse.json({ ok: false }, { status: 401 });
  }
  const contentLength = Number(request.headers.get("content-length") ?? 0);
  if (contentLength > 64_000) return NextResponse.json({ ok: false }, { status: 413 });
  try {
    const update = await request.json();
    await processTelegramUpdate(update);
    return NextResponse.json({ ok: true });
  } catch {
    return NextResponse.json({ ok: false }, { status: 500 });
  }
}
