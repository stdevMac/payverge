// Profile.tsx
"use client";

import { useEffect, useRef, useState } from "react";
import { useProfileData } from "@/hooks/useProfileData";
import { useAccount } from "wagmi";
import { useRouter, useSearchParams } from "next/navigation";
import { useTranslation } from "@/i18n/useTranslation";
import toast from "react-hot-toast";
import {
  Tab,
  Tabs,
  Dropdown,
  DropdownTrigger,
  DropdownMenu,
  DropdownItem,
  Button,
} from "@nextui-org/react";
import {
  IoPersonOutline,
  IoWalletOutline,
  IoShareSocialOutline,
  IoMenuOutline,
  IoTrendingUpOutline,
  IoArrowBackOutline,
  IoHomeOutline,
  IoNotificationsOutline,
} from "react-icons/io5";
import GeneralTab from "@/components/profile/GeneralTab";
import RefereeTab from "@/components/profile/RefereeTab";
import { NotificationsTab } from "@/components/profile/NotificationsTab";
import ProfileHeader from "@/components/profile/ProfileHeader";
import { updateNotificationPreference } from "@/api/users/profile";
import { useUser } from "@/hooks/useUser";
import { getUserProfile } from "@/api/users/profile";
import { useUserStore } from "@/store/useUserStore";
import Link from "next/link";
import { Tooltip } from "@nextui-org/react";

interface Props {
  params: {
    id: string;
  };
}

export default function Profile() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { user, isAuthenticated, isLoading } = useUser();
  const pollingInterval = useRef<NodeJS.Timeout>();
  const pollCount = useRef(0);
  const MAX_POLLS = 30; // Maximum number of polling attempts (5 minutes with 10-second intervals)
  const [isDesktop, setIsDesktop] = useState(false);
  const { t } = useTranslation();

  const tabNames = {
    general: t('profile.tabs.general'),
    referees: t('profile.tabs.referralProgram'),
    notifications: t('profile.tabs.notifications'),
  };

  const tabIcons = {
    general: <IoPersonOutline className="text-lg sm:text-xl" />,
    referees: <IoShareSocialOutline className="text-lg sm:text-xl" />,
    notifications: <IoNotificationsOutline className="text-lg sm:text-xl" />,
  };

  const mobileTabNames = {
    general: t('profile.tabs.generalShort'),
    referees: t('profile.tabs.refereesShort'),
    notifications: t('profile.tabs.notificationsShort'),
  };

  const tabDescriptions = {
    general: t('profile.descriptions.general'),
    referees: t('profile.descriptions.referees'),
    notifications: t('profile.descriptions.notifications'),
  };

  // Get the tab from URL query parameter or default to 'general'
  const [selectedTab, setSelectedTab] = useState(() => {
    const tab = searchParams.get("tab");
    return tab && Object.keys(tabNames).includes(tab) ? tab : "general";
  });

  useEffect(() => {
    const handleResize = () => {
      setIsDesktop(window.innerWidth >= 640);
    };

    // Set initial value
    handleResize();

    // Add event listener
    window.addEventListener("resize", handleResize);

    // Cleanup
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  const {
    user: profileUser,
    loading,
    updateUserProfile,
    refetchUser,
    formData,
    setFormData,
  } = useProfileData(user?.address || "");

  const handleRefetch = async () => {
    await refetchUser();
    if (user?.address) {
      await getUserProfile(user.address)
        .then((userData) => {
          if (userData) {
            useUserStore.getState().setUser(userData);
          }
        })
        .catch(console.error);
    }
    return;
  };

  // Track the last known telegram_chat_id
  const lastTelegramId = useRef<string | null>(null);

  // Effect to stop polling when telegram is connected
  useEffect(() => {
    if (
      profileUser?.telegram_chat_id &&
      (!lastTelegramId.current ||
        lastTelegramId.current !== profileUser.telegram_chat_id)
    ) {
      lastTelegramId.current = profileUser.telegram_chat_id;
      if (pollingInterval.current) {
        clearInterval(pollingInterval.current);
        pollingInterval.current = undefined;
        pollCount.current = 0;
        toast.success(t('profile.toast.telegramConnected'));
      }
    }
  }, [profileUser?.telegram_chat_id, t]); // Added 't' to dependency array

  // Cleanup polling on unmount
  useEffect(() => {
    return () => {
      if (pollingInterval.current) {
        clearInterval(pollingInterval.current);
        pollingInterval.current = undefined;
        pollCount.current = 0;
      }
    };
  }, [t]); // Added 't' to dependency array to fix ESLint warning

  const startPollingForTelegramConnection = () => {
    // Clear any existing polling
    if (pollingInterval.current) {
      clearInterval(pollingInterval.current);
      pollingInterval.current = undefined;
      pollCount.current = 0;
    }

    // Start polling every 15 seconds
    pollingInterval.current = setInterval(async () => {
      pollCount.current += 1;

      try {
        // Check if we've reached max attempts
        if (pollCount.current >= MAX_POLLS) {
          if (pollingInterval.current) {
            clearInterval(pollingInterval.current);
            pollingInterval.current = undefined;
            pollCount.current = 0;
          }
          toast.error(t('profile.toast.telegramTimeout'));
          return;
        }

        // Fetch latest user data
        const userData = await refetchUser();

        // The useEffect above will handle successful connection
        if (!userData) {
          console.log("Polling attempt", pollCount.current, "of", MAX_POLLS);
        }
      } catch (error) {
        console.error("Error during telegram polling:", error);
        // Don't stop polling on error, just log it
      }
    }, 15000); // Poll every 15 seconds instead of 10
  };

  const handleCopyReferral = () => {
    if (profileUser?.referral_code) {
      const referralLink = `${window.location.origin}/referee/${profileUser.referral_code}`;
      navigator.clipboard
        .writeText(referralLink)
        .then(() => {
          toast.success(t('profile.toast.referralCopied'), {
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
          toast.error(t('profile.toast.referralCopyFailed'));
        });
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setFormData({
      ...formData,
      [e.target.name]: e.target.value,
    });
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    await updateUserProfile(formData);
  };

  const handleConnectTelegram = () => {
    if (!user?.address) {
      toast.error(t('profile.toast.connectWalletFirst'));
      return;
    }

    // Create the bot link with the user's address as the start parameter
    const botUrl = `https://t.me/web3boilerplate_bot?start=${user.address}`;

    // Open the bot link in a new tab
    window.open(botUrl, "_blank");

    toast.success(
      t('profile.toast.telegramInstructions'),
      {
        duration: 5000,
        position: "top-center",
      }
    );

    // Start polling for connection status
    startPollingForTelegramConnection();
  };

  const handleUpdateNotificationPreference = async (
    preference: "email" | "telegram"
  ) => {
    if (!user?.address) {
      toast.error(t('profile.toast.connectWalletFirst'));
      return;
    }

    if (preference === "telegram" && !profileUser?.telegram_chat_id) {
      toast.error(t('profile.toast.connectTelegramFirst'));
      return;
    }

    try {
      // Call the API to update the preference
      await updateNotificationPreference(user.address, preference);

      // Update local state
      await refetchUser();

      toast.success(t('profile.toast.notificationPreferenceUpdated').replace('{preference}', preference));
    } catch (error) {
      console.error("Error updating notification preference:", error);
      toast.error(t('profile.toast.notificationPreferenceUpdateFailed'));
    }
  };

  // Update URL when tab changes
  const handleTabChange = (key: React.Key) => {
    const tabKey = String(key);
    setSelectedTab(tabKey);
    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", tabKey);
    router.push(`/profile?${params.toString()}`);
  };

  const tabContent = {
    general: profileUser ? (
      <GeneralTab
        user={profileUser}
        formData={formData}
        handleChange={handleChange}
        handleSubmit={handleSubmit}
        loading={loading}
        onConnectTelegram={handleConnectTelegram}
        onUpdateNotificationPreference={handleUpdateNotificationPreference}
        onTabChange={handleTabChange}
      />
    ) : null,
    referees: profileUser ? (
      <RefereeTab
        user={profileUser}
        reFetchUser={handleRefetch}
      />
    ) : null,
    notifications: profileUser ? (
      <NotificationsTab
        user={profileUser}
        loading={loading}
        onConnectTelegram={handleConnectTelegram}
        onUpdateNotificationPreference={handleUpdateNotificationPreference}
      />
    ) : null,
  };

  if (isLoading || loading) {
    return (
      <div className="container mx-auto px-4 sm:px-6 py-4 sm:py-8 max-w-7xl">
        {/* Profile Header Skeleton */}
        <div className="bg-white rounded-xl p-6 shadow-sm">
          <div className="flex flex-col sm:flex-row gap-6 items-start sm:items-center">
            <div className="h-24 w-24 rounded-full bg-gray-200 animate-pulse" />
            <div className="flex-1 space-y-4 w-full">
              <div className="space-y-2">
                <div className="h-6 bg-gray-200 rounded w-1/4 animate-pulse" />
                <div className="h-4 bg-gray-200 rounded w-1/3 animate-pulse" />
              </div>
              <div className="flex flex-wrap gap-3">
                <div className="h-8 w-24 bg-gray-200 rounded animate-pulse" />
                <div className="h-8 w-32 bg-gray-200 rounded animate-pulse" />
              </div>
            </div>
          </div>
        </div>

        {/* Profile Content Skeleton */}
        <div className="mt-4 sm:mt-8">
          <div className="flex items-center justify-between mb-4">
            <div className="h-6 bg-gray-200 rounded w-48 animate-pulse" />
            <div className="h-6 bg-gray-200 rounded w-32 animate-pulse" />
          </div>

          {/* Tab Button Skeleton */}
          <div className="relative z-[6]">
            <div className="w-full h-16 sm:h-20 bg-gray-100 rounded-xl border-2 border-gray-200 animate-pulse">
              <div className="flex items-center justify-between p-4 sm:p-6">
                <div className="flex items-center gap-3">
                  <div className="h-8 w-8 bg-gray-200 rounded animate-pulse" />
                  <div className="space-y-2">
                    <div className="h-5 bg-gray-200 rounded w-32 animate-pulse" />
                    <div className="h-4 bg-gray-200 rounded w-48 hidden sm:block animate-pulse" />
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <div className="h-5 bg-gray-200 rounded w-24 animate-pulse" />
                  <div className="h-6 w-6 bg-gray-200 rounded animate-pulse" />
                </div>
              </div>
            </div>

            {/* Tab Content Skeleton */}
            <div className="mt-6 sm:mt-8">
              <div className="max-w-2xl mx-auto">
                <div className="bg-white rounded-lg p-6 shadow-sm space-y-6">
                  <div className="h-6 bg-gray-200 rounded w-1/3 animate-pulse" />
                  <div className="space-y-4">
                    <div className="h-4 bg-gray-200 rounded w-full animate-pulse" />
                    <div className="h-4 bg-gray-200 rounded w-5/6 animate-pulse" />
                    <div className="h-4 bg-gray-200 rounded w-4/6 animate-pulse" />
                  </div>
                  <div className="h-10 bg-gray-200 rounded w-full animate-pulse" />
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (!isAuthenticated) {
    return (
      <div className="container mx-auto px-4 py-8">
        <div className="max-w-2xl mx-auto text-center">
          <h2 className="text-2xl font-bold mb-4">{t('profile.welcome.title')}</h2>
          <p className="text-gray-600 mb-8">
            {t('profile.welcome.description')}
          </p>
          <div className="p-6 bg-gray-50 rounded-lg border border-gray-200">
            <IoWalletOutline className="mx-auto text-4xl text-primary mb-4" />
            <h3 className="text-lg font-semibold mb-2">{t('profile.welcome.connectWallet')}</h3>
            <p className="text-gray-500 text-sm">
              {t('profile.welcome.connectInstructions')}
            </p>
          </div>
        </div>
      </div>
    );
  }

  if (!profileUser) return null;

  return (
    <div className="container mx-auto px-4 sm:px-6 py-4 sm:py-8 max-w-7xl">
      {/* Back Navigation */}
      <div className="sticky top-[88px] -ml-[120px] hidden lg:block float-left">
        <Tooltip content={t('profile.backToHome')} placement="right">
          <Link href="/">
            <Button
              variant="solid"
              className="h-10 bg-background/80 hover:bg-background shadow-md hover:shadow-lg backdrop-blur-md transition-all gap-2 px-4 border border-default-200"
              aria-label="Back to home"
            >
              <IoArrowBackOutline className="text-lg" />
              <IoHomeOutline className="text-lg" />
            </Button>
          </Link>
        </Tooltip>
      </div>
      <ProfileHeader
        user={profileUser}
        onCopyReferral={handleCopyReferral}
        onTabChange={handleTabChange}
        currentTab={selectedTab}
      />

      <div className="mt-4 sm:mt-8">
        <div className="flex items-center justify-between mb-4 text-base sm:text-lg text-gray-500">
          <span className="font-medium">{t('profile.settingsTitle')}</span>
        </div>

        {/* Desktop Layout with Left Sidebar */}
        <div className="hidden sm:flex gap-6">
          {/* Left Sidebar with Tabs */}
          <div className="w-80 flex-shrink-0">
            <div className="space-y-3 sticky top-24">
              {Object.entries(tabNames).map(([key, name]) => (
                <Button
                  key={key}
                  variant={selectedTab === key ? "solid" : "bordered"}
                  onClick={() => handleTabChange(key)}
                  className={`w-full min-h-[4.5rem] p-4 rounded-xl group transition-all duration-200 ${
                    selectedTab === key
                      ? "bg-primary text-white shadow-lg border-primary hover:bg-primary/90"
                      : "bg-background hover:bg-gray-50 border-default-200 hover:border-primary"
                  }`}
                >
                  <div className="flex items-start gap-3 w-full">
                    <div
                      className={`text-2xl flex-shrink-0 mt-0.5 transition-colors duration-200 ${
                        selectedTab === key
                          ? "text-white"
                          : "text-primary group-hover:text-primary/90"
                      }`}
                    >
                      {tabIcons[key as keyof typeof tabIcons]}
                    </div>
                    <div className="text-left min-w-0 flex-1">
                      <p className="font-medium truncate">{name}</p>
                      <p
                        className={`text-sm truncate transition-colors duration-200 ${
                          selectedTab === key
                            ? "text-white/80"
                            : "text-gray-500"
                        }`}
                      >
                        {tabDescriptions[key as keyof typeof tabDescriptions]}
                      </p>
                    </div>
                  </div>
                </Button>
              ))}
            </div>
          </div>

          {/* Main Content */}
          <div className="flex-1">
            {tabContent[selectedTab as keyof typeof tabContent]}
          </div>
        </div>

        {/* Mobile Layout with Dropdown */}
        <div className="sm:hidden">
          <div className="relative z-[6]">
            <Dropdown>
              <DropdownTrigger>
                <Button
                  variant="bordered"
                  className="relative w-full justify-between min-h-[4rem] p-4 bg-background border-2 border-primary shadow-lg hover:shadow-xl transition-shadow duration-200 rounded-xl"
                >
                  <div className="flex items-center gap-3">
                    <div className="text-2xl text-primary">
                      {tabIcons[selectedTab as keyof typeof tabIcons]}
                    </div>
                    <div className="text-left">
                      <p className="font-medium text-lg">
                        {
                          mobileTabNames[
                            selectedTab as keyof typeof mobileTabNames
                          ]
                        }
                      </p>
                      <p className="text-sm text-gray-500 truncate max-w-[150px]">
                        {
                          tabDescriptions[
                            selectedTab as keyof typeof tabDescriptions
                          ]
                        }
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2 text-primary">
                    <IoMenuOutline className="text-2xl" />
                  </div>
                </Button>
              </DropdownTrigger>
              <DropdownMenu
                aria-label="Profile sections"
                onAction={handleTabChange}
                selectedKeys={[selectedTab]}
                className="w-[90vw]"
              >
                {Object.entries(mobileTabNames).map(([key, name]) => (
                  <DropdownItem
                    key={key}
                    startContent={
                      <div className="flex-shrink-0 text-xl text-primary">
                        {tabIcons[key as keyof typeof tabIcons]}
                      </div>
                    }
                    description={
                      tabDescriptions[key as keyof typeof tabDescriptions]
                    }
                    className="py-4"
                    textValue={name}
                  >
                    <span className="font-medium text-base">{name}</span>
                  </DropdownItem>
                ))}
              </DropdownMenu>
            </Dropdown>
            <div className="mt-6">
              {tabContent[selectedTab as keyof typeof tabContent]}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
