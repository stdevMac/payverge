'use client';

import React from 'react';
import { Button, Card, CardBody, Chip } from '@nextui-org/react';
import { Wallet, LogOut, Copy, ExternalLink } from 'lucide-react';
import { useAccount, useConnect, useDisconnect, useBalance } from 'wagmi';
import { formatUnits } from 'viem';

interface WalletConnectorProps {
  showBalance?: boolean;
  showDisconnect?: boolean;
  size?: 'sm' | 'md' | 'lg';
  variant?: 'default' | 'compact';
}

const USDC_CONTRACT_ADDRESS = '0xA0b86a33E6441b8435b662f0E2d0E2E8b0E8E8E8'; // Replace with actual USDC address

export default function WalletConnector({ 
  showBalance = true, 
  showDisconnect = true, 
  size = 'md',
  variant = 'default'
}: WalletConnectorProps) {
  const { address, isConnected, connector } = useAccount();
  const { connect, connectors, isPending } = useConnect();
  const { disconnect } = useDisconnect();

  const { data: usdcBalance } = useBalance({
    address,
    token: USDC_CONTRACT_ADDRESS,
  });

  const { data: ethBalance } = useBalance({
    address,
  });

  const formatAddress = (addr: string) => {
    return `${addr.slice(0, 6)}...${addr.slice(-4)}`;
  };

  const copyAddress = async () => {
    if (address) {
      await navigator.clipboard.writeText(address);
    }
  };

  const openEtherscan = () => {
    if (address) {
      window.open(`https://etherscan.io/address/${address}`, '_blank');
    }
  };

  if (isConnected && address) {
    if (variant === 'compact') {
      return (
        <div className="flex items-center gap-2">
          <Chip
            startContent={<Wallet size={14} />}
            variant="flat"
            color="success"
            size={size}
          >
            {formatAddress(address)}
          </Chip>
          {showDisconnect && (
            <Button
              isIconOnly
              size="sm"
              variant="light"
              onPress={() => disconnect()}
            >
              <LogOut size={16} />
            </Button>
          )}
        </div>
      );
    }

    return (
      <Card>
        <CardBody className="space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Wallet className="text-success" size={20} />
              <div>
                <p className="font-semibold">Connected</p>
                <p className="text-sm text-gray-600">{connector?.name}</p>
              </div>
            </div>
            <Chip color="success" variant="flat" size="sm">
              Connected
            </Chip>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">Address:</span>
              <div className="flex items-center gap-1">
                <span className="text-sm font-mono">{formatAddress(address)}</span>
                <Button
                  isIconOnly
                  size="sm"
                  variant="light"
                  onPress={copyAddress}
                >
                  <Copy size={14} />
                </Button>
                <Button
                  isIconOnly
                  size="sm"
                  variant="light"
                  onPress={openEtherscan}
                >
                  <ExternalLink size={14} />
                </Button>
              </div>
            </div>

            {showBalance && (
              <>
                {usdcBalance && (
                  <div className="flex justify-between">
                    <span className="text-sm font-medium">USDC Balance:</span>
                    <span className="text-sm font-mono">
                      ${formatUnits(usdcBalance.value, usdcBalance.decimals)}
                    </span>
                  </div>
                )}
                {ethBalance && (
                  <div className="flex justify-between">
                    <span className="text-sm font-medium">ETH Balance:</span>
                    <span className="text-sm font-mono">
                      {parseFloat(formatUnits(ethBalance.value, ethBalance.decimals)).toFixed(4)} ETH
                    </span>
                  </div>
                )}
              </>
            )}
          </div>

          {showDisconnect && (
            <Button
              variant="bordered"
              startContent={<LogOut size={16} />}
              onPress={() => disconnect()}
              className="w-full"
            >
              Disconnect
            </Button>
          )}
        </CardBody>
      </Card>
    );
  }

  return (
    <Card>
      <CardBody className="space-y-4">
        <div className="text-center">
          <Wallet className="mx-auto mb-2 text-gray-400" size={32} />
          <h3 className="font-semibold mb-1">Connect Wallet</h3>
          <p className="text-sm text-gray-600">
            Connect your wallet to make payments with USDC
          </p>
        </div>

        <div className="space-y-2">
          {connectors.map((connector) => (
            <Button
              key={connector.id}
              variant="bordered"
              className="w-full justify-start"
              onPress={() => connect({ connector })}
              isLoading={isPending}
              isDisabled={isPending}
            >
              <Wallet size={16} />
              {connector.name}
            </Button>
          ))}
        </div>
      </CardBody>
    </Card>
  );
}
