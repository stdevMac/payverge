"use client";

import { useState, useEffect } from "react";
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Input,
  Textarea,
  Select,
  SelectItem,
  Divider,
  Spinner,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
} from "@nextui-org/react";
import { Mail, Send, Users, CheckCircle, AlertCircle } from "lucide-react";
import { axiosInstance } from "@/api";
import { apiErrorDetail } from "@/utils/apiError";

interface Business {
  id: number;
  name: string;
  owner_name: string;
  email: string;
}

interface EmailResponse {
  message: string;
  success_count: number;
  failed_count: number;
  total: number;
}

export default function EmailManagementPage() {
  const [emailType, setEmailType] = useState<"operational" | "platform">(
    "operational",
  );
  const [recipients, setRecipients] = useState<string[]>(["all_businesses"]);
  const [updateTitle, setUpdateTitle] = useState("");
  const [updateIntro, setUpdateIntro] = useState("");
  const [updateBody, setUpdateBody] = useState("");
  const [language, setLanguage] = useState("en");
  const [businesses, setBusinesses] = useState<Business[]>([]);
  // Server-resolved deliverable count + sample (kind=real, non-deliverable domains
  // already excluded). Prefer these over businesses.length so the compose UI
  // matches what the send path will resolve (Task 13).
  const [recipientCount, setRecipientCount] = useState(0);
  const [recipientSample, setRecipientSample] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingBusinesses, setLoadingBusinesses] = useState(true);
  const [result, setResult] = useState<EmailResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const confirmModal = useDisclosure();

  useEffect(() => {
    fetchBusinesses();
  }, []);

  const fetchBusinesses = async () => {
    try {
      setLoadingBusinesses(true);
      const response = await axiosInstance.get("/admin/emails/business-list");
      const list: Business[] = response.data.businesses || [];
      setBusinesses(list);
      const count =
        typeof response.data.recipient_count === "number"
          ? response.data.recipient_count
          : typeof response.data.with_email === "number"
            ? response.data.with_email
            : list.length;
      setRecipientCount(count);
      const sample: string[] = Array.isArray(response.data.sample)
        ? response.data.sample
        : list.slice(0, 5).map((b) => b.email).filter(Boolean);
      setRecipientSample(sample);
    } catch (err: unknown) {
      console.error("Failed to fetch businesses:", err);
      setError("Failed to load business list");
    } finally {
      setLoadingBusinesses(false);
    }
  };

  // Opening the confirmation dialog is the only thing the Send button does —
  // an irreversible bulk email must never fire on a single click.
  const handleRequestSend = () => {
    if (!updateTitle || !updateIntro || !updateBody) {
      setError("Please fill in all required fields");
      return;
    }
    setError(null);
    confirmModal.onOpen();
  };

  const handleSendEmail = async () => {
    if (!updateTitle || !updateIntro || !updateBody) {
      setError("Please fill in all required fields");
      return;
    }

    confirmModal.onClose();
    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const endpoint =
        emailType === "operational"
          ? "/admin/emails/operational-update"
          : "/admin/emails/platform-update";

      const response = await axiosInstance.post(endpoint, {
        recipients,
        update_title: updateTitle,
        update_intro: updateIntro,
        update_body: updateBody,
        language,
      });

      setResult(response.data);

      // Clear form on success
      setUpdateTitle("");
      setUpdateIntro("");
      setUpdateBody("");
    } catch (err: unknown) {
      console.error("Failed to send emails:", err);
      setError(apiErrorDetail(err) || "Failed to send emails");
    } finally {
      setLoading(false);
    }
  };

  const recipientOptions = [
    { value: "all_businesses", label: "All Businesses" },
    { value: "all_active_businesses", label: "Active Businesses (30 days)" },
  ];

  const selectedRecipientLabel =
    recipientOptions.find((o) => recipients.includes(o.value))?.label ??
    "Select a recipient group";

  // Recipient summary uses the server-resolved deliverable count so a broadcast
  // that would mail 0 real businesses cannot hide behind a fixture count.
  const recipientCountLabel =
    recipientCount === 1 ? "1 recipient" : `${recipientCount} recipients`;

  const recipientSummary = loadingBusinesses
    ? "Loading..."
    : recipients.includes("all_businesses")
      ? `Will send to ${recipientCountLabel}`
      : recipients.includes("all_active_businesses")
        ? "Will send to active businesses only (those with a paid order in the last 30 days)"
        : "Select a recipient group";

  const recipientSampleLabel =
    recipientSample.length === 0
      ? null
      : recipientSample.length < recipientCount
        ? `Sample: ${recipientSample.join(", ")}…`
        : `Sample: ${recipientSample.join(", ")}`;

  return (
    <div className="p-6 max-w-6xl mx-auto">
      <div className="mb-6">
        <h1 className="text-3xl font-bold mb-2">Email Broadcast Management</h1>
        <p className="text-default-500">
          Send operational updates and platform announcements to businesses
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Main Form */}
        <div className="lg:col-span-2 space-y-6">
          {/* Email Type Selection */}
          <Card>
            <CardHeader className="flex gap-3">
              <Mail className="w-5 h-5" />
              <div className="flex flex-col">
                <p className="text-md font-semibold">Email Type</p>
                <p className="text-small text-default-500">
                  Choose the type of update to send
                </p>
              </div>
            </CardHeader>
            <CardBody>
              <div
                role="group"
                aria-label="Email type"
                className="flex gap-4"
              >
                <Button
                  color={emailType === "operational" ? "primary" : "default"}
                  variant={emailType === "operational" ? "solid" : "bordered"}
                  onPress={() => setEmailType("operational")}
                  aria-pressed={emailType === "operational"}
                  className="flex-1"
                >
                  Operational Update
                </Button>
                <Button
                  color={emailType === "platform" ? "primary" : "default"}
                  variant={emailType === "platform" ? "solid" : "bordered"}
                  onPress={() => setEmailType("platform")}
                  aria-pressed={emailType === "platform"}
                  className="flex-1"
                >
                  Platform Update
                </Button>
              </div>
              <p className="text-small text-default-500 mt-2">
                {emailType === "operational"
                  ? "For service updates, maintenance notices, and operational changes"
                  : "For new features, product announcements, and platform improvements"}
              </p>
            </CardBody>
          </Card>

          {/* Recipients */}
          <Card>
            <CardHeader className="flex gap-3">
              <Users className="w-5 h-5" />
              <div className="flex flex-col">
                <p className="text-md font-semibold">Recipients</p>
                <p className="text-small text-default-500">
                  Select who will receive this email
                </p>
              </div>
            </CardHeader>
            <CardBody>
              <Select
                label="Recipient Group"
                placeholder="Select recipient group"
                selectedKeys={recipients}
                onSelectionChange={(keys) =>
                  setRecipients(Array.from(keys) as string[])
                }
                className="mb-4"
              >
                {recipientOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </Select>

              <div className="flex flex-col gap-1 text-sm text-default-500">
                <div className="flex items-center gap-2">
                  <Users className="w-4 h-4 flex-shrink-0" />
                  <span data-testid="recipient-summary">{recipientSummary}</span>
                </div>
                {!loadingBusinesses &&
                  recipients.includes("all_businesses") &&
                  recipientSampleLabel && (
                    <p
                      className="text-xs text-default-500 pl-6 truncate"
                      data-testid="recipient-sample"
                      title={recipientSample.join(", ")}
                    >
                      {recipientSampleLabel}
                    </p>
                  )}
              </div>
            </CardBody>
          </Card>

          {/* Email Content */}
          <Card>
            <CardHeader className="flex gap-3">
              <Send className="w-5 h-5" />
              <div className="flex flex-col">
                <p className="text-md font-semibold">Email Content</p>
                <p className="text-small text-default-500">
                  Compose your update message
                </p>
              </div>
            </CardHeader>
            <CardBody className="space-y-4">
              <Input
                label="Update Title"
                placeholder="e.g., New Feature: QR Code Customization"
                value={updateTitle}
                onValueChange={setUpdateTitle}
                isRequired
              />

              <Textarea
                label="Introduction"
                placeholder="Brief introduction to the update..."
                value={updateIntro}
                onValueChange={setUpdateIntro}
                minRows={2}
                isRequired
              />

              <Textarea
                label="Update Body"
                placeholder="Detailed information about the update..."
                value={updateBody}
                onValueChange={setUpdateBody}
                minRows={6}
                isRequired
              />

              <Select
                label="Language"
                placeholder="Select language"
                selectedKeys={[language]}
                onSelectionChange={(keys) =>
                  setLanguage(Array.from(keys)[0] as string)
                }
              >
                <SelectItem key="en" value="en">
                  English
                </SelectItem>
                <SelectItem key="es" value="es">
                  Spanish
                </SelectItem>
              </Select>

              <Divider />

              <Button
                color="primary"
                size="lg"
                onPress={handleRequestSend}
                isLoading={loading}
                isDisabled={!updateTitle || !updateIntro || !updateBody}
                startContent={!loading && <Send className="w-4 h-4" />}
                className="w-full"
              >
                {loading ? "Sending Emails..." : "Send Email Broadcast"}
              </Button>
            </CardBody>
          </Card>

          {/* Result */}
          {result && (
            <Card
              className={`border-2 ${
                result.failed_count > 0 ? "border-warning" : "border-success"
              }`}
            >
              <CardBody>
                <div className="flex items-start gap-3">
                  {result.failed_count > 0 ? (
                    <AlertCircle className="w-6 h-6 text-warning flex-shrink-0 mt-1" />
                  ) : (
                    <CheckCircle className="w-6 h-6 text-success flex-shrink-0 mt-1" />
                  )}
                  <div className="flex-1">
                    <p className="font-semibold text-lg mb-2">
                      {result.failed_count > 0
                        ? "Broadcast completed with failures"
                        : result.message}
                    </p>
                    <div className="flex gap-4 text-sm">
                      <span className="text-success-600">
                        ✓ {result.success_count} sent
                      </span>
                      {result.failed_count > 0 && (
                        <span className="text-danger-600">
                          ✗ {result.failed_count} failed
                        </span>
                      )}
                      <span className="text-default-500">
                        Total: {result.total}
                      </span>
                    </div>
                  </div>
                </div>
              </CardBody>
            </Card>
          )}

          {/* Error */}
          {error && (
            <Card className="border-2 border-danger">
              <CardBody>
                <div className="flex items-start gap-3">
                  <AlertCircle className="w-6 h-6 text-danger flex-shrink-0 mt-1" />
                  <div>
                    <p className="font-semibold text-danger-600">Error</p>
                    <p className="text-sm text-default-500">{error}</p>
                  </div>
                </div>
              </CardBody>
            </Card>
          )}
        </div>

        {/* Sidebar - Business List */}
        <div className="lg:col-span-1">
          <Card className="sticky top-6">
            <CardHeader>
              <div className="flex flex-col w-full">
                <p className="text-md font-semibold">Business List</p>
                <p className="text-small text-default-500">
                  {recipientCount} deliverable{" "}
                  {recipientCount === 1 ? "recipient" : "recipients"} (kind=real)
                </p>
              </div>
            </CardHeader>
            <CardBody>
              {loadingBusinesses ? (
                <div className="flex justify-center py-8">
                  <Spinner />
                </div>
              ) : (
                <div className="space-y-2 max-h-[600px] overflow-y-auto">
                  {businesses.map((business) => (
                    <div
                      key={business.id}
                      className="p-3 rounded-lg border border-default-200 hover:border-default-300 transition-colors"
                    >
                      <p className="font-medium text-sm">{business.name}</p>
                      <p className="text-xs text-default-500">
                        {business.owner_name}
                      </p>
                      <p className="text-xs text-default-500 truncate">
                        {business.email}
                      </p>
                    </div>
                  ))}
                </div>
              )}
            </CardBody>
          </Card>
        </div>
      </div>

      {/* Info Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mt-6">
        <Card>
          <CardBody>
            <h3 className="font-semibold mb-2">📧 Operational Updates</h3>
            <p className="text-sm text-default-500">
              Use for service maintenance, downtime notices, security updates,
              and operational changes that affect business operations.
            </p>
          </CardBody>
        </Card>

        <Card>
          <CardBody>
            <h3 className="font-semibold mb-2">🚀 Platform Updates</h3>
            <p className="text-sm text-default-500">
              Use for new feature announcements, product improvements, success
              stories, and platform enhancements.
            </p>
          </CardBody>
        </Card>
      </div>

      {/* Confirmation dialog — bulk email is irreversible. */}
      <Modal isOpen={confirmModal.isOpen} onClose={confirmModal.onClose}>
        <ModalContent>
          <ModalHeader>Confirm Email Broadcast</ModalHeader>
          <ModalBody>
            <p className="text-sm text-default-600">
              You are about to send a{" "}
              <span className="font-semibold">
                {emailType === "operational"
                  ? "operational update"
                  : "platform update"}
              </span>{" "}
              email. This cannot be undone.
            </p>
            <div className="text-sm space-y-1">
              <p>
                <span className="text-default-500">Recipients: </span>
                <span className="font-medium">{selectedRecipientLabel}</span>
              </p>
              <p>
                <span className="text-default-500">Reach: </span>
                <span className="font-medium" data-testid="confirm-recipient-count">
                  {recipientSummary}
                </span>
              </p>
              {recipients.includes("all_businesses") && recipientSampleLabel && (
                <p className="text-default-500" data-testid="confirm-recipient-sample">
                  {recipientSampleLabel}
                </p>
              )}
              <p>
                <span className="text-default-500">Title: </span>
                <span className="font-medium">{updateTitle}</span>
              </p>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="flat" onPress={confirmModal.onClose}>
              Cancel
            </Button>
            <Button
              color="primary"
              onPress={handleSendEmail}
              isLoading={loading}
              isDisabled={
                recipients.includes("all_businesses") && recipientCount === 0
              }
              startContent={!loading && <Send className="w-4 h-4" />}
            >
              {recipients.includes("all_businesses")
                ? `Confirm & Send to ${recipientCount}`
                : "Confirm & Send"}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
