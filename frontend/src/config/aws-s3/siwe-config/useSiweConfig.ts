// hooks/useSiweConfig.ts
import {
    createSIWEConfig,
    SIWECreateMessageArgs,
    SIWEVerifyMessageArgs,
} from "@reown/appkit-siwe";
import { setCookie, getCookie } from "@/config/aws-s3/cookie-management/store.helpers";
import { getCsrfToken, getSession, signIn, signOutFromSession } from "@/api";
import { getNetworkId } from "@/config/network";
import { emitSiweVerified } from "./siweEvents";
import { destroyCookie } from "nookies";
import { decodeJwt, isTokenValid } from "@/utils/jwt";

const chains = [getNetworkId()];

export const siweConfig = () => {
    return createSIWEConfig({
        getMessageParams: async () => ({
            domain: typeof window !== "undefined" ? window.location.host : "",
            uri: typeof window !== "undefined" ? window.location.origin : "",
            chains,
            statement:
                "Please sign with your account to make sure this wallet is yours",
        }),
        createMessage: ({ address, nonce, chainId }: SIWECreateMessageArgs) => {
            const domain = window.location.host;
            const uri = window.location.origin;
            const now = new Date();
            const timestamp = now.toLocaleDateString('en-US', { 
                month: 'short',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit'
            });

            // Extract only the wallet address part
            const cleanAddress = address.includes(':') 
                ? address.split(':').pop() 
                : address;
            
            return `🍽️ Welcome to Payverge!

Sign to verify your wallet ownership and authenticate with our platform. No gas fees required.

🔐 Authentication Details:
Wallet: ${cleanAddress}
Verification: ${nonce}
Network: Base #${chainId}
Time: ${timestamp}

By signing, you accept Payverge's terms of service and confirm ownership of this wallet.`;
        },
        getNonce: async (address) => {
            const nonce = await getCsrfToken({ address });
            if (!nonce) throw new Error("Failed to get nonce!");
            return nonce;
        },
        getSession: async () => {
            // First check for local session token
            const token = getCookie("session_token");
            if (!token) {
                return null;
            }

            // Validate token using utility function
            if (!isTokenValid(token)) {
                // Token is expired or invalid, clear it
                destroyCookie(null, "session_token", { path: "/" });
                destroyCookie(null, "token", { path: "/" });
                destroyCookie(null, "persist-web3-login", { path: "/" });
                return null;
            }

            try {
                // Token is valid, decode and return session data
                const tokenData = decodeJwt(token);
                
                // Ensure we have a valid address
                if (!tokenData.address) {
                    throw new Error("No address in token");
                }
                
                return {
                    address: tokenData.address,
                    chainId: tokenData.chainId || chains[0]
                };
            } catch (error) {
                console.error("Error decoding session token:", error);
                // If token is malformed, clear it
                destroyCookie(null, "session_token", { path: "/" });
                destroyCookie(null, "token", { path: "/" });
                destroyCookie(null, "persist-web3-login", { path: "/" });
                return null;
            }
        },
        verifyMessage: async ({
            message,
            signature,
        }: SIWEVerifyMessageArgs) => {
            try {
                const response = await signIn({ message, signature });
                if (response.data.success) {
                    const token = response.data.token;
                    // Set the session token without quotes - they'll be added in the axios interceptor
                    setCookie("session_token", token, 1);
                    // Emit event after successful verification
                    emitSiweVerified();
                    // Return the token so the session is immediately available
                    return token;
                } else {
                    console.error(
                        "SIWE verification failed: Server returned success: false",
                    );
                    return "";
                }
            } catch (error) {
                console.error("SIWE verification failed:", error);
                return "";
            }
        },
        signOut: async () => {
            try {
                await signOutFromSession();
                destroyCookie(null, "token", { path: "/" });
                destroyCookie(null, "session_token", { path: "/" });
                destroyCookie(null, "persist-web3-login", { path: "/" });
                return true;
            } catch (error) {
                console.error("SIWE signout failed:", error);
                // Still clear cookies even if signout fails
                destroyCookie(null, "token", { path: "/" });
                destroyCookie(null, "session_token", { path: "/" });
                destroyCookie(null, "persist-web3-login", { path: "/" });
                return false;
            }
        },
    });
};
