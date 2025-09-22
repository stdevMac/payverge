// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";
import "../src/PayvergePayments.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract DeployPayvergePayments is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);
        
        // Get environment variables
        address usdcToken = vm.envAddress("USDC_TOKEN_ADDRESS");
        address platformFeeRecipient = vm.envAddress("PLATFORM_FEE_RECIPIENT");
        uint256 platformFeeBps = vm.envUint("PLATFORM_FEE_BPS"); // Default: 200 (2%)
        address billCreator = vm.envAddress("BILL_CREATOR_ADDRESS");
        uint256 registrationFee = vm.envUint("REGISTRATION_FEE"); // Default: 0 (free registration)
        
        console.log("Deployer:", deployer);
        console.log("USDC Token:", usdcToken);
        console.log("Platform Fee Recipient:", platformFeeRecipient);
        console.log("Platform Fee BPS:", platformFeeBps);
        console.log("Bill Creator:", billCreator);
        console.log("Registration Fee:", registrationFee);
        
        vm.startBroadcast(deployerPrivateKey);
        
        // Deploy implementation contract
        PayvergePayments implementation = new PayvergePayments();
        console.log("Implementation deployed at:", address(implementation));
        
        // Prepare initialization data
        bytes memory initData = abi.encodeWithSelector(
            PayvergePayments.initialize.selector,
            usdcToken,
            platformFeeRecipient,
            platformFeeBps,
            deployer, // admin
            billCreator,
            registrationFee
        );
        
        // Deploy proxy
        ERC1967Proxy proxy = new ERC1967Proxy(
            address(implementation),
            initData
        );
        
        console.log("Proxy deployed at:", address(proxy));
        
        // Verify the deployment
        PayvergePayments payvergeContract = PayvergePayments(address(proxy));
        console.log("Contract version:", payvergeContract.version());
        console.log("USDC token address:", address(payvergeContract.usdcToken()));
        console.log("Platform treasury:", payvergeContract.platformTreasury());
        console.log("Platform fee rate:", payvergeContract.platformFeeRate());
        console.log("Bill creator address:", payvergeContract.billCreatorAddress());
        console.log("Registration fee:", payvergeContract.getRegistrationFee());
        
        vm.stopBroadcast();
        
        // Output deployment info
        console.log("\n=== DEPLOYMENT COMPLETE ===");
        console.log("Implementation:", address(implementation));
        console.log("Proxy (Main Contract):", address(proxy));
        console.log("Contract Version:", payvergeContract.version());
        console.log("\n=== ENVIRONMENT VARIABLES NEEDED ===");
        console.log("PAYVERGE_CONTRACT_ADDRESS=", address(proxy));
        console.log("USDC_CONTRACT_ADDRESS=", usdcToken);
        console.log("\n=== FRONTEND INTEGRATION ===");
        console.log("Use Proxy address for all frontend interactions");
        console.log("Contract supports unified payment system - single createBill() and processPayment() functions");
    }
}
