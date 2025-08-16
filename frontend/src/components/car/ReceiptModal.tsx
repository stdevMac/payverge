// ReceiptModal.tsx
import React, { useState, useEffect } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Card,
  CardBody,
  Divider,
} from "@nextui-org/react";
import {
  IoDocumentTextOutline,
  IoDownloadOutline,
  IoAlertCircleOutline,
  IoReceiptOutline,
} from "react-icons/io5";
import { useTranslation } from "@/i18n/useTranslation";

interface ReceiptModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  date: string;
  type: string;
  amount: number;
  receiptUrl: string;
}

const ReceiptModal: React.FC<ReceiptModalProps> = ({
  isOpen,
  onClose,
  title,
  date,
  type,
  amount,
  receiptUrl,
}) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [previewUrl, setPreviewUrl] = useState<string>('');

  useEffect(() => {
    if (isOpen && receiptUrl) {
      setLoading(true);
      setError(false);
      setPreviewUrl(receiptUrl);
      
      // Add a small delay to show loading state
      const timer = setTimeout(() => {
        setLoading(false);
      }, 500);

      return () => {
        clearTimeout(timer);
      };
    }

    return () => {
      setPreviewUrl('');
      setError(false);
    };
  }, [isOpen, receiptUrl]);

  const handleDownload = () => {
    const link = window.document.createElement('a');
    link.href = receiptUrl;
    link.download = `receipt-${date.replace(/[\s,]+/g, '-')}.pdf`;
    window.document.body.appendChild(link);
    link.click();
    window.document.body.removeChild(link);
  };

  // Format currency
  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(value);
  };

  return (
    <Modal 
      isOpen={isOpen} 
      onClose={onClose} 
      size="3xl" 
      scrollBehavior="inside"
    >
      <ModalContent>
        {() => (
          <>
            <ModalHeader className="flex gap-2 items-center">
              <IoReceiptOutline className="text-2xl" />
              {title}
            </ModalHeader>
            <ModalBody>
              {/* Receipt Information */}
              <Card className="mb-4">
                <CardBody>
                  <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                    <div className="flex items-center gap-3">
                      <IoReceiptOutline className="text-3xl text-primary" />
                      <div>
                        <p className="font-medium">{title}</p>
                        <p className="text-sm text-gray-600">{date}</p>
                      </div>
                    </div>
                    <div className="flex flex-col items-end gap-1">
                      <p className="text-sm text-gray-600">{type}</p>
                      <p className={`font-medium ${amount < 0 ? "text-red-600" : "text-green-600"}`}>
                        {formatCurrency(amount)}
                      </p>
                    </div>
                  </div>
                </CardBody>
              </Card>

              {/* Document Preview */}
              <div className="w-full h-[50vh] relative">
                {loading && (
                  <div className="absolute inset-0 flex items-center justify-center bg-default-100">
                    <div className="flex flex-col items-center gap-2">
                      <div className="w-8 h-8 border-3 border-primary border-t-transparent rounded-full animate-spin" />
                      <p className="text-sm text-default-600">{t('shared.receiptModal.loading')}</p>
                    </div>
                  </div>
                )}
                {error ? (
                  <div className="absolute inset-0 flex items-center justify-center bg-default-100">
                    <div className="flex flex-col items-center gap-2 text-center px-4">
                      <IoAlertCircleOutline className="text-4xl text-danger" />
                      <p className="text-sm text-danger">{t('shared.receiptModal.error')}</p>
                      <p className="text-sm text-default-600">{t('shared.receiptModal.tryAgain')}</p>
                    </div>
                  </div>
                ) : previewUrl && (
                  <iframe
                    src={previewUrl}
                    className="w-full h-full border-0"
                    title="Receipt Preview"
                    onError={() => setError(true)}
                  />
                )}
              </div>
            </ModalBody>
            <ModalFooter>
              <Button color="danger" variant="light" onPress={onClose}>
                {t('shared.receiptModal.close')}
              </Button>
              <Button 
                color="primary" 
                startContent={<IoDownloadOutline />}
                onPress={handleDownload}
                isDisabled={error}
              >
                {t('shared.receiptModal.download')}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
};

export default ReceiptModal;
