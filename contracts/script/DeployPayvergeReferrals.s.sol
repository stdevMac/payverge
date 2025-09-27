// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";
import "../src/PayvergeReferrals.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract DeployPayvergeReferrals is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);
        
        // Get environment variables
        address usdcToken = vm.envAddress("USDC_TOKEN_ADDRESS");
        address platformTreasury = vm.envAddress("PLATFORM_TREASURY");
        address admin = vm.envAddress("ADMIN_ADDRESS");
        
        console.log("Deploying PayvergeReferrals...");
        console.log("Deployer:", deployer);
        console.log("USDC Token:", usdcToken);
        console.log("Platform Treasury:", platformTreasury);
        console.log("Admin:", admin);

        vm.startBroadcast(deployerPrivateKey);

        // Deploy implementation
        PayvergeReferrals implementation = new PayvergeReferrals();
        console.log("Implementation deployed at:", address(implementation));

        // Prepare initialization data
        bytes memory initData = abi.encodeWithSelector(
            PayvergeReferrals.initialize.selector,
            usdcToken,
            platformTreasury,
            admin
        );

        // Deploy proxy
        ERC1967Proxy proxy = new ERC1967Proxy(
            address(implementation),
            initData
        );
        
        console.log("Proxy deployed at:", address(proxy));
        
        // Wrap proxy in interface
        PayvergeReferrals referrals = PayvergeReferrals(address(proxy));
        
        // Verify initialization
        console.log("USDC Token set to:", address(referrals.usdcToken()));
        console.log("Platform Treasury set to:", referrals.platformTreasury());
        console.log("Total Referrers:", referrals.totalReferrers());
        console.log("Contract Version:", referrals.version());

        vm.stopBroadcast();

        console.log("\n=== Deployment Summary ===");
        console.log("PayvergeReferrals Implementation:", address(implementation));
        console.log("PayvergeReferrals Proxy:", address(proxy));
        console.log("Admin should call setPayvergePaymentsContract() to complete setup");
    }
}
