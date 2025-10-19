import type { Metadata } from "next";

import { TopMenu } from "@/components";
import { UserProvider } from "@/providers/UserProvider";
import SimpleLanguageSwitcher from "@/components/SimpleLanguageSwitcher";

export const metadata: Metadata = {
  title: "Payverge",
  description:
    "Payverge, the future of crypto payments for hospitality!",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <UserProvider>
      <main className="min-h-screen flex flex-col bg-white dark:bg-zinc-950 transition-colors duration-200">
        <TopMenu />
        {/* {getNetwork().id === baseSepolia.id && <InvestmentBanner />} */}
        {/* <Sidebar /> */}
        <div className="flex-1">{children}</div>
        {/* <Footer /> */}
        {/* <OnboardingWrapper /> */}
        <div className="fixed bottom-6 right-6 z-50">
          <SimpleLanguageSwitcher />
        </div>
        {/* <ShareBar /> */}
      </main>
    </UserProvider>
  );
}
