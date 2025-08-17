import React, { useCallback, useMemo, useState, useEffect } from "react";
import { UserInterface } from "@/interface";
import { useTranslation } from "@/i18n/useTranslation";
import {
  readContract,
  writeContract,
  waitForTransactionReceipt,
} from "wagmi/actions";
import { config } from "@/config";
import DealManagerABI from "@/artifacts/DealManager.json";
import {
  IoPeopleOutline,
  IoWalletOutline,
  IoTrendingUpOutline,
  IoGiftOutline,
  IoOpenOutline,
  IoDocumentTextOutline,
  IoCopyOutline,
  IoCarSportOutline,
  IoTimeOutline,
  IoCheckmarkCircleOutline,
  IoStatsChartOutline,
  IoArchiveOutline,
  IoTrophyOutline,
} from "react-icons/io5";
import { useFaucet } from "@/hooks/useFaucet";
import {
  Card,
  CardBody,
  Button,
  Chip,
  Accordion,
  AccordionItem,
} from "@nextui-org/react";
import toast from "react-hot-toast";
import Link from "next/link";
import { claimRewardsOfReferrals } from "@/api/users/claimRewards";
import { TransactionBlocker } from "@/components/common/TransactionBlocker";
import { getNetwork } from "@/config/network";

interface Props {
  user: UserInterface;
  reFetchUser?: () => Promise<void>;
}

const RefereeTab: React.FC<Props> = ({ user, reFetchUser }) => {
  const { t, language } = useTranslation();
  const [openDropdown, setOpenDropdown] = useState<{
    [key: string]: boolean;
  }>({});
  const [isLoading, setIsLoading] = useState(true);
  const [referralRewards, setReferralRewards] = useState<{
    [contractAddress: string]: number;
  }>({});
  const [isCheckingBalance, setIsCheckingBalance] = useState<boolean>(false);
  const [currentPage, setCurrentPage] = useState<{ [key: string]: number }>({});

  const handleCopyReferral = () => {
    if (user?.referral_code) {
      const referralLink = `${window.location.origin}/referee/${user.referral_code}`;
      navigator.clipboard
        .writeText(referralLink)
        .then(() => {
          toast.success(t("profile.refereeTab.referralLink.copySuccess"), {
            duration: 3000,
            position: "top-right",
            style: {
              background: "#10B981",
              color: "#fff",
            },
          });
        })
        .catch((err) => {
          console.error("Could not copy text: ", err);
          toast.error(t("profile.refereeTab.referralLink.copyError"));
        });
    }
  };

  const toggleDropdown = (contractAddress: string) => {
    setOpenDropdown((prev) => ({
      ...prev,
      [contractAddress]: !prev[contractAddress],
    }));
  };

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  };

  const formatDate = (date: string) => {
    return new Intl.DateTimeFormat("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
    }).format(new Date(date));
  };

  // Convert referees object to an array and calculate totals
  const { refereeEntries, totalReferees, totalPoints } = useMemo(() => {
    const entries = Object.entries(user.referees || {}).map(
      ([address, points]) => ({
        address,
        points,
      })
    );
    return {
      refereeEntries: entries,
      totalReferees: entries.length,
      totalPoints: entries.reduce((sum, ref) => sum + ref.points, 0),
    };
  }, [user.referees]);

  const totalRewards = useMemo(() => {
    return Object.values(referralRewards).reduce((a, b) => a + b, 0);
  }, [referralRewards]);

  const { checkAndTopUpAddress } = useFaucet();

  const checkAndTopUpEthBalance = async () => {
    if (!user.address) {
      toast.error(t("shared.errors.connectWallet"));
      return false;
    }

    setIsCheckingBalance(true);

    try {
      const result = await checkAndTopUpAddress(user.address);
      toast.dismiss();

      if (result.status === "topped_up") {
        toast.success(t("shared.investmentProcessModal.faucet.toppedUp"));
        setIsCheckingBalance(false);
        return true;
      } else if (result.status === "sufficient_balance") {
        setIsCheckingBalance(false);
        return true;
      } else if (result.status === "cooldown_period") {
        // If in cooldown period but has balance, we can still proceed
        if (result.balance) {
          setIsCheckingBalance(false);
          return true;
        } else {
          setIsCheckingBalance(false);
          return false;
        }
      } else {
        setIsCheckingBalance(false);
        return false;
      }
    } catch (err: any) {
      toast.dismiss();
      toast.error(err.message || t("shared.investmentProcessModal.faucet.requestFailed"));
      setIsCheckingBalance(false);
      return false;
    }
  };

  const handlePageChange = (contractAddress: string, page: number) => {
    setCurrentPage((prev) => ({ ...prev, [contractAddress]: page }));
  };

  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center min-h-[400px] space-y-4">
        <div
          className="spinner-border animate-spin inline-block w-8 h-8 border-4 rounded-full text-gray-600"
          role="status"
        >
          <span className="sr-only">{t("common.loading")}</span>
        </div>
        <p className="text-default-600 dark:text-default-300 transition-colors duration-200">{t("profile.refereeTab.loading")}</p>
      </div>
    );
  }

  return (
    <>
      <div className="space-y-4 sm:space-y-6">
        {/* Overview Cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
          {/* Total Referees */}
          <div className="bg-primary-50 dark:bg-primary-900/20 p-3 sm:p-4 rounded-xl border border-primary-200 dark:border-primary-800 min-h-[140px] flex flex-col justify-between transition-colors duration-200">
            <div>
              <div className="flex items-center gap-2 mb-2">
                <IoPeopleOutline className="text-lg sm:text-xl text-primary-600" />
                <span className="text-xs sm:text-sm font-medium text-primary-700 dark:text-primary-300 transition-colors duration-200">
                  {t("profile.refereeTab.overview.totalReferees")}
                </span>
              </div>
              <p className="text-xl sm:text-2xl font-bold text-primary-600 dark:text-primary-400 transition-colors duration-200">
                {totalReferees}
              </p>
            </div>
            <p className="text-xs sm:text-sm text-primary-600 dark:text-primary-400 transition-colors duration-200">
              {t("profile.refereeTab.overview.activeReferees")}
            </p>
          </div>
          {/* Total Unclaimed Rewards */}
          <div className="bg-secondary-50 dark:bg-secondary-900/20 p-3 sm:p-4 rounded-xl border border-secondary-200 dark:border-secondary-800 min-h-[140px] flex flex-col justify-between transition-colors duration-200">
            <div>
              <div className="flex items-center gap-2 mb-2">
                <IoGiftOutline className="text-lg sm:text-xl text-secondary-600" />
                <span className="text-xs sm:text-sm font-medium text-secondary-700 dark:text-secondary-300 transition-colors duration-200">
                  {t("profile.refereeTab.overview.totalUnclaimed")}
                </span>
              </div>
              <p className="text-xl sm:text-2xl font-bold text-secondary-600 dark:text-secondary-400 transition-colors duration-200">
                {formatCurrency(totalRewards)}
              </p>
            </div>
            <p className="text-xs sm:text-sm text-secondary-600 dark:text-secondary-400 transition-colors duration-200">
              {t("profile.refereeTab.overview.totalToBeClaimedDesc")}
            </p>
          </div>
        </div>

        {/* Referral Link */}
        <Card className="mb-6 bg-default-50 dark:bg-default-900/20 transition-colors duration-200">
          <CardBody className="p-3 sm:p-6">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="bg-gradient-to-r from-primary-500 to-primary-600 p-2 rounded-lg">
                  <IoGiftOutline className="text-xl text-white" />
                </div>
                <div>
                  <h5 className="text-sm font-semibold text-default-700 dark:text-default-200 transition-colors duration-200">
                    {t("profile.refereeTab.referralLink.title")}
                  </h5>
                  <p className="text-xs text-default-500 dark:text-default-400 transition-colors duration-200">
                    {t("profile.refereeTab.referralLink.description")}
                  </p>
                </div>
              </div>
              <Button
                size="sm"
                variant="flat"
                color="primary"
                onClick={handleCopyReferral}
                startContent={<IoCopyOutline className="text-lg" />}
              >
                {t("profile.refereeTab.referralLink.copyLink")}
              </Button>
            </div>
          </CardBody>
        </Card>

        {/* Referral Benefits Dropdown */}
        <div className="mt-6 bg-primary-50 dark:bg-primary-900/20 rounded-xl border border-primary-200 dark:border-primary-800 transition-colors duration-200">
          <Accordion>
            <AccordionItem
              key="benefits"
              aria-label="Referral Program Benefits"
              title={
                <div className="flex items-center gap-2 py-2">
                  <div className="p-2 rounded-lg bg-primary-100">
                    <IoGiftOutline className="text-lg sm:text-xl text-primary-600" />
                  </div>
                  <div>
                    <span className="font-semibold text-primary-700 dark:text-primary-300 transition-colors duration-200">
                      {t("profile.refereeTab.benefits.title")}
                    </span>
                    <p className="text-sm text-primary-600 dark:text-primary-400 transition-colors duration-200">
                      {t("profile.refereeTab.benefits.subtitle")}
                    </p>
                  </div>
                </div>
              }
            >
              <div className="space-y-4 py-2">
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <IoCarSportOutline className="text-primary" />
                    <span className="font-medium">
                      {t("profile.refereeTab.benefits.fleetSpecific.title")}
                    </span>
                  </div>
                  <p className="text-sm text-default-500 dark:text-default-400 transition-colors duration-200">
                    {t("profile.refereeTab.benefits.fleetSpecific.description")}
                  </p>
                </div>
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <IoWalletOutline className="text-primary" />
                    <span className="font-medium">
                      {t("profile.refereeTab.benefits.earnings.title")}
                    </span>
                  </div>
                  <p className="text-sm text-default-500 dark:text-default-400 transition-colors duration-200">
                    {t("profile.refereeTab.benefits.earnings.description")}
                  </p>
                </div>
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <IoTrendingUpOutline className="text-primary" />
                    <span className="font-medium">
                      {t("profile.refereeTab.benefits.incomeSources.title")}
                    </span>
                  </div>
                  <p className="text-sm text-default-500 dark:text-default-400 transition-colors duration-200">
                    {t("profile.refereeTab.benefits.incomeSources.description")}
                  </p>
                </div>
                <div className="text-sm text-default-500 dark:text-default-400 space-y-1 transition-colors duration-200">
                  <p>• {t("profile.refereeTab.benefits.bulletPoints.0")}</p>
                  <p>• {t("profile.refereeTab.benefits.bulletPoints.1")}</p>
                  <p>• {t("profile.refereeTab.benefits.bulletPoints.2")}</p>
                  <p>• {t("profile.refereeTab.benefits.bulletPoints.3")}</p>
                </div>
              </div>
            </AccordionItem>
          </Accordion>
        </div>
        {/* Rewards History */}
        <div className="mt-6 bg-default-50 dark:bg-default-900/20 rounded-xl p-6 transition-colors duration-200">
          <h5 className="text-lg font-semibold mb-4 flex items-center gap-2 text-default-700 dark:text-default-200 transition-colors duration-200">
            <IoTrophyOutline className="text-lg sm:text-xl" />
            {t("profile.refereeTab.sections.rewardsHistory")}
          </h5>
        </div>
      </div>
    </>
  );
};

export default RefereeTab;
