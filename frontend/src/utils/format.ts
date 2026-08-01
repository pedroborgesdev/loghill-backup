export const formatNumber = (value: number) =>
  new Intl.NumberFormat("pt-BR").format(value);

export const formatBytes = (bytes: number) => {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB"];
  let value = bytes / 1024;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index++;
  }
  return `${value.toFixed(value < 10 ? 1 : 0)} ${units[index]}`;
};

export const formatDate = (value: string | null | undefined) =>
  value
    ? new Intl.DateTimeFormat("pt-BR", {
        dateStyle: "short",
        timeStyle: "medium",
      }).format(new Date(value))
    : "Nunca";

export const relativeDate = (value: string | null | undefined) => {
  if (!value) return "Nunca";
  const seconds = Math.max(
    0,
    Math.floor((Date.now() - new Date(value).getTime()) / 1000),
  );
  if (seconds < 60) return "agora";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}min atrás`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h atrás`;
  return `${Math.floor(seconds / 86400)}d atrás`;
};

export const formatLogTime = (value: string, compact = false) =>
  new Intl.DateTimeFormat("pt-BR", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    fractionalSecondDigits: compact ? undefined : 3,
  }).format(new Date(value));
