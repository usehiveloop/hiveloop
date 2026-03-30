"use client";

import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from "react";

export type DashboardMode = "app" | "platform";

const STORAGE_KEY = "llmvault_dashboard_mode";

type DashboardModeContextValue = {
  mode: DashboardMode;
  setMode: (mode: DashboardMode) => void;
  toggleMode: () => void;
};

const DashboardModeContext = createContext<DashboardModeContextValue | null>(null);

export function DashboardModeProvider({ children }: { children: ReactNode }) {
  const [mode, setModeState] = useState<DashboardMode>("app");
  const [hydrated, setHydrated] = useState(false);

  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === "app" || stored === "platform") {
      setModeState(stored);
    }
    setHydrated(true);
  }, []);

  const setMode = useCallback((newMode: DashboardMode) => {
    setModeState(newMode);
    localStorage.setItem(STORAGE_KEY, newMode);
  }, []);

  const toggleMode = useCallback(() => {
    setMode(mode === "app" ? "platform" : "app");
  }, [mode, setMode]);

  // Prevent flash of wrong nav before hydration
  if (!hydrated) return null;

  return (
    <DashboardModeContext.Provider value={{ mode, setMode, toggleMode }}>
      {children}
    </DashboardModeContext.Provider>
  );
}

export function useDashboardMode(): DashboardModeContextValue {
  const ctx = useContext(DashboardModeContext);
  if (!ctx) {
    throw new Error("useDashboardMode must be used within a DashboardModeProvider");
  }
  return ctx;
}
