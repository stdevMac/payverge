// ContractModal.tsx
import React, { useRef } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
} from "@nextui-org/react";
import SignatureCanvas from "react-signature-canvas";
import jsPDF from "jspdf";
import "jspdf-autotable";
import { UserInterface } from "@/interface";
import { useTranslation } from "@/i18n/useTranslation";
import { formatNumber } from "../../utils/formatters";

interface ContractModalProps {
  isOpen: boolean;
  onClose: () => void;
  investmentAmount: number;
  ownershipPercentage: number;
  expectedReturn: number;
  carSmartContractAddress: string;
  user: UserInterface;
  onSignContract: (signatureData: string, pdfData: Blob) => void;
}

const ContractModal: React.FC<ContractModalProps> = ({
  isOpen,
  onClose,
  investmentAmount,
  ownershipPercentage,
  expectedReturn,
  carSmartContractAddress,
  user,
  onSignContract,
}) => {
  const { t } = useTranslation();
  const sigCanvas = useRef<SignatureCanvas>(null);

  const handleConfirm = () => {
    if (sigCanvas.current && !sigCanvas.current.isEmpty()) {
      const signatureData = sigCanvas.current.toDataURL("image/png");
      generatePDF(signatureData);
    } else {
      alert(t('shared.contractModal.pleaseSign'));
    }
  };

  const clearSignature = () => {
    if (sigCanvas.current) {
      sigCanvas.current.clear();
    }
  };

  const generatePDF = (signatureData: string) => {
    const doc = new jsPDF();

    // Contract Header
    doc.setFontSize(18);
    doc.text(t('shared.contractModal.title'), 105, 20, { align: "center" });

    // Contract Body
    doc.setFontSize(12);
    const contractText = `
${t('shared.contractModal.contractIntro', { amount: formatNumber(investmentAmount), address: carSmartContractAddress })}

${t('shared.contractModal.userPassport')}: ??????

${t('shared.contractModal.userInformation')}:
- ${t('shared.contractModal.name')}: ${user.username}
- ${t('shared.contractModal.address')}: ${user.address}

${t('shared.contractModal.userRights')}:
1. ${t('shared.contractModal.userRightsList.0')}
2. ${t('shared.contractModal.userRightsList.1')}
3. ${t('shared.contractModal.userRightsList.2')}

${t('shared.contractModal.investmentDetails')}:
${t('shared.contractModal.investmentDetailsText', { percentage: ownershipPercentage.toFixed(2), return: expectedReturn.toFixed(2) })}

${t('shared.contractModal.rentSplitAgreement')}:
- ${t('shared.contractModal.rentSplitList.0')}
- ${t('shared.contractModal.rentSplitList.1')}
- ${t('shared.contractModal.rentSplitList.2')}

${t('shared.contractModal.agreementText')}
        `;

    doc.text(contractText, 10, 30, { maxWidth: 190 });

    // Add Signature
    doc.text("Signature:", 10, 160);
    if (signatureData) {
      doc.addImage(signatureData, "PNG", 10, 165, 50, 20);
    }

    // Add Note
    doc.setFontSize(10);
    doc.text(
      t('shared.contractModal.contractNote'),
      10,
      200,
    );

    // Generate PDF as Blob
    const pdfBlob = doc.output("blob");

    // Pass PDF Blob to parent component
    onSignContract(signatureData, pdfBlob);
  };

  return (
    <Modal
      isOpen={isOpen}
      onOpenChange={(openState) => !openState && onClose()}
      scrollBehavior="inside"
    >
      <ModalContent>
        <>
          <ModalHeader className="flex flex-col gap-1">
            {t('shared.contractModal.title')}
          </ModalHeader>
          <ModalBody>
            {/* Contract Content */}
            <div className="mb-4 max-h-96 overflow-y-auto">
              <h2 className="text-lg font-bold mb-2">{t('shared.contractModal.contractAgreement')}</h2>
              <p className="text-sm">
                {t('shared.contractModal.contractIntro', { amount: formatNumber(investmentAmount), address: carSmartContractAddress })}
              </p>
              <p className="text-sm mt-2">
                <strong>{t('shared.contractModal.userPassport')}:</strong> ??????
              </p>
              <h3 className="text-md font-semibold mt-4">{t('shared.contractModal.userInformation')}</h3>
              <p className="text-sm">
                <strong>{t('shared.contractModal.name')}:</strong> {user.username}
              </p>
              <p className="text-sm">
                <strong>{t('shared.contractModal.address')}:</strong> {user.address}
              </p>
              <h3 className="text-md font-semibold mt-4">{t('shared.contractModal.userRights')}</h3>
              <p className="text-sm">
                {t('shared.contractModal.userRightsIntro')}
              </p>
              <ul className="list-disc list-inside text-sm">
                <li>{t('shared.contractModal.userRightsList.0')}</li>
                <li>{t('shared.contractModal.userRightsList.1')}</li>
                <li>{t('shared.contractModal.userRightsList.2')}</li>
              </ul>
              <h3 className="text-md font-semibold mt-4">{t('shared.contractModal.investmentDetails')}</h3>
              <p className="text-sm">
                {t('shared.contractModal.investmentDetailsText', { percentage: ownershipPercentage.toFixed(2), return: expectedReturn.toFixed(2) })}
              </p>
              <h3 className="text-md font-semibold mt-4">
                {t('shared.contractModal.rentSplitAgreement')}
              </h3>
              <p className="text-sm">
                {t('shared.contractModal.rentSplitText')}
              </p>
              <ul className="list-disc list-inside text-sm">
                <li>{t('shared.contractModal.rentSplitList.0')}</li>
                <li>{t('shared.contractModal.rentSplitList.1')}</li>
                <li>{t('shared.contractModal.rentSplitList.2')}</li>
              </ul>
              <p className="text-sm mt-4">
                {t('shared.contractModal.agreementText')}
              </p>
            </div>
            {/* Signature Pad */}
            <div>
              <p className="mb-2">{t('shared.contractModal.signBelow')}:</p>
              <SignatureCanvas
                ref={sigCanvas}
                penColor="black"
                canvasProps={{
                  width: 500,
                  height: 200,
                  className: "border border-gray-300",
                }}
              />
              <Button
                variant="light"
                color="primary"
                onPress={clearSignature}
                className="mt-2"
              >
                {t('shared.contractModal.clearSignature')}
              </Button>
              <p className="text-xs text-gray-500 mt-2">
                {t('shared.contractModal.contractNote')}
              </p>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button color="danger" variant="light" onPress={onClose}>
              {t('shared.contractModal.cancel')}
            </Button>
            <Button color="primary" onPress={handleConfirm}>
              {t('shared.contractModal.confirmAndSign')}
            </Button>
          </ModalFooter>
        </>
      </ModalContent>
    </Modal>
  );
};

export default ContractModal;
