import {
  CalendarDays,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Clock3,
  X,
} from "lucide-react";
import {
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";

type ControlValue = string | number;

export interface ListboxOption<T extends ControlValue> {
  value: T;
  label: string;
  description?: string;
}

function usePopupPosition(
  open: boolean,
  trigger: React.RefObject<HTMLElement | null>,
  width: number,
  estimatedHeight: number,
) {
  const [position, setPosition] = useState({ left: 8, top: 8, width });

  useLayoutEffect(() => {
    if (!open || !trigger.current) return;
    const update = () => {
      const bounds = trigger.current?.getBoundingClientRect();
      if (!bounds) return;
      const left = Math.max(
        8,
        Math.min(bounds.left, window.innerWidth - width - 8),
      );
      const spaceBelow = window.innerHeight - bounds.bottom;
      const popupHeight = Math.min(estimatedHeight, window.innerHeight - 16);
      const top =
        spaceBelow >= popupHeight + 8
          ? bounds.bottom + 6
          : Math.max(8, bounds.top - popupHeight - 6);
      setPosition({ left, top, width: Math.max(width, bounds.width) });
    };
    update();
    window.addEventListener("resize", update);
    window.addEventListener("scroll", update, true);
    return () => {
      window.removeEventListener("resize", update);
      window.removeEventListener("scroll", update, true);
    };
  }, [estimatedHeight, open, trigger, width]);

  return position;
}

export function Listbox<T extends ControlValue>({
  value,
  options,
  onChange,
  label,
  className = "",
  size = "default",
  disabled = false,
}: {
  value: T;
  options: readonly ListboxOption<T>[];
  onChange: (value: T) => void;
  label: string;
  className?: string;
  size?: "compact" | "default";
  disabled?: boolean;
}) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const selectedIndex = Math.max(
    0,
    options.findIndex((option) => option.value === value),
  );
  const [highlighted, setHighlighted] = useState(selectedIndex);
  const selected = options[selectedIndex];
  const width = Math.max(size === "compact" ? 150 : 210, trigger.current?.offsetWidth ?? 0);
  const position = usePopupPosition(
    open,
    trigger,
    width,
    Math.min(options.length * 38 + 8, 280),
  );

  useEffect(() => setHighlighted(selectedIndex), [selectedIndex]);

  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!trigger.current?.contains(target) && !popup.current?.contains(target)) {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [open]);

  const select = (index: number) => {
    const option = options[index];
    if (!option) return;
    onChange(option.value);
    setOpen(false);
    trigger.current?.focus();
  };

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Escape") {
      setOpen(false);
      return;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      setOpen(true);
      const direction = event.key === "ArrowDown" ? 1 : -1;
      setHighlighted((current) =>
        (current + direction + options.length) % options.length,
      );
      return;
    }
    if ((event.key === "Enter" || event.key === " ") && open) {
      event.preventDefault();
      select(highlighted);
    }
  };

  return (
    <>
      <button
        ref={trigger}
        type="button"
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={`${id}-options`}
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
        onKeyDown={onKeyDown}
        className={`inline-flex items-center justify-between gap-2 rounded-lg border border-zinc-700 bg-zinc-950 text-left text-zinc-300 transition-colors duration-150 hover:border-zinc-600 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 disabled:cursor-not-allowed disabled:opacity-50 ${
          size === "compact" ? "h-8 px-2 text-xs" : "h-9 px-3 text-sm"
        } ${className}`}
      >
        <span className="truncate">{selected?.label ?? "Selecionar"}</span>
        <ChevronDown
          className={`size-3.5 shrink-0 text-zinc-600 transition-transform duration-150 ${
            open ? "rotate-180" : ""
          }`}
        />
      </button>
      {open &&
        createPortal(
          <div
            ref={popup}
            id={`${id}-options`}
            role="listbox"
            aria-label={label}
            aria-activedescendant={`${id}-option-${highlighted}`}
            tabIndex={-1}
            onKeyDown={onKeyDown}
            className="fixed z-[190] max-h-72 overflow-auto rounded-lg border border-zinc-700 bg-[#1c1c1f] p-1 shadow-2xl shadow-black/50"
            style={position}
          >
            {options.map((option, index) => (
              <button
                id={`${id}-option-${index}`}
                key={option.value}
                type="button"
                role="option"
                aria-selected={option.value === value}
                onMouseEnter={() => setHighlighted(index)}
                onClick={() => select(index)}
                className={`flex min-h-9 w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-xs outline-none ${
                  highlighted === index ? "bg-zinc-800 text-zinc-100" : "text-zinc-400"
                }`}
              >
                <Check
                  className={`size-3.5 shrink-0 ${
                    option.value === value ? "text-cyan-500" : "opacity-0"
                  }`}
                />
                <span className="min-w-0">
                  <span className="block truncate">{option.label}</span>
                  {option.description && (
                    <span className="block truncate text-[10px] text-zinc-600">
                      {option.description}
                    </span>
                  )}
                </span>
              </button>
            ))}
          </div>,
          document.body,
        )}
    </>
  );
}

const months = [
  "Janeiro",
  "Fevereiro",
  "Março",
  "Abril",
  "Maio",
  "Junho",
  "Julho",
  "Agosto",
  "Setembro",
  "Outubro",
  "Novembro",
  "Dezembro",
];
const weekdays = ["Seg", "Ter", "Qua", "Qui", "Sex", "Sáb", "Dom"];

function toLocalValue(date: Date, hour: string, minute: string) {
  const pad = (value: number | string) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(hour)}:${pad(minute)}`;
}

function dateFromValue(value: string) {
  const parsed = value ? new Date(value) : new Date();
  return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
}

export function DateTimePicker({
  value,
  onChange,
  label,
  placeholder = "Selecionar data e hora",
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
  placeholder?: string;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(() => dateFromValue(value));
  const [visibleMonth, setVisibleMonth] = useState(
    () => new Date(draft.getFullYear(), draft.getMonth(), 1),
  );
  const [hour, setHour] = useState(() => String(draft.getHours()).padStart(2, "0"));
  const [minute, setMinute] = useState(() => String(draft.getMinutes()).padStart(2, "0"));
  const position = usePopupPosition(open, trigger, 304, 410);

  useEffect(() => {
    if (!open) return;
    const current = dateFromValue(value);
    setDraft(current);
    setVisibleMonth(new Date(current.getFullYear(), current.getMonth(), 1));
    setHour(String(current.getHours()).padStart(2, "0"));
    setMinute(String(current.getMinutes()).padStart(2, "0"));
  }, [open, value]);

  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!trigger.current?.contains(target) && !popup.current?.contains(target)) {
        setOpen(false);
      }
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
        trigger.current?.focus();
      }
    };
    document.addEventListener("pointerdown", close);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", close);
      document.removeEventListener("keydown", escape);
    };
  }, [open]);

  const days = useMemo(() => {
    const year = visibleMonth.getFullYear();
    const month = visibleMonth.getMonth();
    const firstWeekday = (new Date(year, month, 1).getDay() + 6) % 7;
    const count = new Date(year, month + 1, 0).getDate();
    return [
      ...Array.from({ length: firstWeekday }, () => null),
      ...Array.from({ length: count }, (_, index) => new Date(year, month, index + 1)),
    ];
  }, [visibleMonth]);

  const displayValue = value
    ? new Intl.DateTimeFormat("pt-BR", {
        dateStyle: "short",
        timeStyle: "short",
      }).format(new Date(value))
    : "";

  const moveMonth = (direction: number) =>
    setVisibleMonth(
      (current) =>
        new Date(current.getFullYear(), current.getMonth() + direction, 1),
    );

  return (
    <>
      <div className="relative">
        <button
          ref={trigger}
          type="button"
          aria-label={label}
          aria-haspopup="dialog"
          aria-expanded={open}
          onClick={() => setOpen((current) => !current)}
          className="flex h-9 w-full items-center gap-2 rounded-lg border border-zinc-700 bg-zinc-950 px-3 pr-9 text-left text-xs transition-colors hover:border-zinc-600 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
        >
          <CalendarDays className="size-4 shrink-0 text-zinc-600" />
          <span className={`min-w-0 flex-1 truncate ${displayValue ? "text-zinc-300" : "text-zinc-600"}`}>
            {displayValue || placeholder}
          </span>
        </button>
        {value && (
          <button
            type="button"
            aria-label={`Limpar ${label}`}
            onClick={(event) => {
              event.stopPropagation();
              onChange("");
            }}
            className="absolute right-2 top-1/2 grid size-6 -translate-y-1/2 place-items-center rounded text-zinc-600 hover:bg-zinc-800 hover:text-zinc-300 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
          >
            <X className="size-3.5" />
          </button>
        )}
      </div>

      {open &&
        createPortal(
          <div
            ref={popup}
            role="dialog"
            aria-label={label}
            className="fixed z-[190] rounded-xl border border-zinc-700 bg-[#161618] p-3 shadow-2xl shadow-black/60"
            style={position}
          >
            <div className="flex items-center justify-between">
              <button
                type="button"
                aria-label="Mês anterior"
                onClick={() => moveMonth(-1)}
                className="grid size-8 place-items-center rounded-lg text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
              >
                <ChevronLeft className="size-4" />
              </button>
              <p className="text-sm font-medium text-zinc-200">
                {months[visibleMonth.getMonth()]} {visibleMonth.getFullYear()}
              </p>
              <button
                type="button"
                aria-label="Próximo mês"
                onClick={() => moveMonth(1)}
                className="grid size-8 place-items-center rounded-lg text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
              >
                <ChevronRight className="size-4" />
              </button>
            </div>

            <div className="mt-2 grid grid-cols-7 gap-1">
              {weekdays.map((weekday) => (
                <span key={weekday} className="grid h-7 place-items-center text-[10px] text-zinc-600">
                  {weekday}
                </span>
              ))}
              {days.map((day, index) =>
                day ? (
                  <button
                    key={day.toISOString()}
                    type="button"
                    aria-label={day.toLocaleDateString("pt-BR")}
                    aria-pressed={
                      day.getFullYear() === draft.getFullYear() &&
                      day.getMonth() === draft.getMonth() &&
                      day.getDate() === draft.getDate()
                    }
                    onClick={() => setDraft(day)}
                    className={`grid size-8 place-items-center rounded-lg border border-transparent text-xs focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 ${
                      day.getFullYear() === draft.getFullYear() &&
                      day.getMonth() === draft.getMonth() &&
                      day.getDate() === draft.getDate()
                        ? "border-zinc-500 bg-zinc-800 font-medium text-zinc-100"
                        : "text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
                    }`}
                  >
                    {day.getDate()}
                  </button>
                ) : (
                  <span key={`empty-${index}`} className="size-8" />
                ),
              )}
            </div>

            <div className="mt-3 flex items-center gap-2 border-t border-zinc-800 pt-3">
              <Clock3 className="size-4 text-zinc-600" />
              <input
                aria-label="Hora"
                inputMode="numeric"
                maxLength={2}
                value={hour}
                onChange={(event) =>
                  setHour(event.target.value.replace(/\D/g, "").slice(0, 2))
                }
                className="h-8 w-12 rounded-lg border border-zinc-700 bg-zinc-950 text-center font-mono text-xs text-zinc-200 outline-none focus:ring-1 focus:ring-white/50"
              />
              <span className="text-zinc-600">:</span>
              <input
                aria-label="Minuto"
                inputMode="numeric"
                maxLength={2}
                value={minute}
                onChange={(event) =>
                  setMinute(event.target.value.replace(/\D/g, "").slice(0, 2))
                }
                className="h-8 w-12 rounded-lg border border-zinc-700 bg-zinc-950 text-center font-mono text-xs text-zinc-200 outline-none focus:ring-1 focus:ring-white/50"
              />
              <button
                type="button"
                onClick={() => {
                  const now = new Date();
                  setDraft(now);
                  setVisibleMonth(new Date(now.getFullYear(), now.getMonth(), 1));
                  setHour(String(now.getHours()).padStart(2, "0"));
                  setMinute(String(now.getMinutes()).padStart(2, "0"));
                }}
                className="ml-auto rounded px-1 text-[11px] text-zinc-500 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
              >
                Agora
              </button>
            </div>

            <div className="mt-3 flex justify-end gap-2 border-t border-zinc-800 pt-3">
              <button
                type="button"
                onClick={() => setOpen(false)}
                className="h-8 rounded-lg px-3 text-xs text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
              >
                Cancelar
              </button>
              <button
                type="button"
                onClick={() => {
                  const normalizedHour = String(Math.min(23, Number(hour) || 0));
                  const normalizedMinute = String(Math.min(59, Number(minute) || 0));
                  onChange(toLocalValue(draft, normalizedHour, normalizedMinute));
                  setOpen(false);
                  trigger.current?.focus();
                }}
                className="h-8 rounded-lg border border-zinc-500 bg-zinc-800 px-3 text-xs font-medium text-zinc-100 hover:border-zinc-400 hover:bg-zinc-700 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
              >
                Aplicar
              </button>
            </div>
          </div>,
          document.body,
        )}
    </>
  );
}
