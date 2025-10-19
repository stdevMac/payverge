"use client";

import { TopMenuMain } from "@/components";
import { UserProvider } from "@/providers/UserProvider";
import { useUserStore } from "@/store/useUserStore";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const { user } = useUserStore();
  const router = useRouter();

  useEffect(() => {
    if (!user || !(user.role === "admin")) {
      router.push("/");
    }
  }, [user, router]);

  if (!user || !(user.role === "admin")) {
    return null;
  }

  return (
    <UserProvider>
      <main className="min-h-screen">
        <TopMenuMain />
        <div className="px-5 sm:px-10">{children}</div>
      </main>
    </UserProvider>
  );
}
