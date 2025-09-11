// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";
import "../src/PayvergePaymentsV2.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

/**
 * @title Deploy PayvergePayments V2
 * @dev Deployment script for upgradeable PayvergePayments contract
 */
contract DeployPayvergeV2 is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address usdcToken = vm.envAddress("USDC_TOKEN_ADDRESS");
        address platformTreasury = vm.envAddress("PLATFORM_TREASURY");
        uint256 platformFeeRate = vm.envUint("PLATFORM_FEE_RATE"); // in basis points
        
        vm.startBroadcast(deployerPrivateKey);

        // Deploy implementation
        PayvergePaymentsV2 implementation = new PayvergePaymentsV2();
        console.log("Implementation deployed at:", address(implementation));

        // Prepare initialization data
        bytes memory initData = abi.encodeWithSelector(
            PayvergePaymentsV2.initialize.selector,
            usdcToken,
            platformTreasury,
            platformFeeRate
        );

        // Deploy proxy
        ERC1967Proxy proxy = new ERC1967Proxy(
            address(implementation),
            initData
        );
        console.log("Proxy deployed at:", address(proxy));

        PayvergePaymentsV2 payverge = PayvergePaymentsV2(address(proxy));
        console.log("Contract version:", payverge.version());
        console.log("Platform fee rate:", payverge.platformFeeRate());
        console.log("USDC token:", address(payverge.usdcToken()));
        console.log("Platform treasury:", payverge.platformTreasury());

        vm.stopBroadcast();
    }
}
