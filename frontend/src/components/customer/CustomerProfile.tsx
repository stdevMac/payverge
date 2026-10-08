import React, { useState, useEffect, useCallback } from "react";
import { Card, CardBody, CardHeader, Button, Input, Chip, Switch, Avatar, Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, Spinner } from "@nextui-org/react";
import { User, Mail, Phone, Cake, Wallet, Store, Star, DollarSign, Pencil, Settings } from "lucide-react";
import { crmAPI, CustomerBusiness } from "@/api/crm";
import { formatGuestCurrency, normalizeGuestLocale } from "@/utils/guestCurrencyFormatter";
import { useCustomerAuth } from "@/contexts/CustomerAuthContext";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import { runWithFeedback } from "@/lib/runWithFeedback";
import { localDateKey } from "@/lib/localDate";
import CustomerAuthModal from "./CustomerAuthModal";

export default function CustomerProfile() {
  const { t, currentLanguage } = useGuestTranslation();
  const fmtCurrency = (amount: number, code: string) =>
    formatGuestCurrency(amount, code, currentLanguage);
  const { customer, loading: authLoading, refreshCustomer } = useCustomerAuth();
  const [businesses, setBusinesses] = useState<CustomerBusiness[]>([]);
  const [businessesLoading, setBusinessesLoading] = useState(false);
  // Surfaces loyalty/businesses fetch failures instead of a silent empty list.
  const [businessesError, setBusinessesError] = useState(false);
  const [authModalOpen, setAuthModalOpen] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [preferencesModal, setPreferencesModal] = useState(false);
  const [deleteModal, setDeleteModal] = useState(false);
  const [saving, setSaving] = useState(false);
  const [formData, setFormData] = useState({
    name: "",
    phone: "",
    birthday: "",
  });
  const [preferences, setPreferences] = useState({
    receive_promotions: true,
    receive_newsletters: true,
    receive_birthday_offers: true,
    // Opt-in: data sharing defaults OFF until the guest explicitly enables it.
    share_data_with_businesses: false,
  });

  const translate = (key: string): string => {
    return t(`customerProfile.${key}`);
  };

  const formatBirthdayForInput = (value?: string): string => {
    if (!value) {
      return "";
    }
    const trimmed = value.trim();
    if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) {
      return trimmed;
    }
    const parsed = new Date(trimmed);
    if (Number.isNaN(parsed.getTime())) {
      return "";
    }
    return localDateKey(parsed);
  };

  const loadBusinesses = useCallback(async () => {
    try {
      setBusinessesLoading(true);
      setBusinessesError(false);
      const businessesData = await crmAPI.getBusinesses();
      setBusinesses(businessesData);
    } catch (error) {
      console.error("Failed to load businesses:", error);
      setBusinessesError(true);
    } finally {
      setBusinessesLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!customer) {
      setBusinesses([]);
      setBusinessesLoading(false);
      return;
    }

    setFormData({
      name: customer.name || "",
      phone: customer.phone || "",
      birthday: formatBirthdayForInput(customer.birthday),
    });
    setPreferences({
      receive_promotions: customer.preferences?.receive_promotions ?? true,
      receive_newsletters: customer.preferences?.receive_newsletters ?? true,
      receive_birthday_offers: customer.preferences?.receive_birthday_offers ?? true,
      // Fail-closed: missing preference means do not share (explicit opt-in).
      share_data_with_businesses: customer.preferences?.share_data_with_businesses ?? false,
    });
    void loadBusinesses();
  }, [customer, loadBusinesses]);

  const handleUpdateProfile = async () => {
    const updates = {
      name: formData.name.trim(),
      phone: formData.phone.trim(),
      birthday: formData.birthday.trim(),
    };
    const result = await runWithFeedback(
      async () => {
        await crmAPI.updateProfile(updates);
        return true;
      },
      {
        setBusy: setSaving,
        success: translate("updateSuccess"),
        error: translate("updateError"),
      },
    );
    // Only leave edit mode + refresh on success; on failure the form stays
    // open with the user's edits intact so they can retry.
    if (result) {
      setEditMode(false);
      await refreshCustomer();
      await loadBusinesses();
    }
  };

  const handleUpdatePreferences = async () => {
    const result = await runWithFeedback(
      async () => {
        await crmAPI.updatePreferences(preferences);
        return true;
      },
      {
        setBusy: setSaving,
        success: translate("updateSuccess"),
        error: translate("updateError"),
      },
    );
    if (result) {
      setPreferencesModal(false);
      await refreshCustomer();
      await loadBusinesses();
    }
  };

  const handleDeleteAccount = async () => {
    const result = await runWithFeedback(
      async () => {
        await crmAPI.deleteAccount();
        return true;
      },
      {
        setBusy: setSaving,
        error: translate("deleteError"),
      },
    );
    // Only redirect after a confirmed deletion; on failure the modal stays
    // open with a visible error toast instead of silently doing nothing.
    if (result) {
      // eslint-disable-next-line @next/next/no-location-assign-relative-destination -- full reload after account deletion drops all client state
      window.location.href = "/";
    }
  };

  if (authLoading || businessesLoading) {
    return (
      <div className="min-h-screen bg-white flex justify-center items-center">
        <div className="text-center">
          <Spinner size="lg" className="mb-4" />
          <p className="text-ink-600">{translate("loading")}</p>
        </div>
      </div>
    );
  }

  if (!customer) {
    return (
      <div
        data-testid="customer-profile-unsigned"
        className="min-h-screen bg-white flex justify-center items-center"
      >
        <div className="text-center">
          <div className="w-16 h-16 bg-warm-100 rounded-full flex items-center justify-center mx-auto mb-3">
            <User className="w-8 h-8 text-ink-400" />
          </div>
          <h1 className="text-ink-900 font-medium text-lg">
            {translate("notFound")}
          </h1>
          <p className="mt-1 text-sm text-ink-600">{translate("signInPrompt")}</p>
          <Button
            color="primary"
            className="mt-4"
            onPress={() => setAuthModalOpen(true)}
          >
            {translate("signIn")}
          </Button>
        </div>
        <CustomerAuthModal
          isOpen={authModalOpen}
          onClose={() => setAuthModalOpen(false)}
          defaultMode="login"
          onSuccess={async () => {
            setAuthModalOpen(false);
            await refreshCustomer();
          }}
        />
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-warm-50">
      <div className="max-w-4xl mx-auto px-4 py-6">
        {/* Header Card */}
        <Card className="mb-6 shadow-sm">
          <CardBody className="p-4 sm:p-6">
            <div className="flex flex-col gap-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Avatar
                    src={customer.profile_image_url}
                    name={customer.name}
                    className="w-16 h-16"
                  />
                  <div>
                    <h1 className="text-xl font-semibold text-ink-900">
                      {customer.name}
                    </h1>
                    <p className="text-sm text-ink-600">{customer.email}</p>
                  </div>
                </div>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="light"
                    isIconOnly
                    aria-label={translate("preferences")}
                    onPress={() => setPreferencesModal(true)}
                  >
                    <Settings className="w-5 h-5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="light"
                    isIconOnly
                    aria-label={translate("edit")}
                    onPress={() => setEditMode(!editMode)}
                  >
                    <Pencil className="w-5 h-5" />
                  </Button>
                </div>
              </div>
            </div>
          </CardBody>
        </Card>

        {businessesError && (
          <Card className="mb-6 border border-rose-200 bg-rose-50 shadow-sm">
            <CardBody className="flex flex-row items-center justify-between gap-3 p-4">
              <p className="text-sm text-rose-700">{translate("loadError")}</p>
              <Button size="sm" variant="flat" onPress={() => void loadBusinesses()}>
                {translate("retry")}
              </Button>
            </CardBody>
          </Card>
        )}

        {/* Quick Stats */}
        {businesses.length > 0 && (
          <div className="grid grid-cols-3 gap-3 mb-6">
            <Card className="shadow-sm">
              <CardBody className="p-3 text-center">
                <Star className="w-5 h-5 text-ink-400 mx-auto mb-1" />
                <p className="text-lg font-semibold text-ink-900">
                  {businesses
                    .reduce((sum, b) => sum + b.loyalty_points, 0)
                    .toLocaleString()}
                </p>
                <p className="text-xs text-ink-600">{translate("points")}</p>
              </CardBody>
            </Card>
            <Card className="shadow-sm">
              <CardBody className="p-3 text-center">
                <DollarSign className="w-5 h-5 text-ink-400 mx-auto mb-1" />
                <p className="text-lg font-semibold text-ink-900">
                  {(() => {
                    // Customers can spend across businesses with different
                    // currencies; if every business uses the same currency,
                    // format with that currency. Otherwise fall back to a
                    // currency-less number to avoid mislabeling the total.
                    const total = businesses.reduce(
                      (sum, b) => sum + b.total_spent,
                      0,
                    );
                    const currencies = Array.from(
                      new Set(
                        businesses.map(
                          (b) => b.business?.default_currency || "USD",
                        ),
                      ),
                    );
                    if (currencies.length === 1) {
                      return fmtCurrency(total, currencies[0]);
                    }
                    return new Intl.NumberFormat(
                      normalizeGuestLocale(currentLanguage),
                      { maximumFractionDigits: 0 },
                    ).format(total);
                  })()}
                </p>
                <p className="text-xs text-ink-600">{translate("spent")}</p>
              </CardBody>
            </Card>
            <Card className="shadow-sm">
              <CardBody className="p-3 text-center">
                <Store className="w-5 h-5 text-ink-400 mx-auto mb-1" />
                <p className="text-lg font-semibold text-ink-900">
                  {businesses.reduce((sum, b) => sum + b.visit_count, 0)}
                </p>
                <p className="text-xs text-ink-600">{translate("visits")}</p>
              </CardBody>
            </Card>
          </div>
        )}

        {/* Profile Information */}
        <Card className="mb-4 shadow-sm">
          <CardHeader className="pb-2">
            <h2 className="text-base font-semibold text-ink-900">
              {translate("profileInfo")}
            </h2>
          </CardHeader>
          <CardBody className="pt-2">
            {editMode ? (
              <div className="space-y-4">
                <Input
                  label={translate("name")}
                  value={formData.name}
                  onValueChange={(value) =>
                    setFormData({ ...formData, name: value })
                  }
                  startContent={<User size={16} className="text-ink-400" />}
                  classNames={{
                    input: "text-base",
                    label: "text-sm font-medium",
                  }}
                />
                <Input
                  label={translate("phone")}
                  value={formData.phone}
                  onValueChange={(value) =>
                    setFormData({ ...formData, phone: value })
                  }
                  startContent={<Phone size={16} className="text-ink-400" />}
                  classNames={{
                    input: "text-base",
                    label: "text-sm font-medium",
                  }}
                />
                <Input
                  label={translate("birthday")}
                  type="date"
                  value={formData.birthday}
                  onValueChange={(value) =>
                    setFormData({ ...formData, birthday: value })
                  }
                  startContent={<Cake size={16} className="text-ink-400" />}
                  classNames={{
                    input: "text-base",
                    label: "text-sm font-medium",
                  }}
                />
                <Button
                  className="bg-brand hover:bg-brand-dark text-white font-semibold w-full mt-4"
                  size="lg"
                  onPress={handleUpdateProfile}
                  isLoading={saving}
                >
                  {translate("saveChanges")}
                </Button>
              </div>
            ) : (
              <div className="space-y-3">
                <div className="flex items-center gap-3 py-2">
                  <Mail className="text-ink-400 w-5 h-5" />
                  <div className="flex-1 min-w-0">
                    <p className="text-xs text-ink-500">
                      {translate("email")}
                    </p>
                    <p className="text-sm font-medium text-ink-900 truncate">
                      {customer.email}
                    </p>
                  </div>
                </div>
                {customer.phone && (
                  <div className="flex items-center gap-3 py-2">
                    <Phone className="text-ink-400 w-5 h-5" />
                    <div className="flex-1">
                      <p className="text-xs text-ink-500">
                        {translate("phone")}
                      </p>
                      <p className="text-sm font-medium text-ink-900">
                        {customer.phone}
                      </p>
                    </div>
                  </div>
                )}
                {customer.birthday && (
                  <div className="flex items-center gap-3 py-2">
                    <Cake className="text-ink-400 w-5 h-5" />
                    <div className="flex-1">
                      <p className="text-xs text-ink-500">
                        {translate("birthday")}
                      </p>
                      <p className="text-sm font-medium text-ink-900">
                        {new Date(customer.birthday).toLocaleDateString()}
                      </p>
                    </div>
                  </div>
                )}
                {customer.wallet_address && (
                  <div className="flex items-center gap-3 py-2">
                    <Wallet className="text-ink-400 w-5 h-5" />
                    <div className="flex-1 min-w-0">
                      <p className="text-xs text-ink-500">
                        {translate("wallet")}
                      </p>
                      <p className="font-mono text-sm font-medium text-ink-900 truncate">
                        {customer.wallet_address.slice(0, 8)}...
                        {customer.wallet_address.slice(-6)}
                      </p>
                    </div>
                  </div>
                )}
              </div>
            )}
          </CardBody>
        </Card>

        {/* Connected Businesses */}
        <Card className="mb-4 shadow-sm">
          <CardHeader className="pb-2">
            <h2 className="text-base font-semibold text-ink-900">
              {translate("connectedBusinesses")}
            </h2>
          </CardHeader>
          <CardBody className="pt-2">
            {businesses.length === 0 ? (
              <div className="text-center py-8">
                <Store className="w-12 h-12 text-ink-500 mx-auto mb-2" />
                <p className="text-sm text-ink-600">
                  {translate("noBusinesses")}
                </p>
              </div>
            ) : (
              <div className="space-y-3">
                {businesses.map((business) => (
                  <div
                    key={business.id}
                    className="p-3 border border-warm-200 rounded-lg"
                  >
                    <div className="flex items-start justify-between mb-3">
                      <div className="flex-1">
                        <h3 className="font-semibold text-ink-900">
                          {business.business?.name ||
                            translate("unknownBusiness")}
                        </h3>
                        {business.last_visit_at && (
                          <p className="text-xs text-ink-500">
                            {translate("lastVisit")}:{" "}
                            {new Date(
                              business.last_visit_at,
                            ).toLocaleDateString()}
                          </p>
                        )}
                      </div>
                      {business.loyalty_tier && (
                        <Chip size="sm" variant="flat">
                          {business.loyalty_tier}
                        </Chip>
                      )}
                    </div>
                    <div className="grid grid-cols-3 gap-2 text-center">
                      <div>
                        <p className="text-lg font-semibold text-ink-900">
                          {business.loyalty_points}
                        </p>
                        <p className="text-xs text-ink-600">
                          {translate("points")}
                        </p>
                      </div>
                      <div>
                        <p className="text-lg font-semibold text-ink-900">
                          {fmtCurrency(
                            business.total_spent,
                            business.business?.default_currency || "USD",
                          )}
                        </p>
                        <p className="text-xs text-ink-600">
                          {translate("spent")}
                        </p>
                      </div>
                      <div>
                        <p className="text-lg font-semibold text-ink-900">
                          {business.visit_count}
                        </p>
                        <p className="text-xs text-ink-600">
                          {translate("visits")}
                        </p>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardBody>
        </Card>

        {/* Account Actions */}
        <Card className="border border-red-200 shadow-sm">
          <CardHeader className="pb-2">
            <h2 className="text-base font-semibold text-red-600">
              {translate("dangerZone")}
            </h2>
          </CardHeader>
          <CardBody className="pt-2">
            <div className="flex items-center justify-between gap-3">
              <div className="flex-1">
                <p className="text-sm font-medium text-ink-900">
                  {translate("deleteAccount")}
                </p>
                <p className="text-xs text-ink-600">
                  {translate("deleteAccountDescription")}
                </p>
              </div>
              <Button
                color="danger"
                variant="flat"
                size="sm"
                onPress={() => setDeleteModal(true)}
              >
                {translate("delete")}
              </Button>
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Preferences Modal */}
      <Modal
        isOpen={preferencesModal}
        onClose={() => !saving && setPreferencesModal(false)}
      >
        <ModalContent>
          <ModalHeader>
            <h2 className="text-lg font-semibold">
              {translate("preferences")}
            </h2>
          </ModalHeader>
          <ModalBody>
            <div className="space-y-4">
              <div className="flex items-center justify-between py-2">
                <p className="text-sm text-ink-900">
                  {translate("receivePromotions")}
                </p>
                <Switch
                  size="sm"
                  isSelected={preferences.receive_promotions}
                  onValueChange={(value) =>
                    setPreferences({
                      ...preferences,
                      receive_promotions: value,
                    })
                  }
                />
              </div>
              <div className="flex items-center justify-between py-2">
                <p className="text-sm text-ink-900">
                  {translate("receiveNewsletters")}
                </p>
                <Switch
                  size="sm"
                  isSelected={preferences.receive_newsletters}
                  onValueChange={(value) =>
                    setPreferences({
                      ...preferences,
                      receive_newsletters: value,
                    })
                  }
                />
              </div>
              <div className="flex items-center justify-between py-2">
                <p className="text-sm text-ink-900">
                  {translate("receiveBirthdayOffers")}
                </p>
                <Switch
                  size="sm"
                  isSelected={preferences.receive_birthday_offers}
                  onValueChange={(value) =>
                    setPreferences({
                      ...preferences,
                      receive_birthday_offers: value,
                    })
                  }
                />
              </div>
              <div className="flex items-center justify-between py-2">
                <p className="text-sm text-ink-900">
                  {/* Explicit opt-in: "Share my profile/data with businesses I visit"; defaults off. */}
                  {translate("shareDataWithBusinesses")}
                </p>
                <Switch
                  size="sm"
                  isSelected={preferences.share_data_with_businesses}
                  onValueChange={(value) =>
                    setPreferences({
                      ...preferences,
                      share_data_with_businesses: value,
                    })
                  }
                />
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button
              variant="light"
              size="sm"
              onPress={() => setPreferencesModal(false)}
              isDisabled={saving}
            >
              {translate("cancel")}
            </Button>
            <Button
              color="primary"
              size="sm"
              onPress={handleUpdatePreferences}
              isLoading={saving}
            >
              {translate("saveChanges")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Delete Account Modal */}
      <Modal
        isOpen={deleteModal}
        onClose={() => !saving && setDeleteModal(false)}
      >
        <ModalContent>
          <ModalHeader>
            <h2 className="text-lg font-semibold text-red-600">
              {translate("deleteAccount")}
            </h2>
          </ModalHeader>
          <ModalBody>
            <p className="text-sm text-ink-700">
              {translate("deleteAccountConfirmation")}
            </p>
          </ModalBody>
          <ModalFooter>
            <Button
              variant="light"
              size="sm"
              onPress={() => setDeleteModal(false)}
              isDisabled={saving}
            >
              {translate("cancel")}
            </Button>
            <Button
              color="danger"
              size="sm"
              onPress={handleDeleteAccount}
              isLoading={saving}
            >
              {translate("deleteConfirm")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
