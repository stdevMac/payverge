// LI.FI SDK Hooks for Payverge Cross-Chain Payments
// Enables users to pay with any token from any chain, settling to USDC on Base

import { useState, useCallback, useEffect } from 'react';
import { getRoutes, getQuote, getTokens, getChains, convertQuoteToRoute, Route, Token, ExtendedChain } from '@lifi/sdk';
import { useAccount, useChainId } from 'wagmi';
import { errMessage } from '@/utils/apiError';

import { parseUnits } from 'viem';
import { initializeLiFi, getLiFiClient, USDC_ADDRESSES, getSettlementChainId, CHAIN_NAMES } from './config';

// Track if SDK is initialized
let sdkInitialized = false;

// Initialize LI.FI SDK once
const ensureSdkInitialized = () => {
  if (!sdkInitialized) {
    initializeLiFi();
    sdkInitialized = true;
  }
};

export interface PaymentRoute {
  route: Route;
  fromToken: Token;
  toToken: Token;
  fromAmount: string;
  toAmount: string;
  estimatedGas: string;
  estimatedTime: number; // in seconds
  steps: number;
}

// Hook to get available tokens on a chain
export const useAvailableTokens = (chainId?: number) => {
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const connectedChainId = useChainId();
  const targetChainId = chainId || connectedChainId;

  useEffect(() => {
    const fetchTokens = async () => {
      if (!targetChainId) return;
      
      // Ensure SDK is initialized
      ensureSdkInitialized();
      
      setLoading(true);
      setError(null);
      
      try {
        const client = getLiFiClient();
        const result = await getTokens(client, { chains: [targetChainId] });
        const chainTokens = result.tokens[targetChainId] || [];
        // Sort by popularity/common tokens first
        const sortedTokens = chainTokens.sort((a, b) => {
          // Prioritize native tokens and stablecoins
          const prioritySymbols = ['ETH', 'MATIC', 'USDC', 'USDT', 'DAI', 'WETH', 'WMATIC'];
          const aIndex = prioritySymbols.indexOf(a.symbol);
          const bIndex = prioritySymbols.indexOf(b.symbol);
          if (aIndex !== -1 && bIndex !== -1) return aIndex - bIndex;
          if (aIndex !== -1) return -1;
          if (bIndex !== -1) return 1;
          return a.symbol.localeCompare(b.symbol);
        });
        setTokens(sortedTokens.slice(0, 50)); // Limit to top 50 tokens
      } catch (err: unknown) {
        console.error('Failed to fetch tokens:', err);
        setError(errMessage(err) || 'Failed to fetch tokens');
      } finally {
        setLoading(false);
      }
    };

    fetchTokens().catch((err) => console.error("fetchTokens failed:", err));
  }, [targetChainId]);

  return { tokens, loading, error };
};

// Hook to get available chains
export const useAvailableChains = () => {
  const [chains, setChains] = useState<ExtendedChain[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchChains = async () => {
      // Ensure SDK is initialized
      ensureSdkInitialized();
      
      setLoading(true);
      setError(null);
      
      try {
        const client = getLiFiClient();
        const result = await getChains(client);
        // Filter to EVM chains only and sort by popularity
        const evmChains = result.filter(chain => chain.chainType === 'EVM');
        setChains(evmChains);
      } catch (err: unknown) {
        console.error('Failed to fetch chains:', err);
        setError(errMessage(err) || 'Failed to fetch chains');
      } finally {
        setLoading(false);
      }
    };

    fetchChains().catch((err) => console.error("fetchChains failed:", err));
  }, []);

  return { chains, loading, error };
};

// Hook to get payment routes
export const usePaymentRoutes = () => {
  const [routes, setRoutes] = useState<PaymentRoute[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { address } = useAccount();

  const fetchRoutes = useCallback(async (params: {
    fromChainId: number;
    fromTokenAddress: string;
    fromTokenDecimals?: number; // Token decimals (default 18 for native tokens)
    fromTokenPriceUsd?: number; // Token price in USD (for calculating amount)
    toAddress: string;
    amountUsd: number;
    isTestnet?: boolean;
  }) => {
    if (!address) {
      setError('Wallet not connected');
      return [];
    }

    // Ensure SDK is initialized
    ensureSdkInitialized();
    
    setLoading(true);
    setError(null);
    setRoutes([]);

    try {
      const settlementChainId = getSettlementChainId(params.isTestnet);
      const settlementUsdcAddress = USDC_ADDRESSES[settlementChainId];
      const tokenDecimals = params.fromTokenDecimals ?? 18;
      
      // Calculate the minimum USDC amount we need to receive (in 6 decimals)
      const minRequiredToAmount = parseUnits(params.amountUsd.toString(), 6).toString();

      // For the fromAmount, we need to estimate how much of the source token to send
      // If we have a price, calculate based on that; otherwise use a generous estimate
      let fromAmountEstimate: string;
      
      if (params.fromTokenPriceUsd && params.fromTokenPriceUsd > 0) {
        // Calculate amount needed based on token price, with 15% buffer for fees/slippage
        const amountNeeded = (params.amountUsd * 1.15) / params.fromTokenPriceUsd;
        fromAmountEstimate = parseUnits(amountNeeded.toFixed(tokenDecimals), tokenDecimals).toString();
      } else {
        // No price available - use a large amount and let LI.FI find routes
        // For native tokens (ETH, BNB, etc.), estimate ~$3000/token as fallback
        const fallbackPrice = 3000; // Conservative estimate
        const amountNeeded = (params.amountUsd * 1.15) / fallbackPrice;
        fromAmountEstimate = parseUnits(Math.max(amountNeeded, 0.01).toFixed(tokenDecimals), tokenDecimals).toString();
      }

      // Build quote request - use getQuote for single best route (more reliable)
      // First, get a quote with a reasonable amount to determine the exchange rate
      const initialQuoteRequest = {
        fromChain: params.fromChainId,
        toChain: settlementChainId,
        fromToken: params.fromTokenAddress,
        toToken: settlementUsdcAddress,
        fromAddress: address,
        toAddress: params.toAddress,
        fromAmount: fromAmountEstimate,
        slippage: 0.03, // 3% slippage for cross-chain
      };

      const client = getLiFiClient();

      try {
        // Get initial quote to determine exchange rate
        const initialQuote = await getQuote(client, initialQuoteRequest);
        
        if (initialQuote) {
          // Calculate the exchange rate from the initial quote
          const fromAmountNum = parseFloat(initialQuote.action.fromAmount) / Math.pow(10, tokenDecimals);
          const toAmountNum = parseFloat(initialQuote.estimate.toAmount) / 1e6; // USDC has 6 decimals
          const exchangeRate = toAmountNum / fromAmountNum; // USDC per source token
          
          // Calculate the exact amount needed to get the target USDC amount (with 5% buffer for slippage)
          const targetUsdcWithBuffer = params.amountUsd * 1.05;
          const exactFromAmount = targetUsdcWithBuffer / exchangeRate;
          const exactFromAmountWei = parseUnits(exactFromAmount.toFixed(tokenDecimals), tokenDecimals).toString();
          
          // Get a new quote with the exact amount needed
          const exactQuoteRequest = {
            ...initialQuoteRequest,
            fromAmount: exactFromAmountWei,
          };
          
          const exactQuote = await getQuote(client, exactQuoteRequest);
          
          if (exactQuote) {
            const toAmountBigInt = BigInt(exactQuote.estimate.toAmount);
            const minRequiredBigInt = BigInt(minRequiredToAmount);
            
            // If this quote delivers enough, use it
            if (toAmountBigInt >= minRequiredBigInt) {
              const route = convertQuoteToRoute(exactQuote);
              const paymentRoute: PaymentRoute = {
                route,
                fromToken: exactQuote.action.fromToken,
                toToken: exactQuote.action.toToken,
                fromAmount: exactQuote.action.fromAmount,
                toAmount: exactQuote.estimate.toAmount,
                estimatedGas: exactQuote.estimate.gasCosts?.[0]?.amountUSD || '0',
                estimatedTime: exactQuote.estimate.executionDuration || 60,
                steps: 1,
              };
              setRoutes([paymentRoute]);
              return [paymentRoute];
            }
            
            // If not enough, try with a bit more (10% extra)
            const bufferFromAmount = exactFromAmount * 1.10;
            const bufferFromAmountWei = parseUnits(bufferFromAmount.toFixed(tokenDecimals), tokenDecimals).toString();
            
            const bufferQuote = await getQuote(client, { ...initialQuoteRequest, fromAmount: bufferFromAmountWei });
            
            if (bufferQuote) {
              const bufferToAmount = BigInt(bufferQuote.estimate.toAmount);
              if (bufferToAmount >= minRequiredBigInt) {
                const route = convertQuoteToRoute(bufferQuote);
                const paymentRoute: PaymentRoute = {
                  route,
                  fromToken: bufferQuote.action.fromToken,
                  toToken: bufferQuote.action.toToken,
                  fromAmount: bufferQuote.action.fromAmount,
                  toAmount: bufferQuote.estimate.toAmount,
                  estimatedGas: bufferQuote.estimate.gasCosts?.[0]?.amountUSD || '0',
                  estimatedTime: bufferQuote.estimate.executionDuration || 60,
                  steps: 1,
                };
                setRoutes([paymentRoute]);
                return [paymentRoute];
              }
            }
          }
          
          setError('Unable to find a route that delivers the required amount. Try a different token.');
          return [];
        }
      } catch {
      }
      
      // Fallback to getRoutes if getQuote fails
      const routesRequest = {
        fromChainId: params.fromChainId,
        fromTokenAddress: params.fromTokenAddress,
        toChainId: settlementChainId,
        toTokenAddress: settlementUsdcAddress,
        fromAddress: address,
        toAddress: params.toAddress,
        fromAmount: fromAmountEstimate,
      };
      
      const result = await getRoutes(client, routesRequest);
      
      if (!result.routes || result.routes.length === 0) {
        setError('No routes available for this token/chain combination. Try a different token like USDC or ETH.');
        return [];
      }

      // Filter and process routes
      const paymentRoutes = processRoutes(result.routes, minRequiredToAmount);
      
      if (paymentRoutes.length === 0) {
        setError('Unable to find a route that delivers the required amount. Try a different token or amount.');
        return [];
      }

      setRoutes(paymentRoutes);
      return paymentRoutes;
    } catch (err: unknown) {
      console.error('Failed to fetch routes:', err);
      const errorMessage = errMessage(err) || 'Failed to fetch payment routes';
      // Provide more helpful error messages
      if (errorMessage.includes('No available quotes')) {
        setError('No liquidity available for this route. Try a more common token like USDC, ETH, or USDT.');
      } else {
        setError(errorMessage);
      }
      return [];
    } finally {
      setLoading(false);
    }
  }, [address]);

  return { routes, loading, error, fetchRoutes };
};

// Helper function to process and filter routes
function processRoutes(routes: Route[], minRequiredToAmount: string): PaymentRoute[] {
  // Filter routes that deliver at least the required amount
  const validRoutes = routes.filter(route => {
    const toAmountBigInt = BigInt(route.toAmount);
    const minRequiredBigInt = BigInt(minRequiredToAmount);
    return toAmountBigInt >= minRequiredBigInt;
  });

  return validRoutes.map(route => ({
    route,
    fromToken: route.fromToken,
    toToken: route.toToken,
    fromAmount: route.fromAmount,
    toAmount: route.toAmount,
    estimatedGas: route.gasCostUSD || '0',
    estimatedTime: route.steps.reduce((acc, step) => acc + (step.estimate?.executionDuration || 0), 0),
    steps: route.steps.length,
  }));
}

// Utility to format token amount
export const formatTokenAmount = (amount: string, decimals: number): string => {
  const value = BigInt(amount);
  const divisor = BigInt(10 ** decimals);
  const integerPart = value / divisor;
  const fractionalPart = value % divisor;
  
  if (fractionalPart === BigInt(0)) {
    return integerPart.toString();
  }
  
  const fractionalStr = fractionalPart.toString().padStart(decimals, '0');
  const trimmedFractional = fractionalStr.replace(/0+$/, '');
  
  return `${integerPart}.${trimmedFractional}`;
};

// Get chain name
export const getChainName = (chainId: number): string => {
  return CHAIN_NAMES[chainId] || `Chain ${chainId}`;
};
