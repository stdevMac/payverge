"use client";
import React, { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAccount } from "wagmi";
import { Button } from "@nextui-org/react";
import { claim_reward } from "@/api/rewards/claimReward";
import toast from "react-hot-toast"; // Import hot-toast

const Web3Button = () => {
  return <appkit-button label="Connect Wallet" balance="hide" />;
};

interface Props {
  params: {
    code: string;
  };
}

export default function ClaimReward({ params }: Props) {
  const { code } = params;
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [points, setPoints] = useState<number | null>(null);
  const { address, isConnected } = useAccount();
  const [mounted, setMounted] = useState(false);
  const [isClaiming, setIsClaiming] = useState(false);
  const router = useRouter();

  useEffect(() => {
    setMounted(true);
  }, []);

  const handleRewardClaim = async (connectedAddress: any) => {
    try {
      setIsClaiming(true);

      const response = await claim_reward({
        address: connectedAddress,
        code: code,
      });

      if (response) {
        setSuccessMessage(response.message);
        setPoints(response.points);
        toast.success(
          `Reward claimed successfully! You received ${response.points} points.`,
        );
      } else {
        const errorMsg = "Failed to claim reward. Please try again.";
        setErrorMessage(errorMsg);
        toast.error(errorMsg);
      }
    } catch (error) {
      console.error("Error during reward claim:", error);
      const errorMsg = "An error occurred. Please try again.";
      setErrorMessage(errorMsg);
      toast.error(errorMsg);
    } finally {
      setIsClaiming(false);
    }
  };

  const handleRedirectToMain = () => {
    router.push("/");
  };

  return (
    <div className="container mx-auto px-4 py-8">
      <h1 className="text-3xl font-bold mb-4">Claim Your Reward</h1>
      <p className="mb-4">
        Reward Code: <span className="font-semibold">{code}</span>
      </p>

      {!isConnected && mounted && (
        <div className="mb-4">
          <p className="mb-2">Connect your wallet to claim your reward:</p>
          <Web3Button />
        </div>
      )}

      {isConnected && !successMessage && (
        <div className="mb-4">
          <p className="text-green-600 font-semibold">
            Wallet connected: {address}
          </p>
          <Button
            color="primary"
            onClick={() => handleRewardClaim(address)}
            disabled={isClaiming}
          >
            {isClaiming ? "Claiming..." : "Claim Reward"}
          </Button>
        </div>
      )}

      {successMessage && (
        <div className="mb-4">
          <p className="text-green-600">{successMessage}</p>
          {points !== null && (
            <p className="text-green-600">You have received {points} points!</p>
          )}
          <Button
            color="primary"
            onClick={handleRedirectToMain}
            className="mt-2"
          >
            Go to Main Page
          </Button>
        </div>
      )}

      {errorMessage && <p className="text-red-600 mb-4">{errorMessage}</p>}
    </div>
  );
}
