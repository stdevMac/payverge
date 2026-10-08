import Web3ModalProvider from "@/context";
import ShellChrome from "./_components/ShellChrome";

// HybridAuthProvider + AuthGate live at the root layout level (app/providers.tsx)
// so session state persists across route-group navigation. This layout owns
// chrome only. The marketing top menu + footer are conditionally rendered by
// ShellChrome so they don't leak into the operator dashboard (/business/<id>/
// dashboard) or /admin views.
//
// The operator wallet stack (WagmiProvider + WalletBridge) mounts here, not in
// the root layout: the wallet button, operator dashboard and payment plugin
// settings live under (shop), and diner routes must not ship wagmi/viem.
export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <main
      id="main-content"
      tabIndex={-1}
      className="min-h-screen flex flex-col bg-white transition-colors duration-200"
    >
      <Web3ModalProvider>
        <ShellChrome>{children}</ShellChrome>
      </Web3ModalProvider>
    </main>
  );
}
