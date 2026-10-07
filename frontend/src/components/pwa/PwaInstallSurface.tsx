"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useSimpleLocale } from "@/i18n/OperatorLocaleProvider";
import { getChromeTranslation } from "@/i18n/operatorChromeCatalog";
import { usePwaInstall } from "@/providers/PwaInstallProvider";
import InstallAppPrompt from "./InstallAppPrompt";
import InstallHelpSheet from "./InstallHelpSheet";

export default function PwaInstallSurface({
  allowFloatingPrompt = false,
}: {
  allowFloatingPrompt?: boolean;
}) {
  const pwa = usePwaInstall();
  const { claimFloatingPrompt, dismissForSevenDays, requestInstall } = pwa;
  const { locale } = useSimpleLocale();
  const [installPending, setInstallPending] = useState(false);
  const installPendingRef = useRef(false);
  const mountedRef = useRef(false);
  const translate = (key: string) =>
    String(getChromeTranslation(`pwa.${key}`, locale));

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    if (!allowFloatingPrompt) return;
    return claimFloatingPrompt();
  }, [allowFloatingPrompt, claimFloatingPrompt]);

  const handleInstall = useCallback(async () => {
    if (installPendingRef.current) return;

    installPendingRef.current = true;
    setInstallPending(true);
    try {
      await requestInstall();
    } finally {
      installPendingRef.current = false;
      if (mountedRef.current) setInstallPending(false);
    }
  }, [requestInstall]);

  const handleDismiss = useCallback(() => {
    if (!installPendingRef.current) dismissForSevenDays();
  }, [dismissForSevenDays]);

  return (
    <>
      {allowFloatingPrompt && pwa.shouldShowFloatingPrompt ? (
        <InstallAppPrompt
          title={translate("card.title")}
          body={translate("card.body")}
          installLabel={translate("actions.install")}
          dismissLabel={translate("actions.dismiss")}
          staffBottomOffset={pwa.identity?.roleType === "staff"}
          isPending={installPending}
          onInstall={() => void handleInstall()}
          onDismiss={handleDismiss}
        />
      ) : null}

      <InstallHelpSheet
        mode={pwa.helpMode}
        manualTitle={translate("manual.title")}
        steps={[
          translate("manual.stepShare"),
          translate("manual.stepAdd"),
          translate("manual.stepConfirm"),
        ]}
        doneLabel={translate("manual.done")}
        unsupportedTitle={translate("unsupported.title")}
        unsupportedBody={translate("unsupported.body")}
        closeLabel={translate("actions.close")}
        onClose={pwa.closeHelp}
        onDone={pwa.confirmManualInstall}
      />
    </>
  );
}
