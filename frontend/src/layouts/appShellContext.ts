import { createContext, useContext } from "react";
import type { StreamState } from "../hooks/useLogStream";

export interface ShellContextValue {
  refreshToken: number;
  refreshing: boolean;
  setRefreshing: (refreshing: boolean) => void;
  streamState: StreamState | null;
  setStreamState: (state: StreamState | null) => void;
  openEmailSettings: (trigger?: HTMLButtonElement) => void;
}

export const ShellContext = createContext<ShellContextValue | null>(null);

export function useAppShell() {
  const value = useContext(ShellContext);
  if (!value) throw new Error("useAppShell deve ser usado dentro de AppShell");
  return value;
}
