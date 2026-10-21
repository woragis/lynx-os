import { redirect } from "next/navigation";
import Image from "next/image";
import { loginAction } from "@/app/actions";
import { hasSession } from "@/lib/server/session";

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ erro?: string }> }) {
  if (await hasSession()) redirect("/provisioning");
  const { erro } = await searchParams;
  return <main className="login-screen">
    <section className="login-card">
      <div className="login-brand">
        <Image src="/lynx.jpg" alt="Símbolo da Lynx" width={72} height={72} className="login-brand-logo" priority />
        <div><p className="eyebrow">LYNX · SISTEMAS E AUTOMAÇÕES</p><p className="login-brand-name">LYNX <span>OS</span></p></div>
      </div>
      <h1>Sua operação<br />precisa de <span>integração.</span></h1>
      <p className="login-description">Conecte processos, cadastre clientes e gerencie seus sistemas em um só lugar.</p>
      {erro && <p className="error-banner" role="alert">E-mail ou senha inválidos.</p>}
      <form action={loginAction} className="stack-form">
        <label>E-mail<input name="email" type="email" autoComplete="username" required /></label>
        <label>Senha<input name="password" type="password" autoComplete="current-password" required /></label>
        <button className="button button-dark" type="submit">Entrar <span aria-hidden>↗</span></button>
      </form>
      <p className="fine-print">Acesso restrito · sessão protegida por cookie HttpOnly</p>
    </section>
    <aside className="login-visual" aria-hidden="true">
      <div className="login-visual-glow" />
      <Image src="/lynx.jpg" alt="" width={400} height={400} className="login-hero-logo" priority />
      <p>LYNX<span> / CENTRAL DE OPERAÇÕES</span></p>
    </aside>
  </main>;
}
