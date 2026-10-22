import { expect, test } from "@playwright/test";

test("autentica, cadastra e revoga uma integração", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("E-mail").fill("admin@lynx.test");
  await page.getByLabel("Senha").fill("e2e-password-secure");
  await page.getByRole("button", { name: /Entrar/ }).click();
  await expect(page).toHaveURL(/\/accounts$/);

  await page.goto("/integrations");
  await page.getByLabel("Nome do sistema").fill("EasyBill E2E");
  await page.getByLabel("URL base").fill("https://easybill.example.test");
  await page.getByLabel("API key").fill("e2e-api-key-secret");
  await page.getByRole("button", { name: /Salvar integração/ }).click();
  await expect(page.getByText("Integração salva.")).toBeVisible();
  await expect(page.getByRole("heading", { name: "EasyBill E2E" })).toBeVisible();

  await page.getByText("Revogar", { exact: true }).click();
  await page.getByRole("button", { name: "Confirmar revogação" }).click();
  await expect(page.getByText("Revogada localmente")).toBeVisible();
  await expect(page.getByText("API key apagada do Lynx OS")).toBeVisible();
});

test("bloqueia o painel sem sessão", async ({ page }) => {
  await page.goto("/integrations");
  await expect(page).toHaveURL(/\/login$/);
});
