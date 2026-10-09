import React, { useCallback, useState, useEffect } from "react";
import {
  Switch,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
} from "@nextui-org/react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { Globe, Repeat, Coins, Link2, Wallet } from "lucide-react";
import { useDynamicContext } from "@/providers/DynamicProvider";
import { businessApi } from "@/api/business";
import toast from "react-hot-toast";
import { PluginConfigFooter } from "./PluginConfigFooter";

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

interface CrossChainPaymentConfigProps {
  plugin: Plugin;
  config: Record<string, any>;
  onConfigChange: (config: Record<string, any>) => void;
  onSave: () => void;
  onCancel: () => void;
  businessProfile?: any;
}

export default function CrossChainPaymentConfig({
  plugin,
  config,
  onConfigChange,
  onSave,
  onCancel,
  businessProfile,
}: CrossChainPaymentConfigProps) {
  const { locale } = useSimpleLocale();
  const { setShowAuthFlow, user } = useDynamicContext();
  const [showAddressModal, setShowAddressModal] = useState(false);
  const [manualAddress, setManualAddress] = useState("");
  const [isSavingAddress, setIsSavingAddress] = useState(false);

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.pluginManager.config.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const isValidEvmAddress = (address: string): boolean =>
    /^0x[a-fA-F0-9]{40}$/.test(address.trim());
  const manualAddressValid = isValidEvmAddress(manualAddress);

  const handleToggle = (field: string, value: boolean) => {
    onConfigChange({ ...config, [field]: value });
  };

  const saveAddress = useCallback(
    async (address: string) => {
      if (!businessProfile?.id) return;
      setIsSavingAddress(true);
      try {
        await businessApi.updateBusiness(businessProfile.id, {
          settlement_address: address,
          tipping_address: address,
        });
        toast.success(t("addressSaved") || "Address saved successfully");
        setShowAddressModal(false);
        onSave();
      } catch (error) {
        console.error("Failed to save address:", error);
        toast.error(t("addressSaveFailed") || "Failed to save address");
      } finally {
        setIsSavingAddress(false);
      }
    },
    [businessProfile?.id, onSave, t],
  );

  const handleEnable = () => {
    if (config.enabled !== false && !businessProfile?.settlement_address) {
      setShowAddressModal(true);
      return;
    }
    onSave();
  };

  useEffect(() => {
    if (showAddressModal && user?.verifiedCredentials?.[0]?.address) {
      const address = user.verifiedCredentials[0].address;
      void saveAddress(address);
    }
  }, [user, showAddressModal, saveAddress]);


  const features = [
    {
      icon: <Globe className="w-5 h-5 text-brand" />,
      title: t("crossChainPayment.features.multiChain"),
      description: t("crossChainPayment.features.multiChainDesc"),
    },
    {
      icon: <Coins className="w-5 h-5 text-amber-500" />,
      title: t("crossChainPayment.features.anyToken"),
      description: t("crossChainPayment.features.anyTokenDesc"),
    },
    {
      icon: <Repeat className="w-5 h-5 text-emerald-500" />,
      title: t("crossChainPayment.features.autoConversion"),
      description: t("crossChainPayment.features.autoConversionDesc"),
    },
    {
      icon: <Link2 className="w-5 h-5 text-brand" />,
      title: t("crossChainPayment.features.poweredByLifi"),
      description: t("crossChainPayment.features.poweredByLifiDesc"),
    },
  ];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4 rounded-2xl border border-warm-200 bg-warm-50/70 p-4 shadow-sm shadow-warm-900/5">
        <div className="w-14 h-14 bg-brand rounded-2xl flex items-center justify-center shadow-lg shadow-brand/20">
          <Globe className="w-7 h-7 text-white" />
        </div>
        <div className="min-w-0">
          <h3 className="text-xl font-semibold text-ink-950">
            {plugin.display_name}
          </h3>
          <p className="text-sm text-ink-600">{plugin.description}</p>
        </div>
      </div>

      {/* Features Grid */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {features.map((feature, index) => (
          <div
            key={index}
            className="flex items-start gap-3 rounded-2xl border border-warm-200/80 bg-white p-4 shadow-sm shadow-warm-900/5"
          >
            <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-warm-200/80 bg-warm-50">
              {feature.icon}
            </div>
            <div>
              <h4 className="font-semibold text-ink-950 text-sm">
                {feature.title}
              </h4>
              <p className="text-xs text-ink-600">{feature.description}</p>
            </div>
          </div>
        ))}
      </div>

      {/* Configuration Options */}
      <div className="space-y-4 rounded-2xl border border-warm-200 bg-white p-6 shadow-sm shadow-warm-900/5">
        <h4 className="font-semibold text-ink-950">
          {t("crossChainPayment.configuration")}
        </h4>

        <div className="flex items-center justify-between gap-4 py-3">
          <div>
            <p className="font-semibold text-ink-950">
              {t("crossChainPayment.enablePayments")}
            </p>
            <p className="text-sm text-ink-600">
              {t("crossChainPayment.enablePaymentsDesc")}
            </p>
          </div>
          <Switch
            isSelected={config.enabled !== false}
            onValueChange={(value) => handleToggle("enabled", value)}
            color="primary"
          />
        </div>
      </div>

      {/* Info Box */}
      <div className="rounded-2xl border border-brand/15 bg-brand/10 p-4 shadow-sm shadow-brand/10">
        <p className="text-sm text-brand-dark">
          <strong>{t("crossChainPayment.howItWorks")}</strong>{" "}
          {t("crossChainPayment.howItWorksDesc")}
        </p>
      </div>

      {/* Powered by LI.FI */}
      <div className="flex items-center justify-center gap-2 text-sm text-ink-600">
        <span>{t("crossChainPayment.poweredBy")}</span>
        <a
          href="https://li.fi"
          target="_blank"
          rel="noopener noreferrer"
          className="text-brand hover:text-brand-dark font-medium"
        >
          LI.FI
        </a>
      </div>

      {/* L6-31: enable vs save must reflect is_enabled (shared footer). */}
      <div className="pt-4">
        <PluginConfigFooter
          onSave={handleEnable}
          onCancel={onCancel}
          isEnabled={Boolean(plugin.is_enabled)}
          cancelLabel={t("cancel")}
          saveLabel={t("save")}
          enableLabel={t("enable")}
        />
      </div>

      {/* Address Requirement Modal */}
      <Modal
        isOpen={showAddressModal}
        onClose={() => setShowAddressModal(false)}
        size="md"
      >
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">
                {t("walletRequired") || "Wallet Required"}
              </ModalHeader>
              <ModalBody className="px-6 py-5">
                <p className="mb-4 text-sm text-ink-600">
                  {t("walletRequiredDesc") || "To enable cross-chain payments, you need to set up a settlement wallet address first."}
                </p>

                <div className="space-y-4">
                  <div className="flex items-center justify-between gap-4 rounded-2xl border border-brand/15 bg-brand/10 p-4">
                    <div>
                      <p className="text-sm font-semibold text-brand-dark">{t("connectWallet")}</p>
                      <p className="text-xs text-brand-dark/80">{t("connectWalletDescription")}</p>
                    </div>
                    <Button
                      size="sm"
                      startContent={<Wallet className="w-4 h-4" />}
                      onPress={() => setShowAuthFlow(true)}
                      className="bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
                    >
                      {t("connect")}
                    </Button>
                  </div>

                  <div className="relative">
                    <div className="absolute inset-0 flex items-center">
                      <span className="w-full border-t border-warm-200" />
                    </div>
                    <div className="relative flex justify-center text-xs uppercase">
                      <span className="bg-white px-2 text-ink-500">{t("orEnterManually")}</span>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <Input
                      label={t("walletAddress")}
                      placeholder="0x..."
                      value={manualAddress}
                      onValueChange={setManualAddress}
                      variant="bordered"
                      isInvalid={Boolean(manualAddress) && !manualAddressValid}
                      errorMessage={
                        manualAddress && !manualAddressValid
                          ? t("invalidAddress")
                          : undefined
                      }
                    />
                    <Button
                      fullWidth
                      variant="flat"
                      isDisabled={!manualAddressValid || isSavingAddress}
                      isLoading={isSavingAddress}
                      onPress={() => saveAddress(manualAddress)}
                      className="bg-brand/10 font-semibold text-brand-dark hover:bg-brand/15"
                    >
                      {t("saveAddress")}
                    </Button>
                  </div>
                </div>
              </ModalBody>
              <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
                <Button
                  variant="light"
                  onPress={onClose}
                  className="font-semibold text-ink-600 hover:bg-white hover:text-ink-950"
                >
                  {t("cancel")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
