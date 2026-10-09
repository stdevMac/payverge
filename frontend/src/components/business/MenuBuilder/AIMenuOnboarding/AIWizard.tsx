"use client";

import React, { useState, useRef, useEffect } from "react";
import {
  Button,
  Card,
  CardBody,
  Input,
  Progress,
  Avatar,
} from "@nextui-org/react";
import {
  Send,
  Sparkles,
  Bot,
  User,
  AlertCircle,
  ChevronRight,
} from "lucide-react";
import { motion, AnimatePresence } from "framer-motion";
import {
  startWizardSession,
  sendWizardMessage,
  generateMenuFromWizard,
  GeneratedMenu,
  WizardResponse,
} from "@/api/business";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import {
  errMessage,
  getApiErrorCode,
  getApiErrorStatus,
  isApiNetworkError,
} from "@/utils/apiError";

// #598: wizard replies routinely take ~150s, so they can outlive proxy hops
// (Caddy/Cloudflare 502s without CORS headers surface as network errors in the
// browser) or the request timeout. In every one of those cases the backend has
// already persisted the user's message and keeps preparing the reply — a
// `retry: true` resend recovers it without re-typing. Classify those failures
// so the UI offers that retry instead of a dead-end "check your connection".
// A structured backend error code means the request DID land and was rejected
// on purpose — never treat that as a gateway failure.
const isGatewayOrNetworkFailure = (err: unknown): boolean => {
  if (getApiErrorCode(err)) return false;
  if (isApiNetworkError(err)) return true; // timeout / offline / CORS-masked 502
  const status = getApiErrorStatus(err);
  return status !== undefined && status >= 500;
};

interface AIWizardProps {
  businessId: number;
  onMenuGenerated: (menu: GeneratedMenu) => void;
}

interface Message {
  role: "user" | "assistant";
  content: string;
  suggestedOptions?: string[];
}

type WizardStep = "idle" | "chatting" | "generating" | "complete";

export default function AIWizard({
  businessId,
  onMenuGenerated,
}: AIWizardProps) {
  const [sessionId, setSessionId] = useState<number | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [step, setStep] = useState<WizardStep>("idle");
  const [isLoading, setIsLoading] = useState(false);
  const [isComplete, setIsComplete] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [retryPending, setRetryPending] = useState(false);
  // F12: state-backed index so the "analyzing → crafting → pricing →
  // finalizing" narration actually rotates (the component otherwise never
  // re-renders during the awaited generation call).
  const [generatingStepIndex, setGeneratingStepIndex] = useState(0);
  const chatContainerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  // F10/F11: lets the operator abandon a hung generation. We can't pass an
  // AbortSignal into the shared api client (out-of-lane file), so we guard the
  // result with a generation id: a Cancel bumps the id, and any in-flight
  // promise that resolves with a stale id is ignored.
  const generationIdRef = useRef(0);
  const { locale } = useSimpleLocale();
  const t = (key: string, params?: Record<string, string | number>) =>
    getTranslation(`aiMenuOnboarding.wizard.${key}`, locale, params);

  // Scroll only the chat container — not the whole page.
  // scrollIntoView() bubbles up scrollable ancestors and was yanking the page on each send.
  const scrollChatToBottom = () => {
    const el = chatContainerRef.current;
    if (!el) return;
    el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  };

  const focusInput = () => {
    inputRef.current?.focus();
  };

  useEffect(() => {
    scrollChatToBottom();
  }, [messages, isLoading]);

  // Keep the cursor in the input through the whole conversation flow.
  useEffect(() => {
    if (step === "chatting" && !isLoading) {
      focusInput();
    }
  }, [step, isLoading]);

  // F12: rotate the generation step label every 2s while generating. Without
  // a state update nothing re-renders mid-await, so the inline Date.now()
  // expression used to freeze on whatever phase was active at kickoff.
  useEffect(() => {
    if (step !== "generating") return;
    setGeneratingStepIndex(0);
    const id = setInterval(() => {
      setGeneratingStepIndex((prev) => (prev + 1) % 4);
    }, 2000);
    return () => clearInterval(id);
  }, [step]);

  const startSession = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const response = await startWizardSession(businessId, locale);
      setSessionId(response.session_id);
      setMessages([
        {
          role: "assistant",
          content: response.response.message,
          suggestedOptions: response.response.suggested_options,
        },
      ]);
      setStep("chatting");
    } catch (err: unknown) {
      setError(errMessage(err) || (t("errors.startSession") as string));
    } finally {
      setIsLoading(false);
    }
  };

  const appendAssistant = (response: WizardResponse) => {
    setMessages((prev) => [
      ...prev,
      {
        role: "assistant",
        content: response.message,
        suggestedOptions: response.suggested_options,
      },
    ]);
    if (response.is_complete) setIsComplete(true);
    setRetryPending(false);
    setError(null);
  };

  const retryFailedResponse = async () => {
    if (!sessionId) return;
    setIsLoading(true);
    try {
      const response = await sendWizardMessage(
        businessId,
        sessionId,
        "",
        locale,
        { retry: true },
      );
      appendAssistant(response.response);
    } catch (err: unknown) {
      // Keep the retry affordance up (retryPending is untouched) and pick the
      // copy for what actually happened: a gateway/network failure means the
      // reply is still being prepared server-side, so trying again later is
      // the right move; anything else keeps the structured-output copy.
      setError(
        isGatewayOrNetworkFailure(err)
          ? (t("errors.replyDelayed") as string)
          : (t("errors.structuredOutput") as string),
      );
    } finally {
      setIsLoading(false);
    }
  };

  const handleSend = async (messageText?: string) => {
    const text = messageText || input;
    if (!text.trim() || !sessionId) return;

    const userMessage: Message = { role: "user", content: text };
    setMessages((prev) => [...prev, userMessage]);
    setInput("");
    setIsLoading(true);
    setError(null);
    setRetryPending(false);

    try {
      const response = await sendWizardMessage(
        businessId,
        sessionId,
        text,
        locale,
      );
      appendAssistant(response.response);
    } catch (err: unknown) {
      if (getApiErrorCode(err) === "ai_structured_output_invalid") {
        setRetryPending(true);
        setError(t("errors.structuredOutput") as string);
      } else if (isGatewayOrNetworkFailure(err)) {
        // #598: the proxy/browser gave up but the backend persisted the
        // message and keeps preparing the reply. Offer the same retry seam
        // (`retry: true`) instead of a dead-end connection error.
        setRetryPending(true);
        setError(t("errors.replyDelayed") as string);
      } else {
        setError(errMessage(err) || (t("errors.sendMessage") as string));
      }
    } finally {
      setIsLoading(false);
    }
  };

  const handleGenerateMenu = async () => {
    if (!sessionId) return;

    const myGenerationId = ++generationIdRef.current;
    setError(null);
    setStep("generating");
    try {
      const response = await generateMenuFromWizard(businessId, sessionId);
      // Ignore a stale result if the operator cancelled and (possibly)
      // restarted generation while this call was in flight.
      if (generationIdRef.current !== myGenerationId) return;
      setStep("complete");
      onMenuGenerated(response.menu);
    } catch (err: unknown) {
      if (generationIdRef.current !== myGenerationId) return;
      // Retry-specific copy: the Generate Menu button stays visible
      // (isComplete is untouched), so guide the operator to press it again.
      setError(errMessage(err) || (t("errors.generateRetry") as string));
      setStep("chatting");
    }
  };

  // F10: abandon a slow/hung generation. We bump the generation id so the
  // in-flight promise's result is discarded, then return to the chat with a
  // retry-able message. The Generate Menu button remains (isComplete stays
  // true) so the operator can immediately try again.
  const handleCancelGeneration = () => {
    generationIdRef.current++;
    setStep("chatting");
    setError(t("errors.generationCancelled") as string);
    focusInput();
  };


  // Idle state - Start session
  if (step === "idle") {
    return (
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: "spring", damping: 25, stiffness: 200 }}
        className="w-full"
      >
        <Card className="group overflow-hidden rounded-3xl border border-warm-200 bg-white/85 shadow-sm shadow-warm-900/5 backdrop-blur-xl">
          <CardBody className="p-10 text-center flex flex-col items-center">
            <motion.div
              className="w-20 h-20 mb-6 relative"
              whileHover={{ scale: 1.1 }}
            >
              <div className="absolute inset-0 bg-brand/20 blur-2xl rounded-full scale-150 motion-safe:animate-pulse" />
              <div className="relative w-20 h-20 bg-gradient-to-br from-brand to-brand-dark rounded-3xl flex items-center justify-center shadow-2xl shadow-brand/30">
                <Sparkles className="w-10 h-10 text-white" />
              </div>
            </motion.div>

            <h3 className="text-2xl font-bold mb-4 text-brand-dark">
              {t("title")}
            </h3>
            <p className="mb-8 max-w-lg mx-auto leading-relaxed text-ink-600">
              {t("descriptionPro")}
            </p>

            <Button
              size="lg"
              className="h-14 rounded-2xl bg-brand px-10 font-bold text-white shadow-xl shadow-brand/20 transition-all duration-500 hover:bg-brand-dark hover:shadow-brand/40"
              onPress={startSession}
              isLoading={isLoading}
            >
              {t("startConversation")}
            </Button>
          </CardBody>
        </Card>
      </motion.div>
    );
  }

  return (
    <div className="flex h-[600px] max-h-[90dvh] flex-col overflow-hidden rounded-3xl border border-warm-200 bg-white/90 shadow-2xl shadow-warm-900/15 backdrop-blur-xl sm:h-[650px]">
      {/* Chat Area */}
      <div
        ref={chatContainerRef}
        className="flex-1 overflow-y-auto px-4 py-6 space-y-6 scrollbar-thin scrollbar-thumb-warm-300"
      >
        <AnimatePresence mode="popLayout">
          {messages.map((message, index) => (
            <motion.div
              key={`${index}-${message.role}`}
              initial={{
                opacity: 0,
                x: message.role === "user" ? 20 : -20,
                y: 10,
              }}
              animate={{ opacity: 1, x: 0, y: 0 }}
              transition={{
                type: "spring",
                damping: 25,
                stiffness: 200,
                bounce: 0.2,
              }}
              className={`flex gap-3 sm:gap-4 ${message.role === "user" ? "flex-row-reverse" : ""}`}
            >
              <motion.div
                whileHover={{ scale: 1.1 }}
                className="hidden sm:block"
              >
                <Avatar
                  size="sm"
                  icon={
                    message.role === "assistant" ? (
                      <Bot className="w-5 h-5 text-white" />
                    ) : (
                      <User className="w-5 h-5 text-white" />
                    )
                  }
                  className={
                    message.role === "assistant" ? "bg-brand" : "bg-ink-500"
                  }
                />
              </motion.div>

              <div
                className={`flex flex-col gap-1.5 max-w-[85%] sm:max-w-[75%] ${message.role === "user" ? "items-end" : "items-start"}`}
              >
                <div
                  className={`
                                        px-4 py-3 rounded-2xl shadow-sm relative group
                                        ${
                                          message.role === "user"
                                            ? "bg-brand text-white rounded-tr-none"
                                            : "border border-warm-200 bg-white text-ink-800 rounded-tl-none"
                                        }
                                    `}
                >
                  <p className="text-sm sm:text-base leading-relaxed whitespace-pre-wrap">
                    {message.content}
                  </p>

                  {/* Subtle highlight effect on assistant messages */}
                  {message.role === "assistant" && (
                    <div className="absolute inset-0 bg-gradient-to-br from-brand/5 to-transparent opacity-0 group-hover:opacity-100 rounded-2xl transition-opacity pointer-events-none" />
                  )}
                </div>

                {/* Suggested Options */}
                <AnimatePresence>
                  {message.suggestedOptions &&
                    message.suggestedOptions.length > 0 &&
                    index === messages.length - 1 && (
                      <motion.div
                        initial={{ opacity: 0, y: 5 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ delay: 0.2 }}
                        className="mt-2 flex flex-wrap gap-2"
                      >
                        {message.suggestedOptions.map((option, optIndex) => (
                          <motion.div
                            key={optIndex}
                            whileHover={{ scale: 1.05, y: -2 }}
                            whileTap={{ scale: 0.95 }}
                          >
                            <Button
                              type="button"
                              size="sm"
                              variant="flat"
                              className="h-auto min-h-8 whitespace-normal border border-brand/20 bg-white/80 py-1 text-brand-dark"
                              onPress={() => {
                                void handleSend(option);
                                focusInput();
                              }}
                              endContent={
                                <ChevronRight
                                  className="w-3 h-3 text-brand"
                                  aria-hidden="true"
                                />
                              }
                            >
                              {option}
                            </Button>
                          </motion.div>
                        ))}
                      </motion.div>
                    )}
                </AnimatePresence>
              </div>
            </motion.div>
          ))}
        </AnimatePresence>

        {/* Loading State with Wave Animation */}
        {isLoading && (
          <motion.div
            initial={{ opacity: 0, x: -20 }}
            animate={{ opacity: 1, x: 0 }}
            className="flex gap-3 sm:gap-4"
          >
            <Avatar
              size="sm"
              icon={<Bot className="w-5 h-5 text-white" />}
              className="bg-brand hidden sm:flex"
            />
            <div className="rounded-2xl rounded-tl-none border border-warm-200 bg-white px-5 py-4 shadow-sm shadow-warm-900/5">
              <div className="flex gap-1.5 items-center h-4">
                {[0, 1, 2].map((i) => (
                  <motion.span
                    key={i}
                    className="w-1.5 h-1.5 bg-brand rounded-full"
                    animate={{ y: [0, -6, 0] }}
                    transition={{
                      duration: 0.8,
                      repeat: Infinity,
                      delay: i * 0.15,
                      ease: "easeInOut",
                    }}
                  />
                ))}
              </div>
            </div>
          </motion.div>
        )}
      </div>

      {/* Bottom Actions / Input */}
      <motion.div
        layout
        className="border-t border-warm-200/80 bg-warm-50/70 p-4 sm:p-6"
      >
        {/* Error Bubble */}
        {error && (
          <motion.div
            initial={{ opacity: 0, y: 10, scale: 0.9 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            className="mb-4 flex items-center gap-2 rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700"
          >
            <AlertCircle className="w-4 h-4" />
            {error}
            {retryPending && (
              <button
                type="button"
                className="ml-auto font-semibold underline"
                onClick={() => void retryFailedResponse()}
              >
                {t("retryResponse")}
              </button>
            )}
            <button
              type="button"
              className={`${retryPending ? "" : "ml-auto"} underline`}
              onClick={() => {
                setError(null);
                focusInput();
              }}
            >
              {t("dismiss")}
            </button>
          </motion.div>
        )}

        <div className="relative flex flex-col gap-4">
          {/* Generation Progress Indicator */}
          <AnimatePresence>
            {step === "generating" && (
              <motion.div
                initial={{ opacity: 0, height: 0 }}
                animate={{ opacity: 1, height: "auto" }}
                exit={{ opacity: 0, height: 0 }}
                className="overflow-hidden rounded-2xl border border-brand/10 bg-brand/10 p-4"
              >
                <div className="flex justify-between items-center mb-3">
                  <span className="flex items-center gap-2 text-sm font-bold text-brand-dark">
                    <motion.div
                      animate={{ rotate: 360 }}
                      transition={{
                        duration: 4,
                        repeat: Infinity,
                        ease: "linear",
                      }}
                    >
                      <Sparkles className="w-4 h-4" />
                    </motion.div>
                    {getTranslation(
                      "aiMenuOnboarding.wizard.generating.title",
                      locale,
                    )}
                  </span>
                  <motion.span
                    animate={{ opacity: [0.5, 1, 0.5] }}
                    transition={{ duration: 2, repeat: Infinity }}
                    className="text-xs font-medium text-brand/70"
                  >
                    {(() => {
                      const steps = [
                        "analyzing",
                        "crafting",
                        "pricing",
                        "finalizing",
                      ];
                      const stepKey = steps[generatingStepIndex % steps.length];
                      return getTranslation(
                        `aiMenuOnboarding.wizard.generating.steps.${stepKey}`,
                        locale,
                      );
                    })()}
                  </motion.span>
                </div>
                <Progress
                  isIndeterminate
                  aria-label={
                    getTranslation(
                      "aiMenuOnboarding.wizard.generating.title",
                      locale,
                    ) as string
                  }
                  size="sm"
                  radius="full"
                  classNames={{
                    indicator: "bg-brand shadow-sm shadow-brand/30",
                  }}
                />
                <div className="mt-3 flex justify-center">
                  <Button
                    size="sm"
                    variant="light"
                    className="text-xs text-ink-600"
                    onPress={handleCancelGeneration}
                  >
                    {t("generating.cancel") as string}
                  </Button>
                </div>
              </motion.div>
            )}
          </AnimatePresence>

          <div className="flex gap-3 relative min-h-[56px]">
            {/* Generate Button - Expands when complete or clicked */}
            <AnimatePresence mode="popLayout">
              {isComplete && (
                <motion.div
                  layout
                  initial={{ opacity: 0, scale: 0.9, width: 0 }}
                  animate={{
                    opacity: 1,
                    scale: 1,
                    width: step === "generating" ? "100%" : "30%",
                    minWidth: step === "generating" ? "100%" : "120px",
                  }}
                  exit={{ opacity: 0, scale: 0.9, width: 0 }}
                  transition={{ type: "spring", damping: 25, stiffness: 200 }}
                  className="h-14 z-10"
                >
                  <Button
                    color="primary"
                    className="w-full h-full font-bold rounded-2xl bg-brand shadow-xl shadow-brand/20 hover:shadow-brand/40 transform active:scale-95 transition-all text-sm sm:text-base"
                    onPress={handleGenerateMenu}
                    isLoading={step === "generating"}
                    startContent={
                      step !== "generating" && <Sparkles className="w-5 h-5" />
                    }
                  >
                    {step === "generating" ? "" : t("generateMenu")}
                  </Button>
                </motion.div>
              )}
            </AnimatePresence>

            {/* Input Area - Shrinks when generate button is shown */}
            <motion.div
              layout
              className={`flex-1 flex gap-2 sm:gap-3 bg-white p-1.5 sm:p-2 rounded-2xl border border-warm-200 focus-within:border-brand/50 focus-within:ring-4 focus-within:ring-brand/5 transition-all h-14 ${step === "generating" ? "hidden" : "flex"}`}
            >
              <Input
                ref={inputRef}
                autoFocus
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder={t("inputPlaceholder") as string}
                onKeyDown={(e) =>
                  e.key === "Enter" && !e.shiftKey && handleSend()
                }
                disabled={isLoading || step === "generating"}
                maxLength={500}
                className="flex-1"
                variant="flat"
                classNames={{
                  inputWrapper:
                    "bg-transparent shadow-none border-none p-0 px-2 h-full",
                  input: "text-base sm:text-lg",
                }}
              />
              <Button
                isIconOnly
                aria-label={t("sendAria") as string}
                size="lg"
                className="min-w-11 w-11 h-11 rounded-xl bg-brand text-white shadow-lg"
                onPress={() => handleSend()}
                isDisabled={!input.trim() || isLoading || step === "generating"}
              >
                <Send className="w-5 h-5" />
              </Button>
            </motion.div>
          </div>
          {input.length > 400 && (
            <p className="-mt-2 text-right text-[10px] text-ink-500">
              {t("charCount", { count: input.length, max: 500 }) as string}
            </p>
          )}
        </div>
      </motion.div>
    </div>
  );
}
