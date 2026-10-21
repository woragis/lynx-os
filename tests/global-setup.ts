import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";

export default function setup() {
  const databasePath = resolve(".test/lynx-os-vitest.db");
  mkdirSync(dirname(databasePath), { recursive: true });
  process.env.DATABASE_URL = `file:${databasePath}`;
  execFileSync("npx", ["prisma", "db", "push", "--schema", "prisma/schema.prisma"], {
    stdio: "inherit",
    env: { ...process.env, DATABASE_URL: `file:${databasePath}` },
  });
}
