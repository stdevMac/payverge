"use client";

import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { Switch } from "@nextui-org/react";
import { toast } from "react-hot-toast";
import { useTranslation } from "@/i18n/useTranslation";
import {
  NotificationPreferences,
  updateNotificationPreferences,
} from "@/api/users/notification-preferences";

interface NotificationPreferencesFormProps {
  initialPreferences?: Partial<NotificationPreferences>;
  activeColor?: string;
}

export default function NotificationPreferencesForm({
  initialPreferences,
  activeColor = "#f59e0b", // default to amber-500
}: NotificationPreferencesFormProps) {
  const { t } = useTranslation();
  const { register, handleSubmit, setValue, watch } =
    useForm<NotificationPreferences>({
      defaultValues: {
        email_enabled: true,
        news_enabled: false,
        updates_enabled: false,
        investment_enabled: false,
        fleet_updates_enabled: false,
        car_purchase_enabled: false,
        car_sale_enabled: false,
        car_status_enabled: false,
        rewards_enabled: false,
        dividends_enabled: false,
        transactional_enabled: false,
        security_enabled: true,
        reports_enabled: false,
        statistics_enabled: false,
        ...initialPreferences,
      },
    });

  const formValues = watch();

  // Handle automatic enabling/disabling based on email_enabled
  useEffect(() => {
    if (!formValues.email_enabled) {
      // Disable all other preferences when email_enabled is false
      Object.keys(formValues).forEach((key) => {
        if (key !== "email_enabled") {
          setValue(key as keyof NotificationPreferences, false);
        }
      });
    }
  }, [formValues, formValues.email_enabled, setValue]);

  const handlePreferenceChange = (
    key: keyof NotificationPreferences,
    checked: boolean
  ) => {
    if (key === "email_enabled") {
      setValue(key, checked);
    } else {
      if (checked && !formValues.email_enabled) {
        // If trying to enable any preference while email_enabled is false,
        // enable email_enabled first
        setValue("email_enabled", true);
      }
      setValue(key, checked);
    }
  };

  useEffect(() => {
    if (initialPreferences) {
      Object.entries(initialPreferences).forEach(([key, value]) => {
        setValue(key as keyof NotificationPreferences, value);
      });
    }
  }, [initialPreferences, setValue]);

  const onSubmit = async (data: NotificationPreferences) => {
    try {
      await updateNotificationPreferences({
        preferences: data,
      });
      toast.success(t('profile.notificationsTab.preferencesForm.successMessage'));
    } catch (error) {
      toast.error(t('profile.notificationsTab.preferencesForm.errorMessage'));
      console.error("Error updating notification preferences:", error);
    }
  };

  const handleEnableAll = async () => {
    // Enable all preferences
    preferences.forEach(({ key }) => {
      setValue(key as keyof NotificationPreferences, true);
    });

    // Submit the form with all preferences enabled
    const data = preferences.reduce(
      (acc, { key }) => ({
        ...acc,
        [key]: true,
      }),
      {} as NotificationPreferences
    );

    await onSubmit(data);
  };

  const preferences = [
    { key: "email_enabled", label: t('profile.notificationsTab.preferencesForm.allowNotifications') },
    { key: "news_enabled", label: t('profile.notificationsTab.preferencesForm.newsletter') },
    { key: "updates_enabled", label: t('profile.notificationsTab.preferencesForm.platformUpdates') },
    { key: "transactional_enabled", label: t('profile.notificationsTab.preferencesForm.transactions') },
    { key: "security_enabled", label: t('profile.notificationsTab.preferencesForm.security') },
    { key: "reports_enabled", label: t('profile.notificationsTab.preferencesForm.reports') },
    { key: "statistics_enabled", label: t('profile.notificationsTab.preferencesForm.statistics') },
  ];

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-6">
      <div className="grid gap-6">
        {preferences.map(({ key, label }) => (
          <div
            key={key}
            className={`flex items-center ${
              key === "email_enabled"
                ? "bg-default-100 p-4 rounded-xl border-2 border-default-200 shadow-sm"
                : ""
            }`}
          >
            <div className="flex-1">
              <label
                htmlFor={key}
                className={`${
                  key === "email_enabled"
                    ? "text-base font-semibold"
                    : "text-sm font-medium"
                } text-gray-700`}
              >
                {label}
              </label>
              {key === "email_enabled" && (
                <p className="text-sm text-gray-500 mt-1">
                  {t('profile.notificationsTab.preferencesForm.enableDescription')}
                </p>
              )}
            </div>
            <Switch
              id={key}
              isSelected={formValues[key as keyof NotificationPreferences]}
              onValueChange={(checked) =>
                handlePreferenceChange(
                  key as keyof NotificationPreferences,
                  checked
                )
              }
              size={key === "email_enabled" ? "lg" : "md"}
              aria-label={label}
              classNames={{
                wrapper:
                  activeColor === "#0088cc"
                    ? "group-data-[selected=true]:bg-[#0088cc]"
                    : "group-data-[selected=true]:bg-amber-500",
                thumb: "group-data-[selected=true]:bg-white",
              }}
            />
          </div>
        ))}
      </div>

      <div className="flex justify-end space-x-4">
        <button
          type="submit"
          className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md shadow-sm hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
        >
          {t('profile.notificationsTab.preferencesForm.saveChanges')}
        </button>
        <button
          type="button"
          onClick={handleEnableAll}
          className="px-4 py-2 text-sm font-medium text-white bg-primary-600 border border-transparent rounded-md shadow-sm hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
        >
          {t('profile.notificationsTab.preferencesForm.enableAllSave')}
        </button>
      </div>
    </form>
  );
}
