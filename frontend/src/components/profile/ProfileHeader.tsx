"use client";

import { Button, Chip } from "@nextui-org/react";
import { UserInterface } from "@/interface";
import {
  IoMailOutline,
  IoWalletOutline,
  IoCopyOutline,
  IoLinkOutline,
} from "react-icons/io5";
import { useTranslation } from "@/i18n/useTranslation";

interface ProfileHeaderProps {
  user: UserInterface;
  onCopyReferral: () => void;
  onTabChange?: (tab: string) => void;
  currentTab?: string;
}

const ProfileHeader: React.FC<ProfileHeaderProps> = ({
  user,
  onCopyReferral,
  onTabChange,
  currentTab,
}) => {
  const { t } = useTranslation();
  const shortenAddress = (address: string) => {
    if (!address) return "";
    return `${address.slice(0, 6)}...${address.slice(-4)}`;
  };

  return (
    <div className="relative mb-8">
      {/* Header Background */}
      <div className="h-36 sm:h-48 w-full bg-gradient-to-r from-blue-500 to-purple-600 rounded-xl relative overflow-hidden">
        {/* Abstract Pattern */}
        <div className="absolute inset-0 opacity-10">
          <div className="absolute inset-0 transform -skew-y-12 bg-white/20" />
          <div className="absolute inset-0 transform skew-x-12 bg-white/20" />
        </div>

        {/* User Info Overlay */}
        <div className="absolute bottom-0 left-0 right-0 p-4 sm:p-6 text-white">
          <div className="flex flex-col sm:flex-row justify-between items-start sm:items-end gap-4 sm:gap-0">
            {/* User Info */}
            <div className="flex flex-col sm:flex-row justify-between items-start sm:items-end gap-4">
              <div>
                <div className="flex flex-wrap items-center gap-2 sm:gap-3 mb-2 sm:mb-3">
                  <h2 className="text-2xl sm:text-3xl font-bold capitalize">
                    {user.email ? user.email.split("@")[0] : t('profile.profileHeader.anonymousUser')}
                  </h2>
                </div>

                <div className="flex flex-col sm:flex-row items-start sm:items-center gap-2 sm:gap-6 text-xs sm:text-sm text-white/90">
                  {user.email && (
                    <div className="flex items-center gap-2">
                      <IoMailOutline className="text-lg" />
                      {user.email}
                    </div>
                  )}
                  <div className="flex items-center gap-2">
                    <IoWalletOutline className="text-lg" />
                    <span className="font-mono">
                      {shortenAddress(user.address)}
                    </span>
                    <button
                      onClick={() =>
                        navigator.clipboard.writeText(user.address)
                      }
                      className="text-white/70 hover:text-white transition-colors"
                    >
                      <IoCopyOutline />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Referral Link Section */}
      {user.referral_code && (
        <div className="mt-4 bg-gradient-to-r from-purple-50 to-blue-50 dark:from-purple-900/20 dark:to-blue-900/20 rounded-xl border border-purple-100 dark:border-purple-800 transition-colors duration-200">
          <div className="p-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="bg-gradient-to-r from-purple-500 to-blue-500 p-2 rounded-lg">
                  <IoLinkOutline className="text-xl text-white" />
                </div>
                <div>
                  <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-200 transition-colors duration-200">
                    {t('profile.profileHeader.referralLink.title')}
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400 transition-colors duration-200">
                    {t('profile.profileHeader.referralLink.description')}
                  </p>
                </div>
              </div>
              <Button
                size="sm"
                color="secondary"
                variant="flat"
                onClick={onCopyReferral}
                startContent={<IoCopyOutline />}
              >
                {t('profile.profileHeader.referralLink.copyLink')}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default ProfileHeader;
