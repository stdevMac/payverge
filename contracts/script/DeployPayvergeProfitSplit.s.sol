// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";
import "../src/PayvergeProfitSplit.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract DeployPayvergeProfitSplit is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);
        
        // Get environment variables
        address usdcToken = vm.envAddress("USDC_TOKEN_ADDRESS");
        address platformTreasury = vm.envAddress("PLATFORM_TREASURY");
        address admin = vm.envAddress("ADMIN_ADDRESS");
        
        console.log("Deploying PayvergeProfitSplit...");
        console.log("Deployer:", deployer);
        console.log("USDC Token:", usdcToken);
        console.log("Platform Treasury:", platformTreasury);
        console.log("Admin:", admin);

        vm.startBroadcast(deployerPrivateKey);

        // Deploy implementation
        PayvergeProfitSplit implementation = new PayvergeProfitSplit();
        console.log("Implementation deployed at:", address(implementation));

        // Prepare initialization data
        bytes memory initData = abi.encodeWithSelector(
            PayvergeProfitSplit.initialize.selector,
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
        PayvergeProfitSplit profitSplit = PayvergeProfitSplit(payable(address(proxy)));
        
        // Verify initialization
        console.log("USDC Token set to:", address(profitSplit.usdcToken()));
        console.log("Platform Treasury set to:", profitSplit.platformTreasury());
        console.log("Beneficiary Count:", profitSplit.beneficiaryCount());
        console.log("Contract Version:", profitSplit.version());

        vm.stopBroadcast();

        console.log("\n=== Deployment Summary ===");
        console.log("PayvergeProfitSplit Implementation:", address(implementation));
        console.log("PayvergeProfitSplit Proxy:", address(proxy));
        console.log("Admin can now add beneficiaries and configure profit sharing");
    }
}
