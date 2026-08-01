import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Database,
  Mail,
  RotateCcw,
  Settings2,
  SlidersHorizontal,
  TimerReset,
  X,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { api } from "../api";
import {
  APIError,
  type NumberUnitValue,
  type Settings,
  type StorageUnit,
} from "../types/api";
import { Listbox } from "./controls";
import { EmailSettings } from "./settings/EmailSettings";
import { Button, IconButton, Skeleton, Tooltip } from "./ui";

const defaultSettings: Omit<Settings, "updated_at"> = {
  log_limit: { value: 10_000, unit: "lines" },
  inactive_preservation: { value: 2_000, unit: "lines" },
};

const unitOptions = [
  { value: "lines" as const, label: "Linhas" },
  { value: "mb" as const, label: "MB" },
];

const unitLabel: Record<StorageUnit, string> = {
  lines: "Linhas",
  mb: "MB",
};

interface NumberUnitDraft {
  value: string;
  unit: StorageUnit;
}

interface SettingsDraft {
  log_limit: NumberUnitDraft;
  inactive_preservation: NumberUnitDraft;
}

export type SettingsCategory = "general" | "storage" | "inactivity" | "email";

function toDraft(value: Omit<Settings, "updated_at">): SettingsDraft {
  return {
    log_limit: {
      value: String(value.log_limit.value),
      unit: value.log_limit.unit,
    },
    inactive_preservation: {
      value: String(value.inactive_preservation.value),
      unit: value.inactive_preservation.unit,
    },
  };
}

function validateValue(value: string) {
  if (!value.trim()) return "Informe um valor entre 0 e 10.000.";
  if (!/^-?\d+$/.test(value)) return "O valor deve ser um número inteiro.";
  const parsed = Number(value);
  if (parsed < 0 || parsed > 10_000) {
    return "Informe um valor entre 0 e 10.000.";
  }
  return "";
}

function toNumberUnit(value: NumberUnitDraft): NumberUnitValue {
  return { value: Number(value.value), unit: value.unit };
}

export function SettingsButton({
  collapsed,
  onOpen,
}: {
  collapsed: boolean;
  onOpen: (trigger: HTMLButtonElement) => void;
}) {
  return (
    <Tooltip label={collapsed ? "Configurações" : ""} className="block w-full">
      <button
        type="button"
        aria-label="Abrir configurações"
        onClick={(event) => onOpen(event.currentTarget)}
        className={`flex h-10 w-full items-center gap-3 rounded-lg px-3 text-sm text-zinc-500 transition-colors duration-150 hover:bg-zinc-900 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 ${
          collapsed ? "justify-center" : ""
        }`}
      >
        <Settings2 className="size-5 shrink-0" aria-hidden="true" />
        {!collapsed && <span className="truncate">Configurações</span>}
      </button>
    </Tooltip>
  );
}

function NumberUnitInput({
  label,
  description,
  helper,
  value,
  original,
  disabled,
  error,
  changed,
  zeroMessage,
  onChange,
}: {
  label: string;
  description: string;
  helper: string;
  value: NumberUnitDraft;
  original: NumberUnitValue;
  disabled: boolean;
  error?: string;
  changed: boolean;
  zeroMessage: string;
  onChange: (value: NumberUnitDraft) => void;
}) {
  const id = useId();
  const parsed = Number(value.value);
  const isZero = value.value !== "" && !error && parsed === 0;
  const unitChanged = value.unit !== original.unit;
  const isLegacy = original.value > 10_000;
  const numericValue = Number(value.value);
  const hasValidInteger = /^\d+$/.test(value.value);

  const adjustValue = (direction: -1 | 1) => {
    const base = hasValidInteger ? numericValue : 0;
    const next = Math.min(10_000, Math.max(0, base + direction));
    onChange({ ...value, value: String(next) });
  };

  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-950/55 p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <label htmlFor={id} className="text-sm font-medium text-zinc-100">
            {label}
          </label>
          <p className="mt-1 max-w-xl text-xs leading-5 text-zinc-500">
            {description}
          </p>
        </div>
        {changed && (
          <span className="shrink-0 rounded-full border border-amber-900/80 bg-amber-950/30 px-2 py-1 text-[10px] text-amber-400">
            Alterado
          </span>
        )}
      </div>

      <div className="mt-4 flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="relative">
          <input
            id={id}
            type="number"
            inputMode="numeric"
            min={0}
            max={10_000}
            step={1}
            value={value.value}
            disabled={disabled}
            aria-invalid={Boolean(error)}
            aria-describedby={`${id}-help ${error ? `${id}-error` : ""}`}
            onKeyDown={(event) => {
              if (event.key === "ArrowUp" || event.key === "ArrowDown") {
                event.preventDefault();
                adjustValue(event.key === "ArrowUp" ? 1 : -1);
              }
            }}
            onChange={(event) =>
              onChange({ ...value, value: event.target.value })
            }
            className={`themed-number-input h-10 w-full rounded-lg border bg-zinc-900 px-3 pr-11 font-mono text-sm text-zinc-100 outline-none focus:ring-1 focus:ring-white/50 disabled:cursor-not-allowed disabled:opacity-60 ${
              error
                ? "border-red-800 focus:border-red-700"
                : "border-zinc-700"
            }`}
          />
            <div className="absolute bottom-px right-px top-px flex w-8 flex-col overflow-hidden rounded-r-[7px] border-l border-zinc-700 bg-zinc-950">
              <button
                type="button"
                aria-label={`Aumentar ${label}`}
                aria-controls={id}
                disabled={disabled || (hasValidInteger && numericValue >= 10_000)}
                onClick={() => adjustValue(1)}
                className="grid min-h-0 flex-1 place-items-center border-b border-zinc-800 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-100 focus-visible:z-10 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-white/50 disabled:cursor-not-allowed disabled:text-zinc-700"
              >
                <ChevronUp className="size-3" />
              </button>
              <button
                type="button"
                aria-label={`Diminuir ${label}`}
                aria-controls={id}
                disabled={disabled || (hasValidInteger && numericValue <= 0)}
                onClick={() => adjustValue(-1)}
                className="grid min-h-0 flex-1 place-items-center text-zinc-500 hover:bg-zinc-800 hover:text-zinc-100 focus-visible:z-10 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-white/50 disabled:cursor-not-allowed disabled:text-zinc-700"
              >
                <ChevronDown className="size-3" />
              </button>
            </div>
          </div>
          {error && (
            <p id={`${id}-error`} role="alert" className="mt-1.5 text-xs text-red-400">
              {error}
            </p>
          )}
        </div>
        <Listbox
          value={value.unit}
          options={unitOptions}
          onChange={(unit) => onChange({ ...value, unit })}
          label={`Unidade de ${label}`}
          disabled={disabled}
          className="h-10 w-28"
        />
      </div>

      <p id={`${id}-help`} className="mt-2 text-[11px] leading-5 text-zinc-600">
        {helper} Use 0 para desativar o limite automático correspondente.
      </p>
      <p className="mt-1 text-[11px] text-zinc-500">
        Valor atual: {original.value.toLocaleString("pt-BR")} {unitLabel[original.unit]}
      </p>
      {isLegacy && (
        <p className="mt-2 rounded-lg border border-amber-950 bg-amber-950/20 px-3 py-2 text-[11px] leading-5 text-amber-500">
          Configuração legada acima de 10.000. Escolha um valor válido antes de salvar esta seção.
        </p>
      )}
      {unitChanged && (
        <p className="mt-2 rounded-lg border border-cyan-950 bg-cyan-950/20 px-3 py-2 text-[11px] text-cyan-500">
          A unidade foi alterada sem converter o valor. Revise o número antes de salvar.
        </p>
      )}
      {isZero && (
        <p className="mt-2 rounded-lg border border-amber-950 bg-amber-950/20 px-3 py-2 text-[11px] leading-5 text-amber-500">
          {zeroMessage}
        </p>
      )}
    </div>
  );
}

function SettingsSkeleton() {
  return (
    <div aria-label="Carregando configurações" role="status" className="space-y-4">
      {[0, 1].map((item) => (
        <div key={item} className="rounded-xl border border-zinc-800 p-4">
          <Skeleton className="h-4 w-52" />
          <Skeleton className="mt-3 h-3 w-full max-w-md" />
          <div className="mt-5 flex gap-2">
            <Skeleton className="h-10 flex-1" />
            <Skeleton className="h-10 w-28" />
          </div>
        </div>
      ))}
    </div>
  );
}

function focusableElements(container: HTMLElement | null) {
  if (!container) return [];
  return Array.from(
    container.querySelectorAll<HTMLElement>(
      'button:not([disabled]), input:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((element) => !element.hasAttribute("aria-hidden"));
}

export function SettingsDialog({
  onClose,
  returnFocus,
  initialCategory = "storage",
}: {
  onClose: () => void;
  returnFocus: HTMLButtonElement | null;
  initialCategory?: SettingsCategory;
}) {
  const titleId = useId();
  const descriptionId = useId();
  const dialog = useRef<HTMLDivElement>(null);
  const discardDialog = useRef<HTMLDivElement>(null);
  const [original, setOriginal] = useState<Settings>();
  const [draft, setDraft] = useState<SettingsDraft>();
  const [isInitialLoading, setIsInitialLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [saveError, setSaveError] = useState("");
  const [backendFieldErrors, setBackendFieldErrors] = useState<Record<string, string>>({});
  const [saveSucceeded, setSaveSucceeded] = useState(false);
  const [discardOpen, setDiscardOpen] = useState(false);
  const [category, setCategory] = useState<SettingsCategory>(initialCategory);
  const [emailDirty, setEmailDirty] = useState(false);
  const [emailMounted, setEmailMounted] = useState(initialCategory === "email");

  const load = useCallback(async () => {
    setIsInitialLoading(true);
    setLoadError("");
    try {
      const value = await api.settings();
      setOriginal(value);
      setDraft(toDraft(value));
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : "Não foi possível carregar as configurações.");
    } finally {
      setIsInitialLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const isDirty = useMemo(() => {
    if (!draft || !original) return false;
    return JSON.stringify(draft) !== JSON.stringify(toDraft(original));
  }, [draft, original]);

  const errors = useMemo(() => {
    if (!draft) return {} as Record<string, string>;
    const next: Record<string, string> = {
      "log_limit.value": validateValue(draft.log_limit.value),
      "inactive_preservation.value": validateValue(
        draft.inactive_preservation.value,
      ),
      ...backendFieldErrors,
    };
    if (
      !next["log_limit.value"] &&
      !next["inactive_preservation.value"] &&
      draft.log_limit.unit === draft.inactive_preservation.unit &&
      Number(draft.log_limit.value) > 0 &&
      Number(draft.inactive_preservation.value) >
        Number(draft.log_limit.value)
    ) {
      next["inactive_preservation.value"] =
        "A quantidade preservada não pode ser maior que o limite máximo.";
    }
    return next;
  }, [backendFieldErrors, draft]);

  const hasErrors = Object.values(errors).some(Boolean);

  const requestClose = useCallback(() => {
    if (isDirty || emailDirty) setDiscardOpen(true);
    else onClose();
  }, [emailDirty, isDirty, onClose]);

  useEffect(() => {
    const timer = window.setTimeout(() => dialog.current?.focus());
    return () => {
      window.clearTimeout(timer);
      if (returnFocus?.isConnected) returnFocus.focus();
    };
  }, [returnFocus]);

  useEffect(() => {
    if (discardOpen) focusableElements(discardDialog.current)[0]?.focus();
  }, [discardOpen]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        if (discardOpen) setDiscardOpen(false);
        else requestClose();
        return;
      }
      if (event.key !== "Tab") return;
      const scope = discardOpen ? discardDialog.current : dialog.current;
      const elements = focusableElements(scope);
      if (!elements.length) return;
      const first = elements[0];
      const last = elements[elements.length - 1];
      if (document.activeElement === scope) {
        event.preventDefault();
        (event.shiftKey ? last : first).focus();
      } else if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [discardOpen, requestClose]);

  const changeDraft = (
    field: keyof SettingsDraft,
    value: NumberUnitDraft,
  ) => {
    setDraft((current) => current && { ...current, [field]: value });
    setBackendFieldErrors({});
    setSaveError("");
    setSaveSucceeded(false);
  };

  const save = async () => {
    if (!draft || hasErrors || !isDirty || isSaving) return;
    setIsSaving(true);
    setSaveError("");
    setBackendFieldErrors({});
    try {
      const response = await api.updateSettings({
        log_limit: toNumberUnit(draft.log_limit),
        inactive_preservation: toNumberUnit(draft.inactive_preservation),
      });
      const next: Settings = {
        ...response.settings,
        updated_at: response.updated_at,
      };
      setOriginal(next);
      setDraft(toDraft(next));
      setSaveSucceeded(true);
    } catch (error) {
      if (error instanceof APIError && error.field) {
        setBackendFieldErrors({ [error.field]: error.message });
      }
      setSaveError(error instanceof Error ? error.message : "Não foi possível salvar as configurações.");
    } finally {
      setIsSaving(false);
    }
  };

  const sectionInformation = {
    general: {
      title: "Geral",
      description: "Consulte os valores atualmente aplicados e como as alterações entram em vigor.",
    },
    storage: {
      title: "Armazenamento de logs",
      description: "Defina o limite máximo usado durante a gravação de novos logs.",
    },
    inactivity: {
      title: "Inatividade",
      description: "Defina quanto do histórico será preservado quando um sender ficar inativo.",
    },
    email: {
      title: "E-mail",
      description: "Configure o provedor usado para entregar os alertas de logs.",
    },
  }[category];

  return createPortal(
    <div className="fixed inset-0 z-[150] flex items-end justify-center sm:items-center sm:p-4">
      <button
        type="button"
        aria-label="Fechar configurações"
        className="absolute inset-0 bg-black/75"
        onClick={requestClose}
      />
      <div
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        tabIndex={-1}
        className="relative flex h-[92dvh] w-full flex-col overflow-hidden rounded-t-2xl border border-zinc-800 bg-zinc-950 shadow-2xl shadow-black/70 outline-none sm:h-[min(680px,85vh)] sm:max-w-3xl sm:rounded-xl"
      >
        <header className="flex shrink-0 items-start justify-between gap-4 border-b border-zinc-800 bg-zinc-950 px-4 py-4 sm:px-5">
          <div>
            <h2 id={titleId} className="text-base font-semibold text-zinc-100">
              Configurações
            </h2>
            <p id={descriptionId} className="mt-1 text-xs leading-5 text-zinc-500">
              Configure os limites de armazenamento e compactação dos logs.
            </p>
          </div>
          <IconButton
            label="Fechar configurações"
            onClick={requestClose}
            className="size-8"
          >
            <X className="size-4" />
          </IconButton>
        </header>

        <div className="min-h-0 flex-1 sm:grid sm:grid-cols-[184px_minmax(0,1fr)]">
          <aside className="hidden border-r border-zinc-800 bg-[#111113] p-3 sm:block">
            <p className="px-2 pb-2 text-[10px] font-medium uppercase tracking-wider text-zinc-600">
              Categorias
            </p>
            <button
              type="button"
              aria-current={category === "general" ? "page" : undefined}
              onClick={() => setCategory("general")}
              className={`flex h-9 w-full items-center gap-2 rounded-lg px-2 text-left text-xs transition-colors ${
                category === "general"
                  ? "bg-zinc-800 text-zinc-100"
                  : "text-zinc-500 hover:bg-zinc-900 hover:text-zinc-200"
              }`}
            >
              <SlidersHorizontal className="size-4" /> Geral
            </button>
            <button
              type="button"
              aria-current={category === "storage" ? "page" : undefined}
              onClick={() => setCategory("storage")}
              className={`flex h-9 w-full items-center gap-2 rounded-lg px-2 text-left text-xs transition-colors ${
                category === "storage"
                  ? "bg-zinc-800 text-zinc-100"
                  : "text-zinc-500 hover:bg-zinc-900 hover:text-zinc-200"
              }`}
            >
              <Database className="size-4" /> Armazenamento de logs
            </button>
            <button
              type="button"
              aria-current={category === "inactivity" ? "page" : undefined}
              onClick={() => setCategory("inactivity")}
              className={`flex h-9 w-full items-center gap-2 rounded-lg px-2 text-left text-xs transition-colors ${
                category === "inactivity"
                  ? "bg-zinc-800 text-zinc-100"
                  : "text-zinc-500 hover:bg-zinc-900 hover:text-zinc-200"
              }`}
            >
              <TimerReset className="size-4" /> Inatividade
            </button>
            <button
              type="button"
              aria-current={category === "email" ? "page" : undefined}
              onClick={() => { setEmailMounted(true); setCategory("email"); }}
              className={`flex h-9 w-full items-center gap-2 rounded-lg px-2 text-left text-xs transition-colors ${
                category === "email"
                  ? "bg-zinc-800 text-zinc-100"
                  : "text-zinc-500 hover:bg-zinc-900 hover:text-zinc-200"
              }`}
            >
              <Mail className="size-4" /> E-mail
            </button>
          </aside>

          <div className="min-h-0 overflow-y-auto">
            <div className="flex gap-1 overflow-x-auto border-b border-zinc-800 p-2 sm:hidden">
              <button
                type="button"
                aria-current={category === "general" ? "page" : undefined}
                onClick={() => setCategory("general")}
                className={`h-8 shrink-0 rounded-lg px-3 text-xs ${category === "general" ? "bg-zinc-800 text-zinc-100" : "text-zinc-500"}`}
              >Geral</button>
              <button
                type="button"
                aria-current={category === "storage" ? "page" : undefined}
                onClick={() => setCategory("storage")}
                className={`h-8 shrink-0 rounded-lg px-3 text-xs ${category === "storage" ? "bg-zinc-800 text-zinc-100" : "text-zinc-500"}`}
              >Armazenamento de logs</button>
              <button
                type="button"
                aria-current={category === "inactivity" ? "page" : undefined}
                onClick={() => setCategory("inactivity")}
                className={`h-8 shrink-0 rounded-lg px-3 text-xs ${category === "inactivity" ? "bg-zinc-800 text-zinc-100" : "text-zinc-500"}`}
              >Inatividade</button>
              <button
                type="button"
                aria-current={category === "email" ? "page" : undefined}
                onClick={() => { setEmailMounted(true); setCategory("email"); }}
                className={`h-8 shrink-0 rounded-lg px-3 text-xs ${category === "email" ? "bg-zinc-800 text-zinc-100" : "text-zinc-500"}`}
              >E-mail</button>
            </div>
            <section aria-labelledby={`${titleId}-storage`} className="p-4 sm:p-5">
              <div className="mb-4">
                <h3 id={`${titleId}-storage`} className="text-sm font-semibold text-zinc-100">
                  {sectionInformation.title}
                </h3>
                <p className="mt-1 text-xs leading-5 text-zinc-500">
                  {sectionInformation.description}
                </p>
              </div>

              {emailMounted && <div className={category === "email" ? "" : "hidden"}>
                <EmailSettings onDirtyChange={setEmailDirty} />
              </div>}
              {category !== "email" && (isInitialLoading ? (
                <SettingsSkeleton />
              ) : loadError ? (
                <div role="alert" className="grid min-h-64 place-items-center rounded-xl border border-red-950 bg-red-950/10 p-6 text-center">
                  <div>
                    <AlertTriangle className="mx-auto size-5 text-red-500" />
                    <p className="mt-3 text-sm text-red-300">{loadError}</p>
                    <Button className="mt-4" onClick={() => void load()}>Tentar novamente</Button>
                  </div>
                </div>
              ) : draft && original ? (
                <div className="space-y-4">
                  {category === "general" && (
                    <div className="space-y-4">
                      <div className="grid gap-3 sm:grid-cols-2">
                        <div className="rounded-xl border border-zinc-800 bg-zinc-950/55 p-4">
                          <p className="text-xs text-zinc-500">Limite máximo atual</p>
                          <p className="mt-2 font-mono text-lg text-zinc-100">
                            {original.log_limit.value.toLocaleString("pt-BR")} {unitLabel[original.log_limit.unit]}
                          </p>
                        </div>
                        <div className="rounded-xl border border-zinc-800 bg-zinc-950/55 p-4">
                          <p className="text-xs text-zinc-500">Preservação após inatividade</p>
                          <p className="mt-2 font-mono text-lg text-zinc-100">
                            {original.inactive_preservation.value.toLocaleString("pt-BR")} {unitLabel[original.inactive_preservation.unit]}
                          </p>
                        </div>
                      </div>
                      <div className="rounded-xl border border-zinc-800 bg-[#111113] p-4 text-xs leading-6 text-zinc-500">
                        As alterações salvas entram em vigor nos próximos ciclos de gravação e manutenção, sem reiniciar o LogHill.
                      </div>
                    </div>
                  )}
                  {category === "storage" && (
                    <NumberUnitInput
                      label="Limite máximo de logs"
                      description="Define o tamanho máximo do arquivo de logs de cada sender. Quando o limite for atingido, os registros mais antigos serão removidos primeiro."
                      helper="Quando o limite for atingido, os logs mais antigos serão removidos."
                      value={draft.log_limit}
                      original={original.log_limit}
                      disabled={isSaving}
                      error={errors["log_limit.value"]}
                      changed={JSON.stringify(draft.log_limit) !== JSON.stringify(toDraft(original).log_limit)}
                      zeroMessage="Sem limite: o arquivo poderá continuar crescendo enquanto o sender estiver ativo."
                      onChange={(value) => changeDraft("log_limit", value)}
                    />
                  )}
                  {category === "inactivity" && (
                    <NumberUnitInput
                      label="Logs preservados após inatividade"
                      description="Define quanto do histórico será mantido quando um sender for marcado como inativo."
                      helper="Quando o sender ficar inativo, somente os registros mais recentes dentro deste limite serão mantidos."
                      value={draft.inactive_preservation}
                      original={original.inactive_preservation}
                      disabled={isSaving}
                      error={errors["inactive_preservation.value"]}
                      changed={JSON.stringify(draft.inactive_preservation) !== JSON.stringify(toDraft(original).inactive_preservation)}
                      zeroMessage="Nenhum log será preservado quando o sender ficar inativo."
                      onChange={(value) => changeDraft("inactive_preservation", value)}
                    />
                  )}
                </div>
              ) : null)}
            </section>
          </div>
        </div>

        {category !== "email" && <footer className="flex shrink-0 flex-col gap-3 border-t border-zinc-800 bg-[#111113] px-4 py-3 sm:px-5 md:flex-row md:items-center md:justify-between">
          <div className="min-h-5 text-[11px]">
            {isDirty && !saveSucceeded && (
              <span className="inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap text-amber-500">
                <span className="size-1.5 rounded-full bg-current" />
                <button
                  type="button"
                  disabled={isSaving}
                  onClick={() => {
                    if (original) setDraft(toDraft(original));
                    setBackendFieldErrors({});
                    setSaveError("");
                  }}
                  className="ml-1 underline decoration-zinc-700 underline-offset-2 hover:text-amber-300 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
                >
                  Desfazer alterações
                </button>
              </span>
            )}
            {saveSucceeded && (
              <span role="status" className="inline-flex items-center gap-1.5 text-emerald-500">
                <CheckCircle2 className="size-3.5" /> Configurações salvas com sucesso.
              </span>
            )}
            {saveError && <span role="alert" className="text-red-400">{saveError}</span>}
          </div>
          <div className="flex flex-wrap items-center justify-end gap-2 sm:shrink-0 sm:flex-nowrap">
            <Button
              onClick={() => {
                setDraft(toDraft(defaultSettings));
                setBackendFieldErrors({});
                setSaveError("");
                setSaveSucceeded(false);
              }}
              disabled={!draft || isInitialLoading || isSaving}
              className="mr-auto border-transparent bg-transparent text-zinc-500 sm:mr-2"
            >
              <RotateCcw className="size-3.5" /> Restaurar padrões
            </Button>
            <Button onClick={requestClose} disabled={isSaving} className="border-transparent bg-transparent">
              Cancelar
            </Button>
            <Button
              onClick={() => void save()}
              disabled={!draft || !isDirty || hasErrors || isSaving}
              className="border-zinc-600 bg-zinc-800 text-zinc-100 hover:border-zinc-500 hover:bg-zinc-700 disabled:text-zinc-500"
            >
              {isSaving ? "Salvando..." : "Salvar alterações"}
            </Button>
          </div>
        </footer>}
      </div>

      {discardOpen && (
        <div className="fixed inset-0 z-[220] grid place-items-center p-4">
          <button
            type="button"
            aria-label="Fechar confirmação e continuar editando"
            className="absolute inset-0 bg-black/75"
            onClick={() => setDiscardOpen(false)}
          />
          <div
            ref={discardDialog}
            role="alertdialog"
            aria-modal="true"
            aria-labelledby={`${titleId}-discard`}
            className="relative w-full max-w-sm rounded-xl border border-zinc-700 bg-[#161618] p-5 shadow-2xl shadow-black/70"
          >
            <h3 id={`${titleId}-discard`} className="text-sm font-semibold text-zinc-100">
              Descartar alterações?
            </h3>
            <p className="mt-2 text-xs leading-5 text-zinc-500">
              As alterações feitas nesta janela ainda não foram salvas.
            </p>
            <div className="mt-5 flex justify-end gap-2">
              <Button onClick={() => setDiscardOpen(false)}>
                Continuar editando
              </Button>
              <Button
                onClick={onClose}
                className="border-red-900 bg-red-950/30 text-red-300 hover:bg-red-950/60"
              >
                Descartar
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>,
    document.body,
  );
}
