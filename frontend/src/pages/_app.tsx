import type { AppProps } from "next/app";
import "@/app/globals.css";
import { inter, poppins, titleFont } from "@/config";

export default function App({ Component, pageProps }: AppProps) {
  return (
    <div
      className={`${inter.variable} ${poppins.variable} ${titleFont.variable} font-sans antialiased`}
    >
      <Component {...pageProps} />
    </div>
  );
}
