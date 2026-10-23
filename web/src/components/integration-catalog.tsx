"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { editIntegrationAction, revokeIntegrationAction, rotateIntegrationKeyAction, toggleIntegrationAction } from "@/app/actions";

type Integration = { id: string; name: string; baseUrl: string; appUrl?: string; active: boolean; revokedAt?: string };

export function IntegrationCatalog({ integrations }: { integrations: Integration[] }) {
  const [editing, setEditing] = useState<Integration | null>(null);
  const closeEditor = useCallback(() => setEditing(null), []);

  return <>
    <div className="integration-list">
      {integrations.map((item) => <article className={`integration-card integration-card-large ${item.revokedAt ? "integration-card-revoked" : ""}`} key={item.id}>
        <div className="integration-card-main">
          <div className="integration-logo integration-logo-large">{item.name.slice(0, 1).toUpperCase()}</div>
          <div className="integration-details">
            <div className="integration-name-row"><h3>{item.name}</h3><span className={`state-chip ${item.revokedAt ? "paused" : item.active ? "active" : "paused"}`}>{item.revokedAt ? "Revogada" : item.active ? "Ativa" : "Pausada"}</span></div>
            <p className="integration-api-url">API · {item.baseUrl}</p>
            {item.appUrl ? <p className="integration-app-url">Painel · {item.appUrl}</p> : <p className="integration-app-url integration-no-app">Nenhum link de painel configurado</p>}
            <div className="integration-contract-info"><span className="contract-check">✓</span><span>Contrato de provisionamento Lynx v1</span><span className="integration-info-separator">·</span><span>{item.revokedAt ? "Chave removida deste ambiente" : "Chave protegida no servidor"}</span></div>
          </div>
        </div>

        <div className="integration-card-actions">
          {!item.revokedAt && <>
            {item.active
              ? <Link className="button button-dark integration-primary-action" href={`/provisioning?integrationId=${encodeURIComponent(item.id)}`}>＋ Novo cadastro neste SaaS</Link>
              : <p className="integration-revoked-explanation">Ative a integração para voltar a cadastrar e consultar registros neste SaaS.</p>}
            <div className="integration-secondary-actions">
              {item.active && <Link className="quiet-button" href={`/accounts?integrationId=${encodeURIComponent(item.id)}`}>Ver empresas e acessos</Link>}
              {item.appUrl && <a className="quiet-button" href={item.appUrl} target="_blank" rel="noreferrer">Abrir painel ↗</a>}
              <button className="quiet-button" type="button" onClick={() => setEditing(item)}>Editar integração</button>
              <form action={toggleIntegrationAction}><input type="hidden" name="id" value={item.id} /><button className="quiet-button" type="submit">{item.active ? "Pausar integração" : "Ativar integração"}</button></form>
              <details className="integration-action-menu"><summary>Mais ações</summary><div className="integration-action-popover">
                <form action={rotateIntegrationKeyAction}><input type="hidden" name="id" value={item.id} /><label>Trocar API key<input type="password" name="apiKey" autoComplete="new-password" placeholder="Nova API key" required /></label><button className="quiet-button" type="submit">Validar e substituir chave</button></form>
                <form action={revokeIntegrationAction} className="integration-revoke-form"><input type="hidden" name="id" value={item.id} /><p>Remove a chave local e bloqueia as chamadas. Também revogue a chave no painel do SaaS.</p><button type="submit">Revogar integração</button></form>
              </div></details>
            </div>
          </>}
          {item.revokedAt && <p className="integration-revoked-explanation">Esta integração não pode ser reativada. Conecte novamente o SaaS para gerar um novo vínculo.</p>}
        </div>
      </article>)}
    </div>
    {editing && <EditIntegrationDialog integration={editing} onClose={closeEditor} />}
  </>;
}

function EditIntegrationDialog({ integration, onClose }: { integration: Integration; onClose: () => void }) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    const close = () => onClose();
    dialog.addEventListener("close", close);
    dialog.showModal();
    return () => dialog.removeEventListener("close", close);
  }, [onClose]);

  return <dialog className="integration-edit-dialog" ref={dialogRef} onClick={(event) => { if (event.target === event.currentTarget) event.currentTarget.close(); }}>
    <div className="integration-dialog-heading"><div><p className="eyebrow">CONFIGURAÇÃO DO SAAS</p><h2>Editar {integration.name}</h2><p>Atualize os endereços usados para conectar e abrir este sistema.</p></div><button className="dialog-close" type="button" aria-label="Fechar" onClick={() => dialogRef.current?.close()}>×</button></div>
    <form action={editIntegrationAction} className="integration-edit-form"><input type="hidden" name="id" value={integration.id} />
      <label>Nome do sistema<input name="name" defaultValue={integration.name} required maxLength={120} /></label>
      <label>URL da API<input name="baseUrl" type="url" defaultValue={integration.baseUrl} required /></label>
      <label>URL do painel <span className="optional">(opcional)</span><input name="appUrl" type="url" defaultValue={integration.appUrl ?? ""} placeholder="https://seusistema.com.br/login" /></label>
      <div className="integration-dialog-footer"><button className="quiet-button" type="button" onClick={() => dialogRef.current?.close()}>Cancelar</button><button className="button button-dark" type="submit">Salvar alterações</button></div>
    </form>
  </dialog>;
}
