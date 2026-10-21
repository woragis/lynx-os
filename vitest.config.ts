import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

process.env.DATABASE_URL ??= `file:${resolve(".test/lynx-os-vitest.db")}`;
import { defineConfig } from "vitest/config";
import tsconfigPaths from "vite-tsconfig-paths";

export default defineConfig({
  plugins: [tsconfigPaths()],
  resolve: {
    alias: {
      "server-only": fileURLToPath(new URL("./tests/server-only.ts", import.meta.url)),
    },
  },
  test: {
    globalSetup: ["./tests/global-setup.ts"],
    include: ["tests/unit/**/*.test.ts", "tests/integration/**/*.test.ts"],
    environment: "node",
    testTimeout: 15_000,
  },
});
