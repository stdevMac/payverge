import React, { useCallback, useState } from "react";
import Image from "next/image";
import { Card, CardBody, Select, SelectItem, Switch } from "@nextui-org/react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { resolveReportTimezone } from "@/utils/reportTimezone";
import { reportTimezoneSelectOptions } from "@/utils/reportTimezoneOptions";

interface Plugin {
  id: number;
  name: string;
  display_name: string;
  description: string;
  image: string;
  category: string;
  version: string;
  features: string;
  is_enabled: boolean;
  config: string;
  config_schema?: string;
}

interface WeeklyReportConfigProps {
  plugin: Plugin;
  config: Record<string, any>;
  onConfigChange: (config: Record<string, any>) => void;
  onSave: () => void;
  onCancel: () => void;
  /** Business profile IANA timezone — used as default when config has none. */
  businessTimezone?: string | null;
}

function weekdayOptions(locale: string): { value: number; label: string }[] {
  // Use a fixed week (2024-01-07 is a Sunday) so Intl returns ordered weekday names.
  return Array.from({ length: 7 }, (_, value) => {
    const date = new Date(Date.UTC(2024, 0, 7 + value));
    const label = new Intl.DateTimeFormat(locale || "en", { weekday: "long", timeZone: "UTC" }).format(date);
    return { value, label };
  });
}

const HOURS = Array.from({ length: 24 }, (_, i) => ({
  value: i,
  label: `${i.toString().padStart(2, "0")}:00`,
}));

export default function WeeklyReportConfig({
  plugin,
  config,
  onConfigChange,
  onSave,
  onCancel,
  businessTimezone = null,
}: WeeklyReportConfigProps) {
  const { locale } = useSimpleLocale();

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.pluginManager.config.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const [enabled, setEnabled] = useState<boolean>(config.enabled !== false);
  const [dayOfWeek, setDayOfWeek] = useState<number>(config.day_of_week || 1);
  const [hour, setHour] = useState<number>(config.hour || 9);
  const [timezone, setTimezone] = useState<string>(() =>
    resolveReportTimezone(config.timezone, businessTimezone),
  );
  const timezoneOptions = reportTimezoneSelectOptions(businessTimezone);

  const handleSave = () => {
    onConfigChange({
      enabled,
      day_of_week: dayOfWeek,
      hour,
      timezone,
    });
    onSave();
  };

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3 rounded-2xl border border-warm-200 bg-warm-50/70 p-3 shadow-sm shadow-warm-900/5">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl border border-warm-200 bg-white shadow-inner shadow-warm-900/5">
          <Image
            src={plugin.image || "/images/plugins/weekly-report.png"}
            alt={plugin.display_name}
            width={24}
            height={24}
            className="rounded"
          />
        </div>
        <div className="min-w-0">
          <h3 className="text-lg font-semibold text-ink-950">
            {plugin.display_name}
          </h3>
          <p className="text-sm leading-5 text-ink-600">
            {plugin.description}
          </p>
        </div>
      </div>

      <Card className="rounded-3xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
        <CardBody className="space-y-6 p-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <h4 className="text-base font-semibold text-ink-950">
                {t("enableReports")}
              </h4>
              <p className="text-sm text-ink-600">
                {t("receiveWeeklyReports")}
              </p>
            </div>
            <Switch
              isSelected={enabled}
              onValueChange={setEnabled}
              color="default"
              classNames={{
                wrapper: "group-data-[selected=true]:bg-brand",
              }}
            />
          </div>

          {enabled && (
            <>
              <div className="space-y-2">
                <label className="text-sm font-semibold text-ink-700">
                  {t("dayOfWeek")}
                </label>
                <Select
                  selectedKeys={[dayOfWeek.toString()]}
                  onChange={(e) => setDayOfWeek(parseInt(e.target.value, 10))}
                  placeholder={t("selectDay")}
                  className="w-full"
                  size="lg"
                  classNames={{
                    trigger:
                      "rounded-2xl border border-warm-200 bg-warm-50/60 shadow-none data-[hover=true]:bg-warm-50",
                    value: "text-ink-800",
                  }}
                >
                  {weekdayOptions(locale).map((day) => (
                    <SelectItem
                      key={day.value.toString()}
                      value={day.value.toString()}
                    >
                      {day.label}
                    </SelectItem>
                  ))}
                </Select>
                <p className="text-xs text-ink-500">
                  {t("dayOfWeekHelp")}
                </p>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-semibold text-ink-700">
                  {t("timeOfDay")}
                </label>
                <Select
                  selectedKeys={[hour.toString()]}
                  onChange={(e) => setHour(parseInt(e.target.value, 10))}
                  placeholder={t("selectTime")}
                  className="w-full"
                  size="lg"
                  classNames={{
                    trigger:
                      "rounded-2xl border border-warm-200 bg-warm-50/60 shadow-none data-[hover=true]:bg-warm-50",
                    value: "text-ink-800",
                  }}
                >
                  {HOURS.map((h) => (
                    <SelectItem
                      key={h.value.toString()}
                      value={h.value.toString()}
                    >
                      {h.label}
                    </SelectItem>
                  ))}
                </Select>
                <p className="text-xs text-ink-500">
                  {t("timeOfDayHelp")}
                </p>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-semibold text-ink-700">
                  {t("timezone")}
                </label>
                <Select
                  selectedKeys={[timezone]}
                  onChange={(e) => setTimezone(e.target.value)}
                  placeholder={t("selectTimezone")}
                  className="w-full"
                  size="lg"
                  classNames={{
                    trigger:
                      "rounded-2xl border border-warm-200 bg-warm-50/60 shadow-none data-[hover=true]:bg-warm-50",
                    value: "text-ink-800",
                  }}
                >
                  {timezoneOptions.map((tz) => (
                    <SelectItem key={tz.value} value={tz.value}>
                      {tz.label}
                    </SelectItem>
                  ))}
                </Select>
                <p className="text-xs text-ink-500">
                  {t("timezoneHelp")}
                </p>
              </div>

              <div className="rounded-2xl border border-brand/20 bg-brand/10 p-4 shadow-sm shadow-brand/5">
                <p className="text-sm text-brand-dark">
                  <strong className="font-medium">{t("preview")}:</strong>{" "}
                  {t("youWillReceive")} {weekdayOptions(locale)[dayOfWeek].label}{" "}
                  {t("at")} {HOURS[hour].label} ({timezone})
                </p>
              </div>
            </>
          )}
        </CardBody>
      </Card>

      <div className="flex gap-3 justify-end">
        <button
          onClick={onCancel}
          className="rounded-xl px-4 py-2 text-sm font-semibold text-ink-600 transition-colors hover:bg-warm-100 hover:text-ink-950"
        >
          {t("cancel")}
        </button>
        <button
          onClick={handleSave}
          className="rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-white shadow-sm shadow-brand/20 transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2"
        >
          {t("saveConfiguration")}
        </button>
      </div>
    </div>
  );
}
