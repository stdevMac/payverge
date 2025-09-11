// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";
import "../src/PayvergePaymentsV2.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract DeployPayvergePaymentsV2 is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);
        
        // Get environment variables
        address usdcToken = vm.envAddress("USDC_TOKEN_ADDRESS");
        address platformFeeRecipient = vm.envAddress("PLATFORM_FEE_RECIPIENT");
        uint256 platformFeeBps = vm.envUint("PLATFORM_FEE_BPS"); // Default: 200 (2%)
        
        console.log("Deployer:", deployer);
        console.log("USDC Token:", usdcToken);
        console.log("Platform Fee Recipient:", platformFeeRecipient);
        console.log("Platform Fee BPS:", platformFeeBps);
        
        vm.startBroadcast(deployerPrivateKey);
        
        // Deploy implementation contract
        PayvergePaymentsV2 implementation = new PayvergePaymentsV2();
        console.log("Implementation deployed at:", address(implementation));
        
        // Prepare initialization data
        bytes memory initData = abi.encodeWithSelector(
            PayvergePaymentsV2.initialize.selector,
            usdcToken,
            platformFeeRecipient,
            platformFeeBps
        );
        
        // Deploy proxy
        ERC1967Proxy proxy = new ERC1967Proxy(
            address(implementation),
            initData
        );
        
        console.log("Proxy deployed at:", address(proxy));
        
        // Verify the deployment
        PayvergePaymentsV2 payvergeContract = PayvergePaymentsV2(address(proxy));
        console.log("Contract version:", payvergeContract.version());
        console.log("USDC token address:", address(payvergeContract.usdcToken()));
        console.log("Platform fee recipient:", payvergeContract.platformFeeRecipient());
        console.log("Platform fee BPS:", payvergeContract.platformFeeBps());
        
        vm.stopBroadcast();
        
        // Output deployment info
        console.log("\n=== DEPLOYMENT COMPLETE ===");
        console.log("Implementation:", address(implementation));
        console.log("Proxy (Main Contract):", address(proxy));
        console.log("Use Proxy address for frontend integration");
    }
}
