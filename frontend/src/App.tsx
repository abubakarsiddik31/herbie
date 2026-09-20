import { useEffect, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ThemeProvider } from "next-themes";
import { BrowserRouter, Navigate, Route, Routes } from "react-router";
import { Toaster } from "@/components/ui/sonner";
import { RequireAuth } from "@/components/RequireAuth";
import { AppLayout } from "@/components/layout/AppLayout";
import { LoginPage } from "@/pages/LoginPage";
import { RegisterPage } from "@/pages/RegisterPage";
import { LandingPage } from "@/pages/LandingPage";
import { OAuthCallbackPage } from "@/pages/OAuthCallbackPage";
import { PreferencesPage } from "@/pages/PreferencesPage";
import { SharedPage } from "@/pages/SharedPage";
import { ChatPage } from "@/pages/ChatPage";
import { ToolsPage } from "@/pages/ToolsPage";
import { UsagePage } from "@/pages/UsagePage";
import { ProjectsPage } from "@/pages/ProjectsPage";
import { ProjectWorkspacePage } from "@/pages/ProjectWorkspacePage";
import { WorkflowsPage } from "@/pages/WorkflowsPage";
import { WorkflowCanvasPage } from "@/pages/WorkflowCanvasPage";
import { tryRefresh } from "@/lib/api";
import { useAuth } from "@/stores/auth";

const queryClient = new QueryClient();

export default function App() {
  const [initializing, setInitializing] = useState(() => {
    if (typeof window !== "undefined" && window.location.pathname.startsWith("/auth/callback")) {
      return false;
    }
    return true;
  });
  const token = useAuth((s) => s.accessToken);

  useEffect(() => {
    if (!initializing) return;
    void tryRefresh().finally(() => {
      setInitializing(false);
    });
  }, [initializing]);

  if (initializing) {
    return (
      <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
        <div className="flex min-h-svh items-center justify-center bg-background text-foreground">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        </div>
      </ThemeProvider>
    );
  }

  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
        <Routes>
          <Route path="/" element={<LandingPage />} />
          <Route path="/landing" element={<LandingPage />} />
          <Route path="/login" element={token ? <Navigate to="/chat" replace /> : <LoginPage />} />
          <Route path="/register" element={token ? <Navigate to="/chat" replace /> : <RegisterPage />} />
          <Route path="/auth/callback" element={<OAuthCallbackPage />} />
          <Route path="/s/:token" element={<SharedPage />} />
          <Route element={<RequireAuth />}>
            <Route element={<AppLayout />}>
              <Route path="/chat/:conversationId?" element={<ChatPage />} />
              <Route path="/projects" element={<ProjectsPage />} />
              <Route path="/projects/:projectId" element={<ProjectWorkspacePage />} />
              <Route path="/workflows" element={<WorkflowsPage />} />
              <Route path="/workflows/:workflowId" element={<WorkflowCanvasPage />} />
              <Route path="/tools" element={<ToolsPage />} />
              <Route path="/usage" element={<UsagePage />} />
              <Route path="/preferences" element={<PreferencesPage />} />
              <Route path="/documents" element={<Navigate to="/projects" replace />} />
            </Route>
          </Route>
          <Route path="*" element={<Navigate to="/chat" replace />} />
        </Routes>
        <Toaster position="top-center" />
      </BrowserRouter>
    </QueryClientProvider>
    </ThemeProvider>
  );
}
