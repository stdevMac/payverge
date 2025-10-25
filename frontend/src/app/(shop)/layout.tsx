"use client";
import { useState } from "react";
import { TopMenu } from "@/components";
import { UserProvider } from "@/providers/UserProvider";
import SimpleLanguageSwitcher from "@/components/SimpleLanguageSwitcher";
import { TestnetFaucetBanner } from "@/components/TestnetFaucetBanner";
import { FaucetModal } from "@/components/FaucetModal";

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const [isFaucetModalOpen, setIsFaucetModalOpen] = useState(false);

  const handleFaucetClick = () => {
    setIsFaucetModalOpen(true);
  };

  const handleFaucetClose = () => {
    setIsFaucetModalOpen(false);
  };

  return (
    <UserProvider>
      <main className="min-h-screen flex flex-col bg-white dark:bg-zinc-950 transition-colors duration-200">
        <TopMenu />
        <TestnetFaucetBanner onFaucetClick={handleFaucetClick} />
        <div className="flex-1">{children}</div>
        <div className="fixed bottom-6 right-6 z-50">
          <SimpleLanguageSwitcher />
        </div>
        <FaucetModal 
          isOpen={isFaucetModalOpen} 
          onClose={handleFaucetClose} 
        />
      </main>
    </UserProvider>
  );
}
