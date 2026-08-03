import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type PropsWithChildren,
} from "react";
import { authApi } from "../api/auth";
import { setUnauthorizedHandler } from "../api/client";
import { queryClient } from "../api/queryClient";

type AuthState = "loading" | "authenticated" | "anonymous";

interface AuthContextValue {
  state: AuthState;
  authRequired: boolean;
  login: (password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: PropsWithChildren) {
  const [state, setState] = useState<AuthState>("loading");
  const [authRequired, setAuthRequired] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const session = await authApi.session();
      setAuthRequired(session.auth_required);
      setState(session.authenticated || !session.auth_required ? "authenticated" : "anonymous");
    } catch {
      setAuthRequired(true);
      setState("anonymous");
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    setUnauthorizedHandler(() => {
      setState("anonymous");
      queryClient.clear();
    });
    return () => setUnauthorizedHandler(null);
  }, []);

  const login = useCallback(async (password: string) => {
    const session = await authApi.login(password);
    setAuthRequired(session.auth_required);
    setState("authenticated");
  }, []);

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } finally {
      queryClient.clear();
      setState("anonymous");
    }
  }, []);

  const value = useMemo(
    () => ({ state, authRequired, login, logout, refresh }),
    [authRequired, login, logout, refresh, state],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used within AuthProvider");
  return value;
}
