import React, { createContext, useContext, useEffect, useState } from "react";
import type { SettingsTab } from "@/features/settings/SettingsDialog";

interface SidebarContextType {
  collapsed: boolean;
  setCollapsed: (collapsed: boolean) => void;
  toggleSidebar: () => void;
  mobileOpen: boolean;
  setMobileOpen: (open: boolean) => void;
  settingsOpen: boolean;
  setSettingsOpen: (open: boolean) => void;
  settingsTab: SettingsTab;
  openSettings: (tab?: SettingsTab) => void;
}

const SidebarContext = createContext<SidebarContextType | undefined>(undefined);

export function SidebarProvider({ children }: { children: React.ReactNode }) {
  const [collapsed, setCollapsedState] = useState<boolean>(() => {
    try {
      return localStorage.getItem("golem:sidebar-collapsed") === "true";
    } catch {
      return false;
    }
  });
  const [mobileOpen, setMobileOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTab, setSettingsTab] = useState<SettingsTab>("general");

  const setCollapsed = (val: boolean) => {
    setCollapsedState(val);
    try {
      localStorage.setItem("golem:sidebar-collapsed", String(val));
    } catch {
      // localStorage unavailable
    }
  };

  const toggleSidebar = () => setCollapsed(!collapsed);

  const openSettings = (tab: SettingsTab = "general") => {
    setSettingsTab(tab);
    setSettingsOpen(true);
  };

  // Keyboard shortcut: Cmd/Ctrl + B to toggle sidebar
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && (e.key === "b" || e.key === "B")) {
        // Prevent browser bookmarks shortcut if applicable
        e.preventDefault();
        setCollapsedState((prev) => {
          const next = !prev;
          try {
            localStorage.setItem("golem:sidebar-collapsed", String(next));
          } catch { /* ignore */ }
          return next;
        });
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  return (
    <SidebarContext.Provider
      value={{
        collapsed,
        setCollapsed,
        toggleSidebar,
        mobileOpen,
        setMobileOpen,
        settingsOpen,
        setSettingsOpen,
        settingsTab,
        openSettings,
      }}
    >
      {children}
    </SidebarContext.Provider>
  );
}

const defaultSidebarContext: SidebarContextType = {
  collapsed: false,
  setCollapsed: () => {},
  toggleSidebar: () => {},
  mobileOpen: false,
  setMobileOpen: () => {},
  settingsOpen: false,
  setSettingsOpen: () => {},
  settingsTab: "general",
  openSettings: () => {},
};

export function useSidebar() {
  const ctx = useContext(SidebarContext);
  return ctx ?? defaultSidebarContext;
}
