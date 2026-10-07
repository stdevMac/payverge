"use client";

import React, { Suspense, useCallback, useEffect } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import StaffLogin from "../../../../components/staff/StaffLogin";
import { useAuth } from "@/providers/HybridAuthProvider";
import toast from "react-hot-toast";
import { getSearchParam } from "@/utils/nextRouteParams";
import { isSafeRedirectUrl } from "@/utils/safeRedirect";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { RouteLoadingFallback } from "@/components/ui/AsyncState";
import { staffHomeHref } from "./staffHomeHref";

function StaffLoginPageInner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { locale } = useSimpleLocale();
  const tString = useCallback(
    (key: string): string => {
      const result = getTranslation(`staffLogin.errors.${key}`, locale);
      return typeof result === "string" && result.length > 0 ? result : key;
    },
    [locale],
  );
  const {
    refreshStaffData,
    isInitialized,
    isLoading,
    isStaffUser,
    isOAuthUser,
    isWeb3User,
  } = useAuth();

  useEffect(() => {
    const error = getSearchParam(searchParams, "staff_error");
    if (!error) return;

    if (error === "not_found") {
      toast.error(tString("notFound"));
    } else if (error === "inactive") {
      toast.error(tString("inactive"));
    } else {
      toast.error(tString("generic"));
    }

    router.replace("/staff/login");
  }, [searchParams, router, tString]);

  // Authenticated users that land back on /staff/login (browser back
  // button, bookmark, stale email link) should bounce to their dashboard
  // instead of being asked to log in again. Wait for HybridAuthProvider
  // to finish initialising so we don't redirect during the first paint.
  useEffect(() => {
    if (!isInitialized || isLoading) return;
    const alreadyAuthed = isStaffUser || isOAuthUser || isWeb3User;
    if (!alreadyAuthed) return;
    const redirectParam = getSearchParam(searchParams, "redirect");
    const safeRedirect =
      redirectParam &&
      isSafeRedirectUrl(redirectParam) &&
      !redirectParam.startsWith("/staff/login")
        ? redirectParam
        : null;
    // Authenticated staff bounce to their own mobile-first dashboard (Slice 1);
    // an explicit ?redirect= still wins. Non-staff operators keep /dashboard.
    const fallback = isStaffUser ? staffHomeHref(locale) : "/dashboard";
    router.replace(safeRedirect ?? fallback);
  }, [
    isInitialized,
    isLoading,
    isStaffUser,
    isOAuthUser,
    isWeb3User,
    searchParams,
    router,
    locale,
  ]);

  const handleLoginSuccess = async () => {
    const redirectParam = getSearchParam(searchParams, "redirect");
    const safeRedirect =
      redirectParam &&
      isSafeRedirectUrl(redirectParam) &&
      !redirectParam.startsWith("/staff/login")
        ? redirectParam
        : null;
    // Staff land on their own mobile-first dashboard (Slice 1). An explicit
    // ?redirect= still wins (deep-links into operator service tools, etc.).
    const destination = safeRedirect ?? staffHomeHref(locale);

    try {
      await refreshStaffData();
      await new Promise((resolve) => setTimeout(resolve, 100));
    } catch (error) {
      console.error("[StaffLoginPage] Failed to refresh auth data:", error);
    } finally {
      router.push(destination);
    }
  };

  return <StaffLogin onLoginSuccess={handleLoginSuccess} />;
}

export default function StaffLoginClient() {
  return (
    <Suspense fallback={<RouteLoadingFallback />}>
      <StaffLoginPageInner />
    </Suspense>
  );
}
