import { afterEach, describe, expect, it } from "vitest";
import { decryptSecret, encryptSecret } from "@/lib/server/crypto";

const originalKey = process.env.LYNX_ENCRYPTION_KEY;

afterEach(() => {
  if (originalKey === undefined) delete process.env.LYNX_ENCRYPTION_KEY;
  else process.env.LYNX_ENCRYPTION_KEY = originalKey;
});

describe("criptografia de credenciais", () => {
  it("cifra com IV distinto e recupera o segredo original", () => {
    process.env.LYNX_ENCRYPTION_KEY = "a".repeat(64);
    const first = encryptSecret("lynx-api-key-super-secreta");
    const second = encryptSecret("lynx-api-key-super-secreta");

    expect(first.ciphertext).not.toContain("lynx-api-key-super-secreta");
    expect(first.iv).not.toBe(second.iv);
    expect(decryptSecret(first.ciphertext, first.iv, first.authTag)).toBe("lynx-api-key-super-secreta");
  });

  it("falha fechado se a chave estiver ausente ou inválida", () => {
    delete process.env.LYNX_ENCRYPTION_KEY;
    expect(() => encryptSecret("segredo")).toThrow(/32 bytes em hexadecimal/);
    process.env.LYNX_ENCRYPTION_KEY = "curta";
    expect(() => encryptSecret("segredo")).toThrow(/32 bytes em hexadecimal/);
  });

  it("rejeita conteúdo adulterado", () => {
    process.env.LYNX_ENCRYPTION_KEY = "b".repeat(64);
    const encrypted = encryptSecret("segredo");
    expect(() => decryptSecret(`${encrypted.ciphertext.slice(0, -4)}AAAA`, encrypted.iv, encrypted.authTag)).toThrow();
  });
});
