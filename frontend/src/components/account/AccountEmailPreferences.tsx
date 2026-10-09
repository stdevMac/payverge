"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Switch } from "@nextui-org/react";
import { Mail } from "lucide-react";

import {
  getEmailNotificationPreferences,
  updateEmailNotificationPreferences,
  type AccountEmailPreferences as Prefs,
} from "@/api/notificationPreferences";
import { useToast } from "@/contexts/ToastContext";
import { sectionHeadingClass } from "@/components/ui/headingStyles";

interface ItemCopy {
  label: string;
  description: string;
}

// Pre-localised copy, injected from the account page so this component stays
// self-contained but i18n lives in one place.
export interface AccountEmailPreferencesCopy {
  sectionTitle: string;
  sectionDescription: string;
  loadingLabel: string;
  errorMessage: string;
  savedToast: string;
  saveErrorToast: string;
  items: {
    transactional: ItemCopy;
    reports: ItemCopy;
  };
}

const DEFAULT_PREFS: Prefs = {
  transactional_enabled: true,
  reports_enabled: false,
  news_enabled: false,
  updates_enabled: true,
  security_enabled: true,
};

// Only transactional and reports are offered. news, updates, security,
// statistics, and the email_enabled master round-trip untouched on the
// full-object save.
const EDITABLE_FIELDS = ["transactional", "reports"] as const;

type EditableKey = (typeof EDITABLE_FIELDS)[number];
const fieldFor = (key: EditableKey): keyof Prefs =>
  `${key}_enabled` as keyof Prefs;

/**
 * Account-level email preference toggles, rendered on /account. Edits the
 * user-scoped /inside/settings/notifications record — the canonical home for
 * "manage preferences / unsubscribe" from operator emails. Saves each toggle
 * immediately (optimistic), reverting on failure. Mirrors the dashboard's
 * Settings → Notifications email section, which edits the same record.
 */
export function AccountEmailPreferences({
  copy,
}: {
  copy: AccountEmailPreferencesCopy;
}) {
  const { showSuccess, showError } = useToast();
  const [prefs, setPrefs] = useState<Prefs>(DEFAULT_PREFS);
  // Authoritative latest-intended prefs, mirrored synchronously so concurrent
  // toggles build their payload from current state, not a stale closure.
  const prefsRef = useRef<Prefs>(DEFAULT_PREFS);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getEmailNotificationPreferences()
      .then((data) => {
        if (cancelled) return;
        prefsRef.current = data;
        setPrefs(data);
      })
      .catch(() => {
        if (!cancelled) setError(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const handleToggle = useCallback(
    async (key: EditableKey, value: boolean) => {
      const field = fieldFor(key);
      const previous = prefsRef.current;
      const next = { ...previous, [field]: value };
      prefsRef.current = next;
      setPrefs(next);
      setSaving(true);
      try {
        await updateEmailNotificationPreferences(next);
        showSuccess(copy.savedToast);
      } catch {
        // Revert to the last server-confirmed state so the UI never lies.
        prefsRef.current = previous;
        setPrefs(previous);
        showError(copy.saveErrorToast);
      } finally {
        setSaving(false);
      }
    },
    [copy.savedToast, copy.saveErrorToast, showError, showSuccess],
  );

  const items: { key: EditableKey; copy: ItemCopy }[] = EDITABLE_FIELDS.map(
    (key) => ({ key, copy: copy.items[key] }),
  );

  return (
    <section className="mb-12">
      <div className="flex items-center gap-3 mb-2">
        <div className="w-9 h-9 rounded-xl bg-ink-50 border border-ink-100 flex items-center justify-center text-ink-500 shrink-0">
          <Mail className="w-4 h-4" />
        </div>
        <h2 className={sectionHeadingClass}>
          {copy.sectionTitle}
        </h2>
      </div>
      <p className="text-sm text-ink-600 mb-4 ml-12">
        {copy.sectionDescription}
      </p>

      {loading ? (
        <div
          role="status"
          className="border border-ink-200 rounded-2xl px-6 py-8 text-sm text-ink-500 flex items-center gap-3"
        >
          <span className="h-4 w-4 animate-spin rounded-full border-2 border-ink-200 border-t-ink-500 motion-safe:animate-spin" />
          {copy.loadingLabel}
        </div>
      ) : error ? (
        <div className="border border-rose-200 bg-rose-50 rounded-2xl px-6 py-5 text-sm text-rose-800">
          {copy.errorMessage}
        </div>
      ) : (
        <div className="border border-ink-200 rounded-2xl divide-y divide-ink-100 overflow-hidden">
          {items.map(({ key, copy: item }) => (
            <div
              key={key}
              className="flex items-center justify-between gap-4 px-6 py-4"
            >
              <div className="min-w-0">
                <p className="text-sm font-medium text-ink-900">{item.label}</p>
                <p className="text-xs text-ink-500">{item.description}</p>
              </div>
              <Switch
                size="sm"
                isSelected={Boolean(prefs[fieldFor(key)])}
                isDisabled={saving}
                onValueChange={(val) => handleToggle(key, val)}
                aria-label={item.label}
              />
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
