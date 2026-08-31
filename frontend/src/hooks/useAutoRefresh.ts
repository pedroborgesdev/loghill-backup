import { useEffect, useRef, useState } from "react";

const STORAGE_KEY = "page-refresh";
const LEGACY_STORAGE_KEY = "dashboard-refresh";
const VALID_INTERVALS = new Set([0, 15, 30, 60]);

function initialInterval() {
  const stored = Number(
    localStorage.getItem(STORAGE_KEY) ??
      localStorage.getItem(LEGACY_STORAGE_KEY) ??
      30,
  );
  return VALID_INTERVALS.has(stored) ? stored : 30;
}

export function useAutoRefresh(refresh: () => void) {
  const [interval, setIntervalValue] = useState(initialInterval);
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;

  useEffect(() => {
    if (!interval) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") refreshRef.current();
    }, interval * 1_000);
    return () => window.clearInterval(timer);
  }, [interval]);

  const setInterval = (value: number) => {
    setIntervalValue(value);
    localStorage.setItem(STORAGE_KEY, String(value));
  };

  return [interval, setInterval] as const;
}
