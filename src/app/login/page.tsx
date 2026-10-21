import { redirect } from "next/navigation";
import { loginAction } from "@/app/actions";
import { hasSession } from "@/lib/server/session";

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ erro?: string }> }) {
  if (await hasSession()) redirect("/provisioning");
  const { erro } = await searchParams;
  return <main className="login-screen"><section className="login-card">
    <div className="brand-mark">L</div><p className="eyebrow">LYNX · OPERAÇÕES</p>
    <h1>Seu ponto central.</h1><p className="muted">Acesse o ambiente de provisionamento da Lynx.</p>
    {erro && <p className="error-banner" role="alert">E-mail ou senha inválidos.</p>}
    <form action={loginAction} className="stack-form">
      <label>E-mail<input name="email" type="email" autoComplete="username" required /></label>
      <label>Senha<input name="password" type="password" autoComplete="current-password" required /></label>
      <button className="button button-dark" type="submit">Entrar <span aria-hidden>↗</span></button>
    </form>
    <p className="fine-print">Acesso restrito · sessão protegida por cookie HttpOnly</p>
  </section></main>;
}
