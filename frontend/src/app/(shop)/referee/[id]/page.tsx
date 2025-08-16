"use client";
import React, { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAccount } from "wagmi";
import { Button, Divider, Spinner } from "@nextui-org/react";
import { set_referrer } from "@/api/users/updateReferal";
import { parseCookies } from "nookies";
import axios from "axios";
import { useUser } from "@/hooks/useUser";
import { titleFont } from "@/config/font/font";
import { PrimarySpinner } from "@/components";
import { useTranslation } from "@/i18n/useTranslation";

// Web3Modal Component
const Web3Button = () => {
  return <appkit-button label="Connect Wallet" balance="hide" />;
};

interface Props {
  params: {
    id: string;
  };
}

export default function RefereeProfile({ params }: Props) {
  const { t } = useTranslation();
  const { id } = params;
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const { address, isConnected } = useAccount();
  const [mounted, setMounted] = useState(false);
  const router = useRouter();
  const { user, isLoading, isAuthenticated } = useUser();
  const [isConnecting, setIsConnecting] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  const handleReferralUpdate = useCallback(
    async (connectedAddress: string) => {
      setIsConnecting(true);
      try {
        if (!isAuthenticated) {
          throw new Error("Not authenticated");
        }

        const response = await set_referrer({
          address: connectedAddress,
          referrer_code: id,
        });

        if (response.success) {
          setSuccessMessage(response.message);
          setErrorMessage("");
        } else {
          setErrorMessage(response.message);
          setSuccessMessage("");
        }
      } catch (error) {
        console.error("Error during referral update:", error);
        setErrorMessage("An unexpected error occurred. Please try again.");
        setSuccessMessage("");
      } finally {
        setIsConnecting(false);
      }
    },
    [isAuthenticated, id],
  );

  useEffect(() => {
    if (isAuthenticated && address && !isLoading) {
      handleReferralUpdate(address).catch(console.error);
    }
  }, [isAuthenticated, address, isLoading, handleReferralUpdate]);

  const handleRedirectToMain = () => {
    router.push("/");
  };

  if (!mounted) return null;

  if (isLoading) {
    return (
      <div className="flex justify-center items-center min-h-screen">
        <PrimarySpinner />
      </div>
    );
  }

  return (
    <div className="mt-5 pb-10 px-5 relative">
      {isConnecting && (
        <div className="absolute inset-0 bg-white/80 flex items-center justify-center z-10">
          <Spinner size="lg" color="primary" />
        </div>
      )}

      <h1 className={`${titleFont} antialiased font-bold text-3xl mb-4`}>
        {t('referee.title')}
      </h1>
      <Divider className="mb-12" />

      <div className="max-w-4xl mx-auto">
        {!isConnected && (
          <div className="bg-white rounded-lg p-8 shadow-sm border border-gray-200 mb-6 text-center">
            <h3 className="text-2xl font-semibold text-gray-800 mb-4">
              {t('referee.connectWallet.title')}
            </h3>
            <p className="text-gray-600 mb-2">
              {t('referee.connectWallet.description')}
            </p>
            <p className="text-sm text-gray-500 mb-6">
              {t('referee.connectWallet.referralCode')} <span className="font-medium">{id}</span>
            </p>
            <Web3Button />
          </div>
        )}

        {isConnected && !isAuthenticated && (
          <div className="bg-white rounded-lg p-8 shadow-sm border border-gray-200 mb-6 text-center">
            <h3 className="text-2xl font-semibold text-gray-800 mb-4">
              {t('referee.signIn.title')}
            </h3>
            <p className="text-gray-600 mb-2">
              {t('referee.signIn.description')}
            </p>
            <p className="text-sm text-gray-500 mb-6">
              {t('referee.signIn.referralCode')} <span className="font-medium">{id}</span>
            </p>
            <Button
              color="primary"
              onClick={() => handleReferralUpdate(address || "")}
            >
              {t('referee.signIn.buttonLabel')}
            </Button>
          </div>
        )}

        {isConnected && isAuthenticated && !successMessage && !errorMessage && (
          <div className="bg-white rounded-lg p-8 shadow-sm border border-gray-200 mb-6 text-center">
            <h3 className="text-2xl font-semibold text-gray-800 mb-4">
              {t('referee.loading.title')}
            </h3>
            <p className="text-gray-600 mb-2">
              {t('referee.loading.description')}
            </p>
            <p className="text-sm text-gray-500 mb-6">
              {t('referee.loading.referralCode')} <span className="font-medium">{id}</span>
            </p>
            <div className="flex justify-center">
              <Spinner color="primary" />
            </div>
          </div>
        )}

        {successMessage && (
          <div className="bg-white rounded-lg p-8 shadow-sm border border-gray-200 mb-6 text-center">
            <h3 className="text-2xl font-semibold text-gray-800 mb-4">
              {t('referee.success.title')}
            </h3>
            <p className="text-gray-600 mb-6">
              {t('referee.success.description')}
            </p>
            <Button color="primary" onClick={handleRedirectToMain}>
              {t('referee.success.buttonLabel')}
            </Button>
          </div>
        )}

        {errorMessage && (
          <div className="bg-white rounded-lg p-8 shadow-sm border border-gray-200 mb-6 text-center">
            <h3 className="text-2xl font-semibold text-gray-800 mb-4">
              {t('referee.error.title')}
            </h3>
            <p className="text-gray-600 mb-2">{errorMessage}</p>
            <p className="text-sm text-gray-500 mb-6">
              {t('referee.error.referralCode')} <span className="font-medium">{id}</span>
            </p>
            <Button color="primary" onClick={handleRedirectToMain}>
              {t('referee.error.buttonLabel')}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
