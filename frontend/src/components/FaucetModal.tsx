"use client";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Card,
  CardBody,
  Spinner,
  Chip,
} from "@nextui-org/react";
import { useState, useEffect, useCallback } from "react";
import { useAccount, useConnect, useChainId } from "wagmi";
import { injected } from "wagmi/connectors";
import { faucetAPI } from '@/api/faucet';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

interface FaucetModalProps {
  isOpen: boolean;
  onClose: () => void;
}

interface FaucetResponse {
  status: string;
  eth_tx_hash?: string;
  usdc_tx_hash?: string;
  to_address?: string;
  eth_sent?: string;
  usdc_sent?: string;
  eth_balance?: string;
  usdc_balance?: string;
  error?: string;
  message?: string;
}

type ModalStep = 'connect' | 'eligibility' | 'confirm' | 'processing' | 'success' | 'error';

export const FaucetModal = ({ isOpen, onClose }: FaucetModalProps) => {
  const [currentStep, setCurrentStep] = useState<ModalStep>('connect');
  const [eligibilityData, setEligibilityData] = useState<FaucetResponse | null>(null);
  const [transactionData, setTransactionData] = useState<FaucetResponse | null>(null);
  const [error, setError] = useState<string>('');
  const [isLoading, setIsLoading] = useState(false);

  const { address, isConnected } = useAccount();
  const { connect } = useConnect();
  const chainId = useChainId();
  const { locale } = useSimpleLocale();
  
  // Translation helper function
  const t = (key: string) => getTranslation(key, locale) as string;

  const BASE_SEPOLIA_CHAIN_ID = 84532;

  const checkEligibility = useCallback(async () => {
    if (!address) return;
    
    setIsLoading(true);
    setCurrentStep('eligibility');
    
    try {
      const data = await faucetAPI.checkEligibility(address);
      
      setEligibilityData(data);
      
      if (data.status === 'eligible') {
        setCurrentStep('confirm');
      } else if (data.status === 'sufficient_balance') {
        setCurrentStep('success');
        setTransactionData(data);
      } else {
        setCurrentStep('error');
        setError(data.error || data.message || 'Address not eligible for faucet');
      }
    } catch (err) {
      setCurrentStep('error');
      setError(err instanceof Error ? err.message : 'Failed to check eligibility. Please try again.');
    } finally {
      setIsLoading(false);
    }
  }, [address]);

  // Reset modal state when opened
  useEffect(() => {
    if (isOpen) {
      if (isConnected && address && chainId === BASE_SEPOLIA_CHAIN_ID) {
        checkEligibility();
      } else {
        setCurrentStep('connect');
      }
      setError('');
      setEligibilityData(null);
      setTransactionData(null);
    }
  }, [isOpen, isConnected, address, chainId, checkEligibility]);

  const requestTokens = async () => {
    if (!address) return;
    
    setIsLoading(true);
    setCurrentStep('processing');
    
    try {
      const data = await faucetAPI.requestTokens({ address });
      setTransactionData(data);
      
      if (data.status === 'completed') {
        setCurrentStep('success');
      } else {
        setCurrentStep('error');
        setError(data.error || 'Failed to send tokens');
      }
    } catch (err) {
      setCurrentStep('error');
      setError(err instanceof Error ? err.message : 'Failed to request tokens. Please try again.');
    } finally {
      setIsLoading(false);
    }
  };

  const handleConnectWallet = () => {
    connect({ connector: injected() });
  };

  const formatAmount = (amount: string, decimals: number = 18) => {
    const value = BigInt(amount);
    const divisor = BigInt(10 ** decimals);
    const result = Number(value) / Number(divisor);
    return result.toFixed(decimals === 6 ? 2 : 4);
  };

  const renderConnectStep = () => (
    <>
      <ModalHeader className="flex flex-col gap-1">
        <h2 className="text-2xl font-bold">{t('faucet.modal.connect.title')}</h2>
        <p className="text-gray-600 font-normal">{t('faucet.modal.connect.description')}</p>
      </ModalHeader>
      <ModalBody>
        <Card className="bg-gradient-to-br from-blue-50 to-indigo-50 border border-blue-100">
          <CardBody className="p-6">
            <div className="flex items-center gap-4 mb-4">
              <div className="w-12 h-12 bg-blue-100 rounded-full flex items-center justify-center">
                <svg className="w-6 h-6 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 9V7a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2m2 4h10a2 2 0 002-2v-6a2 2 0 00-2-2H9a2 2 0 00-2 2v6a2 2 0 002 2zm7-5a2 2 0 11-4 0 2 2 0 014 0z" />
                </svg>
              </div>
              <div>
                <h3 className="font-semibold text-gray-900">{t('faucet.modal.connect.whatYouGet')}</h3>
                <p className="text-sm text-gray-600">{t('faucet.modal.connect.whatYouGetDescription')}</p>
              </div>
            </div>
            <div className="space-y-2 text-sm">
              <div className="flex items-center gap-2">
                <div className="w-2 h-2 bg-green-500 rounded-full"></div>
                <span>{t('faucet.modal.connect.ethAmount')}</span>
              </div>
              <div className="flex items-center gap-2">
                <div className="w-2 h-2 bg-green-500 rounded-full"></div>
                <span>{t('faucet.modal.connect.usdcAmount')}</span>
              </div>
            </div>
          </CardBody>
        </Card>
        
        {chainId !== BASE_SEPOLIA_CHAIN_ID && (
          <Card className="bg-orange-50 border border-orange-200">
            <CardBody className="p-4">
              <div className="flex items-center gap-2">
                <svg className="w-5 h-5 text-orange-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
                </svg>
                <span className="text-orange-800 text-sm font-medium">
                  {t('faucet.modal.connect.networkWarning')}
                </span>
              </div>
            </CardBody>
          </Card>
        )}
      </ModalBody>
      <ModalFooter>
        <Button variant="light" onPress={onClose}>
          {t('faucet.modal.connect.cancelButton')}
        </Button>
        <Button 
          color="primary" 
          onPress={handleConnectWallet}
          isDisabled={chainId !== BASE_SEPOLIA_CHAIN_ID}
        >
          {t('faucet.modal.connect.connectButton')}
        </Button>
      </ModalFooter>
    </>
  );

  const renderEligibilityStep = () => (
    <>
      <ModalHeader>
        <h2 className="text-2xl font-bold">{t('faucet.modal.eligibility.title')}</h2>
      </ModalHeader>
      <ModalBody>
        <div className="flex flex-col items-center gap-4 py-8">
          <Spinner size="lg" color="primary" />
          <p className="text-gray-600">{t('faucet.modal.eligibility.checking')}</p>
        </div>
      </ModalBody>
    </>
  );

  const renderConfirmStep = () => (
    <>
      <ModalHeader>
        <h2 className="text-2xl font-bold">{t('faucet.modal.confirm.title')}</h2>
        <p className="text-gray-600 font-normal">{t('faucet.modal.confirm.description')}</p>
      </ModalHeader>
      <ModalBody>
        <Card className="bg-green-50 border border-green-200">
          <CardBody className="p-6">
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <span className="font-medium">Your Address:</span>
                <code className="text-sm bg-gray-100 px-2 py-1 rounded">
                  {address?.slice(0, 6)}...{address?.slice(-4)}
                </code>
              </div>
              
              {eligibilityData?.message && (
                <div className="bg-blue-50 border border-blue-200 rounded-lg p-3">
                  <p className="text-blue-800 text-sm">{eligibilityData.message}</p>
                </div>
              )}
              
              <div className="border-t pt-4">
                <h4 className="font-medium mb-2">Current Balances:</h4>
                <div className="space-y-1 text-sm">
                  <div className="flex justify-between">
                    <span>ETH:</span>
                    <span>{eligibilityData?.eth_balance ? formatAmount(eligibilityData.eth_balance) : '0'}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>USDC:</span>
                    <span>{eligibilityData?.usdc_balance ? formatAmount(eligibilityData.usdc_balance, 6) : '0'}</span>
                  </div>
                </div>
              </div>
            </div>
          </CardBody>
        </Card>
      </ModalBody>
      <ModalFooter>
        <Button variant="light" onPress={onClose}>
          {t('faucet.modal.confirm.cancelButton')}
        </Button>
        <Button color="primary" onPress={requestTokens}>
          {t('faucet.modal.confirm.confirmButton')}
        </Button>
      </ModalFooter>
    </>
  );

  const renderProcessingStep = () => (
    <>
      <ModalHeader>
        <h2 className="text-2xl font-bold">{t('faucet.modal.processing.title')}</h2>
      </ModalHeader>
      <ModalBody>
        <div className="flex flex-col items-center gap-4 py-8">
          <Spinner size="lg" color="primary" />
          <div className="text-center">
            <p className="text-lg font-semibold mb-2">{t('faucet.modal.processing.description')}</p>
            <p className="text-gray-600">{t('faucet.modal.processing.pleaseWait')}</p>
          </div>
        </div>
      </ModalBody>
    </>
  );

  const renderSuccessStep = () => (
    <>
      <ModalHeader>
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 bg-green-100 rounded-full flex items-center justify-center">
            <svg className="w-5 h-5 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
          </div>
          <h2 className="text-2xl font-bold text-green-600">{t('faucet.modal.success.title')}</h2>
        </div>
        <p className="text-gray-600 font-normal">{t('faucet.modal.success.description')}</p>
      </ModalHeader>
      <ModalBody>
        <Card className="bg-green-50 border border-green-200">
          <CardBody className="p-6">
            <h3 className="font-semibold text-gray-900 mb-4">{t('faucet.modal.success.transactionDetails')}</h3>
            
            {transactionData?.eth_tx_hash && (
              <div className="mb-4">
                <p className="text-sm font-medium mb-1">ETH Transaction:</p>
                <a
                  href={`https://sepolia.basescan.org/tx/${transactionData.eth_tx_hash}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-blue-600 hover:text-blue-800 text-sm break-all"
                >
                  {transactionData.eth_tx_hash}
                </a>
              </div>
            )}
            
            {transactionData?.usdc_tx_hash && (
              <div className="mb-4">
                <p className="text-sm font-medium mb-1">USDC Transaction:</p>
                <a
                  href={`https://sepolia.basescan.org/tx/${transactionData.usdc_tx_hash}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-blue-600 hover:text-blue-800 text-sm break-all"
                >
                  {transactionData.usdc_tx_hash}
                </a>
              </div>
            )}

            <h3 className="font-semibold text-gray-900 mb-2">{t('faucet.modal.success.newBalance')}</h3>
            <div className="space-y-2">
              <div className="flex justify-between">
                <span>ETH:</span>
                <span>{transactionData?.eth_balance ? formatAmount(transactionData.eth_balance) : '0'}</span>
              </div>
              <div className="flex justify-between">
                <span>USDC:</span>
                <span>{transactionData?.usdc_balance ? formatAmount(transactionData.usdc_balance, 6) : '0'}</span>
              </div>
            </div>
          </CardBody>
        </Card>
      </ModalBody>
      <ModalFooter>
        {transactionData?.eth_tx_hash && (
          <Button
            color="primary"
            variant="light"
            onPress={() => {
              if (transactionData?.eth_tx_hash) {
                window.open(`https://sepolia.basescan.org/tx/${transactionData.eth_tx_hash}`, '_blank');
              }
            }}
            className="w-full"
          >
            {t('faucet.modal.success.viewOnExplorer')}
          </Button>
        )}
        <Button color="primary" onPress={onClose} className="w-full">
          {t('faucet.modal.success.doneButton')}
        </Button>
      </ModalFooter>
    </>
  );

  const renderErrorStep = () => (
    <>
      <ModalHeader>
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 bg-red-100 rounded-full flex items-center justify-center">
            <svg className="w-5 h-5 text-red-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </div>
          <h2 className="text-2xl font-bold text-red-800">{t('faucet.modal.error.title')}</h2>
        </div>
      </ModalHeader>
      <ModalBody>
        <Card className="bg-red-50 border border-red-200">
          <CardBody className="p-6">
            <p className="text-red-800">{error}</p>
            
            {error.includes('cooldown') && (
              <div className="mt-4 p-3 bg-orange-50 border border-orange-200 rounded">
                <p className="text-orange-800 text-sm">
                  💡 The faucet has a 24-hour cooldown period to prevent abuse. 
                  Try again tomorrow or ask in our Discord for help!
                </p>
              </div>
            )}
          </CardBody>
        </Card>
      </ModalBody>
      <ModalFooter>
        <Button variant="light" onPress={onClose}>
          {t('faucet.modal.error.closeButton')}
        </Button>
        <Button color="primary" onPress={checkEligibility}>
          {t('faucet.modal.error.tryAgainButton')}
        </Button>
      </ModalFooter>
    </>
  );

  const renderCurrentStep = () => {
    switch (currentStep) {
      case 'connect':
        return renderConnectStep();
      case 'eligibility':
        return renderEligibilityStep();
      case 'confirm':
        return renderConfirmStep();
      case 'processing':
        return renderProcessingStep();
      case 'success':
        return renderSuccessStep();
      case 'error':
        return renderErrorStep();
      default:
        return renderConnectStep();
    }
  };

  return (
    <Modal 
      isOpen={isOpen} 
      onClose={onClose}
      size="2xl"
      scrollBehavior="inside"
      backdrop="opaque"
      isDismissable={true}
      isKeyboardDismissDisabled={false}
      classNames={{
        base: "bg-white",
        header: "border-b border-gray-200",
        footer: "border-t border-gray-200",
      }}
    >
      <ModalContent>
        {renderCurrentStep()}
      </ModalContent>
    </Modal>
  );
};
