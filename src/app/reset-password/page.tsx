import type { Metadata } from "next";
import Image from "next/image";
import { OneTimeAccessForm } from "@/components/one-time-access-form";

export const metadata: Metadata = { title: "Redefinir senha · Lynx OS", robots: { index: false, follow: false }, referrer: "no-referrer" };

export default async function ResetPasswordPage({ searchParams }: { searchParams: Promise<{ token?: string }> }) {
  const { token = "" } = await searchParams;
  return <main className="login-screen"><section className="login-card">
    <div className="login-brand"><Image src="/lynx.jpg" alt="Símbolo da Lynx" width={72} height={72} className="login-brand-logo" priority /><div><p className="eyebrow">LYNX · SISTEMAS E AUTOMAÇÕES</p><p className="login-brand-name">LYNX <span>OS</span></p></div></div>
    <h1>Defina uma <span>nova senha.</span></h1><p className="login-description">Use uma senha com pelo menos 12 caracteres. O link expira em 15 minutos.</p>
    <OneTimeAccessForm mode="reset" token={token} />
  </section><aside className="login-visual" aria-hidden="true"><div className="login-visual-glow" /><Image src="/lynx.jpg" alt="" width={400} height={400} className="login-hero-logo" priority /><p>LYNX<span> / CENTRAL DE OPERAÇÕES</span></p></aside></main>;
}
