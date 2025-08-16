"use client";

import { Button, Switch, Card } from "@nextui-org/react";
import {
  IoNotificationsOutline,
  IoMailOutline,
  IoOptionsOutline,
} from "react-icons/io5";
import { FaTelegram } from "react-icons/fa";
import { UserInterface } from "@/interface";
import { useTranslation } from "@/i18n/useTranslation";
import NotificationPreferencesForm from "@/components/notification-preferences/NotificationPreferencesForm";
import {
  NotificationPreferences,
  getNotificationPreferences,
} from "@/api/users/notification-preferences";
import { useState, useEffect } from "react";

interface NotificationsTabProps {
  user: UserInterface;
  loading?: boolean;
  onConnectTelegram?: () => void;
  onUpdateNotificationPreference?: (preference: "email" | "telegram") => void;
}

export function NotificationsTab({
  user,
  loading,
  onConnectTelegram,
  onUpdateNotificationPreference,
}: NotificationsTabProps) {
  const { t } = useTranslation();
  const [notificationPreferences, setNotificationPreferences] =
    useState<NotificationPreferences | null>(null);
  const [isLoadingPreferences, setIsLoadingPreferences] = useState(true);

  useEffect(() => {
    async function fetchNotificationPreferences() {
      try {
        const response = await getNotificationPreferences();
        setNotificationPreferences(response.preferences);
      } catch (error) {
        console.error("Failed to fetch notification preferences:", error);
      } finally {
        setIsLoadingPreferences(false);
      }
    }

    fetchNotificationPreferences();
  }, []);

  return (
    <div className="space-y-6">
      {/* Notification Preferences */}
      <Card className="p-4">
        <h3 className="text-xl font-semibold mb-4 flex items-center gap-2">
          <IoNotificationsOutline className="text-2xl" />
          {t("profile.notificationsTab.title")}
        </h3>

        <div className="space-y-4">
          {/* Telegram Connection */}
          <div className="border-b pb-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <FaTelegram className="text-2xl text-[#0088cc]" />
                <div>
                  <p className="font-medium">
                    {t("profile.notificationsTab.telegramConnection.title")}
                  </p>
                  <p className="text-sm text-gray-500">
                    {user.telegram_chat_id
                      ? t(
                          "profile.notificationsTab.telegramConnection.connected"
                        )
                      : t(
                          "profile.notificationsTab.telegramConnection.notConnected"
                        )}
                  </p>
                </div>
              </div>
              <Button
                color={user.telegram_chat_id ? "success" : "primary"}
                variant="flat"
                onClick={onConnectTelegram}
                isDisabled={loading}
              >
                {user.telegram_chat_id
                  ? t(
                      "profile.notificationsTab.telegramConnection.connectedButton"
                    )
                  : t(
                      "profile.notificationsTab.telegramConnection.connectButton"
                    )}
              </Button>
            </div>
          </div>

          {/* Notification Method Selection */}
          <div className="border-b pb-4">
            <h4 className="text-lg font-medium mb-4">
              {t("profile.notificationsTab.notificationMethod.title")}
            </h4>
            <div className="flex flex-col sm:flex-row items-center gap-4">
              {/* Email Option */}
              <div className="w-full sm:flex-1 flex items-center gap-3 p-3 rounded-lg bg-default-50">
                <div className="flex items-center gap-2">
                  <IoMailOutline className="text-xl text-amber-500" />
                  <div className="min-w-0 flex-1">
                    <p className="font-medium">
                      {t("profile.notificationsTab.notificationMethod.email")}
                    </p>
                    <p className="text-xs text-gray-500 truncate">
                      {user.email}
                    </p>
                  </div>
                </div>
              </div>

              {/* Switch */}
              <div className="flex items-center justify-center w-full sm:w-auto">
                <div className="flex flex-col items-center gap-1">
                  <Switch
                    isSelected={user.notification_preference === "telegram"}
                    onValueChange={(isSelected) =>
                      onUpdateNotificationPreference?.(
                        isSelected ? "telegram" : "email"
                      )
                    }
                    isDisabled={!user.telegram_chat_id}
                    size="lg"
                    classNames={{
                      wrapper:
                        "group-data-[selected=true]:bg-[#0088cc] bg-amber-500",
                      thumb: "group-data-[selected=true]:bg-white",
                    }}
                  />
                  <span className="text-xs text-gray-500">
                    {!user.telegram_chat_id
                      ? t(
                          "profile.notificationsTab.notificationMethod.switchHelp.connectFirst"
                        )
                      : t(
                          "profile.notificationsTab.notificationMethod.switchHelp.swipeToChange"
                        )}
                  </span>
                </div>
              </div>

              {/* Telegram Option */}
              <div className="w-full sm:flex-1 flex items-center gap-3 p-3 rounded-lg bg-default-50">
                <div className="flex items-center gap-2">
                  <FaTelegram className="text-xl text-[#0088cc]" />
                  <div className="min-w-0 flex-1">
                    <p className="font-medium">
                      {t(
                        "profile.notificationsTab.notificationMethod.telegram"
                      )}
                    </p>
                    <p className="text-xs text-gray-500">
                      {user.telegram_chat_id
                        ? t(
                            "profile.notificationsTab.telegramConnection.connected"
                          )
                        : t(
                            "profile.notificationsTab.telegramConnection.notConnected"
                          )}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Current Notification Method */}
          <div className="border-b pb-4">
            <p className="text-sm text-gray-500">
              {!user.telegram_chat_id ? (
                <span className="text-warning-500">
                  {t(
                    "profile.notificationsTab.notificationMethod.telegramWarning"
                  )}
                </span>
              ) : (
                t(
                  "profile.notificationsTab.notificationMethod.currentMethod"
                ).replace(
                  "{{method}}",
                  user.notification_preference === "telegram"
                    ? t("profile.notificationsTab.notificationMethod.telegram")
                    : t("profile.notificationsTab.notificationMethod.email")
                )
              )}
            </p>
          </div>

          {/* Customize Notifications */}
          {notificationPreferences && (
            <div className="border-b pb-4">
              <h4 className="text-lg font-medium mb-4 flex items-center gap-2">
                <IoOptionsOutline className="text-xl" />
                {t("profile.notificationsTab.customize.title")}
              </h4>
              <NotificationPreferencesForm
                initialPreferences={notificationPreferences}
                activeColor={
                  user.notification_preference === "telegram"
                    ? "#0088cc"
                    : "#f59e0b"
                }
              />
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}
