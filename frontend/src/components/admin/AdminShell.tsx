"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Button } from "@nextui-org/react";
import { Menu, X, ExternalLink } from "lucide-react";
import { useState } from "react";
import { titleFont } from "@/config";
import { ADMIN_NAV_GROUPS, findAdminNavItem } from "@/config/adminNav";
import SimpleLanguageSwitcherLazy from "@/components/SimpleLanguageSwitcherLazy";

export function AdminShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false);
  const current = findAdminNavItem(pathname ?? "");

  const nav = (
    <nav className="flex flex-col gap-6" aria-label="Admin navigation">
      {ADMIN_NAV_GROUPS.map((group) => (
        <div key={group.key}>
          <p className="px-3 mb-2 text-[11px] font-semibold uppercase tracking-wider text-ink-500">
            {group.label}
          </p>
          <ul className="space-y-0.5">
            {group.items.map((item) => {
              const active =
                item.href === "/admin"
                  ? pathname === "/admin"
                  : pathname?.startsWith(item.href);
              const Icon = item.icon;
              return (
                <li key={item.key}>
                  <Link
                    href={item.href}
                    onClick={() => setMobileOpen(false)}
                    className={`flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors ${
                      active
                        ? "bg-brand/10 text-brand font-medium"
                        : "text-ink-600 hover:bg-warm-100 hover:text-ink-900"
                    }`}
                    aria-current={active ? "page" : undefined}
                  >
                    <Icon size={18} className="shrink-0" />
                    <span>{item.label}</span>
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </nav>
  );

  return (
    <div className="min-h-screen bg-warm-50">
      <div className="fixed right-16 top-3 z-50 lg:right-6 lg:top-4">
        <SimpleLanguageSwitcherLazy />
      </div>
      {/* Mobile header */}
      <header className="sticky top-0 z-40 flex items-center justify-between border-b border-warm-200 bg-white/90 px-4 py-3 backdrop-blur-md lg:hidden">
        <Link href="/admin" className={`${titleFont} text-xl font-bold text-ink-900`}>
          Payverge Admin
        </Link>
        <Button
          isIconOnly
          variant="light"
          aria-label={mobileOpen ? "Close menu" : "Open menu"}
          onPress={() => setMobileOpen((open) => !open)}
        >
          {mobileOpen ? <X size={20} /> : <Menu size={20} />}
        </Button>
      </header>

      {mobileOpen && (
        <div
          className="fixed inset-0 z-30 bg-ink-900/30 lg:hidden"
          onClick={() => setMobileOpen(false)}
          aria-hidden
        />
      )}

      <div className="mx-auto flex w-full max-w-[1600px]">
        {/* Sidebar */}
        <aside
          className={`fixed inset-y-0 left-0 z-40 w-64 shrink-0 border-r border-warm-200 bg-white px-4 py-6 transition-transform lg:static lg:translate-x-0 ${
            mobileOpen ? "translate-x-0" : "-translate-x-full"
          }`}
        >
          <div className="mb-8 hidden lg:block">
            <Link href="/admin" className="block">
              <span className={`${titleFont} text-xl font-bold text-ink-900`}>
                Payverge
              </span>
              <span className="mt-0.5 block text-xs font-medium text-brand">
                Command Center
              </span>
            </Link>
          </div>

          {nav}

          <div className="mt-8 border-t border-warm-200 pt-4">
            <Link
              href="/dashboard"
              className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-ink-500 hover:bg-warm-100 hover:text-ink-800"
            >
              <ExternalLink size={16} />
              Operator dashboard
            </Link>
          </div>
        </aside>

        {/* Main content */}
        <div className="min-w-0 flex-1">
          <div className="hidden border-b border-warm-200 bg-white/80 px-6 py-4 backdrop-blur-md lg:block">
            <h1 className="font-serif text-2xl font-bold text-ink-900">
              {current?.label ?? "Admin"}
            </h1>
            {current?.description ? (
              <p className="mt-0.5 text-sm text-ink-500">{current.description}</p>
            ) : null}
          </div>
          <div className="px-4 py-6 sm:px-6 lg:px-8">{children}</div>
        </div>
      </div>
    </div>
  );
}
