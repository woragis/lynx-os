import { redirect } from "next/navigation";
import Image from "next/image";
import Link from "next/link";
import { hasSession, isAuthConfigured } from "@/lib/server/session";
import { LoginForm } from "@/components/login-form";

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ sucesso?: string }> }) {
  if (await hasSession()) redirect("/accounts");
  const { sucesso } = await searchParams;
  const authConfigured = isAuthConfigured();
  return <main className="login-screen">
    <section className="login-card">
      <div className="login-brand">
        <Image src="/lynx.jpg" alt="Símbolo da Lynx" width={72} height={72} className="login-brand-logo" priority />
        <div><p className="eyebrow">LYNX · SISTEMAS E AUTOMAÇÕES</p><p className="login-brand-name">LYNX <span>OS</span></p></div>
      </div>
      <h1>Sua operação<br />precisa de <span>integração.</span></h1>
      <p className="login-description">Conecte processos, cadastre clientes e gerencie seus sistemas em um só lugar.</p>
      {sucesso === "senha" && <p className="success-banner" role="status">Senha atualizada. Entre com a nova senha.</p>}
      {sucesso === "convite" && <p className="success-banner" role="status">Conta ativada. Entre com seu e-mail e senha.</p>}
      {!authConfigured && <p className="error-banner" role="alert">Backend Go não configurado. Defina LYNX_GO_API_URL e LYNX_GO_INTERNAL_URL para habilitar o login.</p>}
      <LoginForm disabled={!authConfigured} />
      <Link className="auth-forgot-link" href="/forgot-password">Esqueci minha senha</Link>
      <p className="fine-print">Acesso restrito · sessão protegida por cookie HttpOnly</p>
    </section>
    <aside className="login-visual" aria-hidden="true">
      <div className="login-visual-glow" />
      <Image src="/lynx.jpg" alt="" width={400} height={400} className="login-hero-logo" priority />
      <p>LYNX<span> / CENTRAL DE OPERAÇÕES</span></p>
    </aside>
  </main>;
}
