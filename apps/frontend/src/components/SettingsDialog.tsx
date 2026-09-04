import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Database,
  Mail,
  RotateCcw,
  Settings as SettingsIcon,
  SlidersHorizontal,
  TimerReset,
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
import { CONTROL_SURFACE } from "./controlStyles";
import { EmailSettings } from "./settings/EmailSettings";
import { Button, ModalCloseButton, Skeleton, Tooltip } from "./ui";
import { restoreFocusWithoutTooltip } from "../utils/tooltipFocus";
import { useCachedState } from "../hooks/useCachedState";
import { waitForMinimumLoading } from "../utils/minimumLoading";

const defaultSettings: Omit<Settings, "updated_at"> = {
  log_limit: { value: 10_000, unit: "lines" },
  inactive_preservation: { value: 2_000, unit: "lines" },
  inactive_after_seconds: 300,
  delete_inactive_after_days: 7,
};

const unitOptions = [
  { value: "lines" as const, label: "Lines" },
  { value: "mb" as const, label: "MB" },
];

const unitLabel: Record<StorageUnit, string> = {
  lines: "Lines",
  mb: "MB",
};

interface NumberUnitDraft {
  value: string;
  unit: StorageUnit;
}

interface SettingsDraft {
  log_limit: NumberUnitDraft;
  inactive_preservation: NumberUnitDraft;
  inactive_after_seconds: string;
  delete_inactive_after_days: string;
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
    inactive_after_seconds: String(value.inactive_after_seconds),
    delete_inactive_after_days: String(value.delete_inactive_after_days),
  };
}

function validateValue(value: string) {
  if (!value.trim()) return "Enter a value between 0 and 10,000.";
  if (!/^-?\d+$/.test(value)) return "The value must be an integer.";
  const parsed = Number(value);
  if (parsed < 0 || parsed > 10_000) {
    return "Enter a value between 0 and 10,000.";
  }
  return "";
}

function validateIntegerRange(value: string, minimum: number, maximum: number, unit: string) {
  if (!/^\d+$/.test(value)) return `Enter an integer number of ${unit}.`;
  const parsed = Number(value);
  return parsed < minimum || parsed > maximum ? `Enter a value between ${minimum.toLocaleString("en-US")} and ${maximum.toLocaleString("en-US")} ${unit}.` : "";
}

function toNumberUnit(value: NumberUnitDraft): NumberUnitValue {
  return { value: Number(value.value), unit: value.unit };
}

function SettingsNumberInput({ value, min, max, label, disabled, error, onChange }: { value: string; min: number; max: number; label: string; disabled: boolean; error?: string; onChange: (value: string) => void }) {
  const id = useId();
  const numericValue = Number(value);
  const valid = /^\d+$/.test(value);
  const adjust = (direction: -1 | 1) => onChange(String(Math.min(max, Math.max(min, (valid ? numericValue : min) + direction))));
  return <div className="relative mt-2">
    <input id={id} type="number" inputMode="numeric" min={min} max={max} step={1} value={value} disabled={disabled} aria-invalid={Boolean(error)} onChange={(event) => onChange(event.target.value)} onKeyDown={(event) => { if (event.key === "ArrowUp" || event.key === "ArrowDown") { event.preventDefault(); adjust(event.key === "ArrowUp" ? 1 : -1); } }} className={`themed-number-input h-10 w-full rounded-lg border px-3 pr-11 font-mono text-sm text-zinc-100 disabled:cursor-not-allowed disabled:opacity-60 ${CONTROL_SURFACE} ${error ? "border-red-800" : "border-zinc-700"}`} />
    <div className="absolute bottom-px right-px top-px flex w-8 flex-col overflow-hidden rounded-r-[7px] border-l border-zinc-700 bg-zinc-950">
      <button type="button" aria-label={`Increase ${label}`} aria-controls={id} disabled={disabled || (valid && numericValue >= max)} onClick={() => adjust(1)} className="grid min-h-0 flex-1 place-items-center border-b border-zinc-800 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-100 disabled:cursor-not-allowed disabled:text-zinc-700"><ChevronUp className="size-3" /></button>
      <button type="button" aria-label={`Decrease ${label}`} aria-controls={id} disabled={disabled || (valid && numericValue <= min)} onClick={() => adjust(-1)} className="grid min-h-0 flex-1 place-items-center text-zinc-500 hover:bg-zinc-800 hover:text-zinc-100 disabled:cursor-not-allowed disabled:text-zinc-700"><ChevronDown className="size-3" /></button>
    </div>
  </div>;
}

export function SettingsButton({
  collapsed,
  onOpen,
}: {
  collapsed: boolean;
  onOpen: (trigger: HTMLButtonElement) => void;
}) {
  return (
    <Tooltip label={collapsed ? "Settings" : ""} className="block w-full">
      <button
        type="button"
        aria-label="Open settings"
        onClick={(event) => onOpen(event.currentTarget)}
        className={`flex h-10 w-full items-center gap-3 rounded-lg text-sm text-zinc-500 transition-colors duration-150 hover:bg-zinc-900 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 ${
          collapsed ? "justify-center px-3" : "px-5"
        }`}
      >
        <SettingsIcon className="size-5 shrink-0" aria-hidden="true" />
        {!collapsed && <span className="truncate">Settings</span>}
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
            Changed
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
            className={`themed-number-input h-10 w-full rounded-lg border px-3 pr-11 font-mono text-sm text-zinc-100 disabled:cursor-not-allowed disabled:opacity-60 ${CONTROL_SURFACE} ${
              error
                ? "border-red-800 focus:border-red-700"
                : "border-zinc-700"
            }`}
          />
            <div className="absolute bottom-px right-px top-px flex w-8 flex-col overflow-hidden rounded-r-[7px] border-l border-zinc-700 bg-zinc-950">
              <button
                type="button"
                aria-label={`Increase ${label}`}
                aria-controls={id}
                disabled={disabled || (hasValidInteger && numericValue >= 10_000)}
                onClick={() => adjustValue(1)}
                className="grid min-h-0 flex-1 place-items-center border-b border-zinc-800 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-100 focus-visible:z-10 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-white/50 disabled:cursor-not-allowed disabled:text-zinc-700"
              >
                <ChevronUp className="size-3" />
              </button>
              <button
                type="button"
                aria-label={`Decrease ${label}`}
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
          label={`${label} unit`}
          disabled={disabled}
          className="h-10 w-28"
        />
      </div>

      <p id={`${id}-help`} className="mt-2 text-[11px] leading-5 text-zinc-600">
        {helper} Use 0 to disable the corresponding automatic limit.
      </p>
      <p className="mt-1 text-[11px] text-zinc-500">
        Current value: {original.value.toLocaleString("en-US")} {unitLabel[original.unit]}
      </p>
      {isLegacy && (
        <p className="mt-2 rounded-lg border border-amber-950 bg-amber-950/20 px-3 py-2 text-[11px] leading-5 text-amber-500">
          Legacy setting above 10,000. Choose a valid value before saving this section.
        </p>
      )}
      {unitChanged && (
        <p className="mt-2 rounded-lg border border-cyan-950 bg-cyan-950/20 px-3 py-2 text-[11px] text-cyan-500">
          The unit was changed without converting the value. Review the number before saving.
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
    <div aria-label="Loading settings" role="status" className="space-y-4">
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
  const [original, setOriginal] = useCachedState<Settings>(["modal", "settings"]);
  const [draft, setDraft] = useState<SettingsDraft | undefined>(() => original ? toDraft(original) : undefined);
  const [isInitialLoading, setIsInitialLoading] = useState(!original);
  const [isSaving, setIsSaving] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [saveError, setSaveError] = useState("");
  const [backendFieldErrors, setBackendFieldErrors] = useState<Record<string, string>>({});
  const [saveSucceeded, setSaveSucceeded] = useState(false);
  const [discardOpen, setDiscardOpen] = useState(false);
  const [category, setCategory] = useState<SettingsCategory>(initialCategory);
  const [emailDirty, setEmailDirty] = useState(false);
  const [emailMounted, setEmailMounted] = useState(initialCategory === "email");
  const hasLoadedSettings = useRef(Boolean(original));

  const load = useCallback(async () => {
    const firstLoad = !hasLoadedSettings.current;
    const startedAt = performance.now();
    setIsInitialLoading(firstLoad);
    setLoadError("");
    try {
      const value = await api.settings();
      if (firstLoad) await waitForMinimumLoading(startedAt);
      hasLoadedSettings.current = true;
      setOriginal(value);
      setDraft(toDraft(value));
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : "Unable to load settings.");
    } finally {
      setIsInitialLoading(false);
    }
  }, [setOriginal]);

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
      inactive_after_seconds: validateIntegerRange(draft.inactive_after_seconds, 1, 86_400, "seconds"),
      delete_inactive_after_days: validateIntegerRange(draft.delete_inactive_after_days, 1, 3_650, "days"),
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
        "The retained amount cannot exceed the maximum limit.";
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
      restoreFocusWithoutTooltip(returnFocus);
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
    field: "log_limit" | "inactive_preservation",
    value: NumberUnitDraft,
  ) => {
    setDraft((current) => current && { ...current, [field]: value });
    setBackendFieldErrors({});
    setSaveError("");
    setSaveSucceeded(false);
  };

  const changeTimeDraft = (field: "inactive_after_seconds" | "delete_inactive_after_days", value: string) => {
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
        inactive_after_seconds: Number(draft.inactive_after_seconds),
        delete_inactive_after_days: Number(draft.delete_inactive_after_days),
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
      setSaveError(error instanceof Error ? error.message : "Unable to save settings.");
    } finally {
      setIsSaving(false);
    }
  };

  const sectionInformation = {
    general: {
      title: "General",
      description: "Review the currently applied values and how changes take effect.",
    },
    storage: {
      title: "Log storage",
      description: "Set the maximum limit used when writing new logs.",
    },
    inactivity: {
      title: "Inactivity",
      description: "Set when a sender becomes inactive, how much to retain, and when to delete its logs.",
    },
    email: {
      title: "Email",
      description: "Configure the provider used to deliver log alerts.",
    },
  }[category];

  return createPortal(
    <div className="fixed inset-0 z-[150] flex items-end justify-center sm:items-center sm:p-4">
      <button
        type="button"
        aria-label="Close settings"
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
              Settings
            </h2>
            <p id={descriptionId} className="mt-1 text-xs leading-5 text-zinc-500">
              Configure log storage and compaction limits.
            </p>
          </div>
          <ModalCloseButton
            label="Close settings"
            onClick={requestClose}
          />
        </header>

        <div className="min-h-0 flex-1 sm:grid sm:grid-cols-[184px_minmax(0,1fr)]">
          <aside className="hidden border-r border-zinc-800 bg-[#111113] p-3 sm:block">
            <p className="px-2 pb-2 text-[10px] font-medium uppercase tracking-wider text-zinc-600">
              Categories
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
              <SlidersHorizontal className="size-4" /> General
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
              <Database className="size-4" /> Log storage
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
              <TimerReset className="size-4" /> Inactivity
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
              <Mail className="size-4" /> Email
            </button>
          </aside>

          <div className="min-h-0 overflow-y-auto">
            <div className="flex gap-1 overflow-x-auto border-b border-zinc-800 p-2 sm:hidden">
              <button
                type="button"
                aria-current={category === "general" ? "page" : undefined}
                onClick={() => setCategory("general")}
                className={`h-8 shrink-0 rounded-lg px-3 text-xs ${category === "general" ? "bg-zinc-800 text-zinc-100" : "text-zinc-500"}`}
              >General</button>
              <button
                type="button"
                aria-current={category === "storage" ? "page" : undefined}
                onClick={() => setCategory("storage")}
                className={`h-8 shrink-0 rounded-lg px-3 text-xs ${category === "storage" ? "bg-zinc-800 text-zinc-100" : "text-zinc-500"}`}
              >Log storage</button>
              <button
                type="button"
                aria-current={category === "inactivity" ? "page" : undefined}
                onClick={() => setCategory("inactivity")}
                className={`h-8 shrink-0 rounded-lg px-3 text-xs ${category === "inactivity" ? "bg-zinc-800 text-zinc-100" : "text-zinc-500"}`}
              >Inactivity</button>
              <button
                type="button"
                aria-current={category === "email" ? "page" : undefined}
                onClick={() => { setEmailMounted(true); setCategory("email"); }}
                className={`h-8 shrink-0 rounded-lg px-3 text-xs ${category === "email" ? "bg-zinc-800 text-zinc-100" : "text-zinc-500"}`}
              >Email</button>
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
                    <Button className="mt-4" onClick={() => void load()}>Try again</Button>
                  </div>
                </div>
              ) : draft && original ? (
                <div className="space-y-4">
                  {category === "general" && (
                    <div className="space-y-4">
                      <div className="grid gap-3 sm:grid-cols-2">
                        <div className="rounded-xl border border-zinc-800 bg-zinc-950/55 p-4">
                          <p className="text-xs text-zinc-500">Current maximum limit</p>
                          <p className="mt-2 font-mono text-lg text-zinc-100">
                            {original.log_limit.value.toLocaleString("en-US")} {unitLabel[original.log_limit.unit]}
                          </p>
                        </div>
                        <div className="rounded-xl border border-zinc-800 bg-zinc-950/55 p-4">
                          <p className="text-xs text-zinc-500">Retention after inactivity</p>
                          <p className="mt-2 font-mono text-lg text-zinc-100">
                            {original.inactive_preservation.value.toLocaleString("en-US")} {unitLabel[original.inactive_preservation.unit]}
                          </p>
                        </div>
                      </div>
                      <div className="rounded-xl border border-zinc-800 bg-[#111113] p-4 text-xs leading-6 text-zinc-500">
                        Saved changes take effect during the next write and maintenance cycles without restarting LogMate.
                      </div>
                    </div>
                  )}
                  {category === "storage" && (
                    <NumberUnitInput
                      label="Maximum log limit"
                      description="Sets the maximum size of each sender log file. When the limit is reached, the oldest records are removed first."
                      helper="When the limit is reached, the oldest logs are removed."
                      value={draft.log_limit}
                      original={original.log_limit}
                      disabled={isSaving}
                      error={errors["log_limit.value"]}
                      changed={JSON.stringify(draft.log_limit) !== JSON.stringify(toDraft(original).log_limit)}
                      zeroMessage="No limit: the file may continue growing while the sender is active."
                      onChange={(value) => changeDraft("log_limit", value)}
                    />
                  )}
                  {category === "inactivity" && (
                    <div className="space-y-4">
                      <div className="grid gap-3 sm:grid-cols-2">
                        <label className="rounded-xl border border-zinc-800 bg-[#111113] p-4 text-xs text-zinc-300">
                          Mark inactive after
                          <SettingsNumberInput value={draft.inactive_after_seconds} min={1} max={86400} label="inactivity duration" disabled={isSaving} error={errors.inactive_after_seconds} onChange={(value) => changeTimeDraft("inactive_after_seconds", value)} />
                          <span className="mt-2 block text-[11px] text-zinc-600">Time without logs or health checks, in seconds.</span>
                          {errors.inactive_after_seconds && <span role="alert" className="mt-2 block text-[11px] text-red-400">{errors.inactive_after_seconds}</span>}
                        </label>
                        <label className="rounded-xl border border-zinc-800 bg-[#111113] p-4 text-xs text-zinc-300">
                          Delete inactive instances after
                          <SettingsNumberInput value={draft.delete_inactive_after_days} min={1} max={3650} label="deletion period" disabled={isSaving} error={errors.delete_inactive_after_days} onChange={(value) => changeTimeDraft("delete_inactive_after_days", value)} />
                          <span className="mt-2 block text-[11px] text-zinc-600">Period counted from deactivation; the instance and its logs will be removed. If no other instances remain, the sender also disappears.</span>
                          {errors.delete_inactive_after_days && <span role="alert" className="mt-2 block text-[11px] text-red-400">{errors.delete_inactive_after_days}</span>}
                        </label>
                      </div>
                      <NumberUnitInput
                        label="Logs retained after inactivity"
                        description="Sets how much history is retained when a sender is marked inactive."
                        helper="When the sender becomes inactive, only the most recent records within this limit are retained."
                        value={draft.inactive_preservation}
                        original={original.inactive_preservation}
                        disabled={isSaving}
                        error={errors["inactive_preservation.value"]}
                        changed={JSON.stringify(draft.inactive_preservation) !== JSON.stringify(toDraft(original).inactive_preservation)}
                        zeroMessage="No logs will be retained when the sender becomes inactive."
                        onChange={(value) => changeDraft("inactive_preservation", value)}
                      />
                    </div>
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
                  Undo changes
                </button>
              </span>
            )}
            {saveSucceeded && (
              <span role="status" className="inline-flex items-center gap-1.5 text-emerald-500">
                <CheckCircle2 className="size-3.5" /> Settings saved successfully.
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
              <RotateCcw className="size-3.5" /> Restore defaults
            </Button>
            <Button onClick={requestClose} disabled={isSaving} className="border-transparent bg-transparent">
              Cancel
            </Button>
            <Button
              onClick={() => void save()}
              disabled={!draft || !isDirty || hasErrors || isSaving}
              className="border-zinc-600 bg-zinc-800 text-zinc-100 hover:border-zinc-500 hover:bg-zinc-700 disabled:text-zinc-500"
            >
              {isSaving ? "Saving..." : "Save changes"}
            </Button>
          </div>
        </footer>}
      </div>

      {discardOpen && (
        <div className="fixed inset-0 z-[220] grid place-items-center p-4">
          <button
            type="button"
            aria-label="Close confirmation and continue editing"
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
              Discard changes?
            </h3>
            <p className="mt-2 text-xs leading-5 text-zinc-500">
              Changes made in this window have not been saved yet.
            </p>
            <div className="mt-5 flex justify-end gap-2">
              <Button onClick={() => setDiscardOpen(false)}>
                Keep editing
              </Button>
              <Button
                onClick={onClose}
                className="border-red-900 bg-red-950/30 text-red-300 hover:bg-red-950/60"
              >
                Discard
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>,
    document.body,
  );
}
