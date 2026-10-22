import { SessionManager } from "@/components/session-manager";
import { TelegramLink } from "@/components/telegram-link";

export default function SecurityPage() {
  return <div className="page-wrap">
    <div className="page-heading"><div><p className="eyebrow">CONTA LYNX OS</p><h1>Segurança</h1><p className="muted">Revise e encerre as sessões da sua conta.</p></div><span className="secure-tag"><span /> SESSÕES PROTEGIDAS</span></div>
    <SessionManager />
    <TelegramLink />
  </div>;
}
