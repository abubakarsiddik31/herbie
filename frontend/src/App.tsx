import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ThemeProvider } from "next-themes";
import { BrowserRouter, Navigate, Route, Routes } from "react-router";
import { Toaster } from "@/components/ui/sonner";
import { RequireAuth } from "@/components/RequireAuth";
import { AppLayout } from "@/components/layout/AppLayout";
import { LoginPage } from "@/pages/LoginPage";
import { RegisterPage } from "@/pages/RegisterPage";
import { OAuthCallbackPage } from "@/pages/OAuthCallbackPage";
import { PreferencesPage } from "@/pages/PreferencesPage";
import { SharedPage } from "@/pages/SharedPage";
import { ChatPage } from "@/pages/ChatPage";
import { ToolsPage } from "@/pages/ToolsPage";
import { UsagePage } from "@/pages/UsagePage";
import { DocumentsPage } from "@/pages/DocumentsPage";

const queryClient = new QueryClient();

export default function App() {
  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/auth/callback" element={<OAuthCallbackPage />} />
          <Route path="/s/:token" element={<SharedPage />} />
          <Route element={<RequireAuth />}>
            <Route element={<AppLayout />}>
              <Route path="/chat/:conversationId?" element={<ChatPage />} />
              <Route path="/tools" element={<ToolsPage />} />
              <Route path="/usage" element={<UsagePage />} />
              <Route path="/preferences" element={<PreferencesPage />} />
              <Route path="/documents" element={<DocumentsPage />} />
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
