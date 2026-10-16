"use client";

import * as React from "react";
import { authApi } from "@/features/auth/api";
import type { LoginInput, RegisterInput, User } from "@/features/auth/types";

export type AuthStatus = "loading" | "authenticated" | "anonymous";

interface AuthContextValue {
  status: AuthStatus;
  user: User | null;
  login: (input: LoginInput) => Promise<User>;
  register: (input: RegisterInput) => Promise<User>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
}

const AuthContext = React.createContext<AuthContextValue | null>(null);

/**
 * Provides the current session to the client tree.
 *
 * The session itself is an HTTP-only cookie owned by the API; this provider
 * only mirrors the resolved user. On mount it calls `/auth/me`; a 401 means
 * the visitor is anonymous (not an error worth surfacing).
 */
export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = React.useState<AuthStatus>("loading");
  const [user, setUser] = React.useState<User | null>(null);

  // State is updated only inside promise callbacks (never synchronously in an
  // effect body), which avoids cascading renders. A 401 means the visitor is
  // anonymous; other failures also fail closed to anonymous.
  const load = React.useCallback(
    (): Promise<void> =>
      authApi
        .me()
        .then((me) => {
          setUser(me);
          setStatus("authenticated");
        })
        .catch(() => {
          setUser(null);
          setStatus("anonymous");
        }),
    [],
  );

  React.useEffect(() => {
    void load();
  }, [load]);

  const login = React.useCallback(async (input: LoginInput) => {
    const me = await authApi.login(input);
    setUser(me);
    setStatus("authenticated");
    return me;
  }, []);

  const register = React.useCallback(async (input: RegisterInput) => {
    const me = await authApi.register(input);
    setUser(me);
    setStatus("authenticated");
    return me;
  }, []);

  const logout = React.useCallback(async () => {
    try {
      await authApi.logout();
    } finally {
      setUser(null);
      setStatus("anonymous");
    }
  }, []);

  const value = React.useMemo<AuthContextValue>(
    () => ({ status, user, login, register, logout, refresh: load }),
    [status, user, login, register, logout, load],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = React.useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within <AuthProvider>.");
  return ctx;
}
