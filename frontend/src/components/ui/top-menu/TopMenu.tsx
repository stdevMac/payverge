"use client";
import { useEffect, useState } from "react";
import Image from "next/image";
import { useAccount, useConnect, useDisconnect } from "wagmi";
import { StoreMenu } from "@/store";
import { Button, Spinner, Dropdown, DropdownTrigger, DropdownMenu, DropdownItem } from "@nextui-org/react";
import { IoMenu, IoPersonOutline, IoHomeOutline, IoLogOutOutline, IoLogoReact } from "react-icons/io5";
import { Web3Button } from "@/components/ui/web3-button/Web3Button";
import { useUserStore } from "@/store/useUserStore";
import { isAdmin } from "@/utils/auth";
import { usePathname } from "next/navigation";
import { useRouter } from "next/navigation";
import { useLogout } from "@/hooks";
import LanguageSwitcher from "@/components/LanguageSwitcher";
import { useTranslation } from "@/i18n/useTranslation";

export const TopMenu = () => {
  const { isConnected, status, address } = useAccount();
  const { connect, connectors } = useConnect();
  const { disconnect } = useDisconnect();
  const openMenu = StoreMenu((store) => store.openSideMenu);
  const [mounted, setMounted] = useState(false);
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const [isInitialized, setIsInitialized] = useState(false);
  const { user } = useUserStore();
  const userIsAdmin = isAdmin(user);
  const pathname = usePathname();
  const isProfilePage = pathname === "/profile";
  const isHomePage = pathname === "/";
  const { logout } = useLogout();
  const router = useRouter();
  const { t } = useTranslation();

  useEffect(() => {
    setMounted(true);

    // Attempt to restore the previous connection
    const restorePreviousConnection = async () => {
      try {
        const lastUsedConnector = localStorage.getItem("lastUsedConnector");

        if (lastUsedConnector && !isConnected) {
          const connector = connectors.find((c) => c.id === lastUsedConnector);
          if (connector) {
            await connect({ connector });
          }
        }
      } catch (error) {
        console.error("Error restoring connection:", error);
      } finally {
        // Set initialized after connection attempt, regardless of result
        setIsInitialized(true);
      }
    };

    restorePreviousConnection();
  }, [connect, connectors, isConnected]);

  // Save the last used connector
  useEffect(() => {
    if (isConnected) {
      const activeConnector = connectors.find(
        (c) => c.uid === localStorage.getItem("wagmi.wallet"),
      );
      if (activeConnector) {
        localStorage.setItem("lastUsedConnector", activeConnector.id);
      }
    }
  }, [isConnected, connectors]);

  // Watch for wallet changes
  useEffect(() => {
    const handleWalletChange = () => {
      window.location.reload();
    };

    window.addEventListener("wagmi:wallet", handleWalletChange);

    return () => {
      window.removeEventListener("wagmi:wallet", handleWalletChange);
    };
  }, []);

  if (!mounted) return null; // Don't render anything on the server

  // Show loading state while initializing
  if (!isInitialized || status === 'reconnecting') {
    return (
      <nav className="relative flex items-center justify-between px-2 sm:px-5 py-2 shadow-md bg-white/80 backdrop-blur-md z-[100]">
        <div className="flex items-center">
          <div
            className="cursor-pointer"
            onClick={() => router.push("/")}
          >
            <div className="relative w-20 h-6 sm:w-32 sm:h-8 md:w-40 md:h-10">
              <Image
                src="/images/TokenFleetLogo-01.svg"
                alt={t('app.name', {}, 'Token Fleet')}
                fill
                sizes="(max-width: 640px) 80px, (max-width: 768px) 128px, 160px"
                className="object-contain"
                priority
              />
            </div>
          </div>
        </div>
        <div className="h-[40px] flex items-center justify-center px-4">
          <Spinner size="sm" />
        </div>
      </nav>
    );
  }

  return (
    <nav className="relative flex items-center justify-between px-2 sm:px-5 py-2 shadow-md bg-white/80 backdrop-blur-md z-[100]">
      {/* Logo */}
      <div className="flex items-center">
        <div
          className="cursor-pointer"
          onClick={() => router.push("/")}
        >
          <div className="relative w-20 h-6 sm:w-32 sm:h-8 md:w-40 md:h-10">
            <Image
              src="/images/TokenFleetLogo-01.svg"
              alt="Token Fleet"
              fill
              sizes="(max-width: 640px) 80px, (max-width: 768px) 128px, 160px"
              className="object-contain"
              priority
            />
          </div>
        </div>
      </div>

      {/* Right Side Menu */}
      <div className="flex items-center gap-1 sm:gap-2">
        {/* Language Switcher - hidden on mobile */}
        <div className="hidden md:block">
          <LanguageSwitcher />
        </div>
        
        {/* Web3 button - Always visible */}
        <Web3Button />

        {/* Menu Toggle for Mobile */}
        {isConnected && (
          <div className="flex md:hidden">
            <Button
              isIconOnly
              variant="light"
              onClick={() => setIsMenuOpen(!isMenuOpen)}
              className="focus:outline-none"
              aria-label={t('navigation.toggleMenu')}
            >
              {userIsAdmin ? (
                <IoMenu size={22} />
              ) : isProfilePage ? (
                <IoHomeOutline size={22} />
              ) : (
                <IoPersonOutline size={22} />
              )}
            </Button>
          </div>
        )}

        {/* Desktop Menu Items */}
        {isConnected && (
          <div className="hidden md:flex items-center gap-2">
            {address && (
              <Dropdown>
                <DropdownTrigger>
                  <Button
                    variant="flat"
                    color="primary"
                    className="font-medium"
                    startContent={<IoPersonOutline className="text-xl" />}
                  >
                    {t('navigation.profile')}
                  </Button>
                </DropdownTrigger>
                <DropdownMenu aria-label={t('navigation.profileActions')}>
                  <DropdownItem
                    key="home"
                    startContent={<IoHomeOutline className="text-xl" />}
                    onClick={() => router.push("/")}
                    isDisabled={isHomePage}
                  >
                    {t('navigation.home')}
                  </DropdownItem>
                  <DropdownItem
                    key="profile"
                    startContent={<IoPersonOutline className="text-xl" />}
                    onClick={() => router.push("/profile")}
                  >
                    {t('profile.myProfile')}
                  </DropdownItem>
                  <DropdownItem
                    key="admin"
                    startContent={<IoLogoReact className="text-xl" />}
                    onClick={() => router.push("/admin")}
                    className={userIsAdmin ? "" : "hidden"}
                  >
                    {t('navigation.admin')}
                  </DropdownItem>
                  <DropdownItem
                    key="logout"
                    className="text-danger"
                    color="danger"
                    startContent={<IoLogOutOutline className="text-xl" />}
                    onClick={() => logout()}
                  >
                    {t('navigation.logout')}
                  </DropdownItem>
                </DropdownMenu>
              </Dropdown>
            )}
          </div>
        )}

        {/* Mobile Menu Dropdown */}
        {isConnected && isMenuOpen && (
          <div className="fixed top-[52px] right-0 left-0 md:hidden bg-white shadow-lg z-[999]">
            <div className="p-4 flex flex-col gap-3">
              {address && (
                <>
                  <Button
                    variant="flat"
                    color="primary"
                    className="w-full font-medium justify-start"
                    startContent={<IoHomeOutline className="text-xl" />}
                    isDisabled={isHomePage}
                    onClick={() => {
                      setIsMenuOpen(false);
                      router.push("/");
                    }}
                  >
                    {t('navigation.home')}
                  </Button>

                  <Button
                    variant="flat"
                    color="primary"
                    className="w-full font-medium justify-start"
                    startContent={<IoPersonOutline className="text-xl" />}
                    onClick={() => {
                      setIsMenuOpen(false);
                      router.push("/profile");
                    }}
                  >
                    {t('profile.myProfile')}
                  </Button>

                  {userIsAdmin && (
                    <Button
                      variant="flat"
                      color="primary"
                      className="w-full font-medium justify-start"
                      startContent={<IoLogoReact className="text-xl" />}
                      onClick={() => {
                        setIsMenuOpen(false);
                        router.push("/admin");
                      }}
                    >
                      {t('navigation.admin')}
                    </Button>
                  )}
                </>
              )}

              <Button
                variant="flat"
                color="danger"
                className="w-full font-medium justify-start"
                startContent={<IoLogOutOutline className="text-xl" />}
                onClick={() => {
                  logout();
                  setIsMenuOpen(false);
                }}
              >
                {t('navigation.logout')}
              </Button>
              
              {/* Mobile Language Switcher removed - now using FloatingLanguageSwitcher */}
            </div>
          </div>
        )}
      </div>
    </nav>
  );
};
