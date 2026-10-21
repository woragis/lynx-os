import "server-only";

import { PrismaBetterSqlite3 } from "@prisma/adapter-better-sqlite3";
import { PrismaClient } from "@/generated/prisma/client";

const globalForPrisma = globalThis as unknown as { lynxPrisma?: PrismaClient };

function createPrismaClient() {
  const adapter = new PrismaBetterSqlite3({ url: process.env.DATABASE_URL ?? "file:./lynx-os.db" });
  return new PrismaClient({ adapter });
}

export const db = globalForPrisma.lynxPrisma ?? createPrismaClient();

if (process.env.NODE_ENV !== "production") globalForPrisma.lynxPrisma = db;
