import type { Metadata } from "next";

import { Footer, Sidebar, TopMenu } from "@/components";
import { UserProvider } from "@/providers/UserProvider";
import { ShareBar } from "@/components/shared/ShareBar";
import { OnboardingWrapper } from "@/components/onboarding/OnboardingWrapper";
import { TranslationProvider } from "@/i18n/TranslationContext";
import { LanguageProvider } from "@/i18n/useLanguage";
import FloatingLanguageSwitcher from "@/components/FloatingLanguageSwitcher";

export const metadata: Metadata = {
  title: "Base App Builder",
  description:
    "Base App Builder",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <LanguageProvider>
      <TranslationProvider>
        <UserProvider>
          <main className="min-h-screen flex flex-col bg-white dark:bg-zinc-950 transition-colors duration-200">
            <TopMenu />
            {/* {getNetwork().id === baseSepolia.id && <InvestmentBanner />} */}
            <Sidebar />
            <div className="px-5 sm:px-10 flex-1">{children}</div>
            <Footer />
            <OnboardingWrapper />
            <FloatingLanguageSwitcher />
            {/* <ShareBar /> */}
          </main>
        </UserProvider>
      </TranslationProvider>
    </LanguageProvider>
  );
}
