"use client";
import { StoreMenu } from "@/store";
import clsx from "clsx";
import Link from "next/link";
import { useLogout } from "@/hooks";
import {
  IoCloseOutline,
  IoCogOutline,
  IoLogoReact,
  IoLogOutOutline,
  IoPersonOutline,
  IoSearchOutline,
  IoPodiumOutline,
} from "react-icons/io5";
import { useAccount } from "wagmi";
import { useUserStore } from "@/store/useUserStore";
import { Web3Button } from "@/components/ui/web3-button/Web3Button";
import { useState, useEffect } from "react"; // Import useState and useEffect
import { isAdmin } from "@/utils/auth";

export const Sidebar = () => {
  const { address } = useAccount();
  const { user } = useUserStore();
  const isSideMenuOpen = StoreMenu((state) => state.isSideMenuOpen);
  const closeMenu = StoreMenu((state) => state.closeSideMenu);
  const { logout } = useLogout();
  const [mounted, setMounted] = useState(false); // Add mounted state

  useEffect(() => {
    setMounted(true); // Set mounted to true when the component mounts
  }, []);

  return (
    <div>
      {isSideMenuOpen && (
        <>
          {/* Background overlay */}
          <div
            onClick={() => closeMenu()}
            className="fixed top-0 left-0 w-screen h-screen z-10 bg-black opacity-30"
          />
        </>
      )}

      {/* SideMenu */}
      <nav
        className={clsx(
          "fixed top-0 h-screen bg-white z-20 shadow-2xl transform transition-all duration-300",
          "p-3 sm:p-5",
          {
            "translate-x-full": !isSideMenuOpen,
          },
          "w-[85vw] sm:w-[70vw] md:w-[500px]", // Responsive width
          "right-0" // Keep it on the right side
        )}
      >
        {/* Close Button */}
        <IoCloseOutline
          size={32}
          className="absolute top-3 right-3 sm:top-5 sm:right-5 cursor-pointer hover:text-blue-500"
          onClick={() => closeMenu()}
        />

        {/* Menu Items Container */}
        <div className="mt-12 sm:mt-16">
          {/* Connect Wallet Button (Mobile Only) */}
          {mounted && (
            <div className="block md:hidden mb-4">
              <Web3Button />
            </div>
          )}

          {/* Profile Link */}
          {address && (
            <Link
              onClick={() => closeMenu()}
              href={`/profile`}
              className="flex items-center p-2 hover:bg-gradient-to-r from-blue-200 to-transparent rounded transition-all"
            >
              <IoPersonOutline size={24} className="sm:text-3xl" />
              <span className="ml-3 text-sm sm:text-base md:text-xl">
                Profile
              </span>
            </Link>
          )}

          {/* Conditional Admin Link */}
          {isAdmin(user) && (
            <Link
              onClick={() => closeMenu()}
              href="/admin"
              className="flex items-center mt-4 p-2 hover:bg-gradient-to-r from-blue-200 to-transparent rounded transition-all"
            >
              <IoLogoReact size={24} className="sm:text-3xl" />
              <span className="ml-3 text-sm sm:text-base md:text-xl">
                Admin
              </span>
            </Link>
          )}


          {/* Logout Button */}
          <button
            onClick={() => {
              closeMenu();
              logout();
            }}
            className="flex items-center mt-4 p-2 hover:bg-gradient-to-r from-blue-200 to-transparent rounded transition-all w-full"
          >
            <IoLogOutOutline size={24} className="sm:text-3xl" />
            <span className="ml-3 text-sm sm:text-base md:text-xl">Logout</span>
          </button>
        </div>
      </nav>
    </div>
  );
};
