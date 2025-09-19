// RootLayout.tsx
import { config, titleFont } from "@/config";
import { Providers } from "./providers";
import type { Metadata } from "next";
import "./globals.css";
import { StoreContextProvider } from "@/store/store";

import { headers } from "next/headers";
import { cookieToInitialState } from "wagmi";
import Web3ModalProvider from "@/context";
import { Toaster } from "react-hot-toast";
import WithMaintenance from "@/components/WithMaintenance";
import WithComingSoon from "@/components/WithComingSoon";
import { LanguageProvider } from "@/i18n/useLanguage";
import { TranslationProvider } from "@/i18n/TranslationContext";
import { UserProvider } from "@/providers/UserProvider";
import { TopMenu } from "@/components";

export const metadata: Metadata = {
  title: "Payverge",
  description:
    "Payverge, the future of crypto payments for hospitality!",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const initialState = cookieToInitialState(config.wagmiConfig, headers().get("cookie"));

  // Get language from cookie or default to 'en'
  const cookies = headers().get('cookie') || '';
  const langCookie = cookies.split(';').find(c => c.trim().startsWith('NEXT_LOCALE='));
  const lang = langCookie ? langCookie.split('=')[1] : 'en';

  return (
    <html lang={lang}>
      <body className={titleFont.className} suppressHydrationWarning={true}>
        {/* <WithMaintenance>
          <WithComingSoon> */}
        <Toaster position="top-right" />
        <StoreContextProvider>
          <Web3ModalProvider initialState={initialState}>
            <Providers>
              <LanguageProvider>
                <TranslationProvider>
                  <UserProvider>
                    <main className="min-h-screen flex flex-col bg-white dark:bg-zinc-950 transition-colors duration-200">
                      <TopMenu />
                      {/* {getNetwork().id === baseSepolia.id && <InvestmentBanner />} */}
                      {/* <Sidebar /> */}
                      <div className="px-5 sm:px-10 flex-1">{children}</div>
                      {/* <Footer /> */}
                      {/* <OnboardingWrapper /> */}
                      {/* <FloatingLanguageSwitcher /> */}
                      {/* <ShareBar /> */}
                    </main>
                  </UserProvider>
                </TranslationProvider>
              </LanguageProvider>
            </Providers>
          </Web3ModalProvider>
        </StoreContextProvider>
        {/* </WithComingSoon>
        </WithMaintenance> */}
      </body>
    </html>
  );
}
