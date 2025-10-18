"use client";

import { titleFont } from "@/config";
import { StoreMenu } from "@/store";
import {
  Button,
  Dropdown,
  DropdownTrigger,
  DropdownMenu,
  DropdownItem,
} from "@nextui-org/react";
import React from "react";
import Link from "next/link";
import { useAccount } from "wagmi";
import { IoMenu } from "react-icons/io5";
import { usePathname } from "next/navigation";
import { IoMdArrowDropdown } from "react-icons/io";
import { MdDashboard, MdDirectionsCar, MdPeople, MdCode, MdCardGiftcard } from "react-icons/md";
import { IoPersonOutline } from "react-icons/io5";
import { useUserStore } from "@/store/useUserStore";
import { isAdmin } from "@/utils/auth";

const adminRoutes = [
  {
    key: "dashboard",
    label: "Dashboard",
    href: "/admin",
    icon: MdDashboard,
  },
  {
    key: "management",
    label: "Admin Management",
    href: "/admin/management",
    icon: MdDashboard,
  },
  {
    key: "users",
    label: "Users Management",
    href: "/admin/usersmanagement",
    icon: MdPeople,
  },
  {
    key: "codes",
    label: "Codes Management",
    href: "/admin/codesmanagement",
    icon: MdCardGiftcard,
  }
];

export const TopMenuMain = () => {
  const { isConnected } = useAccount();
  const openMenu = StoreMenu((store) => store.openSideMenu);
  const pathname = usePathname();
  const isAdminRoute = pathname?.startsWith("/admin");
  const { user } = useUserStore();
  const userIsAdmin = isAdmin(user);

  // Get current route info
  const currentRoute = adminRoutes.find((route) => pathname === route.href);
  const availableRoutes = adminRoutes.filter(
    (route) => pathname !== route.href,
  );

  return (
    <nav className="flex px-4 py-2 items-center justify-between w-full border-b bg-white/80 backdrop-blur-md sticky top-0 z-50">
      {/* Logo and Main Page Link */}
      <div>
        <Link href="/">
          <span className={`${titleFont} antialiased font-bold text-2xl`}>
            Payverge
          </span>
        </Link>
      </div>

      {/* Menu Options */}
      <div className="flex items-center gap-2">
        {userIsAdmin && (
          <Dropdown>
            <DropdownTrigger>
              <Button
                variant={isAdminRoute ? "solid" : "light"}
                color={isAdminRoute ? "primary" : "default"}
                className="font-medium"
                endContent={<IoMdArrowDropdown className="text-xl" />}
                startContent={
                  currentRoute?.icon ? (
                    <currentRoute.icon className="text-xl" />
                  ) : (
                    <MdDashboard className="text-xl" />
                  )
                }
              >
                {currentRoute?.label || "Admin Panel"}
              </Button>
            </DropdownTrigger>
            <DropdownMenu
              aria-label="Admin actions"
              variant="flat"
              className="w-[200px]"
            >
              {availableRoutes.map((route) => (
                <DropdownItem
                  key={route.key}
                  startContent={<route.icon className="text-xl" />}
                  as={Link}
                  href={route.href}
                  className="py-2"
                >
                  {route.label}
                </DropdownItem>
              ))}
            </DropdownMenu>
          </Dropdown>
        )}

        <div className="flex items-center gap-2 ml-2">
          <appkit-button balance={"hide"} />
              {isConnected && (
                  <>
                    {userIsAdmin ? (
                        <Button
                            variant="light"
                            isIconOnly
                            onClick={() => openMenu()}
                            className="text-xl"
                        >
                          <IoMenu/>
                        </Button>
                    ) : (
                        <Link href="/profile">
                          <Button
                              variant="light"
                              isIconOnly
                              className="text-xl"
                          >
                            <IoPersonOutline/>
                          </Button>
                        </Link>
                    )}
                  </>
              )}
        </div>
      </div>
    </nav>
  );
};
