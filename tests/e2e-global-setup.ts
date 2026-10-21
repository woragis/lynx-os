import { execFileSync } from "node:child_process";
import { mkdirSync, rmSync } from "node:fs";
import { dirname, resolve } from "node:path";

export default function setup() {
  const databasePath = resolve(".test/lynx-os-e2e.db");
  mkdirSync(dirname(databasePath), { recursive: true });
  rmSync(databasePath, { force: true });
  execFileSync("npx", ["prisma", "db", "push", "--schema", "prisma/schema.prisma"], {
    stdio: "inherit",
    env: { ...process.env, DATABASE_URL: `file:${databasePath}` },
  });
}
