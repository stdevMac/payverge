// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";
import "../src/PayvergePayments.sol";
import "../src/PayvergeReferrals.sol";
import "../src/PayvergeProfitSplit.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract UpgradePayvergeContracts is Script {
    // Contract types for upgrade selection
    enum ContractType {
        PAYMENTS,
        REFERRALS,
        PROFIT_SPLIT,
        ALL
    }

    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);

        // Get existing contract addresses
        address paymentsProxy = vm.envAddress("PAYVERGE_CONTRACT_ADDRESS");
        address referralsProxy = vm.envAddress("PAYVERGE_REFERRALS_ADDRESS");
        address profitSplitProxy = vm.envAddress("PAYVERGE_PROFIT_SPLIT_ADDRESS");

        // Get upgrade type from environment (0=PAYMENTS, 1=REFERRALS, 2=PROFIT_SPLIT, 3=ALL)
        uint256 upgradeTypeInt = vm.envOr("UPGRADE_TYPE", uint256(3)); // Default to ALL
        ContractType upgradeType = ContractType(upgradeTypeInt);

        console.log("Deployer:", deployer);
        console.log("PayvergePayments Proxy:", paymentsProxy);
        console.log("PayvergeReferrals Proxy:", referralsProxy);
        console.log("PayvergeProfitSplit Proxy:", profitSplitProxy);
        console.log("Upgrade Type:", upgradeTypeInt);

        vm.startBroadcast(deployerPrivateKey);

        if (upgradeType == ContractType.PAYMENTS || upgradeType == ContractType.ALL) {
            upgradePayments(paymentsProxy);
        }

        if (upgradeType == ContractType.REFERRALS || upgradeType == ContractType.ALL) {
            upgradeReferrals(referralsProxy);
        }

        if (upgradeType == ContractType.PROFIT_SPLIT || upgradeType == ContractType.ALL) {
            upgradeProfitSplit(profitSplitProxy);
        }

        vm.stopBroadcast();

        console.log("\n=== UPGRADE COMPLETE ===");
        console.log("All specified contracts have been upgraded successfully");
        console.log("Proxy addresses remain the same - no frontend changes needed");
    }

    function upgradePayments(address proxyAddress) internal {
        console.log("\n=== UPGRADING PAYVERGE PAYMENTS ===");

        // Deploy new implementation
        PayvergePayments newImplementation = new PayvergePayments();
        console.log("New PayvergePayments implementation:", address(newImplementation));

        // Get the proxy contract
        PayvergePayments proxy = PayvergePayments(proxyAddress);

        // Verify we have upgrader role
        require(
            proxy.hasRole(proxy.UPGRADER_ROLE(), msg.sender), "Caller does not have UPGRADER_ROLE for PayvergePayments"
        );

        // Store old version for comparison
        string memory oldVersion = proxy.version();
        console.log("Old version:", oldVersion);

        // Perform upgrade
        proxy.upgradeToAndCall(address(newImplementation), "");

        // Verify upgrade
        string memory newVersion = proxy.version();
        console.log("New version:", newVersion);

        // Verify core functionality still works
        console.log("USDC token:", address(proxy.usdcToken()));
        console.log("Platform fee rate:", proxy.platformFeeRate());
        console.log("Referrals contract:", address(proxy.referralsContract()));
        console.log("Profit split contract:", address(proxy.profitSplitContract()));

        console.log("PayvergePayments upgrade successful");
    }

    function upgradeReferrals(address proxyAddress) internal {
        console.log("\n=== UPGRADING PAYVERGE REFERRALS ===");

        // Deploy new implementation
        PayvergeReferrals newImplementation = new PayvergeReferrals();
        console.log("New PayvergeReferrals implementation:", address(newImplementation));

        // Get the proxy contract
        PayvergeReferrals proxy = PayvergeReferrals(proxyAddress);

        // Verify we have upgrader role
        require(
            proxy.hasRole(proxy.UPGRADER_ROLE(), msg.sender), "Caller does not have UPGRADER_ROLE for PayvergeReferrals"
        );

        // Store old version for comparison
        string memory oldVersion = proxy.version();
        console.log("Old version:", oldVersion);

        // Perform upgrade
        proxy.upgradeToAndCall(address(newImplementation), "");

        // Verify upgrade
        string memory newVersion = proxy.version();
        console.log("New version:", newVersion);

        // Verify core functionality still works
        console.log("USDC token:", address(proxy.usdcToken()));
        console.log("Platform treasury:", proxy.platformTreasury());
        console.log("Total referrers:", proxy.totalReferrers());

        console.log("PayvergeReferrals upgrade successful");
    }

    function upgradeProfitSplit(address proxyAddress) internal {
        console.log("\n=== UPGRADING PAYVERGE PROFIT SPLIT ===");

        // Deploy new implementation
        PayvergeProfitSplit newImplementation = new PayvergeProfitSplit();
        console.log("New PayvergeProfitSplit implementation:", address(newImplementation));

        // Get the proxy contract
        PayvergeProfitSplit proxy = PayvergeProfitSplit(payable(proxyAddress));

        // Verify we have upgrader role
        require(
            proxy.hasRole(proxy.UPGRADER_ROLE(), msg.sender),
            "Caller does not have UPGRADER_ROLE for PayvergeProfitSplit"
        );

        // Store old version for comparison
        string memory oldVersion = proxy.version();
        console.log("Old version:", oldVersion);

        // Perform upgrade
        proxy.upgradeToAndCall(address(newImplementation), "");

        // Verify upgrade
        string memory newVersion = proxy.version();
        console.log("New version:", newVersion);

        // Verify core functionality still works
        console.log("USDC token:", address(proxy.usdcToken()));
        console.log("Platform treasury:", proxy.platformTreasury());
        console.log("Beneficiary count:", proxy.beneficiaryCount());
        console.log("Total distributed:", proxy.totalDistributed());

        console.log("PayvergeProfitSplit upgrade successful");
    }
}
