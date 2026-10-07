"use client";

import React, { useState, useCallback } from "react";
import { Card, CardBody, Input, Button, Chip, Divider, Link, Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, useDisclosure } from "@nextui-org/react";
import { Search, MapPin, Star, Users, ExternalLink, Check, X, Globe, AlertTriangle, Settings } from "lucide-react";
import { searchGoogleBusinesses, updateBusinessGoogleInfo, removeBusinessGoogleInfo, type GooglePlaceResult } from "../../api/google";
import { useToast } from "../../contexts/ToastContext";
import {
  useSimpleLocale,
  getTranslation,
} from "../../i18n/SimpleTranslationProvider";

interface GoogleInfoUpdate {
  google_place_id: string;
  google_business_name: string;
  google_review_link: string;
  google_business_url: string;
  google_reviews_enabled: boolean;
}

interface GoogleBusinessSearchProps {
  businessId: string;
  businessName: string;
  businessAddress?: string;
  currentGoogleInfo?: {
    google_place_id?: string;
    google_business_name?: string;
    google_review_link?: string;
    google_business_url?: string;
    google_reviews_enabled?: boolean;
  };
  onUpdate?: (info: GoogleInfoUpdate) => void;
}

export default function GoogleBusinessSearch({
  businessId,
  businessName,
  businessAddress,
  currentGoogleInfo,
  onUpdate,
}: GoogleBusinessSearchProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<GooglePlaceResult[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [isUpdating, setIsUpdating] = useState(false);
  const [selectedBusiness, setSelectedBusiness] =
    useState<GooglePlaceResult | null>(null);
  const { isOpen, onOpen, onClose } = useDisclosure();
  const { showError, showSuccess, showWarning } = useToast();
  const { locale: currentLocale } = useSimpleLocale();

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `googleBusinessSearch.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // Keep search query empty by default - let users type what they want

  // Clear default when user starts typing their own query
  const handleSearchInputChange = (value: string) => {
    setSearchQuery(value);
  };

  const handleSearch = useCallback(async () => {
    if (!searchQuery.trim()) {
      showError(tString("errors.emptyQuery"));
      return;
    }

    setIsSearching(true);
    try {
      const response = await searchGoogleBusinesses(searchQuery.trim());
      setSearchResults(response.results);

      if (response.results.length === 0) {
        showWarning(tString("errors.noResults"));
      }
    } catch (error) {
      console.error("Error searching businesses:", error);
      showError(tString("errors.searchFailed"));
    } finally {
      setIsSearching(false);
    }
  }, [searchQuery, showError, showWarning, tString]);

  const handleSelectBusiness = (business: GooglePlaceResult) => {
    setSelectedBusiness(business);
    onOpen();
  };

  const handleConfirmSelection = async () => {
    if (!selectedBusiness) return;

    setIsUpdating(true);
    try {
      const result = await updateBusinessGoogleInfo(businessId, {
        place_id: selectedBusiness.place_id,
        business_name: selectedBusiness.name,
      });

      showSuccess(tString("success.linked"));
      onClose();
      setSelectedBusiness(null);
      setSearchResults([]);
      setSearchQuery("");
      onUpdate?.({
        google_place_id: result.google_place_id,
        google_business_name: result.google_business_name,
        google_review_link: result.google_review_link,
        google_business_url: result.google_business_url,
        google_reviews_enabled: true,
      });
    } catch (error) {
      console.error("Error updating Google business info:", error);
      showError(tString("errors.linkFailed"));
    } finally {
      setIsUpdating(false);
    }
  };

  const handleRemoveIntegration = async () => {
    setIsUpdating(true);
    try {
      await removeBusinessGoogleInfo(businessId);
      showSuccess(tString("success.removed"));
      onUpdate?.({
        google_place_id: "",
        google_business_name: "",
        google_review_link: "",
        google_business_url: "",
        google_reviews_enabled: false,
      });
    } catch (error) {
      console.error("Error removing Google business info:", error);
      showError(tString("errors.removeFailed"));
    } finally {
      setIsUpdating(false);
    }
  };

  const quickActionClass =
    "group flex items-center gap-3 rounded-2xl border border-warm-200/80 bg-white/80 p-3 text-ink-800 shadow-sm shadow-warm-900/5 transition hover:-translate-y-0.5 hover:border-brand/30 hover:bg-brand/5 hover:text-brand-dark hover:shadow-md hover:shadow-brand/10";

  const renderBusinessCard = (business: GooglePlaceResult) => (
    <Card
      key={business.place_id}
      className="cursor-pointer rounded-2xl border border-warm-200/80 bg-white/90 shadow-sm shadow-warm-900/5 transition hover:-translate-y-0.5 hover:border-brand/30 hover:shadow-lg hover:shadow-brand/10"
      isPressable
      onPress={() => handleSelectBusiness(business)}
    >
      <CardBody className="p-4">
        <div className="flex justify-between items-start mb-2">
          <h4 className="font-semibold text-lg text-ink-950">{business.name}</h4>
          {business.rating && (
            <div className="flex items-center gap-1 rounded-full border border-amber-200 bg-amber-50 px-2 py-1 text-amber-800 shadow-sm shadow-amber-900/10">
              <Star className="w-4 h-4 fill-amber-400 text-amber-500" />
              <span className="text-sm font-semibold">{business.rating}</span>
              {business.user_ratings_total && (
                <span className="text-xs text-amber-700">
                  ({business.user_ratings_total})
                </span>
              )}
            </div>
          )}
        </div>

        <div className="flex items-start gap-2 mb-3">
          <MapPin className="w-4 h-4 text-brand mt-0.5 flex-shrink-0" />
          <p className="text-sm text-ink-600">
            {business.formatted_address}
          </p>
        </div>

        {business.types && business.types.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {business.types.slice(0, 3).map((type) => (
              <Chip
                key={type}
                size="sm"
                variant="flat"
                className="bg-warm-100 text-xs font-medium text-ink-700"
              >
                {type.replace(/_/g, " ")}
              </Chip>
            ))}
          </div>
        )}
      </CardBody>
    </Card>
  );

  // If Google business is already configured
  const isGoogleBusinessConnected =
    currentGoogleInfo?.google_place_id &&
    currentGoogleInfo.google_place_id.trim() !== "" &&
    currentGoogleInfo.google_reviews_enabled;

  if (isGoogleBusinessConnected) {
    return (
      <Card className="rounded-3xl border border-warm-200/80 bg-gradient-to-br from-white via-warm-50/80 to-emerald-50/45 shadow-sm shadow-warm-900/5">
        <CardBody className="p-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-2xl border border-emerald-200 bg-emerald-50 shadow-sm shadow-emerald-900/10">
                <Globe className="w-5 h-5 text-emerald-700" />
              </div>
              <h3 className="text-lg font-semibold text-ink-950">
                {tString("connected.title")}
              </h3>
            </div>
            <Chip
              variant="flat"
              size="sm"
              className="border border-emerald-200 bg-emerald-50 text-emerald-800"
            >
              {tString("connected.status")}
            </Chip>
          </div>

          <div className="space-y-4 mb-6">
            <div className="rounded-2xl border border-warm-200/70 bg-white/75 p-4 shadow-sm shadow-warm-900/5">
              <p className="text-sm font-medium text-warm-600 mb-1">
                {tString("connected.businessName")}
              </p>
              <p className="font-semibold text-ink-950">
                {currentGoogleInfo.google_business_name}
              </p>
            </div>

            <div className="space-y-2">
              <p className="text-sm font-medium text-warm-600">
                {tString("connected.quickActions")}
              </p>

              <div className="flex flex-col gap-2">
                <Link
                  href={`https://business.google.com/locations?hl=en`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={quickActionClass}
                >
                  <Settings className="w-4 h-4 text-brand" />
                  <div className="flex-1">
                    <p className="font-semibold text-sm text-ink-900">
                      {tString("connected.actions.manage.title")}
                    </p>
                    <p className="text-xs text-warm-600">
                      {tString("connected.actions.manage.description")}
                    </p>
                  </div>
                  <ExternalLink className="w-3 h-3 text-warm-500 transition group-hover:text-brand" />
                </Link>

                <Link
                  href={currentGoogleInfo.google_review_link}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={quickActionClass}
                >
                  <Star className="w-4 h-4 text-amber-500" />
                  <div className="flex-1">
                    <p className="font-semibold text-sm text-ink-900">
                      {tString("connected.actions.review.title")}
                    </p>
                    <p className="text-xs text-warm-600">
                      {tString("connected.actions.review.description")}
                    </p>
                  </div>
                  <ExternalLink className="w-3 h-3 text-warm-500 transition group-hover:text-brand" />
                </Link>

                <Link
                  href={currentGoogleInfo.google_business_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={quickActionClass}
                >
                  <MapPin className="w-4 h-4 text-brand" />
                  <div className="flex-1">
                    <p className="font-semibold text-sm text-ink-900">
                      {tString("connected.actions.maps.title")}
                    </p>
                    <p className="text-xs text-warm-600">
                      {tString("connected.actions.maps.description")}
                    </p>
                  </div>
                  <ExternalLink className="w-3 h-3 text-warm-500 transition group-hover:text-brand" />
                </Link>
              </div>
            </div>

            <div className="p-3 bg-emerald-50/90 rounded-2xl border border-emerald-200 shadow-sm shadow-emerald-900/10">
              <div className="flex items-center gap-2">
                <Users className="w-4 h-4 text-emerald-700" />
                <div>
                  <p className="font-semibold text-emerald-950 text-sm">
                    {tString("connected.integration.title")}
                  </p>
                  <p className="text-xs text-emerald-700">
                    {tString("connected.integration.description")}
                  </p>
                </div>
              </div>
            </div>
          </div>

          <Button
            color="danger"
            variant="light"
            onPress={handleRemoveIntegration}
            isLoading={isUpdating}
            startContent={<X className="w-4 h-4" />}
            size="sm"
          >
            {tString("connected.removeButton")}
          </Button>
        </CardBody>
      </Card>
    );
  }

  return (
    <>
      <Card className="rounded-3xl border border-warm-200/80 bg-gradient-to-br from-white via-warm-50/75 to-brand/5 shadow-sm shadow-warm-900/5">
        <CardBody className="p-6">
          <div className="flex items-center gap-3 mb-4">
            <div className="flex h-10 w-10 items-center justify-center rounded-2xl border border-brand/20 bg-brand/10 shadow-sm shadow-brand/10">
              <Globe className="w-5 h-5 text-brand" />
            </div>
            <h3 className="text-lg font-semibold text-ink-950">{tString("search.title")}</h3>
          </div>

          <p className="text-sm text-ink-600 mb-4">
            {tString("search.description")}
          </p>

          <div className="space-y-3 mb-4">
            <Input
              label={tString("search.inputLabel")}
              placeholder={tString("search.inputPlaceholder")}
              value={searchQuery}
              onChange={(e) => handleSearchInputChange(e.target.value)}
              onKeyPress={(e) => e.key === "Enter" && handleSearch()}
              startContent={<Search className="w-4 h-4 text-warm-500" />}
              description={tString("search.inputDescription")}
              classNames={{
                inputWrapper:
                  "border-warm-200 bg-white/90 shadow-sm group-data-[focus=true]:border-brand",
                label: "text-ink-700",
                input: "text-ink-900 placeholder:text-warm-500",
                description: "text-warm-600",
              }}
            />

            <Button
              onPress={handleSearch}
              isLoading={isSearching}
              className="w-full rounded-xl bg-brand font-semibold text-white shadow-sm shadow-brand/20 transition hover:-translate-y-px hover:bg-brand-dark"
            >
              {tString("search.searchButton")}
            </Button>

            <div className="text-xs text-warm-700 space-y-1 rounded-2xl border border-warm-200/70 bg-white/70 p-3">
              <p>
                <strong className="text-ink-800">{tString("search.tips.title")}</strong>
              </p>
              <ul className="list-disc list-inside space-y-0.5 ml-2">
                <li>{tString("search.tips.tip1")}</li>
                <li>{tString("search.tips.tip2")}</li>
                <li>{tString("search.tips.tip3")}</li>
                <li>{tString("search.tips.tip4")}</li>
              </ul>
            </div>
          </div>

          {searchResults.length > 0 && (
            <>
              <Divider className="my-4" />
              <div className="space-y-3">
                <h4 className="font-semibold text-ink-900">
                  {tString("search.selectBusiness")}
                </h4>
                {searchResults.map(renderBusinessCard)}
              </div>
            </>
          )}

          <div className="mt-4 p-3 bg-brand/10 rounded-2xl border border-brand/20 shadow-sm shadow-brand/10">
            <div className="flex items-start gap-2">
              <AlertTriangle className="w-4 h-4 text-brand mt-0.5 flex-shrink-0" />
              <div className="text-sm text-brand-dark">
                <p className="font-medium mb-1">
                  {tString("search.noProfile.title")}
                </p>
                <p>
                  {tString("search.noProfile.description")}{" "}
                  <Link
                    href="https://business.google.com"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="underline"
                  >
                    business.google.com
                  </Link>{" "}
                  {tString("search.noProfile.suffix")}
                </p>
              </div>
            </div>
          </div>
        </CardBody>
      </Card>

      {/* Confirmation Modal */}
      <Modal isOpen={isOpen} onClose={onClose}>
        <ModalContent className="rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          <ModalHeader>{tString("modal.title")}</ModalHeader>
          <ModalBody>
            {selectedBusiness && (
              <div className="space-y-4">
                <p className="text-ink-700">{tString("modal.confirmText")}</p>

                <Card className="rounded-2xl border border-warm-200/80 bg-warm-50/70 shadow-sm shadow-warm-900/5">
                  <CardBody className="p-4">
                    <h4 className="font-semibold text-lg mb-2 text-ink-950">
                      {selectedBusiness.name}
                    </h4>
                    <div className="flex items-start gap-2 mb-2">
                      <MapPin className="w-4 h-4 text-brand mt-0.5 flex-shrink-0" />
                      <p className="text-sm text-ink-600">
                        {selectedBusiness.formatted_address}
                      </p>
                    </div>
                    {selectedBusiness.rating && (
                      <div className="flex items-center gap-1 text-amber-800">
                        <Star className="w-4 h-4 fill-amber-400 text-amber-500" />
                        <span className="text-sm font-semibold">
                          {selectedBusiness.rating}
                        </span>
                        {selectedBusiness.user_ratings_total && (
                          <span className="text-xs text-amber-700">
                            {tString("modal.reviewsCount").replace(
                              "{count}",
                              String(selectedBusiness.user_ratings_total),
                            )}
                          </span>
                        )}
                      </div>
                    )}
                  </CardBody>
                </Card>

                <p className="text-sm text-ink-600">
                  {tString("modal.description")}
                </p>
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onClose}>
              {tString("modal.cancelButton")}
            </Button>
            <Button
              onPress={handleConfirmSelection}
              isLoading={isUpdating}
              startContent={<Check className="w-4 h-4" />}
              className="rounded-xl bg-brand font-semibold text-white shadow-sm shadow-brand/20 transition hover:bg-brand-dark"
            >
              {tString("modal.confirmButton")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </>
  );
}
