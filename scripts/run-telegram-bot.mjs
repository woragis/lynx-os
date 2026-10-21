import "dotenv/config";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const child = spawn("go", ["run", "./cmd/lynx-telegram"], {
  cwd: resolve(root, "telegram-bot"),
  env: process.env,
  stdio: "inherit",
});

child.on("error", (error) => {
  process.stderr.write(`Não foi possível iniciar o serviço Go: ${error.message}\n`);
  process.exitCode = 1;
});
child.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exitCode = code ?? 1;
});
