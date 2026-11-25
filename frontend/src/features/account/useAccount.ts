import { useMutation } from "@tanstack/react-query";
import { apiFetch, apiFetchBlob } from "@/lib/api";

export function useExportAccount() {
  return useMutation({
    mutationFn: async () => {
      const blob = await apiFetchBlob("/api/account/export");
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = "herbie-export.zip";
      document.body.appendChild(link);
      link.click();
      link.remove();
      // Give Firefox a tick to start the download before revoking.
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    },
  });
}

export interface DeleteAccountInput {
  password: string;
  confirm: string;
}

export function useDeleteAccount() {
  return useMutation({
    mutationFn: (input: DeleteAccountInput) =>
      apiFetch<void>("/api/account/delete", { method: "POST", json: input }),
  });
}
