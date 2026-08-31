import { Navigate, Outlet } from "react-router";
import { useAuth } from "@/stores/auth";

export function RequireAuth() {
  const token = useAuth((s) => s.accessToken);
  return token ? <Outlet /> : <Navigate to="/login" replace />;
}
