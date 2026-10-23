import type { Metadata } from "next";
import Image from "next/image";
import { PasswordRecoveryForm } from "@/components/password-recovery-form";

export const metadata: Metadata = { title: "Recuperar senha · Lynx OS", robots: { index: false, follow: false } };

export default function ForgotPasswordPage() {
  return <main className="login-screen"><section className="login-card">
    <div className="login-brand"><Image src="/lynx.jpg" alt="Símbolo da Lynx" width={72} height={72} className="login-brand-logo" priority /><div><p className="eyebrow">LYNX · SISTEMAS E AUTOMAÇÕES</p><p className="login-brand-name">LYNX <span>OS</span></p></div></div>
    <h1>Recuperar <span>acesso.</span></h1><p className="login-description">Informe o e-mail da sua conta. Se ele estiver cadastrado, enviaremos um link de redefinição.</p>
    <PasswordRecoveryForm />
  </section><aside className="login-visual" aria-hidden="true"><div className="login-visual-glow" /><Image src="/lynx.jpg" alt="" width={400} height={400} className="login-hero-logo" priority /><p>LYNX<span> / CENTRAL DE OPERAÇÕES</span></p></aside></main>;
}
