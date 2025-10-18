// RootLayout.tsx
import {config, titleFont} from "@/config";
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
import { SimpleTranslationProvider } from '@/i18n/SimpleTranslationProvider';

export const metadata: Metadata = {
  title: "Payverge",
  description:
    "Payverge, the hospitality solution for the crypto space",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const initialState = cookieToInitialState(config.wagmiConfig, headers().get("cookie"));
  
  // Get language from cookie or default to 'en'
  const cookies = headers().get('cookie') || '';
  const langCookie = cookies.split(';').find(c => c.trim().startsWith('locale='));
  const lang = langCookie ? langCookie.split('=')[1] : 'en';

  return (
    <html lang={lang}>
      <body className={titleFont.className} suppressHydrationWarning={true}>
        {/* <WithMaintenance>
          <WithComingSoon> */}
            <Toaster position="top-right" />
            <SimpleTranslationProvider>
              <StoreContextProvider>
                <Web3ModalProvider initialState={initialState}>
                  <Providers>{children}</Providers>
                </Web3ModalProvider>
              </StoreContextProvider>
            </SimpleTranslationProvider>
          {/* </WithComingSoon>
        </WithMaintenance> */}
      </body>
    </html>
  );
}
