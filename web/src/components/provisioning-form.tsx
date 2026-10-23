"use client";

import { useActionState, useState } from "react";
import { provisionAction, type ProvisionFormState } from "@/app/actions";

type IntegrationOption = { id: string; name: string };
type FieldName = "integrationId" | "accountName" | "accountTaxId" | "accountEmail" | "accountAddress" | "customerTradeName" | "customerLegalName" | "customerTaxId" | "customerEmail" | "customerPhone" | "customerTaxRegime" | "relationshipType" | "customerStatus" | "userName" | "userEmail" | "userPhone";
type TextFieldProps = { name: FieldName; label: string; required?: boolean; optional?: boolean; type?: "text" | "email" | "tel"; placeholder?: string; inputMode?: "text" | "numeric" | "tel"; value: string; error?: string; onChange: (name: FieldName, value: string) => void };

const fieldLabels: Record<string, string> = {
  integrationId: "Sistema", accountName: "Nome do escritório", accountTaxId: "CNPJ do escritório", accountEmail: "E-mail do escritório", accountAddress: "Endereço completo",
  customerTradeName: "Nome fantasia", customerLegalName: "Razão social", customerTaxId: "CNPJ ou CPF do cliente", customerEmail: "E-mail do cliente", customerPhone: "Telefone do cliente", customerTaxRegime: "Regime tributário", relationshipType: "Tipo de relação", customerStatus: "Status do cliente", userName: "Nome completo", userEmail: "E-mail de acesso", userPhone: "Telefone do usuário",
};

function TextField({ name, label, required = false, optional = false, type = "text", placeholder, inputMode, value, error, onChange }: TextFieldProps) {
  const errorId = `${name}-error`;
  return <label>
    <span className="form-field-label">{label}<span className={required ? "required-indicator" : "optional-indicator"}>{required ? "Obrigatório" : optional ? "Opcional" : ""}</span></span>
    <input name={name} type={type} value={value} onChange={(event) => onChange(name, event.target.value)} placeholder={placeholder} inputMode={inputMode} required={required} aria-invalid={Boolean(error)} aria-describedby={error ? errorId : undefined} />
    {error && <span className="field-error" id={errorId}>{error}</span>}
  </label>;
}

export function ProvisioningForm({ integrations, idempotencyKey }: { integrations: IntegrationOption[]; idempotencyKey: string }) {
  const initialState: ProvisionFormState = { values: { idempotencyKey }, fieldErrors: {}, revision: 0 };
  const [state, formAction, pending] = useActionState(provisionAction, initialState);
  return <ProvisioningFields key={state.revision} state={state} formAction={formAction} pending={pending} integrations={integrations} idempotencyKey={idempotencyKey} />;
}

function ProvisioningFields({ state, formAction, pending, integrations, idempotencyKey }: { state: ProvisionFormState; formAction: (formData: FormData) => void; pending: boolean; integrations: IntegrationOption[]; idempotencyKey: string }) {
  const [values, setValues] = useState<Record<string, string>>(state.values);
  const update = (name: FieldName, value: string) => setValues((current) => ({ ...current, [name]: value }));
  const error = (name: FieldName) => state.fieldErrors[name];
  const fieldProps = (name: FieldName, label: string, options: Partial<Omit<TextFieldProps, "name" | "label" | "value" | "error" | "onChange">> = {}): TextFieldProps => ({
    name, label, value: values[name] ?? "", error: error(name), onChange: update, ...options,
  });
  const visibleErrors = Object.entries(state.fieldErrors).filter(([name]) => name !== "_form");

  return <form action={formAction} className="provision-form">
    <input type="hidden" name="idempotencyKey" value={values.idempotencyKey ?? idempotencyKey} />
    {(state.message || visibleErrors.length > 0) && <div className="provision-error" role="alert" aria-live="polite">
      {state.message && <p>{state.message}</p>}
      {visibleErrors.length > 0 && <ul>{visibleErrors.map(([name, message]) => <li key={name}><strong>{fieldLabels[name] ?? "Campo"}:</strong> {message}</li>)}</ul>}
    </div>}
    <p className="form-field-legend"><span className="required-indicator">Obrigatório</span> precisa ser preenchido. <span className="optional-indicator">Opcional</span> pode ficar em branco.</p>

    <section className="panel form-section"><div className="section-heading"><span className="step-number">01</span><div><h2>Sistema e empresa</h2><p>Conta principal do SaaS (tenant).</p></div></div>
      <div className="field-grid"><label className="field-span"><span className="form-field-label">Sistema <span className="required-indicator">Obrigatório</span></span><select name="integrationId" value={values.integrationId ?? ""} onChange={(event) => update("integrationId", event.target.value)} required aria-invalid={Boolean(error("integrationId"))}>{<><option value="" disabled>Selecione um SaaS conectado</option>{integrations.map((item) => <option value={item.id} key={item.id}>{item.name}</option>)}</>}</select>{error("integrationId") && <span className="field-error">{error("integrationId")}</span>}</label>
        <TextField {...fieldProps("accountName", "Nome do escritório", { required: true, placeholder: "Contabilidade Exemplo" })} />
        <TextField {...fieldProps("accountTaxId", "CNPJ do escritório", { required: true, inputMode: "numeric", placeholder: "00.000.000/0001-00" })} />
        <TextField {...fieldProps("accountEmail", "E-mail do escritório", { required: true, type: "email", placeholder: "contato@escritorio.com.br" })} />
        <TextField {...fieldProps("accountAddress", "Endereço completo", { required: true, placeholder: "Rua, número, cidade e UF" })} />
      </div>
    </section>

    <section className="panel form-section"><div className="section-heading"><span className="step-number">02</span><div><h2>Primeiro cliente</h2><p>Registro da carteira dentro do escritório.</p></div></div>
      <div className="field-grid">
        <TextField {...fieldProps("customerTradeName", "Nome fantasia", { required: true, placeholder: "Empresa Cliente" })} />
        <TextField {...fieldProps("customerLegalName", "Razão social", { optional: true, placeholder: "Empresa Cliente Ltda." })} />
        <TextField {...fieldProps("customerTaxId", "CNPJ ou CPF", { required: true, inputMode: "numeric", placeholder: "00.000.000/0001-00" })} />
        <TextField {...fieldProps("customerEmail", "E-mail", { optional: true, type: "email", placeholder: "financeiro@cliente.com.br" })} />
        <TextField {...fieldProps("customerPhone", "Telefone", { optional: true, type: "tel", inputMode: "tel", placeholder: "(11) 99999-9999" })} />
        <TextField {...fieldProps("customerTaxRegime", "Regime tributário", { required: true, placeholder: "Simples Nacional" })} />
        <label><span className="form-field-label">Tipo de relação <span className="required-indicator">Obrigatório</span></span><select name="relationshipType" value={values.relationshipType ?? ""} onChange={(event) => update("relationshipType", event.target.value)} required aria-invalid={Boolean(error("relationshipType"))}><option value="" disabled>Selecione</option><option value="Recorrente">Recorrente</option><option value="Avulso">Avulso</option></select>{error("relationshipType") && <span className="field-error">{error("relationshipType")}</span>}</label>
        <label><span className="form-field-label">Status <span className="required-indicator">Obrigatório</span></span><select name="customerStatus" value={values.customerStatus ?? ""} onChange={(event) => update("customerStatus", event.target.value)} required aria-invalid={Boolean(error("customerStatus"))}><option value="" disabled>Selecione</option><option value="Ativo">Ativo</option><option value="Inativo">Inativo</option></select>{error("customerStatus") && <span className="field-error">{error("customerStatus")}</span>}</label>
      </div>
    </section>

    <section className="panel form-section"><div className="section-heading"><span className="step-number">03</span><div><h2>Usuário responsável</h2><p>Receberá um convite para definir a própria senha.</p></div></div>
      <div className="field-grid">
        <TextField {...fieldProps("userName", "Nome completo", { required: true, placeholder: "Nome da pessoa responsável" })} />
        <TextField {...fieldProps("userEmail", "E-mail de acesso", { required: true, type: "email", placeholder: "pessoa@escritorio.com.br" })} />
        <TextField {...fieldProps("userPhone", "Telefone", { optional: true, type: "tel", inputMode: "tel", placeholder: "(11) 99999-9999" })} />
        <div className="role-preview"><span className="role-icon">♙</span><div><strong>Proprietário · Owner</strong><span>Acesso inicial à empresa</span></div></div>
      </div>
    </section>
    <div className="submit-row"><p>Antes de enviar, revise os dados. A ação cria registros no sistema escolhido.</p><button className="button button-dark" type="submit" disabled={pending}>{pending ? "Validando e enviando…" : <>Criar cadastro <span aria-hidden>↗</span></>}</button></div>
  </form>;
}
