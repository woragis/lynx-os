import { expect, test } from "@playwright/test";

test("autentica, cadastra e revoga uma integração", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("E-mail").fill("admin@lynx.test");
  await page.getByLabel("Senha").fill("e2e-password-secure");
  await page.getByRole("button", { name: /Entrar/ }).click();
  await expect(page).toHaveURL(/\/accounts$/);

  await page.goto("/integrations");
  await page.getByLabel("Nome do sistema").fill("EasyBill E2E");
  await page.getByLabel("URL da API").fill("http://localhost:8182");
  await page.getByLabel("API key").fill("e2e-api-key-secret");
  await page.getByRole("button", { name: /Salvar integração/ }).click();
  await expect(page.getByText("Integração salva.")).toBeVisible();
  await expect(page.getByRole("heading", { name: "EasyBill E2E" })).toBeVisible();

  await page.getByText("Editar SaaS", { exact: true }).click();
  await page.locator('.edit-details input[name="name"]').fill("EasyBill E2E Editado");
  await page.getByRole("button", { name: "Salvar alterações" }).click();
  await expect(page.getByText("Dados da integração atualizados.")).toBeVisible();
  await expect(page.getByRole("heading", { name: "EasyBill E2E Editado" })).toBeVisible();

  await page.getByRole("button", { name: "Pausar" }).click();
  await expect(page.locator(".state-chip")).toHaveText("Pausada");
  await page.getByRole("button", { name: "Ativar" }).click();
  await expect(page.locator(".state-chip")).toHaveText("Ativa");

  await page.getByText("Revogar", { exact: true }).click();
  await page.getByRole("button", { name: "Confirmar revogação" }).click();
  await expect(page.getByText("Revogada localmente")).toBeVisible();
  await expect(page.getByText("API key apagada do Lynx OS")).toBeVisible();
});

test("provisiona empresa, cliente e acesso pela API Go", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("E-mail").fill("admin@lynx.test");
  await page.getByLabel("Senha").fill("e2e-password-secure");
  await page.getByRole("button", { name: /Entrar/ }).click();
  await expect(page).toHaveURL(/\/accounts$/);

  await page.goto("/integrations");
  await page.getByLabel("Nome do sistema").fill("EasyBill Provisionamento E2E");
  await page.getByLabel("URL da API").fill("http://localhost:8182");
  await page.getByLabel("API key").fill("e2e-api-key-secret");
  await page.getByRole("button", { name: /Salvar integração/ }).click();
  await expect(page.getByText("Integração salva.")).toBeVisible();

  await page.goto("/provisioning");
  await page.getByLabel("Sistema").selectOption({ label: "EasyBill Provisionamento E2E" });
  await page.getByLabel("Nome do escritório").fill("Escritório E2E");
  await page.getByLabel("CNPJ do escritório").fill("12345678000199");
  await page.getByLabel("E-mail do escritório").fill("admin@escritorio.test");
  await page.getByLabel("Endereço completo").fill("Rua de Teste, 10");
  await page.getByLabel("Nome fantasia").fill("Cliente E2E");
  await page.getByLabel("CNPJ ou CPF").fill("98765432000199");
  await page.getByLabel("Regime tributário").fill("Simples Nacional");
  await page.getByLabel("Tipo de relação").selectOption("Recorrente");
  await page.getByLabel("Status").selectOption("Ativo");
  await page.getByLabel("Nome completo").fill("Ana E2E");
  await page.getByLabel("E-mail de acesso").fill("ana@escritorio.test");
  await page.getByRole("button", { name: /Criar cadastro/ }).click();

  await expect(page.getByRole("heading", { name: "Cadastro processado" })).toBeVisible();
  await expect(page.getByRole("link", { name: /Abrir tela de login/ })).toHaveAttribute("href", "https://easybill.test/login");
  await expect(page.getByText("Convite enviado para criar a senha.")).toBeVisible();
});

test("bloqueia o painel sem sessão", async ({ page }) => {
  await page.goto("/integrations");
  await expect(page).toHaveURL(/\/login$/);
});

test("mostra e permite revogar a sessão atual", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("E-mail").fill("admin@lynx.test");
  await page.getByLabel("Senha").fill("e2e-password-secure");
  await page.getByRole("button", { name: /Entrar/ }).click();
  await expect(page).toHaveURL(/\/accounts$/);

  await page.goto("/security");
  await expect(page.getByRole("heading", { name: "Sessões abertas" })).toBeVisible();
  await expect(page.getByText("Esta sessão")).toBeVisible();
  page.on("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Revogar" }).click();
  await expect(page).toHaveURL(/\/login$/);
});
