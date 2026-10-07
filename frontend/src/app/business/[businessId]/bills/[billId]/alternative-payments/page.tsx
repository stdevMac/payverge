"use client";

import React, { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Breadcrumbs,
  BreadcrumbItem,
} from "@nextui-org/react";
import {
  ArrowLeft,
  Receipt,
  CreditCard,
  Banknote,
  Smartphone,
  Repeat,
} from "lucide-react";
import Link from "next/link";
import AlternativePaymentManager from "@/components/business/AlternativePaymentManager";
import { getBusiness } from "@/api/business";
import { resolveNumericBusinessId } from "@/utils/resolveBusinessId";
import { getRouteParam } from "@/utils/nextRouteParams";
import { getOperatorDashboardPath } from "@/utils/businessUrl";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { PaymentMethod } from "@/types/alternativePayments";
import {
  recordPaymentMethodI18nSuffix,
  recordPaymentTenderOptions,
} from "@/lib/tenderOptions";

export default function AlternativePaymentsPage() {
  const params = useParams();
  const businessId = getRouteParam(params, "businessId");
  const billId = getRouteParam(params, "billId");
  const { locale } = useSimpleLocale();
  const [currency, setCurrency] = useState("USD");
  const [country, setCountry] = useState<string | null>(null);

  useEffect(() => {
    // The route param may be a numeric id OR a slug (e.g. "demo-admin-1-ai-pro").
    // `Number(slug)` → NaN → /businesses/NaN → 404. Resolve to a numeric id first
    // (numerics short-circuit without an extra call), then fetch the currency.
    if (!businessId) return;
    let cancelled = false;
    resolveNumericBusinessId(businessId)
      .then((id) => {
        if (cancelled || id === null) return;
        return getBusiness(id).then((biz) => {
          if (cancelled || !biz) return;
          if (biz.default_currency) setCurrency(biz.default_currency);
          setCountry(biz.address?.country ?? null);
        });
      })
      .catch(() => {
        // Best-effort — keep USD fallback.
      });
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  const t = (key: string, params?: Record<string, string | number>): string => {
    const result = getTranslation(
      `alternativePaymentManager.page.${key}`,
      locale,
      params,
    );
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  // `/business/:id` is the public custom-URL protocol shim (308 → /b/:slug
  // or 404). Operator chrome must stay on /dashboard.
  const dashboardPath = getOperatorDashboardPath(businessId);
  const billsPath = getOperatorDashboardPath(businessId, "bills");

  return (
    <div className="min-h-screen bg-warm-50">
      <div className="relative z-10">
        {/* Header */}
        <div className="bg-white/80 backdrop-blur-xl border-b border-gray-200 sticky top-0 z-20">
          <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
            <div className="flex items-center justify-between h-16">
              <div className="flex items-center space-x-4">
                <Link href={billsPath}>
                  <Button
                    variant="light"
                    size="sm"
                    startContent={<ArrowLeft className="w-4 h-4" />}
                  >
                    {t("backToBills")}
                  </Button>
                </Link>

                <Breadcrumbs>
                  <BreadcrumbItem>
                    <Link href={dashboardPath}>
                      {t("breadcrumbs.business")}
                    </Link>
                  </BreadcrumbItem>
                  <BreadcrumbItem>
                    <Link href={billsPath}>{t("breadcrumbs.bills")}</Link>
                  </BreadcrumbItem>
                  <BreadcrumbItem>
                    {t("breadcrumbs.bill", { billId })}
                  </BreadcrumbItem>
                  <BreadcrumbItem>
                    {t("breadcrumbs.alternativePayments")}
                  </BreadcrumbItem>
                </Breadcrumbs>
              </div>

              <div className="flex items-center space-x-2">
                <Receipt className="w-5 h-5 text-gray-700" />
                <span className="text-sm font-medium text-gray-700">
                  {t("paymentManagement")}
                </span>
              </div>
            </div>
          </div>
        </div>

        {/* Main Content */}
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
          <div className="mb-8">
            <div className="flex items-center space-x-3 mb-2">
              <CreditCard className="w-8 h-8 text-brand" />
              <h1 className="text-3xl text-gray-900 tracking-wide">
                {t("title")}
              </h1>
            </div>
            <p className="text-gray-700">{t("subtitle")}</p>
          </div>

          {/* Alternative Payment Manager */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
            <div className="lg:col-span-2">
              <AlternativePaymentManager
                billId={billId}
                currency={currency}
                country={country}
                // No placeholder total: AlternativePaymentManager derives the
                // remaining/validation cap from the live payment breakdown it
                // fetches for `billId`. A non-zero literal here would act as a
                // false validation bound before the breakdown loads.
                billTotal={0}
                onPaymentMarked={(isComplete) => {
                  // Only leave once the bill is fully settled. Confirming ONE of
                  // several pending payments used to hard-navigate away while
                  // money was still outstanding (LOW); otherwise stay and let
                  // the manager's polling query refresh the remaining rows.
                  if (isComplete) {
                    window.location.href = billsPath;
                  }
                }}
              />
            </div>

            {/* Sidebar with Instructions */}
            <div className="space-y-6">
              <Card>
                <CardHeader>
                  <h3 className="text-lg font-semibold">
                    {t("howItWorks.title")}
                  </h3>
                </CardHeader>
                <CardBody className="space-y-4">
                  <div className="space-y-3 text-sm">
                    <div className="flex items-start space-x-3">
                      <div className="w-6 h-6 bg-brand/10 text-brand rounded-full flex items-center justify-center text-xs font-semibold flex-shrink-0">
                        1
                      </div>
                      <p>{t("howItWorks.step1")}</p>
                    </div>

                    <div className="flex items-start space-x-3">
                      <div className="w-6 h-6 bg-brand/10 text-brand rounded-full flex items-center justify-center text-xs font-semibold flex-shrink-0">
                        2
                      </div>
                      <p>{t("howItWorks.step2")}</p>
                    </div>

                    <div className="flex items-start space-x-3">
                      <div className="w-6 h-6 bg-brand/10 text-brand rounded-full flex items-center justify-center text-xs font-semibold flex-shrink-0">
                        3
                      </div>
                      <p>{t("howItWorks.step3")}</p>
                    </div>

                    <div className="flex items-start space-x-3">
                      <div className="w-6 h-6 bg-brand/10 text-brand rounded-full flex items-center justify-center text-xs font-semibold flex-shrink-0">
                        4
                      </div>
                      <p>{t("howItWorks.step4")}</p>
                    </div>
                  </div>
                </CardBody>
              </Card>

              <Card>
                <CardHeader>
                  <h3 className="text-lg font-semibold">
                    {t("paymentMethods.title")}
                  </h3>
                </CardHeader>
                <CardBody>
                  <div className="space-y-3">
                    {recordPaymentTenderOptions(locale, country).map((opt) => {
                      const suffix = recordPaymentMethodI18nSuffix(
                        opt.value,
                        locale,
                        country,
                      );
                      const labelResult = getTranslation(
                        `alternativePaymentManager.paymentMethods.${suffix}`,
                        locale,
                      );
                      const label = Array.isArray(labelResult)
                        ? labelResult[0] || suffix
                        : (labelResult as string);
                      const Icon =
                        opt.value === PaymentMethod.CASH
                          ? Banknote
                          : opt.value === PaymentMethod.CARD
                            ? CreditCard
                            : opt.value === PaymentMethod.VENMO
                              ? Smartphone
                              : Repeat;
                      return (
                        <div
                          key={opt.value}
                          className="flex items-center space-x-3"
                        >
                          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-warm-50 text-ink-700">
                            <Icon className="h-5 w-5" strokeWidth={1.75} />
                          </span>
                          <span className="text-sm">{label}</span>
                        </div>
                      );
                    })}
                  </div>
                </CardBody>
              </Card>

              <Card className="bg-green-50 border-green-200">
                <CardBody>
                  <div className="text-center">
                    <div className="text-green-600 mb-2">
                      <Receipt className="w-8 h-8 mx-auto" />
                    </div>
                    <h4 className="font-semibold text-green-800 mb-1">
                      {t("hybrid.title")}
                    </h4>
                    <p className="text-sm text-green-700">
                      {t("hybrid.description")}
                    </p>
                  </div>
                </CardBody>
              </Card>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
