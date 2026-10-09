"use client";

import { useEffect } from "react";
import dynamic from "next/dynamic";
import { useRouter } from "next/navigation";
import { Spinner } from "@nextui-org/react";
import { useAuth } from "@/providers/HybridAuthProvider";
import { useUserStore } from "@/store/useUserStore";
import { isAdminRole, resolveAdminRole } from "@/utils/auth";

// Defer AdminShell (+ admin chrome graph) until after the role check (#288).
// Anonymous / non-admin sessions should not pay for the admin shell chunk on
// first paint. This is still a soft UX gate — not a security boundary — because
// auth cookies are host-only on a split API host such as api.example.com
// (SEC-5 / #314), so Edge/SSR cannot see session_token. /api/v1/admin/* stays
// behind AuthenticationAdminMiddleware.
const AdminShell = dynamic(
  () =>
    import("@/components/admin/AdminShell").then((mod) => mod.AdminShell),
  {
    ssr: false,
    loading: () => (
      <div className="flex min-h-[50vh] items-center justify-center">
        <Spinner size="lg" color="primary" aria-label="Loading admin" />
      </div>
    ),
  },
);

export default function AdminLayoutClient({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const router = useRouter();
  const { isInitialized, isLoading, oauthData, staffData } = useAuth();
  const { user } = useUserStore();
  // Match AuthenticationAdminMiddleware: only user/web3 principals with
  // role=admin. Staff tokens with role=admin are API-rejected.
  const role = resolveAdminRole(oauthData?.role, undefined, user?.role);
  const isAdmin = isAdminRole(role) && !staffData;

  useEffect(() => {
    if (!isInitialized || isLoading) return;
    if (!isAdmin) {
      router.replace("/dashboard");
    }
  }, [isInitialized, isLoading, isAdmin, router]);

  if (!isInitialized || isLoading) {
    return (
      <div className="flex min-h-[50vh] items-center justify-center">
        <Spinner size="lg" color="primary" aria-label="Loading session" />
      </div>
    );
  }

  if (!isAdmin) return null;

  return (
    <main id="main-content" tabIndex={-1}>
      <AdminShell>{children}</AdminShell>
    </main>
  );
}
