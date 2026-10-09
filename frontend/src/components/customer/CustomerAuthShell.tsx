"use client";

import { CustomerAuthProvider } from "@/contexts/CustomerAuthContext";

export default function CustomerAuthShell({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return <CustomerAuthProvider>{children}</CustomerAuthProvider>;
}
