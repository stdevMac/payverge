"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactElement,
} from "react";
import Image from "next/image";
import { useAccount } from "wagmi";
import {
  Button,
  Dropdown,
  DropdownTrigger,
  DropdownMenu,
  DropdownItem,
  DropdownSection,
} from "@nextui-org/react";
import {
  House,
  LogOut,
  Shield,
  ChevronDown,
  LayoutGrid,
  UtensilsCrossed,
  User,
  X,
  Download,
} from "lucide-react";
import { Web3Button } from "@/components/ui/web3-button/Web3Button";
import { useUserStore } from "@/store/useUserStore";
import { isAdmin } from "@/utils/auth";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useLogout } from "@/hooks";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { localizedPublicHref } from "@/i18n/publicPageRoutes";
import { useAuth } from "@/providers/HybridAuthProvider";
import {
  clearStaffSession,
  getRoleColor,
  redirectToStaffLogin,
} from "@/utils/staffAuth";
import { AuthModal } from "@/components/auth/AuthModal";
import { motion, AnimatePresence } from "framer-motion";
import {
  HEADER_SIGN_IN_CLASS,
  HEADER_STAFF_LINK_CLASS,
  isOperatorDashboardPath,
  MOBILE_ONLY_CLASS,
  TopMenuShell,
} from "./TopMenuShell";
import SimpleLanguageSwitcherLazy from "@/components/SimpleLanguageSwitcherLazy";
import { useDialogBehavior } from "@/hooks/useDialogBehavior";
import { usePwaInstall } from "@/providers/PwaInstallProvider";
import { flushSync } from "react-dom";
import { useOperatorAccessError } from "@/hooks/useOperatorAccessError";
import { useInstance } from "@/hooks/useInstance";
import { getOperatorDashboardPath } from "@/utils/businessUrl";

export const TopMenu = ({ onReady }: { onReady?: () => void }) => {
  const { isConnected, address } = useAccount();
  const { isOff } = useInstance();
  const cryptoOff = isOff("crypto");
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const [skipMobileDrawerExit, setSkipMobileDrawerExit] = useState(false);
  const [isAuthModalOpen, setIsAuthModalOpen] = useState(false);
  const [authModalTab, setAuthModalTab] = useState<
    "signin" | "signup" | "staff"
  >("signin");
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const { user } = useUserStore();
  const {
    isWeb3User,
    isStaffUser,
    isOAuthUser,
    oauthData,
    staffData,
    isLoading: authLoading,
    isInitialized: authInitialized,
  } = useAuth();
  const pwaInstall = usePwaInstall();
  const [installPending, setInstallPending] = useState(false);
  const installPendingRef = useRef(false);
  const mountedRef = useRef(true);

  const userIsAdmin = isAdmin(user);
  const pathname = usePathname();
  const { logout } = useLogout();
  const router = useRouter();
  const [scrolled, setScrolled] = useState(false);
  const mobileDrawerRef = useRef<HTMLDivElement | null>(null);
  const isDashboardRoute =
    pathname?.includes("/dashboard") || pathname?.includes("/admin");

  useEffect(() => {
    onReady?.();
  }, [onReady]);

  useEffect(() => {
    const handleScroll = () => {
      setScrolled(window.scrollY > 20 || !!isDashboardRoute);
    };
    handleScroll();
    window.addEventListener("scroll", handleScroll);
    return () => window.removeEventListener("scroll", handleScroll);
  }, [isDashboardRoute]);

  const oauthUserEmail = oauthData?.email;
  const oauthUserName = user?.username || oauthData?.email?.split("@")[0];
  const oauthUserPicture = oauthData?.picture || null;

  const handleOAuthLogout = async () => {
    await logout();
    router.push("/");
  };

  const isAuthenticated =
    !authLoading && (isConnected || isStaffUser || isOAuthUser);

  const searchParams = useSearchParams();
  // `?auth=…` asks for the sign-in dialog. On /dashboard the page owns that
  // dialog (it also drops the parameter once a session exists), so the header
  // must not open a second one from the same parameter: it stayed open over
  // the venue list after a demo sign-in. A signed-in visitor gets no dialog.
  useEffect(() => {
    if (!searchParams || pathname === "/dashboard" || isAuthenticated) return;
    const authQuery = searchParams.get("auth");
    if (
      authQuery === "signin" ||
      authQuery === "signup" ||
      authQuery === "staff"
    ) {
      setAuthModalTab(authQuery);
      setIsAuthModalOpen(true);
    }
  }, [searchParams, pathname, isAuthenticated]);

  // The dialog has done its job once a session exists, whichever path signed
  // the visitor in (header button, `?auth` link, demo one-click).
  useEffect(() => {
    if (isAuthenticated) setIsAuthModalOpen(false);
  }, [isAuthenticated]);
  const onOperatorDashboard = isOperatorDashboardPath(pathname);
  const operatorAccessError = useOperatorAccessError();
  // Dashboard chrome must stay neutral while session hydrates — never flash
  // public Sign-in chrome mid-shift (issues #69 / #148).
  const showPublicNav =
    !operatorAccessError &&
    !onOperatorDashboard &&
    authInitialized &&
    !authLoading &&
    !isAuthenticated;
  const authRailPending =
    authLoading || (onOperatorDashboard && !authInitialized);
  const showInstallAction =
    !operatorAccessError &&
    isAuthenticated &&
    pwaInstall.identity !== null &&
    pwaInstall.state !== "installed";

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const handleInstall = useCallback(async () => {
    if (installPendingRef.current) return;

    installPendingRef.current = true;
    setInstallPending(true);
    try {
      await pwaInstall.requestInstall();
    } catch {
      // The provider owns install help and outcome reporting. A rejected browser
      // prompt must only release this surface's re-entry guard.
    } finally {
      installPendingRef.current = false;
      if (mountedRef.current) setInstallPending(false);
    }
  }, [pwaInstall]);

  // clearStaffSession owns the single POST /staff/logout (and never throws);
  // a second call would always 401 because the first cleared the cookie.
  const handleStaffLogout = async () => {
    try {
      await clearStaffSession();
    } finally {
      redirectToStaffLogin();
    }
  };

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const tString = (key: string): string => {
    const fullKey = `navigation.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const labels = {
    home: tString("home"),
    staffLogin: tString("staffLogin"),
    signIn: tString("signIn"),
    toggleMenuAria: tString("toggleMenuAria") || "Toggle menu",
  };

  useEffect(() => {
    if (isMenuOpen) {
      document.body.style.overflow = "hidden";
    } else {
      document.body.style.overflow = "unset";
    }
    return () => {
      document.body.style.overflow = "unset";
    };
  }, [isMenuOpen]);

  // The mobile nav drawer is a hand-rolled slide-over (a bare motion.div), so it
  // needs the same focus/keyboard affordances a real modal has: move focus in on
  // open, trap Tab within it, close on Escape, and restore focus on close.
  useDialogBehavior({
    isOpen: isMenuOpen,
    onClose: () => setIsMenuOpen(false),
    containerRef: mobileDrawerRef,
  });

  // Public registration is a conversion surface — never surface Admin there
  // even if the signed-in principal is an admin (finding 63).
  const isPublicRegisterRoute =
    pathname === "/business/register" ||
    pathname?.startsWith("/business/register/");

  // Primary desktop nav: Dashboard / Staff Dashboard only. Platform Admin is
  // demoted into the profile menu so it does not sit beside venue chrome (#62).
  const authenticatedLinks = [
    {
      name: tString("dashboard"),
      href: "/dashboard",
      icon: <LayoutGrid size={18} />,
      visible: isWeb3User || isOAuthUser,
    },
    {
      name: tString("staffDashboard"),
      href:
        isStaffUser && staffData
          ? getOperatorDashboardPath(String(staffData.business_slug || staffData.business_id))
          : "#",
      icon: <UtensilsCrossed size={18} />,
      visible: isStaffUser,
    },
  ];

  const visibleAuthLinks = authenticatedLinks.filter((link) => link.visible);
  const showAdminInProfile = userIsAdmin && !isPublicRegisterRoute;

  const desktopCenterNav =
    authLoading || !isAuthenticated ? undefined : (
      <div className="flex items-center gap-0.5">
        <Link
          href="/"
          className={`flex items-center gap-1.5 px-3 py-2 rounded-full text-sm font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand ${
            pathname === "/"
              ? "text-brand bg-brand/10"
              : "text-ink-600 hover:text-ink-950 hover:bg-warm-100/80"
          }`}
        >
          <House size={17} />
          <span>{labels.home}</span>
        </Link>
        {visibleAuthLinks.map((link) => (
          <Link
            key={link.name}
            href={link.href}
            className={`flex items-center gap-1.5 px-3 py-2 rounded-full text-sm font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand ${
              pathname === link.href
                ? "text-brand bg-brand/10"
                : "text-ink-600 hover:text-ink-950 hover:bg-warm-100/80"
            }`}
          >
            <span className="text-lg">{link.icon}</span>
            <span>{link.name}</span>
          </Link>
        ))}
      </div>
    );

  const publicAuthButtons = (
    <motion.div
      data-testid="header-auth-rail"
      className="hidden md:flex items-center gap-2"
      initial={false}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.2 }}
    >
      <Link
        href={localizedPublicHref(locale, "/staff/login")}
        className={HEADER_STAFF_LINK_CLASS}
      >
        {labels.staffLogin}
      </Link>
      <button
        type="button"
        onClick={() => {
          setAuthModalTab("signin");
          setIsAuthModalOpen(true);
        }}
        className={HEADER_SIGN_IN_CLASS}
      >
        {labels.signIn}
      </button>
    </motion.div>
  );

  const rightRail = (
    <>
      {!/^\/business\/[^/]+\/dashboard(\/|$)/.test(pathname ?? "") && (
        <>
          <span data-testid="header-locale-compact" className="xl:hidden">
            <SimpleLanguageSwitcherLazy compact />
          </span>
          <span
            data-testid="header-locale-full"
            className="hidden xl:inline-flex"
          >
            <SimpleLanguageSwitcherLazy />
          </span>
        </>
      )}

      {isStaffUser && staffData && (
        <div className="hidden lg:flex items-center gap-2 px-3 py-1.5 rounded-full bg-brand/5 border border-brand/20">
          <span className="text-sm font-medium text-brand-dark">
            {staffData.name}
          </span>
          <div
            className="w-2 h-2 rounded-full motion-safe:animate-pulse"
            style={{
              backgroundColor: getRoleColor(staffData.role || "") as string,
            }}
          />
        </div>
      )}

      {/* On operator dashboards, never paint Sign in / Staff Login CTAs — the
          shell AuthRailSkeleton covers session hydration instead. */}
      {!onOperatorDashboard &&
        authInitialized &&
        !authLoading &&
        !isAuthenticated &&
        publicAuthButtons}

      {(() => {
        const walletRoutes = ["/dashboard", "/business"];
        const showWalletButton = walletRoutes.some((route) =>
          pathname?.startsWith(route),
        );
        if (!showWalletButton) return null;
        return isConnected && !cryptoOff && !isStaffUser && !isOAuthUser ? (
          <Web3Button />
        ) : null;
      })()}

      {isAuthenticated && (address || staffData || isOAuthUser) && (
        <Dropdown
          placement="bottom-end"
          className="bg-white/95 backdrop-blur-xl border border-warm-200 shadow-xl"
        >
          <DropdownTrigger>
            <button
              type="button"
              aria-label={tString("profileActionsAria") || "Profile actions"}
              className="flex items-center gap-2 cursor-pointer p-1 pr-2 rounded-full border border-warm-200 hover:border-warm-300 hover:bg-warm-50 transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand group bg-white shadow-sm"
            >
              {isOAuthUser && oauthUserPicture ? (
                <Image
                  src={oauthUserPicture}
                  alt="Profile"
                  width={32}
                  height={32}
                  className="rounded-full object-cover ring-2 ring-white"
                  referrerPolicy="no-referrer"
                  unoptimized
                />
              ) : (
                <div className="w-8 h-8 rounded-full bg-brand flex items-center justify-center text-white text-sm font-bold ring-2 ring-white shadow-sm">
                  {(oauthUserName || oauthUserEmail || staffData?.name || "U")
                    ?.charAt(0)
                    .toUpperCase()}
                </div>
              )}
              <ChevronDown
                className="text-ink-400 group-hover:text-ink-600 transition-transform duration-200 group-data-[open=true]:rotate-180"
                size={16}
              />
            </button>
          </DropdownTrigger>
          <DropdownMenu
            aria-label={tString("profileActionsAria") || "Profile Actions"}
            variant="flat"
            className="min-w-[220px]"
          >
            <DropdownSection showDivider>
              <DropdownItem
                key="profile"
                className="h-auto py-3 gap-2 opacity-100 cursor-default"
                textValue="Profile Info"
              >
                <div className="font-semibold text-ink-950 text-base">
                  {oauthUserName || staffData?.name || "User"}
                </div>
                <div className="text-xs text-ink-500 truncate max-w-[180px]">
                  {isStaffUser
                    ? `Staff - ${staffData?.role}`
                    : oauthUserEmail || address}
                </div>
              </DropdownItem>
            </DropdownSection>

            <DropdownSection showDivider className="md:hidden">
              <DropdownItem
                key="home"
                as={Link}
                href="/"
                startContent={<House size={18} />}
              >
                {labels.home}
              </DropdownItem>
              {
                visibleAuthLinks.map((link) => (
                  <DropdownItem
                    key={link.name}
                    as={Link}
                    href={link.href}
                    startContent={<span className="text-lg">{link.icon}</span>}
                  >
                    {link.name}
                  </DropdownItem>
                )) as unknown as ReactElement<any>
              }
            </DropdownSection>

            {showInstallAction ? (
              <DropdownSection showDivider>
                <DropdownItem
                  key="install-payverge"
                  startContent={<Download size={18} />}
                  isDisabled={installPending}
                  aria-busy={installPending}
                  onPress={() => void handleInstall()}
                >
                  {String(getTranslation("pwa.account.label", currentLocale))}
                </DropdownItem>
              </DropdownSection>
            ) : null}

            <DropdownSection showDivider>
              <DropdownItem
                key="account"
                as={Link}
                href="/account"
                startContent={<User size={18} />}
              >
                {tString("account")}
              </DropdownItem>
              {showAdminInProfile ? (
                <DropdownItem
                  key="admin"
                  as={Link}
                  href="/admin"
                  startContent={<Shield size={18} />}
                >
                  {tString("admin")}
                </DropdownItem>
              ) : null}
            </DropdownSection>

            <DropdownSection>
              <DropdownItem
                key="logout"
                className="text-danger"
                color="danger"
                startContent={<LogOut size={18} />}
                onPress={() =>
                  void (isStaffUser
                    ? handleStaffLogout()
                    : isOAuthUser
                      ? handleOAuthLogout()
                      : logout())
                }
              >
                {tString("logout")}
              </DropdownItem>
            </DropdownSection>
          </DropdownMenu>
        </Dropdown>
      )}
    </>
  );

  const mobileDrawer = skipMobileDrawerExit ? null : (
    <AnimatePresence>
      {isMenuOpen && (
        <>
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            className={`fixed inset-0 bg-ink-950/30 backdrop-blur-sm z-[190] ${MOBILE_ONLY_CLASS}`}
            onClick={() => setIsMenuOpen(false)}
          />
          <motion.div
            ref={mobileDrawerRef}
            role="dialog"
            aria-modal="true"
            aria-label={labels.toggleMenuAria}
            tabIndex={-1}
            initial={{ x: "100%" }}
            animate={{ x: 0 }}
            exit={{ x: "100%" }}
            transition={{ type: "spring", damping: 25, stiffness: 200 }}
            className={`fixed top-0 right-0 bottom-0 w-[300px] max-w-[85vw] bg-white shadow-2xl z-[200] ${MOBILE_ONLY_CLASS} outline-none`}
          >
            <div className="flex flex-col h-full">
              <div className="p-5 border-b border-warm-100 flex justify-between items-center bg-gradient-to-r from-brand/5 to-white">
                <div className="flex items-center gap-3">
                  {isAuthenticated ? (
                    <>
                      {isOAuthUser && oauthUserPicture ? (
                        <Image
                          src={oauthUserPicture}
                          alt="Profile"
                          width={40}
                          height={40}
                          className="rounded-full object-cover ring-2 ring-white"
                          referrerPolicy="no-referrer"
                          unoptimized
                        />
                      ) : (
                        <div className="w-10 h-10 rounded-full bg-brand flex items-center justify-center text-white font-bold ring-2 ring-white">
                          {(
                            oauthUserName ||
                            oauthUserEmail ||
                            staffData?.name ||
                            "U"
                          )
                            ?.charAt(0)
                            .toUpperCase()}
                        </div>
                      )}
                      <div>
                        <span className="font-bold text-ink-950 block">
                          {oauthUserName || staffData?.name || "User"}
                        </span>
                        <span className="text-xs text-ink-500">
                          {isStaffUser
                            ? `Staff - ${staffData?.role}`
                            : "Account"}
                        </span>
                      </div>
                    </>
                  ) : (
                    <span className="font-bold text-xl text-ink-950">
                      {tString("profile")}
                    </span>
                  )}
                </div>
                <button
                  type="button"
                  onClick={() => setIsMenuOpen(false)}
                  className="p-2 rounded-full hover:bg-warm-100 text-ink-500 transition-colors"
                  aria-label={labels.toggleMenuAria}
                >
                  <X size={24} />
                </button>
              </div>

              <div className="flex-1 overflow-y-auto p-4">
                {!isAuthenticated && (
                  <div className="mb-6 space-y-3">
                    <button
                      type="button"
                      onClick={() => {
                        setIsMenuOpen(false);
                        setAuthModalTab("signin");
                        setIsAuthModalOpen(true);
                      }}
                      className="flex w-full items-center justify-center rounded-full bg-brand px-5 py-3 text-sm font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark transition-all h-12"
                    >
                      {labels.signIn}
                    </button>
                    <Link
                      href={localizedPublicHref(locale, "/staff/login")}
                      onClick={() => setIsMenuOpen(false)}
                      className="flex w-full items-center justify-center text-sm font-medium text-ink-700 hover:text-ink-950 py-2 transition-colors"
                    >
                      {labels.staffLogin}
                    </Link>
                  </div>
                )}

                <div className="space-y-1">
                  <p className="px-4 py-2 text-xs font-semibold text-ink-500 uppercase tracking-wider">
                    {tString("navigation")}
                  </p>
                  <Link
                    href={localizedPublicHref(locale, "/")}
                    onClick={() => setIsMenuOpen(false)}
                    className={`w-full flex items-center gap-3 px-4 py-3.5 rounded-xl text-left transition-all ${
                      pathname === "/" ||
                      pathname === localizedPublicHref(locale, "/")
                        ? "bg-brand/10 text-brand font-semibold"
                        : "text-ink-700 hover:bg-warm-50"
                    }`}
                  >
                    <House size={20} />
                    <span className="font-medium">{labels.home}</span>
                  </Link>

                  {isAuthenticated &&
                    visibleAuthLinks.map((link) => (
                      <Link
                        key={link.name}
                        href={link.href}
                        onClick={() => setIsMenuOpen(false)}
                        className={`w-full flex items-center gap-3 px-4 py-3.5 rounded-xl text-left transition-all ${
                          pathname === link.href
                            ? "bg-brand/10 text-brand font-semibold"
                            : "text-ink-700 hover:bg-warm-50"
                        }`}
                      >
                        <span className="text-xl">{link.icon}</span>
                        <span className="font-medium">{link.name}</span>
                      </Link>
                    ))}
                </div>
              </div>

              {isAuthenticated && (
                <div className="p-4 border-t border-warm-100 bg-warm-50/50 space-y-2">
                  {showInstallAction ? (
                    <button
                      type="button"
                      disabled={installPending}
                      aria-busy={installPending}
                      onClick={() => {
                        flushSync(() => {
                          setSkipMobileDrawerExit(true);
                          setIsMenuOpen(false);
                        });
                        void handleInstall();
                      }}
                      className="flex min-h-12 w-full items-center gap-3 rounded-xl px-4 py-3 text-left text-ink-700 transition-all hover:bg-warm-50 disabled:cursor-wait disabled:opacity-60"
                    >
                      <Download size={20} aria-hidden="true" />
                      <span className="font-medium">
                        {String(
                          getTranslation("pwa.account.label", currentLocale),
                        )}
                      </span>
                    </button>
                  ) : null}
                  <Link
                    href="/account"
                    onClick={() => setIsMenuOpen(false)}
                    className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl text-left transition-all ${
                      pathname === "/account"
                        ? "bg-brand/10 text-brand font-semibold"
                        : "text-ink-700 hover:bg-warm-50"
                    }`}
                  >
                    <User size={20} />
                    <span className="font-medium">{tString("account")}</span>
                  </Link>
                  {showAdminInProfile ? (
                    <Link
                      href="/admin"
                      onClick={() => setIsMenuOpen(false)}
                      className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl text-left transition-all ${
                        pathname === "/admin" || pathname?.startsWith("/admin/")
                          ? "bg-brand/10 text-brand font-semibold"
                          : "text-ink-700 hover:bg-warm-50"
                      }`}
                    >
                      <Shield size={20} />
                      <span className="font-medium">{tString("admin")}</span>
                    </Link>
                  ) : null}
                  <Button
                    variant="flat"
                    color="danger"
                    className="w-full justify-start font-medium h-12"
                    startContent={<LogOut size={20} />}
                    onPress={() => {
                      if (isStaffUser) void handleStaffLogout();
                      else if (isOAuthUser) void handleOAuthLogout();
                      else void logout();
                      setIsMenuOpen(false);
                    }}
                  >
                    {tString("logout")}
                  </Button>
                </div>
              )}
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );

  return (
    <>
      <TopMenuShell
        pathname={pathname}
        scrolled={scrolled}
        labels={labels}
        showPublicNav={showPublicNav}
        centerSlot={operatorAccessError ? undefined : desktopCenterNav}
        rightSlot={rightRail}
        authLoading={authRailPending}
        isMenuOpen={operatorAccessError ? false : isMenuOpen}
        onToggleMenu={
          operatorAccessError
            ? undefined
            : () => {
                if (!isMenuOpen) setSkipMobileDrawerExit(false);
                setIsMenuOpen(!isMenuOpen);
              }
        }
        mobileDrawer={operatorAccessError ? null : mobileDrawer}
      />

      <AuthModal
        isOpen={isAuthModalOpen}
        onClose={() => setIsAuthModalOpen(false)}
        defaultTab={authModalTab}
      />
    </>
  );
};
