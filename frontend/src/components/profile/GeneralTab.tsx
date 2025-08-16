"use client";

import { UserInterface } from "@/interface";
import { Card, Button } from "@nextui-org/react";
import {
  IoWalletOutline,
  IoGiftOutline,
  IoPeopleOutline,
  IoStatsChartOutline,
  IoArrowForwardOutline,
} from "react-icons/io5";
import { formatDistanceToNow } from "date-fns";
import { enUS, es } from "date-fns/locale";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useTranslation } from "@/i18n/useTranslation";

interface Props {
  user: UserInterface;
  formData?: any;
  handleChange?: (e: any) => void;
  handleSubmit?: (e: any) => void;
  loading?: boolean;
  onConnectTelegram?: () => void;
  onUpdateNotificationPreference?: (preference: "email" | "telegram") => void;
  onTabChange?: (key: React.Key) => void;
}

export const GeneralTab = ({
  user,
  formData,
  handleChange,
  handleSubmit,
  loading,
  onConnectTelegram,
  onUpdateNotificationPreference,
  onTabChange,
}: Props) => {
  const router = useRouter();
  const { t, language } = useTranslation();
  const [currentInvestmentPage, setCurrentInvestmentPage] = useState(1);
  const [currentActivityPage, setCurrentActivityPage] = useState(1);
  const ITEMS_PER_PAGE = 5;

  const PaginationControls = ({
    currentPage,
    totalItems,
    onPageChange,
  }: {
    currentPage: number;
    totalItems: number;
    onPageChange: (page: number) => void;
  }) => {
    const totalPages = Math.ceil(totalItems / ITEMS_PER_PAGE);
    const pageNumbers = Array.from({ length: totalPages }, (_, i) => i + 1);

    return (
      <div className="flex justify-center items-center gap-2 mt-4">
        <Button
          size="sm"
          variant="flat"
          onPress={() => onPageChange(Math.max(1, currentPage - 1))}
          isDisabled={currentPage === 1}
        >
          {t("profile.generalTab.pagination.previous")}
        </Button>
        <div className="flex items-center gap-1">
          {pageNumbers.map((pageNum) => (
            <Button
              key={pageNum}
              size="sm"
              variant={pageNum === currentPage ? "solid" : "flat"}
              color={pageNum === currentPage ? "primary" : "default"}
              onPress={() => onPageChange(pageNum)}
              className="min-w-[32px]"
            >
              {pageNum}
            </Button>
          ))}
        </div>
        <Button
          size="sm"
          variant="flat"
          onPress={() => onPageChange(Math.min(totalPages, currentPage + 1))}
          isDisabled={currentPage >= totalPages}
        >
          {t("profile.generalTab.pagination.next")}
        </Button>
      </div>
    );
  };

  const handleTabChange = (tab: string) => {
    router.replace(`/profile?tab=${tab}`);
  };

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(value);
  };

  const getDateFnsLocale = (lang: string) => {
    switch (lang) {
      case "es":
        return es;
      case "en":
      default:
        return enUS;
    }
  };

  const formatDate = (dateString: string) => {
    try {
      let date: Date;

      // Try parsing as ISO string first
      date = new Date(dateString);
      if (!isNaN(date.getTime())) {
        return formatDistanceToNow(date, {
          addSuffix: true,
          locale: getDateFnsLocale(language),
        });
      }

      // Try extracting datetime from Go format
      const dateMatch = dateString.match(
        /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/
      );
      if (dateMatch) {
        const cleanDate = dateMatch[0].replace(" ", "T") + "Z";
        date = new Date(cleanDate);
        return formatDistanceToNow(date, {
          addSuffix: true,
          locale: getDateFnsLocale(language),
        });
      }

      // Try parsing as Unix timestamp
      const timestamp = parseInt(dateString);
      if (!isNaN(timestamp)) {
        date = new Date(timestamp * 1000); // Convert seconds to milliseconds
        if (!isNaN(date.getTime())) {
          return formatDistanceToNow(date, {
            addSuffix: true,
            locale: getDateFnsLocale(language),
          });
        }
      }

      console.error("Could not parse date:", dateString);
      return "Recently";
    } catch (error) {
      console.error("Error formatting date:", error, dateString);
      return "Recently";
    }
  };

  // Create an array of 5 items, fill with placeholders if needed
  const getPageItems = <T,>(items: T[], currentPage: number): (T | null)[] => {
    const startIndex = (currentPage - 1) * ITEMS_PER_PAGE;
    const pageItems = items.slice(startIndex, startIndex + ITEMS_PER_PAGE);
    // Fill remaining slots with null if less than ITEMS_PER_PAGE
    return Array.from(
      { length: ITEMS_PER_PAGE },
      (_, index) => pageItems[index] || null
    );
  };

  // Calculate total referees and points
  const { totalReferees, totalPoints } = Object.entries(
    user.referees || {}
  ).reduce(
    (acc, [_, points]) => ({
      totalReferees: acc.totalReferees + 1,
      totalPoints: acc.totalPoints + points,
    }),
    { totalReferees: 0, totalPoints: 0 }
  );



  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Overview Stats */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 sm:gap-4">

        {/* Total Rewards */}
        <div className="bg-success-50 p-3 sm:p-4 rounded-xl border border-success-200">
          <div className="flex items-center gap-2 mb-2">
            <IoGiftOutline className="text-lg sm:text-xl text-success-600" />
            <span className="text-xs sm:text-sm font-medium text-success-700">
              {t("profile.generalTab.stats.totalRewards")}
            </span>
          </div>
        </div>

        {/* Referral Program */}
        <div className="bg-secondary-50 p-3 sm:p-4 rounded-xl border border-secondary-200">
          <div className="flex items-center gap-2 mb-2">
            <IoPeopleOutline className="text-lg sm:text-xl text-secondary-600" />
            <span className="text-xs sm:text-sm font-medium text-secondary-700">
              {t("profile.generalTab.stats.referralProgram")}
            </span>
          </div>
          <p className="text-xl sm:text-2xl font-bold text-secondary-600">
            {totalReferees} {t("profile.generalTab.stats.referees")}
          </p>
        </div>
      </div>

      {/* User Stats */}
      <Card className="bg-default-50">
        <div className="p-6 space-y-4">
          <div className="flex items-center gap-2">
            <IoStatsChartOutline className="text-lg sm:text-xl text-default-600" />
            <span className="text-sm font-medium">
              {t("profile.generalTab.accountStats.title")}
            </span>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-4">
            <div className="p-4 bg-default-100 rounded-lg">
              <p className="text-sm text-default-500">
                {t("profile.generalTab.accountStats.totalInvestments")}
              </p>
            </div>
            <div className="p-4 bg-default-100 rounded-lg">
              <p className="text-sm text-default-500">
                {t("profile.generalTab.accountStats.lastClaim")}
              </p>
            </div>
            <div className="p-4 bg-default-100 rounded-lg">
              <p className="text-sm text-default-500">
                {t("profile.generalTab.accountStats.accountStatus")}
              </p>
            </div>
          </div>
        </div>
      </Card>
    </div>
  );
};

export default GeneralTab;
