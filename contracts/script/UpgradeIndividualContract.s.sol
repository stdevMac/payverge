// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";
import "../src/PayvergePayments.sol";
import "../src/PayvergeReferrals.sol";
import "../src/PayvergeProfitSplit.sol";

/**
 * @title UpgradeIndividualContract
 * @dev Script for upgrading individual Payverge contracts
 * 
 * Usage Examples:
 * 
 * Upgrade PayvergePayments:
 * PROXY_ADDRESS=0x... CONTRACT_TYPE=payments forge script script/UpgradeIndividualContract.s.sol --broadcast
 * 
 * Upgrade PayvergeReferrals:
 * PROXY_ADDRESS=0x... CONTRACT_TYPE=referrals forge script script/UpgradeIndividualContract.s.sol --broadcast
 * 
 * Upgrade PayvergeProfitSplit:
 * PROXY_ADDRESS=0x... CONTRACT_TYPE=profit_split forge script script/UpgradeIndividualContract.s.sol --broadcast
 */
contract UpgradeIndividualContract is Script {
    
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);
        
        address proxyAddress = vm.envAddress("PROXY_ADDRESS");
        string memory contractType = vm.envString("CONTRACT_TYPE");
        
        console.log("Deployer:", deployer);
        console.log("Proxy Address:", proxyAddress);
        console.log("Contract Type:", contractType);
        
        vm.startBroadcast(deployerPrivateKey);
        
        if (keccak256(abi.encodePacked(contractType)) == keccak256(abi.encodePacked("payments"))) {
            upgradePayments(proxyAddress);
        } else if (keccak256(abi.encodePacked(contractType)) == keccak256(abi.encodePacked("referrals"))) {
            upgradeReferrals(proxyAddress);
        } else if (keccak256(abi.encodePacked(contractType)) == keccak256(abi.encodePacked("profit_split"))) {
            upgradeProfitSplit(proxyAddress);
        } else {
            revert("Invalid CONTRACT_TYPE. Use: payments, referrals, or profit_split");
        }
        
        vm.stopBroadcast();
        
        console.log("\n=== UPGRADE COMPLETE ===");
        console.log("Contract upgraded successfully");
        console.log("Proxy address remains the same:", proxyAddress);
    }
    
    function upgradePayments(address proxyAddress) internal {
        console.log("\n=== UPGRADING PAYVERGE PAYMENTS ===");
        
        PayvergePayments newImplementation = new PayvergePayments();
        console.log("New implementation deployed:", address(newImplementation));
        
        PayvergePayments proxy = PayvergePayments(proxyAddress);
        
        // Check permissions
        require(
            proxy.hasRole(proxy.UPGRADER_ROLE(), msg.sender),
            "Missing UPGRADER_ROLE"
        );
        
        string memory oldVersion = proxy.version();
        console.log("Upgrading from version:", oldVersion);
        
        proxy.upgradeToAndCall(address(newImplementation), "");
        
        console.log("Upgraded to version:", proxy.version());
        console.log("PayvergePayments upgrade successful");
    }
    
    function upgradeReferrals(address proxyAddress) internal {
        console.log("\n=== UPGRADING PAYVERGE REFERRALS ===");
        
        PayvergeReferrals newImplementation = new PayvergeReferrals();
        console.log("New implementation deployed:", address(newImplementation));
        
        PayvergeReferrals proxy = PayvergeReferrals(proxyAddress);
        
        // Check permissions
        require(
            proxy.hasRole(proxy.UPGRADER_ROLE(), msg.sender),
            "Missing UPGRADER_ROLE"
        );
        
        string memory oldVersion = proxy.version();
        console.log("Upgrading from version:", oldVersion);
        
        proxy.upgradeToAndCall(address(newImplementation), "");
        
        console.log("Upgraded to version:", proxy.version());
        console.log("PayvergeReferrals upgrade successful");
    }
    
    function upgradeProfitSplit(address proxyAddress) internal {
        console.log("\n=== UPGRADING PAYVERGE PROFIT SPLIT ===");
        
        PayvergeProfitSplit newImplementation = new PayvergeProfitSplit();
        console.log("New implementation deployed:", address(newImplementation));
        
        PayvergeProfitSplit proxy = PayvergeProfitSplit(payable(proxyAddress));
        
        // Check permissions
        require(
            proxy.hasRole(proxy.UPGRADER_ROLE(), msg.sender),
            "Missing UPGRADER_ROLE"
        );
        
        string memory oldVersion = proxy.version();
        console.log("Upgrading from version:", oldVersion);
        
        proxy.upgradeToAndCall(address(newImplementation), "");
        
        console.log("Upgraded to version:", proxy.version());
        console.log("PayvergeProfitSplit upgrade successful");
    }
}
