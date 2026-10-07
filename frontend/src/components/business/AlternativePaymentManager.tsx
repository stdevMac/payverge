"use client";

import React, { useState, useEffect } from "react";
import { sectionHeadingClass } from "@/components/ui/headingStyles";
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Input,
  Select,
  SelectItem,
  Chip,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
  Progress,
} from "@nextui-org/react";
import { DecimalInput } from "@/components/ui/DecimalInput";
import {
  PaymentMethod,
  amountMicroToDollars,
  asMoneyString,
  formatUSDCAmount,
  getPaymentMethodIcon,
  normalizeMoneyInput,
  translatePaymentMethodLabel,
} from "@/types/alternativePayments";
import {
  recordPaymentMethodI18nSuffix,
  recordPaymentTenderOptions,
} from "@/lib/tenderOptions";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { useBusinessAlternativePayments } from "@/hooks/useAlternativePayments";
import { toast } from "react-hot-toast";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import {
  alternativePaymentRequestIsExpired,
  formatAlternativePaymentRequestAge,
  formatAlternativePaymentRequestedAt,
} from "@/lib/alternativePaymentExpiry";

interface AlternativePaymentManagerProps {
  billId: string;
  billTotal: number;
  currency?: string;
  /** Business address / fiscal country — used to hide US-only tenders. */
  country?: string | null;
  /**
   * Fired after a payment is marked/confirmed. `isComplete` reflects the fresh
   * breakdown so callers can redirect only once the bill is fully settled
   * instead of after the first of several pending payments (LOW).
   */
  onPaymentMarked?: (isComplete: boolean) => void;
}

const AlternativePaymentManager = React.memo(
  function AlternativePaymentManager({
    billId,
    billTotal,
    currency = "USD",
    country = null,
    onPaymentMarked,
  }: AlternativePaymentManagerProps) {
    const { locale } = useSimpleLocale();
    const [currentLocale, setCurrentLocale] = useState(locale);

    // Update translations when locale changes
    useEffect(() => {
      setCurrentLocale(locale);
    }, [locale]);

    // Translation helper
    const tString = (key: string): string => {
      const fullKey = `alternativePaymentManager.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    };

    // Translate a payment-method label through the paymentMethods.* keys so the
    // dropdown + pending rows aren't stuck on the hardcoded English labels in
    // PAYMENT_METHOD_OPTIONS (R3-BP-4).
    const methodLabel = (method: PaymentMethod): string => {
      const suffix = recordPaymentMethodI18nSuffix(
        method,
        currentLocale,
        country,
      );
      const translated = tString(`paymentMethods.${suffix}`);
      if (translated && translated !== `paymentMethods.${suffix}`) {
        return translated;
      }
      return translatePaymentMethodLabel(method, (key) =>
        tString(`paymentMethods.${key}`),
      );
    };

    // Use the custom hook for real-time data
    const {
      paymentBreakdown,
      pendingPayments,
      loading,
      error: _error,
      markPayment,
      rejectPayment,
    } = useBusinessAlternativePayments(billId);

    const [markingPayment, setMarkingPayment] = useState(false);
    // Per-row confirm spinner id so one "Confirm" click doesn't spin every
    // pending row's button at once (LOW). Null for the manual-entry submit.
    const [confirmingId, setConfirmingId] = useState<string | null>(null);
    const [rejectingId, setRejectingId] = useState<string | null>(null);

    // Manual payment form state
    const [participantName, setParticipantName] = useState("");
    const [amount, setAmount] = useState("");
    const [selectedMethod, setSelectedMethod] = useState<PaymentMethod>(
      PaymentMethod.CASH,
    );

    const { isOpen, onOpen, onClose } = useDisclosure();

    const handleMarkPayment = async (
      paymentAmount: string,
      method: PaymentMethod,
      requestId?: string,
      name?: string,
    ) => {
      if (requestId) {
        const pending = pendingPayments.find((row) => row.id === requestId);
        if (pending && alternativePaymentRequestIsExpired(pending)) {
          toast.error(tString("pendingPayments.expired"));
          return;
        }
        setConfirmingId(requestId);
      } else {
        setMarkingPayment(true);
      }
      try {
        const response = await markPayment(
          undefined,
          asMoneyString(paymentAmount),
          method,
          requestId,
          { participantName: name?.trim() || billId },
        );

        if (response.success) {
          toast.success(tString("messages.success"));
          onPaymentMarked?.(Boolean(response.paymentBreakdown?.isComplete));

          if (!requestId) {
            setParticipantName("");
            setAmount("");
            onClose();
          }
        } else {
          toast.error(response.message || tString("messages.error"));
        }
      } catch (error) {
        console.error("Error marking payment:", error);
        toast.error(tString("messages.error"));
      } finally {
        if (requestId) {
          setConfirmingId(null);
        } else {
          setMarkingPayment(false);
        }
      }
    };

    const handleManualPaymentSubmit = () => {
      if (!amount.trim()) {
        toast.error(tString("messages.validation.amountRequired"));
        return;
      }

      // Normalize to a 2-decimal string (accepting comma decimals) so the
      // validation gate agrees with the asMoneyString submit path — otherwise
      // "10,50"/"10.999"/".50" passed a parseFloat check then threw a generic
      // failure at submit time (R3-BP-6).
      const normalized = normalizeMoneyInput(amount);
      if (normalized === null) {
        toast.error(tString("messages.validation.amountInvalid"));
        return;
      }

      const numAmount = Number(normalized);
      if (numAmount > getRemainingAmount()) {
        toast.error(tString("messages.validation.amountTooHigh"));
        return;
      }

      handleMarkPayment(normalized, selectedMethod, undefined, participantName);
    };

    const handleRejectPayment = async (requestId: string) => {
      setRejectingId(requestId);
      try {
        const response = await rejectPayment(requestId);
        if (response.success) {
          toast.success(tString("messages.rejected"));
        } else {
          toast.error(tString("messages.rejectError"));
        }
      } catch {
        toast.error(tString("messages.rejectError"));
      } finally {
        setRejectingId(null);
      }
    };

    const getPaymentProgress = () => {
      if (!paymentBreakdown) return 0;

      const total = parseFloat(paymentBreakdown.totalAmount) / 1_000_000;
      const paid =
        (parseFloat(paymentBreakdown.cryptoPaid) +
          parseFloat(paymentBreakdown.alternativePaid)) /
        1_000_000;

      return total > 0 ? (paid / total) * 100 : 0;
    };

    const formatMoney = (microAmount: string) =>
      formatCurrencyIntl(
        amountMicroToDollars(microAmount),
        currency,
        undefined,
        currentLocale,
      );

    const getRemainingAmount = () => {
      if (!paymentBreakdown) return billTotal;
      return amountMicroToDollars(paymentBreakdown.remaining);
    };

    if (loading) {
      return (
        <Card className="border border-warm-200 bg-white/95 shadow-sm shadow-warm-900/5">
          <CardBody className="flex items-center justify-center p-8">
            <div className="text-center">
              <div className="mx-auto mb-4 h-8 w-8 animate-spin rounded-full border-b-2 border-brand"></div>
              <p className="text-sm font-medium text-ink-600">
                {tString("messages.loading")}
              </p>
            </div>
          </CardBody>
        </Card>
      );
    }

    return (
      <div className="space-y-6">
        {/* Payment Overview */}
        <Card className="border border-warm-200 bg-white/95 shadow-sm shadow-warm-900/5">
          <CardHeader>
            <div className="flex justify-between items-center w-full">
              <h3 className={sectionHeadingClass}>{tString("title")}</h3>
              {paymentBreakdown?.isComplete && (
                <Chip
                  variant="flat"
                  className="bg-emerald-50 font-medium text-emerald-700"
                >
                  {tString("status.complete")}
                </Chip>
              )}
            </div>
          </CardHeader>
          <CardBody>
            {paymentBreakdown && (
              <div className="space-y-4">
                <Progress
                  value={getPaymentProgress()}
                  color={paymentBreakdown.isComplete ? "success" : "primary"}
                  className="w-full"
                />

                <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                  <div>
                    <p className="text-xs font-medium text-ink-500">
                      {tString("breakdown.total")}
                    </p>
                    <p className="font-semibold text-ink-900">
                      {formatMoney(paymentBreakdown.totalAmount)}
                    </p>
                  </div>
                  <div>
                    <p className="text-xs font-medium text-ink-500">
                      {tString("breakdown.cryptoPaid")}
                    </p>
                    <p className="font-semibold text-emerald-700">
                      {formatMoney(paymentBreakdown.cryptoPaid)}
                    </p>
                  </div>
                  <div>
                    <p className="text-xs font-medium text-ink-500">
                      {tString("breakdown.alternativePaid")}
                    </p>
                    <p className="font-semibold text-brand">
                      {formatMoney(paymentBreakdown.alternativePaid)}
                    </p>
                  </div>
                  <div>
                    <p className="text-xs font-medium text-ink-500">
                      {tString("breakdown.remaining")}
                    </p>
                    <p className="font-semibold text-amber-700">
                      {formatMoney(paymentBreakdown.remaining)}
                    </p>
                  </div>
                </div>
              </div>
            )}
          </CardBody>
        </Card>

        {/* Pending Payments */}
        {pendingPayments.length > 0 && (
          <Card className="border border-warm-200 bg-white/95 shadow-sm shadow-warm-900/5">
            <CardHeader>
              <h3 className={sectionHeadingClass}>
                {tString("pendingPayments.title")}
              </h3>
            </CardHeader>
            <CardBody>
              <div className="space-y-3">
                {pendingPayments.map((payment) => {
                  const expired = alternativePaymentRequestIsExpired(payment);
                  const requestedAt = formatAlternativePaymentRequestedAt(
                    payment.timestamp,
                    locale,
                  );
                  const age = formatAlternativePaymentRequestAge(
                    payment.timestamp,
                  );
                  const requestRef = tString("pendingPayments.requestRef").replace(
                    "{id}",
                    String(payment.id),
                  );
                  const rejectLabel = `${tString("buttons.rejectPayment")} — ${requestRef}`;
                  const confirmLabel = `${tString("buttons.confirmPayment")} — ${requestRef}`;
                  return (
                  <div
                    key={payment.id}
                    className={`flex items-center justify-between rounded-xl border p-3 transition-colors ${
                      expired
                        ? "border-rose-200 bg-rose-50"
                        : "border-warm-200 bg-warm-50/40 hover:bg-white"
                    }`}
                  >
                    <div className="flex items-center space-x-3">
                      <span className="flex h-10 w-10 items-center justify-center rounded-full bg-white text-2xl shadow-sm shadow-warm-900/5">
                        {getPaymentMethodIcon(payment.paymentMethod)}
                      </span>
                      <div>
                        <p className="font-medium text-ink-900">
                          {payment.participantName ||
                            `${payment.participantAddress.slice(0, 6)}...${payment.participantAddress.slice(-4)}`}
                        </p>
                        <p className="text-sm text-ink-600">
                          {formatMoney(payment.amount)}{" "}
                          {tString("pendingPayments.via")}{" "}
                          {methodLabel(payment.paymentMethod)}
                        </p>
                        <p className="mt-1 text-xs font-medium text-ink-700">
                          {requestRef}
                        </p>
                        {requestedAt ? (
                          <p className="mt-1 text-xs text-ink-500">
                            {tString("pendingPayments.requestedAt").replace(
                              "{time}",
                              requestedAt,
                            )}{" "}
                            · {age}
                          </p>
                        ) : null}
                        {expired ? (
                          <p
                            className="mt-1 text-xs font-semibold text-rose-700"
                            data-testid="pending-payment-expired"
                          >
                            {tString("pendingPayments.expired")}
                          </p>
                        ) : null}
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <Button
                        variant="flat"
                        size="sm"
                        className="text-rose-700"
                        isLoading={rejectingId === payment.id}
                        isDisabled={
                          markingPayment ||
                          confirmingId !== null ||
                          (rejectingId !== null && rejectingId !== payment.id)
                        }
                        aria-label={rejectLabel}
                        onPress={() => handleRejectPayment(payment.id)}
                      >
                        {tString("buttons.rejectPayment")}
                      </Button>
                      <Button
                        color="primary"
                        size="sm"
                        aria-label={confirmLabel}
                        isLoading={confirmingId === payment.id}
                        isDisabled={
                          expired ||
                          markingPayment ||
                          rejectingId !== null ||
                          (confirmingId !== null && confirmingId !== payment.id)
                        }
                        onPress={() =>
                          handleMarkPayment(
                            formatUSDCAmount(payment.amount),
                            payment.paymentMethod,
                            payment.id,
                            payment.participantName,
                          )
                        }
                      >
                        {tString("buttons.confirmPayment")}
                      </Button>
                    </div>
                  </div>
                  );
                })}
              </div>
            </CardBody>
          </Card>
        )}

        {/* Manual Payment Entry */}
        <Card className="border border-warm-200 bg-white/95 shadow-sm shadow-warm-900/5">
          <CardHeader>
            <div className="flex justify-between items-center w-full">
              <h3 className={sectionHeadingClass}>
                {tString("manualPayment.title")}
              </h3>
              <Button color="primary" onPress={onOpen}>
                {tString("buttons.addPayment")}
              </Button>
            </div>
          </CardHeader>
          <CardBody>
            <p className="text-sm leading-6 text-ink-600">
              {tString("manualPayment.description")}
            </p>
          </CardBody>
        </Card>

        {/* Manual Payment Modal */}
        <Modal isOpen={isOpen} onClose={onClose} size="lg">
          <ModalContent>
            <ModalHeader>{tString("modal.title")}</ModalHeader>
            <ModalBody>
              <div className="space-y-4">
                <Input
                  label={tString("modal.fields.note.label")}
                  placeholder={tString("modal.fields.note.placeholder")}
                  value={participantName}
                  onValueChange={setParticipantName}
                  description={tString("modal.fields.note.description")}
                />

                <DecimalInput
                  label={tString("fields.amount.label")}
                  placeholder={tString("fields.amount.placeholder")}
                  min={0}
                  step={0.01}
                  maxFractionDigits={2}
                  value={amount}
                  onValueChange={setAmount}
                  isInvalid={
                    amount.trim() !== "" &&
                    normalizeMoneyInput(amount) === null
                  }
                  startContent={
                    <span className="text-ink-500">
                      {formatCurrencyIntl(0, currency)
                        .replace(/[\d.,\s]/g, "")
                        .trim() || "$"}
                    </span>
                  }
                  data-testid="alt-payment-amount"
                />

                <Select
                  label={tString("fields.paymentMethod.label")}
                  selectedKeys={[selectedMethod.toString()]}
                  onSelectionChange={(keys) => {
                    const key = Array.from(keys)[0] as string;
                    setSelectedMethod(parseInt(key, 10) as PaymentMethod);
                  }}
                >
                  {recordPaymentTenderOptions(currentLocale, country).map(
                    (option) => {
                    const label = methodLabel(option.value);
                    return (
                      <SelectItem
                        key={option.value.toString()}
                        value={option.value.toString()}
                        textValue={label}
                      >
                        <div className="flex items-center space-x-2">
                          <span>{option.icon}</span>
                          <span>{label}</span>
                        </div>
                      </SelectItem>
                    );
                  })}
                </Select>

                <div className="rounded-xl border border-brand/20 bg-brand/10 p-3">
                  <p className="text-sm text-brand-dark">
                    <strong>{tString("modal.warning.title")}:</strong>{" "}
                    {tString("modal.warning.message")}
                  </p>
                </div>
              </div>
            </ModalBody>
            <ModalFooter>
              <Button variant="light" onPress={onClose}>
                {tString("buttons.cancel")}
              </Button>
              <Button
                color="primary"
                isLoading={markingPayment}
                onPress={handleManualPaymentSubmit}
              >
                {tString("buttons.markPaid")}
              </Button>
            </ModalFooter>
          </ModalContent>
        </Modal>
      </div>
    );
  },
);

export default AlternativePaymentManager;
