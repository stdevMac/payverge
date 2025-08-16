// Define available locales
export const locales = ["en", "es"] as const;
export type Locale = (typeof locales)[number];

// English translations
export const en = {
  nft: {
    loading: "Loading NFT details...",
    title: "Claim your NFT!",
    connectWallet: {
      title: "Connect Your Wallet",
      description: "Connect your wallet to mint {{name}}",
    },
    signIn: {
      title: "Sign In Required",
      description: "Please sign in to mint your {{name}} NFT",
    },
    mint: {
      benefitsTitle: "NFT Benefits",
      eligibleForGas: "We pay for gas 🤝",
      viewOnExplorer: "View on Explorer",
      buttonLabel: "Mint NFT",
    },
    minting: {
      status: "Minting your NFT...",
      sponsorDelayDisclaimer:
        "Your wallet may take a few seconds to recognize the sponsored gas",
    },
    success: {
      title: "NFT Minted Successfully!",
      viewTransaction: "View Transaction",
      description:
        "Congratulations! Your {{name}} has been minted successfully.",
      shareButton: "Share on X",
      shareOnXContent: "I just minted my {{name}} NFT on @tokenfleet_io! 🚗\n",
      addToWalletButton: "Add to Wallet",
      addingToWalletButton: "Adding to Wallet...",
      mainButton: "Go to Main Page",
    },
    errors: {
      title: "Error",
      alreadyMinted: "This NFT has already been minted or doesn't exist.",
      backButton: "Go to Main Page",
    },
  },
  fleets: {
    gridItem: {
      car: "Car",
      cars: "Cars",
      stage: {
        creating: {
          label: "In Creation",
          metric: "Target Investment",
        },
        posted: {
          label: "Open for Investment",
          metric: "Total Invested",
        },
        funded: {
          label: "Fully Funded",
          metric: "Expected Annual Return",
        },
        onpurchase: {
          label: "Purchase in Progress",
          metric: "Cars Purchased",
          ofTotal: "of",
        },
        active: {
          label: "Operational",
          metric: "Expected Monthly Income",
        },
        closing: {
          label: "Closing",
          metric: "Expected Returns",
        },
        closed: {
          label: "Closed",
          metric: "Final Returns",
        },
        notrealized: {
          label: "Not Realized",
          metric: "Final Returns",
        },
        unknown: {
          label: "Unknown Stage",
          metric: "Total Investment",
        },
      },
      metrics: {
        fleetSize: "Fleet Size",
        targetFunding: "Target Funding",
        expectedApr: "Expected APR",
        totalInvested: "Total Invested",
        apr: "APR",
        timeLeft: "Time Left",
        totalFunded: "Total Funded",
        purchaseCost: "Purchase Cost",
        expectedIncome: "Expected Income",
        netAnnual: "Net Annual",
        totalInvestment: "Total Investment",
        totalProfit: "Total Profit",
        return: "Return",
      },
      timeLeft: {
        closingSoon: "Closing Soon",
        days: "days left",
        day: "day left",
        hours: "h left",
      },
      fundingProgress: {
        funded: "Funded",
      },
    },
    noFleetsFound: "No Fleets Found",
    noStatusFleetsFound: "No {{status}} Fleets Found",
    status: {
      all: "All",
      posted: "Posted",
      funded: "Funded",
      onpurchase: "On Purchase",
      active: "Active",
      closing: "Closing",
      closed: "Closed",
    },
    emptyState: {
      all: "There are currently no fleets available. Please check back later for new investment opportunities.",
      posted:
        "There are no fleets currently available for investment. New opportunities will be posted soon.",
      funded:
        "There are no fleets that have completed their funding phase. Check the Available to Invest section for current opportunities.",
      onpurchase:
        "There are no fleets currently in the purchase process. Check back soon for updates on our fleet acquisitions.",
      active:
        "There are no fleets currently in operation. Check the Available to Invest section for upcoming opportunities.",
      closing:
        "There are no fleets currently in the closing phase. Check the Active Fleets section to see our operating vehicles.",
      closed:
        "There are no completed fleet operations at this time. View our Active Fleets to see current investments.",
      default:
        "No fleets found in this category. Please check other sections for available opportunities.",
    },
  },
  shared: {
    shareBar: {
      message: "Share & earn rewards!",
      buttonText: "Copy Link",
      benefitsTitle: "Referral Program Benefits",
      benefitsSubtitle: "Learn how you can earn rewards",
      linkCopied:
        "Referral link copied! Share it with friends to start earning.",
      copyError: "Failed to copy referral link",
      copied: "Link copied to clipboard!",
      benefits: {
        fleetSpecific: {
          title: "Fleet-Specific Rewards",
          description:
            "Each fleet offers its own referral percentage based on the fleet's structure and margins",
        },
        earnings: {
          title: "Immediate & Ongoing Earnings",
          description:
            "Earn instantly from fleet purchase fees when your referrals invest, plus ongoing rewards from fleet operations",
        },
        incomeSources: {
          title: "Multiple Income Sources",
          description:
            "Earn from fleet purchase fees, rental income, car sales, and other operational returns",
        },
        bulletPoints: [
          "Initial reward from fleet purchase fees when referrals invest",
          "Ongoing rewards automatically calculated and distributed",
          "No limit on the number of referrals per fleet",
          "View fleet-specific referral rates in each fleet's details",
        ],
      },
    },
    contractModal: {
      title: "Investment Contract",
      contractAgreement: "Contract Agreement",
      contractIntro:
        'This Investment Contract ("Contract") is entered into by and between the Investor ("You") and Reliable Route Car Rentals LLC ("Company"). By signing below, you agree to invest {{amount}} USDC in the Company\'s car asset with the Smart Contract Address: {{address}}.',
      userPassport: "User Passport",
      userInformation: "User Information",
      name: "Name",
      address: "Address",
      userRights: "User Rights",
      userRightsIntro:
        "As an investor, you are entitled to the following rights:",
      userRightsList: [
        "Receive regular updates on the investment status.",
        "Participate in major decisions regarding the car asset.",
        "Access to detailed financial reports upon request.",
      ],
      investmentDetails: "Investment Details",
      investmentDetailsText:
        "Your investment entitles you to an ownership percentage of {{percentage}}% in the car asset. Based on your investment, the expected return is ${{return}}.",
      rentSplitAgreement: "Rent Split Agreement",
      rentSplitText:
        "The distribution of rents from the car asset will be as follows:",
      rentSplitList: [
        "10% on the purchase of the car.",
        "30% each time the car is rented.",
        "10% once the car is sold.",
      ],
      agreementText:
        "By signing this contract, you acknowledge and agree to the terms and conditions outlined above.",
      signBelow: "Please sign below",
      clearSignature: "Clear Signature",
      contractNote:
        "*The contract will be sent via email and accessible on your profile.",
      cancel: "Cancel",
      confirmAndSign: "Confirm and Sign",
      pleaseSign: "Please provide a signature before confirming.",
    },
    documentModal: {
      loading: "Loading document...",
      error: "Unable to load the document.",
      tryAgain: "Please try again later.",
      close: "Close",
      download: "Download",
    },
    receiptModal: {
      loading: "Loading receipt...",
      error: "Unable to load the receipt.",
      tryAgain: "Please try again later.",
      close: "Close",
      download: "Download",
    },
    unverifiedUserModal: {
      title: "Verification Required",
      description:
        "Before you can invest in this fleet, we need to verify your identity to comply with regulatory requirements.",
      whyTitle: "Why do we need this?",
      reasons: {
        compliance: "Ensure compliance with financial regulations",
        security: "Protect against fraud and maintain security",
        investments: "Enable safe and secure investments",
      },
      buttons: {
        cancel: "Cancel",
        startVerification: "Start Verification",
      },
    },
    investmentProcessModal: {
      title: "Investment Process",
      steps: {
        investmentDetails: "Investment Details",
        setupRequired: "Setup Required",
        contractReview: "Contract Review",
        success: "Welcome Aboard!",
        currentAndTotal: "Step {{current}} of {{total}}",
      },
      setupRequired: {
        oneTimeSetup: "One-Time Setup Required",
        setupDescription:
          "Before proceeding with your investment, we need to complete two quick steps:",
        activateAccount: "Activate Your Account",
        activateDescription: "Enable your verified wallet for investing",
        createPortfolio: "Create Your Portfolio",
        portfolioDescription: "Your investment command center",
        oneTimeProcess:
          "This is a one-time process that will enable you to invest in any fleet on our platform.",
        proceedToSetup: "Proceed to Account Activation and Portfolio Setup",
      },
      buttons: {
        cancel: "Cancel",
        proceed: "Proceed",
        proceedToPortfolio: "Proceed to Portfolio Setup",
        proceedToContract: "Proceed to Contract",
        startVerification: "Start Verification",
        signContract: "Sign Contract",
        continue: "Continue",
      },
      investmentDetails: {
        investmentAmount: "Investment Amount",
        balance: "Balance",
        enterAmount: "Enter amount",
        max: "Max",
        investmentSummary: "Investment Summary",
        projection: "4-year projection",
        ownershipShare: "Ownership Share",
        expectedReturn: "Expected Return",
        totalApr: "Total APR",
        annual: "annual",
        termsAndConditions: "I have read and agree to the terms and conditions",
        minInvestment: "Minimum investment is ${{amount}}",
        maxInvestment: "Maximum investment is ${{amount}}",
        rewardCode: "Reward Code",
        enterCode: "Enter reward code",
        applyCode: "Apply",
        removeCode: "Remove",
        codeValid: "Valid code",
        codeInvalid: "Invalid code",
        codeError: "Error checking code",
        codeApplied: "Reward code applied! ${{amount}} USDC will be added to your account when you complete this investment.",
        // New translations for reward code UI
        fromRewardCode: "(from reward code)",
        totalAvailable: "Total available",
        maximumInvestmentWithReward: "Maximum investment is ${{amount}} USDC (includes ${{rewardAmount}} from reward code)"
      },
      tooltips: {
        investmentLimits:
          "Min: ${{min}} | Max: ${{max}}\nYour USDC Balance: ${{balance}}\nRemaining to Fund: ${{remaining}}",
        projectionPeriod:
          "Our projections are based on a 4-year investment period.\nYou'll have flexibility to adjust this timeframe based on\nmarket conditions and fleet performance.",
        ownershipPercentage:
          "Your percentage ownership of the fleet based on your investment amount",
        returnBreakdown:
          "Total projected returns including:\n• Rental income\n• Vehicle resale value\n• Returned security deposit",
        rewardCode: "Enter a reward code to get bonus USDC for your investment",
      },
      contractReview: {
        scrollToRead: "Please scroll through the entire contract to proceed",
        thanksForReading: "Thank you for reading the contract",
        contractTitle: "Investment Contract Agreement",
        contractIntro:
          'This Investment Contract Agreement ("Agreement") is entered into as of {{date}} by and between:',
        investor: 'Investor ("You"):',
        fullName: "Full Legal Name:",
        documentType: "Document Type:",
        documentNumber: "Document Number:",
        walletAddress: "Wallet Address:",
        emailAddress: "Email Address:",
        company: "Company:",
        companyName: "Name:",
        companyNameValue: 'Reliable Route Car Rentals LLC ("Company")',
        registeredOffice: "Registered Office:",
        officeLocation: "Dubai, UAE",
        parties: 'Collectively referred to as the "Parties."',
        investmentDetails: "1. Investment Details",
        investmentAgreement:
          "You hereby agree to invest the amount of ${{amount}} USDC into the following fleet:",
        fleetName: "Fleet Name:",
        fleetDescription: "Fleet Description:",
        contractAddress: "Contract Address:",
        ownershipPercentage: "Ownership Percentage:",
        fleetAssets: "2. Fleet Assets",
        fleetAssetsDescription:
          "The fleet comprises the following vehicles, which will be actively managed and rented by the Company:",
        revenueDistribution: "3. Revenue Distribution",
        revenueDescription:
          "As an investor, you are entitled to receive revenue distributions based on your ownership percentage in the fleet:",
        rentalIncome: "Rental Income:",
        rentalIncomeInvestors:
          "Investors shall receive 70% of the gross rental income, proportional to their ownership share",
        rentalIncomePlatform:
          "The platform shall retain 30% of the gross rental income for operational costs and fees",
        finalSaleRewards: "Final Sale Rewards:",
        finalSaleDescription:
          "Upon vehicle sales, investors receive 90% of net proceeds, proportional to ownership",
        securityDeposit: "Security Deposit:",
        securityDepositAmount:
          "100% of security deposit (proportional to ownership share)",
        securityDepositPurpose:
          "Serves as reserve fund for repairs and unexpected expenses",
        termsAndConditions: "4. Terms and Conditions",
        transparency: "Transparency and Access:",
        transparencyAccess:
          "Full access to transaction details, revenue reports, and fleet performance metrics",
        transparencyBlockchain:
          "Blockchain technology ensures automated calculations and distributions",
        rewardProcessing: "Reward Processing:",
        rewardCalculation:
          "All rental income calculated based on smart contract specifications",
        rewardAvailability: "Unclaimed rewards remain available until claimed",
        rewardDistributions:
          "Revenue distributions available in your Token Fleet platform account",
        gasFees: "Gas Fees:",
        gasFeesResponsibility:
          "Investors are responsible for blockchain network fees",
        gasFeesApply:
          "Fees apply when claiming rewards or executing transactions",
        managementRights: "Management Rights:",
        managementCompany:
          "Company retains exclusive right to manage fleet assets",
        managementIncludes:
          "Includes rights to rent and sell vehicles for optimal returns",
        marketRisks: "Market Risks:",
        risksInherent:
          "All investments carry inherent market and operational risks",
        risksVariation: "Returns may vary based on market conditions",
        legalCompliance: "Legal and Compliance:",
        legalGoverned: "Agreement governed by the laws of Dubai, UAE",
        legalDisputes: "Disputes resolved under Dubai jurisdiction",
        dataProtection: "Data Protection:",
        dataSecure: "Personal and financial data securely stored and processed",
        dataUsage: "Used solely for fulfilling this Agreement",
        forceMajeure: "Force Majeure:",
        forceMajeureNotLiable:
          "The Company is not liable for delays or losses caused by unforeseen circumstances",
        forceMajeureIncludes:
          "Including natural disasters, regulatory changes, or blockchain network failures",
        amendments: "Amendments:",
        amendmentsAgreement:
          "Any changes to this Agreement must be agreed upon in writing by both Parties",
        amendmentsDocumented: "Modifications must be documented and signed",
        expectedReturns: "5. Expected Returns",
        projectedApr: "Projected Total APR:",
        distributionFrequency: "Distribution Frequency:",
        distributionImmediate: "Immediate upon rental completion",
        returnsNote:
          "Note: Actual returns may vary based on market performance and operational factors.",
        investorResponsibilities: "6. INVESTOR RESPONSIBILITIES",
        responsibilitiesConfirm:
          "By signing this Agreement, you confirm that you:",
        understandRisks:
          "1. Understand the nature and risks associated with blockchain-based investments",
        reviewedTerms:
          "2. Have reviewed all terms and conditions outlined in this Agreement",
        maintainWallet:
          "3. Agree to maintain an active wallet compatible with the platform",
        notifyChanges:
          "4. Will promptly notify the Company of any changes to your contact information",
        additionalClauses: "7. ADDITIONAL CLAUSES",
        dataProtectionTitle: "Data Protection:",
        dataProtectionContent:
          "Your personal and financial data will be securely stored and processed",
        forceMajeureTitle: "Force Majeure:",
        forceMajeureContent1:
          "The Company is not liable for delays or losses caused by unforeseen circumstances",
        forceMajeureContent2:
          "Including natural disasters, regulatory changes, or blockchain failures",
        terminationTitle: "Termination:",
        terminationContent:
          "This Agreement may be terminated by mutual consent or by written notice",
        amendmentsTitle: "Amendments:",
        amendmentsContent:
          "Any changes to this Agreement must be agreed upon in writing by both Parties",
        signatureTitle: "8. Signature",
        signatureConfirm:
          "By signing below, you confirm that you have read, understood, and agree to all terms and conditions outlined in this Agreement.",
        representativeName: "Authorized Representative Name:",
        representativeSignature: "Representative Signature",
        date: "Date:",
        investorTitle: "Investor:",
        clearSignature: "Clear Signature",
        signatureAdded: "Signature added",
        signAbove: "Sign above to continue",
        digitalAccess:
          "A copy of this signed Agreement will be available in your Token Fleet platform profile for future reference. For legal purposes in Dubai, the contract will be generated in English.",
        pleaseReadContract: "Please read the entire contract",
        pleaseSignContract: "Please sign the contract",
        generatingContract: "Generating Contract...",
      },
      success: {
        title: "Welcome Aboard!",
        scrollForMore: "Scroll for more",
        congratulations: "Congratulations! You're now a fleet investor",
        investmentSuccess:
          "Your investment of ${{amount}} has been processed successfully. You now own {{percentage}}% of the fleet.",
        whatsNext: "What's Next?",
        steps: {
          monitor: {
            title: "Monitor Your Investment",
            description:
              "Access real-time fleet performance and earnings through your personalized dashboard",
          },
          grow: {
            title: "Grow Your Portfolio",
            description:
              "Explore more investment opportunities and diversify your fleet holdings",
          },
          invite: {
            title: "Invite & Earn",
            description:
              "Share with friends and earn passive income from their investments",
          },
        },
        shareOnX: "Share on X and earn from your referrals!",
        increaseReturns: "Increase Your Returns",
        bonusPercentage: "{{percentage}}% Bonus",
        referralDescription:
          "Earn extra returns by inviting friends to Token Fleet. You'll receive {{percentage}}% of our platform earnings from:",
        referralBenefits: {
          fees: "Investment fees we collect",
          rentalIncome: "Our share of rental income",
          sharesSold: "Our earnings when fleet shares are sold",
        },
        referralLinkCopied:
          "Referral link copied! Share it with friends to start earning.",
        failedToCopy: "Failed to copy referral link",
        copyReferralLink: "Copy Referral Link",
      },
      statusMessages: {
        processing: "Processing...",
        initiatingInvestment: "Initiating investment process...",
        checkingApproval: "Checking token approval...",
        approvingTokens: "Approving tokens...",
        processingDeposit: "Processing deposit...",
        finalizingInvestment: "Finalizing your investment...",
        updatingProfile: "Updating your profile...",
        uploadingContract: "Uploading contract...",
        generatingContract: "Generating your investment contract...",
        checkingWhitelist: "Checking whitelist status...",
        gettingSignature: "Getting whitelist signature...",
        accountActivated:
          "Account already activated! Creating your investment portfolio...",
        activatingAccount:
          "Activating your account and creating your investment portfolio...",
        finalizingActivation: "Finalizing account activation...",
        savingPortfolio: "Saving portfolio details...",
        applyingRewardCode: "Applying reward code...",
      },
      faucet: {
        unknownError: "An unknown error occurred",
        requestFailed: "Failed to process your request",
        invalidAddress: "Invalid Ethereum address",
        sufficientBalance: "Your wallet already has sufficient ETH",
        cooldownPeriod: "This address has received ETH in the last 3 days",
        toppedUp: "Your wallet has been topped up with ETH",
        checkingBalance: "Checking your wallet balance...",
        transactionHash: "Transaction hash",
        amountSent: "Amount sent",
        currentBalance: "Current balance",
      },
      errors: {
        connectWallet: "Please connect your wallet to proceed",
        minInvestment: "Minimum investment amount is ${{amount}}",
        maxInvestment: "Maximum investment amount is ${{amount}}",
        acceptTerms: "Please accept the terms and conditions",
        failedProceed: "Failed to proceed to next step",
        signBeforeProceed: "Please sign the contract before proceeding",
        contractUrlEmpty: "Contract S3 URL is empty just before investing",
        failedGenerate: "Failed to generate contract",
        failedProcess: "Failed to process investment. Please try again.",
        investmentRefresh:
          "Investment successful but failed to refresh data. Please refresh the page.",
      },
    },
    learnMore: "Learn more",
  },
  app: {
    name: "Shares Car",
    tagline: "Share your ride, share your journey",
  },
  profile: {
    myProfile: "My Profile",
    home: "Home",
    tabs: {
      general: "General",
      generalShort: "General",
      kycVerification: "KYC Verification",
      kycShort: "KYC",
      investmentsRewards: "Investments & Rewards",
      investmentsShort: "Investments",
      referralProgram: "Referral Program",
      refereesShort: "Referees",
      notifications: "Notifications",
      notificationsShort: "Notifications",
    },
    descriptions: {
      general: "Manage your profile and settings",
      kyc: "Complete your identity verification",
      investments: "Track your investments and rewards",
      referees: "View and manage your referrals",
      notifications: "Manage your notification preferences",
    },
    toast: {
      telegramConnected: "Successfully connected to Telegram!",
      telegramTimeout: "Telegram connection timeout. Please try again.",
      referralCopied: "Referral link copied to clipboard!",
      referralCopyFailed: "Failed to copy referral link",
      connectWalletFirst: "Please connect your wallet first",
      connectTelegramFirst: "Please connect your Telegram account first",
      notificationPreferenceUpdated:
        "Notifications will now be sent via {preference}",
      notificationPreferenceUpdateFailed:
        "Failed to update notification preference",
      telegramInstructions:
        "Please send a message to our Telegram bot to complete the connection",
    },
    welcome: {
      title: "Welcome to Your Profile",
      description:
        "Connect your wallet to view and manage your profile settings, investments, and rewards.",
      connectWallet: "Connect Your Wallet",
      connectInstructions:
        "Use the connect button in the top menu to link your wallet and access your profile.",
    },
    settingsTitle: "Profile Settings & Details",
    backToHome: "Back to Home",
    kycTab: {
      title: "KYC Verification",
      status: "Status",
      verified: "Verified ✅",
      pendingVerification: "Pending Verification 🕛",
      notVerified: "Not Verified ❌",
      requiredDocumentation: "Required Documentation",
      verificationIntro:
        "To complete your verification, we will need to collect the following:",
      documentList: {
        governmentId:
          "A valid Government-issued ID (passport, driver's license, or national ID card)",
        email: "Your email address for communication",
        livenessCheck:
          "A brief liveness check using your device's camera to match with your ID photo",
      },
      securityNote:
        "The verification process is quick and secure. Your data will be handled with strict confidentiality.",
      verifyNow: "Verify Now",
      verifiedMessage:
        "Your account is fully verified. You can now access all features.",
      processingMessage:
        "Your verification is being processed. This may take a few minutes.",
    },
    refereeTab: {
      overview: {
        totalReferees: "Total Referees",
        activeReferees: "Active referees",
        activeFleets: "Active Fleets",
        fleetsWithReferrals: "Fleets with referral investments",
        totalUnclaimed: "Total Unclaimed Rewards",
        totalToBeClaimedDesc: "Total rewards to be claimed",
        totalClaimed: "Total Claimed",
        totalClaimedDesc: "Total claimed from referrals",
      },
      referralLink: {
        title: "Your Referral Link",
        description: "Share this link to earn rewards!",
        copyLink: "Copy Link",
        copySuccess: "Referral link copied to clipboard!",
        copyError: "Failed to copy referral link",
      },
      benefits: {
        title: "Referral Program Benefits",
        subtitle: "Learn how you can earn rewards",
        fleetSpecific: {
          title: "Fleet-Specific Rewards",
          description:
            "Each fleet offers its own referral percentage based on the fleet's structure and margins",
        },
        earnings: {
          title: "Immediate & Ongoing Earnings",
          description:
            "Earn instantly from fleet purchase fees when your referrals invest, plus ongoing rewards from fleet operations",
        },
        incomeSources: {
          title: "Multiple Income Sources",
          description:
            "Earn from fleet purchase fees, rental income, car sales, and other operational returns",
        },
        bulletPoints: [
          "Initial reward from fleet purchase fees when referrals invest",
          "Ongoing rewards automatically calculated and distributed",
          "No limit on the number of referrals per fleet",
          "View fleet-specific referral rates in each fleet's details",
        ],
      },
      sections: {
        refereeFleets: "Referee Fleets",
        rewardsHistory: "Rewards History",
      },
      fleetStatus: {
        creating: "Fleet in preparation",
        posted: "Open for referrals",
        funded: "Investment target reached",
        onPurchase: "Acquiring vehicles",
        active: "Generating referral rewards",
        closing: "Fleet liquidation",
        closed: "Fleet completed",
        notRealized: "Fleet cancelled",
        unknown: "Status unavailable",
      },
      statusDescriptions: {
        creating:
          "This fleet is being created. Referral rewards will be available once the fleet opens for investment.",
        posted:
          "Share your referral link with friends to earn rewards when they invest in this fleet.",
        funded:
          "This fleet has been fully funded. You'll earn ongoing rewards from fleet operations.",
        onPurchase:
          "Fleet is purchasing vehicles. You'll start earning rewards once vehicles are operational.",
        active:
          "Earn rewards from your referrals' investments and fleet operations.",
        closing:
          "Fleet is being liquidated. Final rewards will be distributed after completion.",
        closed:
          "This fleet has completed its lifecycle. No more referral rewards available.",
        notRealized:
          "This fleet was not realized. No referral rewards will be distributed.",
        unknown: "Fleet status is unknown.",
      },
      fleetMetrics: {
        referees: "Referees",
        available: "Available",
        totalEarned: "Total Earned",
      },
      buttons: {
        claimRewards: "Claim Rewards",
        noRewards: "No Rewards",
        viewHistory: "View History",
        close: "Close",
        hideDetails: "Hide Details",
        showDetails: "Show Details",
        previous: "Previous",
        next: "Next",
      },
      loading: "Loading referral data...",
      claimingRewards: "Claiming referral rewards for {fleetName}...",
      emptyStates: {
        noFleets: "No referee fleets found.",
        noFleetsDescription:
          "Fleets will appear here when your referrals make investments",
        noRewards: "No rewards history yet",
        noRewardsDescription:
          "Rewards will appear here when your referrals make investments",
        noRewardsAvailable: "No rewards history available",
      },
      rewardStatus: {
        claimed: "Claimed",
        unclaimed: "Unclaimed",
      },
    },
    notificationsTab: {
      title: "Notification Preferences",
      telegramConnection: {
        title: "Telegram Connection",
        connected: "Connected to Telegram",
        notConnected: "Not connected to Telegram",
        connectButton: "Connect to Bot",
        connectedButton: "Connected",
      },
      notificationMethod: {
        title: "Choose Your Notification Method",
        email: "Email",
        telegram: "Telegram",
        switchHelp: {
          connectFirst: "Connect Telegram first",
          swipeToChange: "Swipe to change",
        },
        currentMethod: "Currently receiving notifications via {{method}}",
        telegramWarning:
          "Connect your Telegram account above to enable Telegram notifications",
      },
      customize: {
        title: "Customize",
      },
      preferencesForm: {
        allowNotifications: "Allow Notifications",
        enableDescription:
          "Enable this to receive important updates and notifications",
        newsletter: "Newsletter",
        platformUpdates: "Platform Updates",
        investmentOpportunities: "Investment Opportunities",
        fleetUpdates: "Fleet Updates",
        carPurchase: "Car Purchase Notifications",
        carSale: "Car Sale Notifications",
        carStatus: "Car Status Updates",
        rewards: "Rewards and Promotions",
        dividends: "Dividend Notifications",
        transactions: "Transaction Updates",
        security: "Security Alerts",
        reports: "Performance Reports",
        statistics: "Statistics and Analytics",
        saveChanges: "Save Changes",
        enableAllSave: "Enable All & Save",
        successMessage: "Notification preferences updated successfully",
        errorMessage: "Failed to update notification preferences",
      },
    },
    profileHeader: {
      anonymousUser: "Anonymous User",
      verified: "Verified",
      verifyAccount: "Verify Account",
      referralLink: {
        title: "Your Referral Link",
        description: "Share this link to earn rewards!",
        copyLink: "Copy Link",
      },
      kycWarning:
        "To access all features and start investing, please verify your account.",
    },
    onboarding: {
      navigation: {
        previous: "Previous",
        next: "Next",
        skip: "Skip",
        startInvesting: "Start Investing",
      },
      languageSelector: {
        title: "Choose Your Language",
        english: "English",
        spanish: "Español",
      },
      steps: {
        step0: {
          title: "Welcome to Token Fleet",
          description:
            "Discover the future of car fleet investments in Dubai's thriving market! Start building your portfolio with as little as $100 and earn passive income through our secure, transparent, and regulated platform.",
        },
        step1: {
          title: "Connect Your Wallet",
          description:
            "Start by connecting your Web3 wallet. This is your key to investing in tokenized car shares and managing your portfolio securely on the blockchain.",
        },
        step2: {
          title: "Complete KYC Verification",
          description:
            "Verify your identity to comply with regulations. All documents are securely stored and verified to ensure a safe investment environment.",
        },
        step3: {
          title: "Create Your Portfolio NFT",
          description:
            "Get your unique Portfolio NFT that will securely hold all your car share investments. This NFT represents your stake in the Token Fleet ecosystem.",
        },
        step4: {
          title: "Browse Investment Opportunities",
          description:
            "Start with as little as $100! Each fleet is thoroughly documented with verified ownership papers, maintenance records, and rental agreements - all secured on the blockchain.",
        },
        step5: {
          title: "Invest & Track Performance",
          description:
            "Every investment is secured by a legal contract between you and Token Fleet, accessible from your profile. Your portfolio NFT holds your fleet shares, representing your ownership. Monitor your investments' performance, verify documentation, and track earnings in real-time through your dashboard.",
        },
        step6: {
          title: "Start Investing Now",
          description:
            "Your journey to earning passive income through Dubai's premium car rental market starts here. Join Token Fleet today and transform the way you invest!",
        },
      },
    },
    investmentsTab: {
      overview: {
        totalInvestment: "Total Investment",
        activeFleets: "active fleets",
        totalReturns: "Total Returns",
        unclaimed: "unclaimed",
        lastClaim: "Last Claim",
        noClaimsYet: "No claims yet",
        totalClaims: "Total Claims",
        fromFleets: "From {count} fleets",
      },
      sections: {
        yourFleets: "Your Fleets",
        investmentHistory: "Investment History",
        returnsHistory: "Returns History",
        fleetDetails: "Fleet Details",
      },
      fleetStatus: {
        creating: "Fleet in preparation",
        posted: "Investment opportunity open",
        funded: "Investment target reached",
        onPurchase: "Acquiring vehicles",
        active: "Fleet is generating returns",
        closing: "Fleet is being liquidated",
        closed: "Fleet has completed its lifecycle",
        notRealized: "Fleet investment cancelled",
        unknown: "Status unavailable",
      },
      statusDescriptions: {
        creating:
          "This fleet is currently being created. Investment history will be available once the fleet opens for investment.",
        posted:
          "This fleet is open for investment. Make your first investment to see your investment history.",
        funded:
          "This fleet has been fully funded. Your investment history is shown below.",
        onPurchase:
          "This fleet is in the purchase phase. Your investment history is shown below.",
        active:
          "This fleet is operational. Your investment history is shown below.",
        closing:
          "This fleet is in the closing phase. Your investment history is shown below.",
        closed:
          "This fleet has been closed. Your investment history is shown below.",
        notRealized:
          "This fleet was not realized. Your investment history is shown below.",
        unknown: "Fleet status is unknown.",
      },
      buttons: {
        claimRewards: "Claim Rewards",
        redeemFunds: "Redeem Funds",
        noRewards: "No Rewards",
        investMore: "Invest More",
        showDetails: "Show Details",
        hideDetails: "Hide Details",
        previous: "Previous",
        next: "Next",
        viewContract: "View Contract",
        viewTransaction: "View Transaction",
        close: "Close",
        download: "Download",
      },
      contractModal: {
        title: "Contract Preview",
        loading: "Loading contract...",
        error: "Unable to load the contract.",
        tryAgain: "Please try again later.",
      },
      fleetMetrics: {
        investment: "Investment",
        available: "Available",
        totalReturns: "Total Returns",
        currentStage: "Current Stage",
        totalValue: "Total Value",
        yourInvestment: "Your Investment",
        availableToClaim: "Available to Claim",
      },
      stageDescriptions: {
        posted: "Open for new investments",
        funded: "Fleet has reached its investment target",
        onPurchase: "Fleet manager is acquiring the vehicles",
        active: "Fleet is operating and generating returns",
        closing: "Fleet is being prepared for closure",
        closed: "Fleet has completed its lifecycle",
        notRealized: "Fleet investment was cancelled",
        creating: "Fleet is being prepared for investment",
      },
      emptyStates: {
        noFleets: "No fleets found.",
        noFleetsDescription:
          "Fleets will appear here when you make investments",
        noReturns: "No returns history available",
        noReturnsDescription:
          "Your returns history will appear here once you start earning returns",
      },
      yourFleets: {
        investmentLabel: "Investment",
        availableLabel: "Available",
        totalReturnsLabel: "Total Returns",
        redeeming: "Redeeming...",
        claiming: "Claiming...",
        redeemFunds: "Redeem Funds",
        claimRewards: "Claim Rewards",
        noRewards: "No Rewards",
        investMore: "Invest More",
        showDetails: "Show Details",
        hideDetails: "Hide Details",
      },
    },
    generalTab: {
      pagination: {
        previous: "Previous",
        next: "Next",
      },
      stats: {
        totalInvestment: "Total Investment",
        activeFleets: "active fleets",
        totalRewards: "Total Rewards",
        unclaimed: "unclaimed",
        claimNow: "Claim now",
        referralProgram: "Referral Program",
        referees: "referees",
        claimed: "claimed",
        checkNewRewards: "Check new rewards",
      },
      metrics: {
        investmentDistribution: "Investment Distribution",
        noInvestmentData: "No investment data",
        recentActivity: "Recent Activity",
        noActivityData: "No activity data",
        lastRewardClaim: "Last Reward Claim",
        newInvestment: "New Investment",
      },
      accountStats: {
        title: "Account Statistics",
        totalFleets: "Total Fleets",
        fleets: "fleets",
        totalInvestments: "Total Investments",
        investments: "investments",
        lastClaim: "Last Claim",
        noClaimsYet: "No claims yet",
        accountStatus: "Account Status",
        verified: "Verified",
        unverified: "Unverified",
      },
    },
  },
  navigation: {
    home: "Home",
    cars: "Cars",
    fleet: "Fleet",
    profile: "Profile",
    connect: "Connect Wallet",
    admin: "Admin",
    login: "Login",
    signup: "Sign Up",
    logout: "Logout",
    disconnect: "Disconnect",
    toggleMenu: "Toggle menu",
    profileActions: "Profile actions",
    walletActions: "Wallet actions",
  },
  common: {
    loading: "Loading...",
    error: "An error occurred",
    notFound: "Not found",
    back: "Back",
    next: "Next",
    close: "Close",
    currentLanguage: "Current Language",
  },
  homepage: {
    availableFleets: "Available Fleets",
    fleetSubtitle: "Choose from our curated collection",
  },
  fleet: {
    status: "Status",
    selectStatus: "Select Fleet Status",
    stages: {
      all: "All Fleets",
      posted: "Available to Invest",
      funded: "Investment Complete",
      onPurchase: "Purchase in Progress",
      active: "Currently Operating",
      closing: "Closing Process",
      closed: "Operation Complete",
      creating: "Fleet is being created",
      notRealized: "Fleet was not realized",
    },
    investmentBanner: {
      creating: {
        message1: "Fleet is being created",
        highlight1: "Coming soon",
        message2: "New investment opportunity in preparation",
        highlight2: "Stay tuned",
        message3: "Expert team assembling this fleet",
        highlight3: "In progress",
        message4: "Finalizing fleet details",
        highlight4: "Almost ready",
      },
      postedAlmostFunded: {
        message1: "Get ready! This fleet is about to come to life",
        highlight1: "Coming soon",
        message2: "Final preparations in progress",
        highlight2: "Almost there",
        message3: "Investment goal nearly achieved",
        highlight3: "Last chance",
        message4: "Fleet activation approaching",
        highlight4: "Stay tuned",
        message5: "Preparing for fleet launch",
        highlight5: "Almost ready",
      },
      posted: {
        message1: "Ready to invest in this fleet?",
        highlight1: "Invest now",
        message2: "Start your investment journey",
        highlight2: "Get started",
        message3: "Join our community of investors",
        highlight3: "Invest now",
        message4: "Secure your share in this fleet",
        highlight4: "Join now",
        message5: "Be part of this exciting opportunity",
        highlight5: "Invest now",
      },
      funded: {
        message1: "Investment goal achieved! Fleet is fully funded",
        highlight1: "Funded",
        message2: "Target reached! Moving to next phase",
        highlight2: "Success",
        message3: "Fleet funding complete",
        highlight3: "Goal reached",
        message4: "Preparing for vehicle acquisition",
        highlight4: "Next steps",
        message5: "Thank you investors! Fleet fully funded",
        highlight5: "Complete",
      },
      onPurchase: {
        message1: "Fleet vehicles are being purchased",
        highlight1: "In progress",
        message2: "Acquiring vehicles for the fleet",
        highlight2: "Processing",
        message3: "Working with dealers to secure vehicles",
        highlight3: "In motion",
        message4: "Fleet purchase process underway",
        highlight4: "Active",
        message5: "Finalizing vehicle acquisitions",
        highlight5: "In progress",
      },
      active: {
        message1: "Fleet is generating returns",
        highlight1: "Active",
        message2: "Your investment is hard at work",
        highlight2: "Operating",
        message3: "Fleet operations running smoothly",
        highlight3: "In service",
        message4: "Vehicles actively generating revenue",
        highlight4: "Working",
        message5: "Fleet performing as planned",
        highlight5: "On track",
      },
      closing: {
        message1: "Returns distribution in progress",
        highlight1: "Closing",
        message2: "Preparing final returns",
        highlight2: "Processing",
        message3: "Fleet closing procedures initiated",
        highlight3: "In progress",
        message4: "Finalizing investment returns",
        highlight4: "Almost done",
        message5: "Processing final distributions",
        highlight5: "Wrapping up",
      },
      closed: {
        message1: "Fleet operation completed",
        highlight1: "Closed",
        message2: "Investment cycle successfully concluded",
        highlight2: "Complete",
        message3: "Thank you for investing with us",
        highlight3: "Finished",
        message4: "Fleet journey has concluded",
        highlight4: "Completed",
        message5: "Returns have been distributed",
        highlight5: "Closed",
        totalDistributed: "Total Distributed",
        totalNetProfits: "Total Net Profits",
        totalGrossProfits: "Total Gross Profits",
        investmentDuration: "Investment Duration",
      },
      notRealized: {
        message1: "Fleet investment opportunity not realized",
        highlight1: "Not realized",
        message2: "Investment conditions not met",
        highlight2: "Cancelled",
        message3: "Fleet requirements not achieved",
        highlight3: "Not proceeding",
        message4: "Investment opportunity closed",
        highlight4: "Terminated",
        message5: "Fleet assembly discontinued",
        highlight5: "Not realized",
      },
    },
    loading: "Loading fleet details...",
    notFound: "Fleet not found",
    cars: {
      purchasePrice: "Purchase Price",
      expectedApr: "Expected APR",
      fourYears: "(4 Years)",
      expectedAnnualIncome: "Expected Annual Income",
      generatedProfits: "Generated Profits",
      netGeneratedProfits: "Net Generated Profits",
      viewDetails: "View Details",
      investmentClosingSoon: "Investment Closing Soon",
      luxuryDescription:
        "Luxury and reliability merge in this executive sedan, crafted for comfort and precision.",
      investInFleet: "Invest in Fleet",
      status: {
        label: "Status",
        listed: "Listed for Investment",
        funded: "Funded",
        onPurchase: "Purchase in Progress",
        reparations: "Under Maintenance",
        rented: "Currently Rented",
        sold: "Sold",
        available: "Available for Rent",
      },
    },
    performance: {
      returnsProgress: "Returns Progress",
      ofRaisedFundsReturned: "Of raised funds returned",
      totalDistributions: "Total Distributions",
      paymentsMade: "payments made",
      totalFleetValue: "Total Fleet Value",
      premiumVehicles: "premium vehicles",
      recentActivity: "Recent Activity",
      download: "Download",
      items: "items",
      distribution: "Distribution",
      noDistributionsYet: "No distributions yet",
      progressTowardsTargetReturn: "Progress Towards Target Return",
      totalProgress: "Total Progress",
      current: "Current",
      target: "Target",
    },
    financials: {
      totalInvestment: "Total Investment",
      funded: "Funded",
      expectedAnnualReturns: "Expected Annual Returns",
      monthlyProfit: "Monthly Profit",
      monthlyNetProfit: "Monthly Net Profit",
      projectedApr: "Projected APR",
      totalProfit: "Total Profit",
      netProfit: "Net Car Profits",
      grossProfit: "Gross Profit",
      returnsProjection: "Returns Projection",
      costBreakdown: "Cost Breakdown",
      totalPurchasePrice: "Total Purchase Price",
      basePriceAllVehicles: "Base price of all vehicles",
      companyFee: "Company Fee",
      oneTimePlatformFee: "One-time platform fee",
      securityDeposit: "Security Deposit",
      refundableAfterFourYears: "Refundable after 4 years",
      annualExpenses: "Annual Expenses",
      insuranceAndRegistration: "Insurance & Registration",
      investmentAndReturnsByVehicle: "Investment and Returns by Vehicle",
      vehicle: "Vehicle",
      purchasePrice: "Purchase Price",
      tooltips: {
        totalInvestment:
          "Total amount required for fleet acquisition and setup",
        expectedAnnualReturns:
          "Projected yearly earnings based on rental income and asset appreciation",
        projectedApr: "Return on Investment over the 4-year period",
        totalPurchasePrice:
          "Combined purchase price of all vehicles in the fleet",
        companyFee: {
          intro: "One-time fee that covers:",
          marketing: "Initial marketing",
          setup: "Platform setup",
          maintenance: "Basic maintenance preparation",
        },
        securityDeposit: {
          intro: "Refundable deposit reserved for:",
          repairs: "Major repairs if needed",
          maintenance: "Unexpected maintenance",
          return: "Will be returned after 4 years",
        },
        annualExpenses: {
          intro: "Yearly costs including:",
          insurance: "Insurance",
          maintenance: "Regular maintenance",
          firstYear: "First year included in total investment",
        },
      },
    },
    overview: {
      capitalRequirements: "Capital Requirements",
      finalReturns: "ROI",
      closedOn: "Closed on",
      totalDistributed: "Total Distributed",
      totalNetProfits: "Total Net Profits",
      totalGrossProfits: "Total Gross Profits",
      investmentDuration: "Investment Duration",
      tooltips: {
        capitalRequirements:
          "Total investment required for the fleet. This includes all costs such as vehicle purchases and initial setup.",
        expectedApr:
          "Expected annual return on investment.",
        minInvestment: "The minimum amount required to participate.",
        totalCars: "Number of vehicles in the fleet.",
      },
      totalFundingRequired: "Total funding required",
      expectedApr: "Expected APR",
      fourYearReturn: "Expected yearly return",
      minInvestment: "Min Investment",
      entryLevel: "Entry level",
      totalCars: "Total Cars",
      premiumVehicles: "Premium vehicles",
      lastDeposit: "Last deposit",
      purchaseInitiated: "Purchase initiated",
      fundingStatus: "Funding Status",
      purchaseTimeline: "Purchase Timeline",
      investmentPerformance: "Investment Performance",
      progress: "Progress",
      remaining: "remaining",
      investInFleet: "Invest in Fleet",
      fundingComplete: "Funding Complete",
      raised: "raised",
      purchaseInProgress: "Purchase In Progress",
      current: "Current",
      initiated: "Initiated",
      activeOperations: "Active Operations",
      comingSoon: "Coming soon",
      currentReturns: "Current Returns",
      latestDistribution: "Latest Distribution",
      totalProfits: "Total Profits",
      target: "Target",
      netAnnualRentalIncome: "Net Annual Rental Income",
      contractDetails: "Contract Details",
      address: "Address",
      copyAddress: "Copy Address",
      addressCopied: "Contract address copied to clipboard!",
      viewOnBaseScan: "View on Base Scan",
    },
    fleetDetails: {
      backToHome: "Back to Home",
      statisticsAndDetails: "Fleet Statistics & Details",
      tabs: {
        cars: "Cars",
        financials: "Financials",
        performance: "Performance",
      },
      tabDescriptions: {
        cars: "View and manage fleet vehicles",
        financials: "Track financial metrics and returns",
        performance: "Monitor fleet performance metrics",
      },
      tooltips: {
        financialsDisabled:
          "Financial details will be available once the fleet is posted",
        performanceDisabled:
          "Performance metrics will be available once the fleet is active",
      },
      messages: {
        financialsDisabled:
          "Financial details will be available once the fleet is posted.",
        performanceDisabled:
          "Performance metrics will be available once the fleet is active.",
      },
    },
  },
  carDetails: {
    welcome: "Welcome to Car Details",
    connectWallet:
      "Connect your wallet to view detailed information about this car and participate in fleet investments.",
    connectWalletTitle: "Connect Your Wallet",
    connectWalletDescription:
      "Use the connect button in the top menu to link your wallet and access car investment details.",
    loading: "Loading car details...",
    notFound: "Car not found",
    backToFleet: "Back to Fleet",
    partOfFleet: "Part of Fleet:",
    fleetInfo: {
      cars: "cars",
      totalValue: "total value",
      expectedApr: "expected APR",
    },
    viewFleetDetails: "View Fleet Details",
    investInFleet: "Invest in the Fleet",
    aboutThis: "About this",
    investmentOverview: {
      totalInvestment: "Total Investment",
      totalApr: "Total APR",
      tooltips: {
        investment:
          "Total upfront payment required:\n• Purchase price: ${{purchasePrice}}\n• Company fee: ${{companyFee}}\n• Security deposit: ${{securityDeposit}}\n• First year expenses: ${{firstYearExpenses}}\n\nNote: Security deposit will be returned after 4 years",
        roi: "Total Return on Investment over 4 years\nBased on net profits including:\n• Rental income\n• Car sale value\n• Minus all expenses and fees",
      },
    },
    accordion: {
      investmentDetails: "Investment Details",
      projectedReturns: "Projected Returns",
      technicalDetails: "Technical Details",
      documentation: "Documentation",
      rentalIncome: "Rental Income",
      maintenanceExpenses: "Maintenance & Expenses",
      investment: {
        purchasePrice: "Purchase Price",
        purchasePriceTooltip:
          "Base price of the vehicle before fees and deposits",
        companyFee: "Company Fee",
        companyFeeTooltip:
          "One-time fee ({{percentage}}% of purchase price)\nThis fee covers:\n• Initial marketing\n• Platform setup\n• Basic maintenance preparation",
        securityDeposit: "Security Deposit",
        securityDepositTooltip:
          "Refundable deposit ({{percentage}}% of purchase price)\nReserved for:\n• Major repairs if needed\n• Unexpected maintenance\nWill be returned if unused after rental period",
        firstYearExpenses: "First Year Expenses",
        firstYearExpensesTooltip:
          "Annual costs for the first year:\n• Insurance: ${{insurance}}\n• GPS and Registration: ${{registration}}\nThese expenses are included in the total investment",
        totalInvestment: "Total Investment",
        totalInvestmentTooltip:
          "Total upfront payment required to setup the fleet.\nNote: Security deposit will be returned when the deal ends.",
      },
      returns: {
        totalApr: "Total APR",
        totalAprTooltip:
          "Investment Structure:\n• 4-year rental operation period\n• Vehicle sale at market value after period ends\n• Security deposit returned upon sale\n\nReturns are distributed from:\n• Rental income during operation\n• Final settlement after vehicle sale",
        rentalReturnFrequency: "Rental Return Frequency",
        rentalReturnFrequencyTooltip:
          "About Return Frequency:\n• This car performs best with {{frequency}} rentals\n• Based on market analysis and rental patterns\n• Maximizes your potential returns\nWhy {{frequency}}?\n• Optimal pricing point\n• Higher occupancy rate\n• Better matched to local demand",
        baseRentalRate: "Base Rental Rate",
        baseRentalRateTooltip:
          "Base rate: {{frequency}} rental\nThis rate is calculated considering:\n• Market demand in Dubai\n• Vehicle category & features\n• Seasonal pricing trends\n\nFrom this amount:\n• {{investorPercentage}}% distributed to shareholders\n• {{platformPercentage}}% platform fee\n• In case of taking from the security deposit, will be covered before distribution.",
        annualOperatingExpenses: "Annual Operating Expenses",
        annualOperatingExpensesTooltip:
          "Yearly costs including:\n• Insurance: ${{insurance}}\n• Registration: ${{registration}}\nFirst year expenses are included in the total investment",
        netAnnualRentalIncome: "Net Annual Rental Income",
        netAnnualRentalIncomeTooltip:
          "Annual rental income after:\n• Company fee ({{percentage}}%)\n• Operating expenses\nBased on expected occupancy:\n{{occupancyDetails}}",
        expectedDepreciation: "Expected Depreciation",
        expectedDepreciationTooltip:
          "Car value projection:\n• Initial price: ${{initialPrice}}\n• Minimum value: ${{minimumValue}} (70%)\n• Maximum depreciation: ${{maximumDepreciation}} (50%)\nNote: Actual depreciation may vary based on market conditions and rental duration.",
        netProfits: "Net Profits (4 Years)",
        netProfitsTooltip:
          "Projected profits for minimum 4-year period:\n• Rental income (minus {{rentalFee}}% fee)\n• Car sale value (Around 60% of purchase price)\n• Minus operating expenses\n• Plus 100% of security deposit\n• Minus company selling fee ({{sellingFee}}%)\n\nAdditional profits possible if rental period extends beyond 4 years.",
      },
      technical: {
        basicSpecifications: "Basic Specifications",
        performance: "Performance",
        dimensions: "Dimensions",
        featuresComfort: "Features & Comfort",
        safetyAdditional: "Safety & Additional Features",
        extraFeatures: "Extra Features",
        availableAfterPurchase: "Available once the car is purchased",
        specs: {
          plate: "Plate",
          mileage: "Mileage",
          engine: "Engine",
          transmission: "Transmission",
          drivetrain: "Drivetrain",
          fuelType: "Fuel Type",
          bodyType: "Body Type",
          horsepower: "Horsepower",
          torque: "Torque",
          topSpeed: "Top Speed",
          acceleration: "0-100 km/h",
          battery: "Battery",
          range: "Range",
          length: "Length",
          width: "Width",
          height: "Height",
          wheelbase: "Wheelbase",
          trunkCapacity: "Trunk Capacity",
          airbags: "Airbags",
          color: "Color",
          licenseCategory: "License Category",
          safetyRating: "Safety Rating",
        },
      },
      documents: {
        noDocuments: "No documents available",
        rowsPerPage: "Rows per page:",
      },
      income: {
        noIncome: "No rental income records available",
        deposit: "Deposit",
      },
      expenses: {
        noExpenses: "No maintenance & expenses records available",
      },
    },
  },
  footer: {
    backToTop: "Back to top",
    tagline:
      "Invest in cars, earn from each rental. Token Fleet lets you own a share and earn every time the wheels turn.",
    about: "About",
    aboutLinks: {
      mission: "Our Mission",
      howItWorks: "How It Works",
      team: "Team",
    },
    resources: "Resources",
    resourceLinks: {
      documentation: "Documentation",
      terms: "Terms of Use",
      privacy: "Privacy Policy",
    },
    contact: "Contact Us",
    companyBy: "by Reliable Route Rental Car LLC.",
    allRightsReserved: "All Rights Reserved.",
  },
  howItWorks: {
    title: "How It Works",
    subtitle:
      "Join Token Fleet and start earning passive income through Dubai's premium car rental market. Our blockchain-powered platform makes investing in vehicle fleets accessible, secure, and transparent.",
    investmentJourney: "Your Investment Journey",
    platformFeatures: "Platform Features",
    benefitsOfInvesting: "Benefits of Investing",
    steps: [
      {
        title: "Connect Your Wallet",
        description:
          "Start by connecting your Web3 wallet to our platform. This secure connection allows you to invest in tokenized car shares and manage your portfolio on the blockchain.",
        details: [
          "Support for major Web3 wallets",
          "Secure blockchain transactions",
          "Easy portfolio management",
        ],
      },
      {
        title: "Complete Verification",
        description:
          "Complete our KYC process to ensure compliance with regulations. This step helps maintain a secure and transparent investment environment.",
        details: [
          "Quick identity verification",
          "Secure document storage",
          "Regulatory compliance",
        ],
      },
      {
        title: "Choose Your Investment",
        description:
          "Browse our curated selection of premium vehicles. Each fleet is thoroughly documented with verified ownership papers and maintenance records.",
        details: [
          "Premium vehicle selection",
          "Transparent documentation",
          "Flexible investment options",
        ],
      },
      {
        title: "Receive Your Portfolio NFT",
        description:
          "Get your unique Portfolio NFT that securely holds all your car share investments, representing your stake in the Token Fleet ecosystem.",
        details: [
          "Unique portfolio identifier",
          "Secure ownership record",
          "Easy transfer and management",
        ],
      },
    ],
    features: [
      {
        title: "Smart Contract Security",
        description:
          "Your investments are secured by smart contracts on the blockchain, ensuring transparent and immutable ownership records.",
      },
      {
        title: "Real-Time Analytics",
        description:
          "Track your fleet's performance, revenue generation, and maintenance status through our comprehensive dashboard.",
      },
      {
        title: "Passive Income Generation",
        description:
          "Earn regular returns from rental revenue, with profits automatically distributed to your wallet.",
      },
    ],
    benefits: [
      {
        title: "Low Entry Barrier",
        description:
          "Start building your portfolio with as little as $100. Our fractional ownership model makes premium vehicle investment accessible to everyone.",
      },
      {
        title: "Transparent Operations",
        description:
          "All fleet operations, maintenance records, and revenue distribution are recorded on the blockchain, ensuring complete transparency.",
      },
      {
        title: "Real Asset Backing",
        description:
          "Every token is backed by real, premium vehicles operating in Dubai's thriving rental market, providing tangible asset security.",
      },
    ],
  },
  banner: {
    messages: [
      {
        text: "Need testnet tokens? Join our Telegram",
        highlight: "Request tokens",
      },
      {
        text: "Want to try our testnet platform?",
        highlight: "Get free tokens",
      },
      {
        text: "Join our Telegram community",
        highlight: "Get test tokens",
      },
      {
        text: "Test Token Fleet with free tokens",
        highlight: "Join Telegram",
      },
    ],
  },
  kyc: {
    title: "Complete Your KYC Verification",
    description:
      "To comply with regulatory requirements, we need to verify your identity before you can start investing.",
    startVerification: "Start Verification",
  },
  featuredFleet: {
    messages: [
      {
        text: "Start investing from just $100",
        highlight: "View featured fleet",
      },
      {
        text: "Fractional car ownership made easy",
        highlight: "Explore now",
      },
      {
        text: "Earn passive income from rentals",
        highlight: "Learn more",
      },
      {
        text: "Tokenized car shares available",
        highlight: "Join today",
      },
    ],
    error: "Error fetching featured fleet",
    carousel: {
      banners: [
        {
          alt: "Luxury car fleet investment opportunity",
          title: "Premium Fleet Investment",
          subtitle: "Start investing from just $100",
        },
        {
          alt: "Diverse car portfolio",
          title: "Diversified Portfolio",
          subtitle: "Multiple car investments in one fleet",
        },
        {
          alt: "Rental car returns",
          title: "Passive Income",
          subtitle: "Earn from car rentals",
        },
      ],
      viewFeaturedFleet: "Click to view featured fleet →",
    },
  },
  mission: {
    title: "Our Mission",
    subtitle:
      "Bridging the gap between real-world assets and blockchain technology, starting with revolutionizing the car rental industry through transparent ownership and innovative investment opportunities.",
    vision: {
      title: "Our Vision",
      democratizing: {
        title: "Democratizing Vehicle Investment",
        description:
          "We're creating a future where anyone can own a piece of a premium vehicle fleet. Through blockchain technology, we're transforming traditional car rental into an accessible investment opportunity, allowing investors to earn passive income while participating in the growing mobility economy.",
      },
      bridging: {
        title: "Bridging Traditional and Digital Finance",
        description:
          "Token Fleet stands at the intersection of traditional asset management and blockchain innovation. We're pioneering the tokenization of real-world assets, starting with vehicles, to create a new standard for transparent, efficient, and accessible investment opportunities.",
      },
    },
    difference: {
      title: "What Sets Us Apart",
      ownership: {
        title: "True Asset Ownership",
        description:
          "Unlike traditional investment vehicles, Token Fleet provides direct ownership in our car fleet. Each token represents a real stake in physical assets, backed by smart contracts and legal frameworks that ensure your investment is secure and tangible.",
      },
      transparency: {
        title: "Blockchain-Powered Transparency",
        description:
          "Our platform leverages blockchain technology to provide unprecedented transparency. Every aspect of fleet performance, from revenue generation to maintenance costs, is recorded on the blockchain and accessible to our investors in real-time.",
      },
    },
    values: {
      title: "Our Core Values",
      items: [
        {
          title: "Real Ownership",
          description:
            "Every token represents genuine ownership in our vehicle fleet, backed by real assets and secured through blockchain technology.",
          icon: "🔐",
        },
        {
          title: "Transparency",
          description:
            "Full visibility into fleet performance, revenue distribution, and operational metrics through our blockchain-based platform.",
          icon: "📊",
        },
        {
          title: "Innovation",
          description:
            "Bridging traditional car rental with blockchain technology to create a new paradigm of fractional vehicle ownership.",
          icon: "💡",
        },
        {
          title: "Accessibility",
          description:
            "Making car fleet investment accessible to everyone, regardless of their investment capacity.",
          icon: "🌍",
        },
        {
          title: "Sustainability",
          description:
            "Promoting efficient vehicle utilization and responsible asset management for long-term value creation.",
          icon: "🌱",
        },
        {
          title: "Community",
          description:
            "Building a community of forward-thinking investors who believe in the future of tokenized real-world assets.",
          icon: "🤝",
        },
      ],
    },
  },
  team: {
    title: "Meet Our Team",
    subtitle:
      "The driving force behind Token Fleet combines deep expertise in automotive, blockchain technology, and financial markets. Our diverse team is united by a single mission: revolutionizing car ownership through innovative investment solutions.",
    members: [
      {
        name: "Marcos Maceo",
        position: "Chief Executive Officer",
        bio: "With extensive experience in blockchain technology and automotive industry, Marcos leads Token Fleet's vision of revolutionizing car investments through innovative blockchain solutions.",
        imageUrl: "https://tokenfleet.io/images/image-5.webp",
      },
      {
        name: "Daniela Suárez",
        position: "Chief Marketing Officer",
        bio: "Daniela brings strategic marketing expertise to Token Fleet, driving our brand growth and market presence while ensuring our innovative investment platform reaches the right audience.",
        imageUrl: "https://tokenfleet.io/images/daniela.jpeg",
      },
      {
        name: "Andy Ledesma",
        position: "Chief Technology Officer",
        bio: "Andy leads Token Fleet's technical innovation, bringing extensive blockchain development expertise to ensure our platform remains at the cutting edge of technology.",
        imageUrl: "https://tokenfleet.io/images/andy.jpeg",
      },
      {
        name: "Muhamed Syfian",
        position: "Head of Operations",
        bio: "With deep experience in fleet management, Muhamed ensures optimal performance of our vehicle portfolio and maintenance systems, maximizing value for our investors.",
        imageUrl: "https://tokenfleet.io/images/image-8.webp",
      },
      {
        name: "Antonio Bermudez",
        position: "Head of Design",
        bio: "Antonio crafts Token Fleet's user experience and visual identity, ensuring our platform is both beautiful and intuitive while maintaining the highest standards of design excellence.",
        imageUrl: "https://tokenfleet.io/images/image-7.webp",
      },
    ],
  },
  terms: {
    title: "Terms and Conditions",
    sections: {
      acceptance: {
        title: "1. Acceptance of Terms",
        content:
          'By accessing and using the Token Fleet platform ("Platform"), you agree to be bound by these Terms and Conditions ("Terms"). These Terms constitute a legally binding agreement between you ("User", "Investor") and Token Fleet, operated by Reliable Route Car Rentals LLC ("Company", "we", "us", "our").',
      },
      definitions: {
        title: "2. Definitions",
        items: [
          '"Platform" refers to the Token Fleet investment platform.',
          '"Investment Share" refers to the fractional ownership stake in a vehicle.',
          '"Investment Period" refers to the standard 4-year term of investment.',
          '"Rental Income" refers to the revenue generated from renting the vehicle.',
          '"Smart Contract" refers to the blockchain-based contract governing the investment.',
        ],
      },
      eligibility: {
        title: "3. Eligibility",
        intro: "To use the Platform and invest, you must:",
        requirements: [
          "Be at least 18 years of age",
          "Have the legal capacity to enter into contracts",
          "Comply with all applicable laws and regulations",
        ],
      },
      investment: {
        title: "4. Investment Terms",
        structure: {
          title: "4.1 Investment Structure",
          content:
            "Users can purchase fractional ownership in vehicles through the Platform. The minimum investment amount is 100 USDC. Each Investment Share represents a proportional ownership stake in the underlying vehicle.",
        },
        period: {
          title: "4.2 Investment Period",
          content:
            "The standard Investment Period is 4 years from the date of vehicle acquisition. While direct redemptions before the end of the period are not supported, Token Fleet is developing a secondary liquidity market that will enable investors to trade their positions with others before maturity.",
        },
      },
      fees: {
        title: "5. Fees and Payments",
        setup: {
          title: "5.1 Initial Setup Fee (10%)",
          content:
            "A company fee of 10% is added to the base vehicle or fleet purchase price. This fee covers:",
          items: [
            "Platform operational costs",
            "Smart contract deployment and management",
            "Administrative and legal documentation",
            "Business development and marketing",
          ],
        },
        deposit: {
          title: "5.2 Security Deposit (10%)",
          content:
            "A 10% security deposit of the total investment amount (including the base price and company fee) is held in reserve. This deposit is:",
          items: [
            "Fully refundable at the end of the investment term",
            "Used as a buffer for unexpected maintenance or repairs",
            "Applied to protect both the investor and the platform",
          ],
        },
        charges: {
          title: "5.3 Fixed Charges",
          content:
            "Additional fixed charges are applied to cover essential services:",
          items: [
            "First-year insurance coverage",
            "GPS tracking system installation and subscription",
            "Vehicle documentation and registration",
            "Initial maintenance package",
          ],
        },
      },
      income: {
        title: "6. Income Distribution",
        items: [
          "6.1 After deducting the 30% platform fee, investors receive 70% of rental income, distributed proportionally based on their Investment Share.",
          "6.2 Tokens representing your investment share will be available to claim on the Platform. All distributions of rental income are made in USDC through the Platform.",
          "6.3 The Company reserves the right to withhold distributions for outstanding fees or maintenance costs.",
        ],
      },
      vehicle: {
        title: "7. Vehicle Management",
        items: [
          "7.1 The Company maintains full operational control of the vehicle for rental purposes.",
          "7.2 The vehicle is registered under Reliable Route Car Rentals LLC in compliance with UAE regulations.",
          "7.3 The Company is responsible for maintenance, insurance, and regulatory compliance.",
        ],
      },
      termination: {
        title: "8. Term and Termination",
        items: [
          "8.1 At the end of the Investment Period, the vehicle will be sold at market value.",
          "8.2 Sale proceeds, less the 10% exit fee, will be distributed to investors based on their Investment Share.",
          "8.3 The security deposit will be returned, less any outstanding expenses or claims.",
        ],
      },
      risks: {
        title: "9. Risks and Disclaimers",
        items: [
          "9.1 Investment involves risks, including potential loss of principal.",
          "9.2 The Company does not guarantee returns or vehicle value.",
          "9.3 Market conditions may affect rental income and resale value.",
          "9.4 Past performance is not indicative of future results.",
        ],
      },
      law: {
        title: "10. Governing Law",
        content:
          "These Terms are governed by the laws of the United Arab Emirates. Any disputes shall be subject to the exclusive jurisdiction of the courts of Dubai, UAE.",
      },
      contact: {
        title: "11. Contact Information",
        intro: "For inquiries regarding these Terms, please contact:",
        company: "Reliable Route Car Rentals LLC",
        address1: "Rasis Business Center, 4th Floor, Office 75",
        address2: "Al Barsha, Dubai, UAE",
        phone: "Phone/WhatsApp: +971521703229",
        email: "Email: info@tokenfleet.io",
      },
      lastUpdated: "Last updated: January 17, 2025",
    },
  },
  privacy: {
    title: "Privacy Policy",
    sections: {
      introduction: {
        title: "1. Introduction",
        content:
          'Token Fleet, operated by Reliable Route Car Rentals LLC ("we," "our," or "us"), is committed to protecting your privacy. This Privacy Policy explains how we collect, use, disclose, and safeguard your information when you use our platform.',
      },
      information: {
        title: "2. Information We Collect",
        personal: {
          title: "2.1 Personal Information",
          items: [
            "Name and contact information",
            "Date of birth and nationality",
            "Government-issued identification",
            "Blockchain wallet addresses",
            "Investment history and preferences",
            "Banking and payment information",
          ],
        },
        technical: {
          title: "2.2 Technical Information",
          items: [
            "IP address and device information",
            "Browser type and version",
            "Usage data and analytics",
            "Cookies and similar tracking technologies",
          ],
        },
      },
      usage: {
        title: "3. How We Use Your Information",
        intro: "We use your information to:",
        items: [
          "Process your investments and transactions",
          "Verify your identity and prevent fraud",
          "Provide investment-related communications",
          "Provide customer support",
          "Comply with legal obligations",
          "Improve our services and user experience",
        ],
      },
      sharing: {
        title: "4. Information Sharing",
        intro: "We may share your information with:",
        items: [
          "Service providers and business partners",
          "Regulatory authorities and law enforcement",
          "Professional advisors and auditors",
        ],
        note: "We do not sell your personal information to third parties.",
      },
      security: {
        title: "5. Data Security",
        content:
          "We implement appropriate technical and organizational measures to protect your personal information. However, no method of transmission over the internet or electronic storage is 100% secure.",
      },
      rights: {
        title: "6. Your Rights",
        intro: "You have the right to:",
        items: [
          "Access your personal information",
          "Correct inaccurate information",
          "Request deletion of your information",
          "Object to processing of your information",
          "Data portability",
        ],
      },
      cookies: {
        title: "7. Cookies and Tracking",
        content:
          "We use cookies and similar tracking technologies to enhance your experience on our platform. You can control cookie settings through your browser preferences.",
      },
      changes: {
        title: "8. Changes to Privacy Policy",
        content:
          "We may update this Privacy Policy from time to time. We will notify you of any material changes by posting the updated policy on our platform.",
      },
      contact: {
        title: "9. Contact Us",
        intro: "For privacy-related inquiries, please contact:",
        company: "Reliable Route Car Rentals LLC",
        address1: "Rasis Business Center, 4th Floor, Office 75",
        address2: "Al Barsha, Dubai, UAE",
        phone: "Phone/WhatsApp: +971521703229",
        email: "Email: info@tokenfleet.io",
      },
      lastUpdated: "Last updated: January 17, 2025",
    },
  },
  referee: {
    title: "Join Token Fleet",
    connectWallet: {
      title: "Ready to Start Your Journey?",
      description:
        "Connect your wallet to unlock exclusive benefits with Token Fleet",
      referralCode: "Referral Code:",
      buttonLabel: "Connect Wallet",
    },
    signIn: {
      title: "One More Step!",
      description:
        "Sign in with your wallet to activate your exclusive membership",
      referralCode: "Referral Code:",
      buttonLabel: "Sign In",
    },
    loading: {
      title: "Almost There!",
      description: "We're setting up your Token Fleet membership",
      referralCode: "Referral Code:",
    },
    success: {
      title: "Welcome Aboard! 🎉",
      description: "You're now officially part of Token Fleet",
      buttonLabel: "Explore Token Fleet",
    },
    error: {
      title: "Oops! Something's Not Right",
      referralCode: "Referral Code:",
      buttonLabel: "Go to Main Page",
    },
  },
};

// Spanish translations
export const es = {
  nft: {
    title: "¡Reclama tu NFT!",
    loading: "Cargando detalles del NFT...",
    connectWallet: {
      title: "Conecta Tu Billetera",
      description: "Conecta tu billetera para reclamar {{name}}",
    },
    signIn: {
      title: "Inicio de Sesión Requerido",
      description: "Por favor inicia sesión para reclamar tu NFT {{name}}",
    },
    mint: {
      benefitsTitle: "Beneficios del NFT",
      eligibleForGas: "Pagamos por el gas 🤝",
      viewOnExplorer: "Ver en Explorador",
      buttonLabel: "Reclamar NFT",
    },
    minting: {
      status: "Reclamando tu NFT...",
      sponsorDelayDisclaimer:
        "Tu billetera puede demorar unos segundos en reconocer el gas que patrocinamos",
    },
    success: {
      title: "¡NFT Reclamado Exitosamente!",
      viewTransaction: "Ver Transacción",
      description:
        "¡Felicidades! Tu NFT {{name}} ha sido reclamado exitosamente.",
      shareButton: "Compartir en X",
      shareOnXContent:
        "¡Acabo de reclamar mi {{name}} NFT en @tokenfleet_io! 🚗\n",
      addToWalletButton: "Añadir a Billetera",
      addingToWalletButton: "Añadiendo a Billetera...",
      mainButton: "Ir a la Página Principal",
    },
    errors: {
      title: "Error",
      alreadyMinted: "Este NFT ya ha sido reclamado o no existe.",
      backButton: "Ir a la Página Principal",
    },
  },
  fleets: {
    gridItem: {
      car: "Coche",
      cars: "Coches",
      stage: {
        creating: {
          label: "En Creación",
          metric: "Inversión Objetivo",
        },
        posted: {
          label: "Abierto para Inversión",
          metric: "Total Invertido",
        },
        funded: {
          label: "Totalmente Financiado",
          metric: "Retorno Anual Esperado",
        },
        onpurchase: {
          label: "Compra en Progreso",
          metric: "Coches Comprados",
          ofTotal: "de",
        },
        active: {
          label: "Operacional",
          metric: "Ingreso Mensual Esperado",
        },
        closing: {
          label: "Cerrando",
          metric: "Retornos Esperados",
        },
        closed: {
          label: "Cerrado",
          metric: "Retornos Finales",
        },
        notrealized: {
          label: "No Realizado",
          metric: "Retornos Finales",
        },
        unknown: {
          label: "Etapa Desconocida",
          metric: "Inversión Total",
        },
      },
      metrics: {
        fleetSize: "Tamaño de Flota",
        targetFunding: "Financiamiento Objetivo",
        expectedApr: "APR Esperado",
        totalInvested: "Total Invertido",
        apr: "APR",
        timeLeft: "Tiempo Restante",
        totalFunded: "Total Financiado",
        purchaseCost: "Costo de Compra",
        expectedIncome: "Ingreso Esperado",
        netAnnual: "Neto Anual",
        totalInvestment: "Inversión Total",
        totalProfit: "Beneficio Total",
        return: "Retorno",
      },
      timeLeft: {
        closingSoon: "Cerrando Pronto",
        days: "días restantes",
        day: "día restante",
        hours: "h restantes",
      },
      fundingProgress: {
        funded: "Financiado",
      },
    },
    noFleetsFound: "No se encontraron flotas",
    noStatusFleetsFound: "No se encontraron flotas {{status}}",
    status: {
      all: "Todas",
      posted: "Publicadas",
      funded: "Financiadas",
      onpurchase: "En Compra",
      active: "Activas",
      closing: "Cerrando",
      closed: "Cerradas",
    },
    emptyState: {
      all: "Actualmente no hay flotas disponibles. Por favor, vuelva más tarde para ver nuevas oportunidades de inversión.",
      posted:
        "No hay flotas disponibles para inversión en este momento. Pronto se publicarán nuevas oportunidades.",
      funded:
        "No hay flotas que hayan completado su fase de financiación. Consulte la sección Disponible para Invertir para ver las oportunidades actuales.",
      onpurchase:
        "No hay flotas actualmente en proceso de compra. Vuelva pronto para ver actualizaciones sobre nuestras adquisiciones de flotas.",
      active:
        "No hay flotas actualmente en operación. Consulte la sección Disponible para Invertir para ver próximas oportunidades.",
      closing:
        "No hay flotas actualmente en fase de cierre. Consulte la sección Flotas Activas para ver nuestros vehículos en operación.",
      closed:
        "No hay operaciones de flota completadas en este momento. Vea nuestras Flotas Activas para ver las inversiones actuales.",
      default:
        "No se encontraron flotas en esta categoría. Por favor, consulte otras secciones para ver oportunidades disponibles.",
    },
  },
  shared: {
    shareBar: {
      message: "¡Comparte y gana recompensas!",
      buttonText: "Copiar Enlace",
      benefitsTitle: "Beneficios del Programa de Referidos",
      linkCopied:
        "¡Enlace de referido copiado! Compártelo con amigos para comenzar a ganar.",
      copyError: "Error al copiar el enlace de referido",
      copied: "¡Enlace copiado al portapapeles!",
      benefitsSubtitle: "Aprende cómo puedes ganar recompensas",
      benefits: {
        fleetSpecific: {
          title: "Recompensas Específicas por Flota",
          description:
            "Cada flota ofrece su propio porcentaje de referido basado en la estructura y márgenes de la flota",
        },
        earnings: {
          title: "Ganancias Inmediatas y Continuas",
          description:
            "Gana instantáneamente de las comisiones de compra de flotas cuando tus referidos invierten, más recompensas continuas de las operaciones de la flota",
        },
        incomeSources: {
          title: "Múltiples Fuentes de Ingresos",
          description:
            "Gana de las comisiones de compra de flotas, ingresos por alquiler, ventas de coches y otros rendimientos operativos",
        },
        bulletPoints: [
          "Recompensa inicial de las comisiones de compra de flotas cuando los referidos invierten",
          "Recompensas continuas calculadas y distribuidas automáticamente",
          "Sin límite en el número de referidos por flota",
          "Consulta las tasas de referencia específicas de cada flota en los detalles de la flota",
        ],
      },
    },
    contractModal: {
      title: "Contrato de Inversión",
      contractAgreement: "Acuerdo de Contrato",
      contractIntro:
        'Este Contrato de Inversión ("Contrato") se celebra entre el Inversor ("Usted") y Reliable Route Car Rentals LLC ("Compañía"). Al firmar a continuación, acepta invertir {{amount}} USDC en el activo de automóvil de la Compañía con la Dirección de Contrato Inteligente: {{address}}.',
      userPassport: "Pasaporte del Usuario",
      userInformation: "Información del Usuario",
      name: "Nombre",
      address: "Dirección",
      userRights: "Derechos del Usuario",
      userRightsIntro:
        "Como inversor, tiene derecho a los siguientes derechos:",
      userRightsList: [
        "Recibir actualizaciones regulares sobre el estado de la inversión.",
        "Participar en decisiones importantes relacionadas con el activo del automóvil.",
        "Acceso a informes financieros detallados a petición.",
      ],
      investmentDetails: "Detalles de la Inversión",
      investmentDetailsText:
        "Su inversión le otorga un porcentaje de propiedad del {{percentage}}% en el activo del automóvil. Basado en su inversión, el retorno esperado es de ${{return}}.",
      rentSplitAgreement: "Acuerdo de División de Alquiler",
      rentSplitText:
        "La distribución de los alquileres del activo del automóvil será la siguiente:",
      rentSplitList: [
        "10% en la compra del automóvil.",
        "30% cada vez que se alquila el automóvil.",
        "10% una vez que se vende el automóvil.",
      ],
      agreementText:
        "Al firmar este contrato, reconoce y acepta los términos y condiciones descritos anteriormente.",
      signBelow: "Por favor firme a continuación",
      clearSignature: "Borrar Firma",
      contractNote:
        "*El contrato se enviará por correo electrónico y será accesible en su perfil.",
      cancel: "Cancelar",
      confirmAndSign: "Confirmar y Firmar",
      pleaseSign: "Por favor proporcione una firma antes de confirmar.",
    },
    documentModal: {
      loading: "Cargando documento...",
      error: "No se puede cargar el documento.",
      tryAgain: "Por favor intente de nuevo más tarde.",
      close: "Cerrar",
      download: "Descargar",
    },
    receiptModal: {
      loading: "Cargando recibo...",
      error: "No se puede cargar el recibo.",
      tryAgain: "Por favor intente de nuevo más tarde.",
      close: "Cerrar",
      download: "Descargar",
    },
    unverifiedUserModal: {
      title: "Verificación Requerida",
      description:
        "Antes de que puedas invertir en esta flota, necesitamos verificar tu identidad para cumplir con los requisitos regulatorios.",
      whyTitle: "¿Por qué necesitamos esto?",
      reasons: {
        compliance: "Asegurar el cumplimiento de las regulaciones financieras",
        security: "Proteger contra fraudes y mantener la seguridad",
        investments: "Permitir inversiones seguras y protegidas",
      },
      buttons: {
        cancel: "Cancelar",
        startVerification: "Iniciar Verificación",
      },
    },
    investmentProcessModal: {
      title: "Proceso de Inversión",
      steps: {
        investmentDetails: "Detalles de Inversión",
        setupRequired: "Configuración Requerida",
        contractReview: "Revisión del Contrato",
        success: "¡Bienvenido a Bordo!",
        currentAndTotal: "Paso {{current}} de {{total}}",
      },
      setupRequired: {
        oneTimeSetup: "Configuración Única Requerida",
        setupDescription:
          "Antes de continuar con tu inversión, necesitamos completar dos pasos rápidos:",
        activateAccount: "Activa Tu Cuenta",
        activateDescription: "Habilita tu billetera verificada para invertir",
        createPortfolio: "Crea Tu Portafolio",
        portfolioDescription: "Tu centro de control de inversiones",
        oneTimeProcess:
          "Este es un proceso único que te permitirá invertir en cualquier flota en nuestra plataforma.",
        proceedToSetup:
          "Continuar a Activación de Cuenta y Configuración de Portafolio",
      },
      buttons: {
        cancel: "Cancelar",
        proceed: "Continuar",
        proceedToPortfolio: "Continuar a Configuración de Portafolio",
        proceedToContract: "Continuar al Contrato",
        startVerification: "Iniciar Verificación",
        signContract: "Firmar Contrato",
        continue: "Continuar",
      },
      investmentDetails: {
        investmentAmount: "Monto de Inversión",
        balance: "Saldo",
        enterAmount: "Ingrese monto",
        max: "Máx",
        investmentSummary: "Resumen de Inversión",
        projection: "Proyección a 4 años",
        ownershipShare: "Porcentaje de Propiedad",
        expectedReturn: "Retorno Esperado",
        totalApr: "Total APR",
        annual: "anual",
        termsAndConditions: "He leído y acepto los términos y condiciones",
        minInvestment: "La inversión mínima es de ${{amount}}",
        maxInvestment: "La inversión máxima es de ${{amount}}",
        rewardCode: "Código de Recompensa",
        enterCode: "Ingrese código de recompensa",
        applyCode: "Aplicar",
        removeCode: "Eliminar",
        codeValid: "Código válido",
        codeInvalid: "Código inválido",
        codeError: "Error al verificar el código",
        codeApplied: "¡Código de recompensa aplicado! Se agregarán ${{amount}} USDC a su cuenta cuando complete esta inversión.",
      },
      tooltips: {
        investmentLimits:
          "Mín: ${{min}} | Máx: ${{max}}\nTu Saldo USDC: ${{balance}}\nRestante por Financiar: ${{remaining}}",
        projectionPeriod:
          "Nuestras proyecciones se basan en un período de inversión de 4 años.\nTendrás flexibilidad para ajustar este plazo según\nlas condiciones del mercado y el rendimiento de la flota.",
        ownershipPercentage:
          "Tu porcentaje de propiedad de la flota basado en tu monto de inversión",
        returnBreakdown:
          "Retornos proyectados totales que incluyen:\n• Ingresos por alquiler\n• Valor de reventa del vehículo\n• Depósito de seguridad devuelto",
        rewardCode: "Ingrese un código de recompensa para obtener USDC adicional para su inversión",
      },
      contractReview: {
        scrollToRead:
          "Por favor, desplácese por todo el contrato para continuar",
        thanksForReading: "Gracias por leer el contrato",
        contractTitle: "Acuerdo de Contrato de Inversión",
        contractIntro:
          'Este Acuerdo de Contrato de Inversión ("Acuerdo") se celebra a partir del {{date}} entre:',
        investor: 'Inversor ("Usted"):',
        fullName: "Nombre Legal Completo:",
        documentType: "Tipo de Documento:",
        documentNumber: "Número de Documento:",
        walletAddress: "Dirección de Billetera:",
        emailAddress: "Dirección de Correo Electrónico:",
        company: "Empresa:",
        companyName: "Nombre:",
        companyNameValue: 'Reliable Route Car Rentals LLC ("Empresa")',
        registeredOffice: "Oficina Registrada:",
        officeLocation: "Dubai, EAU",
        parties: 'Colectivamente referidos como las "Partes."',
        investmentDetails: "1. Detalles de la Inversión",
        investmentAgreement:
          "Por la presente, acepta invertir la cantidad de ${{amount}} USDC en la siguiente flota:",
        fleetName: "Nombre de la Flota:",
        fleetDescription: "Descripción de la Flota:",
        contractAddress: "Dirección del Contrato:",
        ownershipPercentage: "Porcentaje de Propiedad:",
        fleetAssets: "2. Activos de la Flota",
        fleetAssetsDescription:
          "La flota comprende los siguientes vehículos, que serán gestionados activamente y alquilados por la Empresa:",
        revenueDistribution: "3. Distribución de Ingresos",
        revenueDescription:
          "Como inversor, tiene derecho a recibir distribuciones de ingresos basadas en su porcentaje de propiedad en la flota:",
        rentalIncome: "Ingresos por Alquiler:",
        rentalIncomeInvestors:
          "Los inversores recibirán el 70% de los ingresos brutos por alquiler, proporcional a su participación de propiedad",
        rentalIncomePlatform:
          "La plataforma retendrá el 30% de los ingresos brutos por alquiler para costos operativos y tarifas",
        finalSaleRewards: "Recompensas por Venta Final:",
        finalSaleDescription:
          "Tras la venta de vehículos, los inversores reciben el 90% de los ingresos netos, proporcional a la propiedad",
        securityDeposit: "Depósito de Seguridad:",
        securityDepositAmount:
          "100% del depósito de seguridad (proporcional a la participación de propiedad)",
        securityDepositPurpose:
          "Sirve como fondo de reserva para reparaciones y gastos inesperados",
        termsAndConditions: "4. Términos y Condiciones",
        transparency: "Transparencia y Acceso:",
        transparencyAccess:
          "Acceso completo a detalles de transacciones, informes de ingresos y métricas de rendimiento de la flota",
        transparencyBlockchain:
          "La tecnología blockchain garantiza cálculos y distribuciones automatizados",
        rewardProcessing: "Procesamiento de Recompensas:",
        rewardCalculation:
          "Todos los ingresos por alquiler calculados según las especificaciones del contrato inteligente",
        rewardAvailability:
          "Las recompensas no reclamadas permanecen disponibles hasta que se reclamen",
        rewardDistributions:
          "Distribuciones de ingresos disponibles en su cuenta de la plataforma Token Fleet",
        gasFees: "Tarifas de Gas:",
        gasFeesResponsibility:
          "Los inversores son responsables de las tarifas de la red blockchain",
        gasFeesApply:
          "Se aplican tarifas al reclamar recompensas o ejecutar transacciones",
        managementRights: "Derechos de Gestión:",
        managementCompany:
          "La Empresa conserva el derecho exclusivo de gestionar los activos de la flota",
        managementIncludes:
          "Incluye derechos para alquilar y vender vehículos para obtener rendimientos óptimos",
        marketRisks: "Riesgos de Mercado:",
        risksInherent:
          "Todas las inversiones conllevan riesgos inherentes de mercado y operativos",
        risksVariation:
          "Los rendimientos pueden variar según las condiciones del mercado",
        legalCompliance: "Legal y Cumplimiento:",
        legalGoverned: "Acuerdo regido por las leyes de Dubái, EAU",
        legalDisputes: "Disputas resueltas bajo la jurisdicción de Dubái",
        dataProtection: "Protección de Datos:",
        dataSecure:
          "Datos personales y financieros almacenados y procesados de forma segura",
        dataUsage: "Utilizados únicamente para cumplir con este Acuerdo",
        forceMajeure: "Fuerza Mayor:",
        forceMajeureNotLiable:
          "La Empresa no es responsable por retrasos o pérdidas causadas por circunstancias imprevistas",
        forceMajeureIncludes:
          "Incluyendo desastres naturales, cambios regulatorios o fallos en la red blockchain",
        amendments: "Enmiendas:",
        amendmentsAgreement:
          "Cualquier cambio a este Acuerdo debe ser acordado por escrito por ambas Partes",
        amendmentsDocumented:
          "Las modificaciones deben ser documentadas y firmadas",
        expectedReturns: "5. Rendimientos Esperados",
        projectedApr: "APR Total Proyectado:",
        distributionFrequency: "Frecuencia de Distribución:",
        distributionImmediate: "Inmediata tras la finalización del alquiler",
        returnsNote:
          "Nota: Los rendimientos reales pueden variar según el rendimiento del mercado y factores operativos.",
        investorResponsibilities: "6. RESPONSABILIDADES DEL INVERSOR",
        responsibilitiesConfirm: "Al firmar este Acuerdo, confirma que usted:",
        understandRisks:
          "1. Comprende la naturaleza y los riesgos asociados con las inversiones basadas en blockchain",
        reviewedTerms:
          "2. Ha revisado todos los términos y condiciones descritos en este Acuerdo",
        maintainWallet:
          "3. Acepta mantener una billetera activa compatible con la plataforma",
        notifyChanges:
          "4. Notificará prontamente a la Empresa de cualquier cambio en su información de contacto",
        additionalClauses: "7. CLÁUSULAS ADICIONALES",
        dataProtectionTitle: "Protección de Datos:",
        dataProtectionContent:
          "Sus datos personales y financieros serán almacenados y procesados de forma segura",
        forceMajeureTitle: "Fuerza Mayor:",
        forceMajeureContent1:
          "La Empresa no es responsable por retrasos o pérdidas causadas por circunstancias imprevistas",
        forceMajeureContent2:
          "Incluyendo desastres naturales, cambios regulatorios o fallos en blockchain",
        terminationTitle: "Terminación:",
        terminationContent:
          "Este Acuerdo puede ser terminado por consentimiento mutuo o por notificación escrita",
        amendmentsTitle: "Enmiendas:",
        amendmentsContent:
          "Cualquier cambio a este Acuerdo debe ser acordado por escrito por ambas Partes",
        signatureTitle: "8. Firma",
        signatureConfirm:
          "Al firmar a continuación, confirma que ha leído, entendido y acepta todos los términos y condiciones descritos en este Acuerdo.",
        representativeName: "Nombre del Representante Autorizado:",
        representativeSignature: "Firma del Representante",
        date: "Fecha:",
        investorTitle: "Inversor:",
        clearSignature: "Borrar Firma",
        signatureAdded: "Firma añadida",
        signAbove: "Firme arriba para continuar",
        digitalAccess:
          "Una copia de este Acuerdo firmado estará disponible en su perfil de la plataforma Token Fleet para referencia futura. Por motivos legales en Dubai, el contrato se generará en inglés.",
        pleaseReadContract: "Por favor lea todo el contrato",
        pleaseSignContract: "Por favor firme el contrato",
        generatingContract: "Generando Contrato...",
      },
      success: {
        title: "¡Bienvenido a Bordo!",
        scrollForMore: "Desplácese para más",
        congratulations: "¡Felicidades! Ahora eres un inversor de flota",
        investmentSuccess:
          "Tu inversión de ${{amount}} ha sido procesada con éxito. Ahora posees el {{percentage}}% de la flota.",
        whatsNext: "¿Qué Sigue?",
        steps: {
          monitor: {
            title: "Monitorea Tu Inversión",
            description:
              "Accede al rendimiento de la flota en tiempo real y a las ganancias a través de tu panel personalizado",
          },
          grow: {
            title: "Haz Crecer Tu Portafolio",
            description:
              "Explora más oportunidades de inversión y diversifica tus participaciones en flotas",
          },
          invite: {
            title: "Invita y Gana",
            description:
              "Comparte con amigos y gana ingresos pasivos de sus inversiones",
          },
        },
        shareOnX: "¡Comparte en X y gana con tus referencias!",
        increaseReturns: "Aumenta Tus Rendimientos",
        bonusPercentage: "{{percentage}}% de Bonificación",
        referralDescription:
          "Gana rendimientos adicionales invitando a amigos a Token Fleet. Recibirás el {{percentage}}% de nuestras ganancias de plataforma de:",
        referralBenefits: {
          fees: "Comisiones de inversión que cobramos",
          rentalIncome: "Nuestra parte de los ingresos por alquiler",
          sharesSold:
            "Nuestras ganancias cuando se venden acciones de la flota",
        },
        referralLinkCopied:
          "¡Enlace de referencia copiado! Compártelo con amigos para comenzar a ganar.",
        failedToCopy: "Error al copiar el enlace de referencia",
        copyReferralLink: "Copiar Enlace de Referencia",
      },
      statusMessages: {
        processing: "Procesando...",
        initiatingInvestment: "Iniciando proceso de inversión...",
        checkingApproval: "Verificando aprobación de tokens...",
        approvingTokens: "Aprobando tokens...",
        processingDeposit: "Procesando depósito...",
        finalizingInvestment: "Finalizando su inversión...",
        updatingProfile: "Actualizando su perfil...",
        uploadingContract: "Subiendo contrato...",
        generatingContract: "Generando su contrato de inversión...",
        checkingWhitelist: "Verificando estado de lista blanca...",
        gettingSignature: "Obteniendo firma de lista blanca...",
        accountActivated:
          "¡Cuenta ya activada! Creando su portafolio de inversión...",
        activatingAccount:
          "Activando su cuenta y creando su portafolio de inversión...",
        finalizingActivation: "Finalizando activación de cuenta...",
        savingPortfolio: "Guardando detalles del portafolio...",
        applyingRewardCode: "Aplicando código de recompensa...",
      },
      faucet: {
        unknownError: "Ha ocurrido un error desconocido",
        requestFailed: "No se pudo procesar tu solicitud",
        invalidAddress: "Dirección de Ethereum inválida",
        sufficientBalance: "Tu billetera ya tiene suficiente ETH",
        cooldownPeriod: "Esta dirección ha recibido ETH en los últimos 3 días",
        toppedUp: "Tu billetera ha sido recargada con ETH",
        checkingBalance: "Verificando el saldo de tu billetera...",
        transactionHash: "Hash de transacción",
        amountSent: "Cantidad enviada",
        currentBalance: "Saldo actual",
      },
      errors: {
        connectWallet: "Por favor conecte su billetera para continuar",
        minInvestment: "El monto mínimo de inversión es ${{amount}}",
        maxInvestment: "El monto máximo de inversión es ${{amount}}",
        acceptTerms: "Por favor acepte los términos y condiciones",
        failedProceed: "No se pudo proceder al siguiente paso",
        signBeforeProceed: "Por favor firme el contrato antes de continuar",
        contractUrlEmpty:
          "La URL del contrato S3 está vacía justo antes de invertir",
        failedGenerate: "No se pudo generar el contrato",
        failedProcess:
          "No se pudo procesar la inversión. Por favor intente de nuevo.",
        investmentRefresh:
          "Inversión exitosa pero no se pudieron actualizar los datos. Por favor actualice la página.",
      },
    },
    learnMore: "Saber más",
  },
  app: {
    name: "Shares Car",
    tagline: "Comparte tu viaje, comparte tu camino",
  },
  navigation: {
    home: "Inicio",
    cars: "Coches",
    fleet: "Flota",
    profile: "Perfil",
    connect: "Conectar Billetera",
    admin: "Administrador",
    login: "Iniciar Sesión",
    signup: "Registrarse",
    logout: "Cerrar Sesión",
    disconnect: "Desconectar",
    toggleMenu: "Alternar menú",
    profileActions: "Acciones de perfil",
    walletActions: "Acciones de billetera",
  },
  common: {
    loading: "Cargando...",
    error: "Ha ocurrido un error",
    notFound: "No encontrado",
    back: "Atrás",
    next: "Siguiente",
    close: "Cerrar",
    currentLanguage: "Idioma Actual",
  },
  homepage: {
    availableFleets: "Flotas Disponibles",
    fleetSubtitle: "Elige de nuestra colección seleccionada",
  },
  fleet: {
    status: "Estado",
    selectStatus: "Seleccionar Estado de Flota",
    stages: {
      all: "Todas las Flotas",
      posted: "Disponible para Invertir",
      funded: "Inversión Completada",
      onPurchase: "Compra en Progreso",
      active: "Actualmente Operando",
      closing: "Proceso de Cierre",
      closed: "Operación Completada",
      creating: "La flota está siendo creada",
      notRealized: "La flota no se realizó",
    },
    investmentBanner: {
      creating: {
        message1: "La flota está siendo creada",
        highlight1: "Próximamente",
        message2: "Nueva oportunidad de inversión en preparación",
        highlight2: "Mantente atento",
        message3: "Equipo experto ensamblando esta flota",
        highlight3: "En progreso",
        message4: "Finalizando detalles de la flota",
        highlight4: "Casi lista",
      },
      postedAlmostFunded: {
        message1: "¡Prepárate! Esta flota está a punto de cobrar vida",
        highlight1: "Próximamente",
        message2: "Preparativos finales en progreso",
        highlight2: "Casi allí",
        message3: "¡Meta de inversión casi alcanzada!",
        highlight3: "Última oportunidad",
        message4: "Activación de la flota se aproxima",
        highlight4: "Mantente atento",
        message5: "Preparando el lanzamiento de la flota",
        highlight5: "Casi lista",
      },
      posted: {
        message1: "¿Listo para invertir en esta flota?",
        highlight1: "Invertir ahora",
        message2: "Comienza tu viaje de inversión",
        highlight2: "Comenzar",
        message3: "Únete a nuestra comunidad de inversores",
        highlight3: "Invertir ahora",
        message4: "Asegura tu participación en esta flota",
        highlight4: "Únete ahora",
        message5: "Sé parte de esta emocionante oportunidad",
        highlight5: "Invertir ahora",
      },
      funded: {
        message1:
          "¡Objetivo de inversión alcanzado! La flota está completamente financiada",
        highlight1: "Financiada",
        message2: "¡Objetivo alcanzado! Avanzando a la siguiente fase",
        highlight2: "Éxito",
        message3: "Financiamiento de flota completo",
        highlight3: "Meta alcanzada",
        message4: "Preparando la adquisición de vehículos",
        highlight4: "Próximos pasos",
        message5: "¡Gracias inversores! Flota completamente financiada",
        highlight5: "Completo",
      },
      onPurchase: {
        message1: "Los vehículos de la flota están siendo comprados",
        highlight1: "En progreso",
        message2: "Adquiriendo vehículos para la flota",
        highlight2: "Procesando",
        message3: "Trabajando con concesionarios para asegurar vehículos",
        highlight3: "En movimiento",
        message4: "Proceso de compra de flota en marcha",
        highlight4: "Activo",
        message5: "Finalizando adquisiciones de vehículos",
        highlight5: "En progreso",
      },
      active: {
        message1: "La flota está generando retornos",
        highlight1: "Activa",
        message2: "Tu inversión está trabajando duro",
        highlight2: "Operando",
        message3: "Operaciones de la flota funcionando sin problemas",
        highlight3: "En servicio",
        message4: "Vehículos generando ingresos activamente",
        highlight4: "Trabajando",
        message5: "Flota funcionando según lo planeado",
        highlight5: "En camino",
      },
      closing: {
        message1: "Distribución de retornos en progreso",
        highlight1: "Cerrando",
        message2: "Preparando retornos finales",
        highlight2: "Procesando",
        message3: "Procedimientos de cierre de flota iniciados",
        highlight3: "En progreso",
        message4: "Finalizando retornos de inversión",
        highlight4: "Casi terminado",
        message5: "Procesando distribuciones finales",
        highlight5: "Concluyendo",
      },
      closed: {
        message1: "Operación de flota completada",
        highlight1: "Cerrada",
        message2: "Ciclo de inversión concluido exitosamente",
        highlight2: "Completo",
        message3: "Gracias por invertir con nosotros",
        highlight3: "Finalizado",
        message4: "El viaje de la flota ha concluido",
        highlight4: "Completado",
        message5: "Los retornos han sido distribuidos",
        highlight5: "Cerrado",
        totalDistributed: "Total Distribuido",
        totalNetProfits: "Beneficios Netos Totales",
        totalGrossProfits: "Beneficios Brutos Totales",
        investmentDuration: "Duración de la Inversión",
      },
      notRealized: {
        message1: "Oportunidad de inversión en flota no realizada",
        highlight1: "No realizada",
        message2: "Condiciones de inversión no cumplidas",
        highlight2: "Cancelado",
        message3: "Requisitos de flota no alcanzados",
        highlight3: "No procede",
        message4: "Oportunidad de inversión cerrada",
        highlight4: "Terminado",
        message5: "Ensamblaje de flota discontinuado",
        highlight5: "No realizada",
      },
    },
    loading: "Cargando detalles de la flota...",
    notFound: "Flota no encontrada",
    cars: {
      purchasePrice: "Precio de Compra",
      expectedApr: "APR Esperado",
      fourYears: "(4 Años)",
      expectedAnnualIncome: "Ingreso Anual Esperado",
      generatedProfits: "Ganancias Generadas",
      netGeneratedProfits: "Ganancias Netas Generadas",
      viewDetails: "Ver Detalles",
      investmentClosingSoon: "Inversión Cerrando Pronto",
      luxuryDescription:
        "Lujo y fiabilidad se fusionan en esta berlina ejecutiva, diseñada para la comodidad y la precisión.",
      investInFleet: "Invertir en Flota",
      status: {
        label: "Estado",
        listed: "Disponible para Inversión",
        funded: "Financiado",
        onPurchase: "Compra en Progreso",
        reparations: "En Mantenimiento",
        rented: "Actualmente Alquilado",
        sold: "Vendido",
        available: "Disponible para Alquiler",
      },
    },
    performance: {
      returnsProgress: "Progreso de Retornos",
      ofRaisedFundsReturned: "De fondos recaudados devueltos",
      totalDistributions: "Distribuciones Totales",
      paymentsMade: "pagos realizados",
      totalFleetValue: "Valor Total de la Flota",
      premiumVehicles: "vehículos premium",
      recentActivity: "Actividad Reciente",
      download: "Descargar",
      items: "elementos",
      distribution: "Distribución",
      noDistributionsYet: "Aún no hay distribuciones",
      progressTowardsTargetReturn: "Progreso Hacia el Retorno Objetivo",
      totalProgress: "Progreso Total",
      current: "Actual",
      target: "Objetivo",
    },
    financials: {
      totalInvestment: "Inversión Total",
      funded: "Financiado",
      expectedAnnualReturns: "Retornos Anuales Esperados",
      monthlyProfit: "Beneficio Mensual",
      monthlyNetProfit: "Beneficio Neto Mensual",
      projectedApr: "APR Proyectado",
      totalProfit: "Beneficio Total",
      netProfit: "Beneficio Neto del Auto",
      grossProfit: "Beneficio Bruto",
      returnsProjection: "Proyección de Retornos",
      costBreakdown: "Desglose de Costos",
      totalPurchasePrice: "Precio Total de Compra",
      basePriceAllVehicles: "Precio base de todos los vehículos",
      companyFee: "Tarifa de la Compañía",
      oneTimePlatformFee: "Tarifa única de plataforma",
      securityDeposit: "Depósito de Seguridad",
      refundableAfterFourYears: "Reembolsable después de 4 años",
      annualExpenses: "Gastos Anuales",
      insuranceAndRegistration: "Seguro y Registro",
      investmentAndReturnsByVehicle: "Inversión y Retornos por Vehículo",
      vehicle: "Vehículo",
      purchasePrice: "Precio de Compra",
      tooltips: {
        totalInvestment:
          "Cantidad total requerida para la adquisición y configuración de la flota",
        expectedAnnualReturns:
          "Ganancias anuales proyectadas basadas en ingresos por alquiler y apreciación de activos",
        projectedApr: "Retorno de la Inversión durante el período de 4 años",
        totalPurchasePrice:
          "Precio de compra combinado de todos los vehículos en la flota",
        companyFee: {
          intro: "Tarifa única que cubre:",
          marketing: "Marketing inicial",
          setup: "Configuración de la plataforma",
          maintenance: "Preparación de mantenimiento básico",
        },
        securityDeposit: {
          intro: "Depósito reembolsable reservado para:",
          repairs: "Reparaciones importantes si son necesarias",
          maintenance: "Mantenimiento inesperado",
          return: "Se devolverá después de 4 años",
        },
        annualExpenses: {
          intro: "Costos anuales que incluyen:",
          insurance: "Seguro",
          maintenance: "Mantenimiento regular",
          firstYear: "Primer año incluido en la inversión total",
        },
      },
    },
    overview: {
      capitalRequirements: "Requisitos de Capital",
      finalReturns: "ROI",
      closedOn: "Cerrado el",
      totalDistributed: "Total Distribuido",
      totalNetProfits: "Beneficios Netos Totales",
      totalGrossProfits: "Beneficios Brutos Totales",
      investmentDuration: "Duración de la Inversión",
      tooltips: {
        capitalRequirements:
          "Inversión total requerida para la flota. Esto incluye todos los costos como compras de vehículos y configuración inicial.",
        expectedApr:
          "Retorno anual esperado sobre la inversión.",
        minInvestment: "La cantidad mínima requerida para participar.",
        totalCars: "Número de vehículos en la flota.",
      },
      totalFundingRequired: "Financiación total requerida",
      expectedApr: "APR esperado",
      fourYearReturn: "Retorno anual esperado",
      minInvestment: "Inversión mínima",
      entryLevel: "Nivel de entrada",
      totalCars: "Total de coches",
      premiumVehicles: "Vehículos premium",
      lastDeposit: "Último depósito",
      purchaseInitiated: "Compra iniciada",
      fundingStatus: "Estado de financiación",
      purchaseTimeline: "Cronograma de compra",
      investmentPerformance: "Rendimiento de la inversión",
      progress: "Progreso",
      remaining: "restante",
      investInFleet: "Invertir en la flota",
      fundingComplete: "Financiación completa",
      raised: "recaudado",
      purchaseInProgress: "Compra en progreso",
      current: "Actual",
      initiated: "Iniciado",
      activeOperations: "Operaciones activas",
      comingSoon: "Próximamente",
      currentReturns: "Retornos actuales",
      latestDistribution: "Última distribución",
      totalProfits: "Beneficios totales",
      target: "Objetivo",
      netAnnualRentalIncome: "Ingreso neto anual por alquiler",
      contractDetails: "Detalles del contrato",
      address: "Dirección",
      copyAddress: "Copiar dirección",
      addressCopied: "¡Dirección del contrato copiada al portapapeles!",
      viewOnBaseScan: "Ver en Base Scan",
    },
    fleetDetails: {
      backToHome: "Volver al Inicio",
      statisticsAndDetails: "Estadísticas y Detalles de la Flota",
      tabs: {
        cars: "Coches",
        financials: "Finanzas",
        performance: "Rendimiento",
      },
      tabDescriptions: {
        cars: "Ver y gestionar vehículos de la flota",
        financials: "Seguimiento de métricas financieras y retornos",
        performance: "Monitorear métricas de rendimiento de la flota",
      },
      tooltips: {
        financialsDisabled:
          "Los detalles financieros estarán disponibles una vez que la flota sea publicada",
        performanceDisabled:
          "Las métricas de rendimiento estarán disponibles una vez que la flota esté activa",
      },
      messages: {
        financialsDisabled:
          "Los detalles financieros estarán disponibles una vez que la flota sea publicada.",
        performanceDisabled:
          "Las métricas de rendimiento estarán disponibles una vez que la flota esté activa.",
      },
    },
  },
  profile: {
    myProfile: "Mi Perfil",
    home: "Inicio",
    tabs: {
      general: "General",
      generalShort: "General",
      kycVerification: "Verificación KYC",
      kycShort: "KYC",
      investmentsRewards: "Inversiones y Recompensas",
      investmentsShort: "Inversiones",
      referralProgram: "Programa de Referidos",
      refereesShort: "Referidos",
      notifications: "Notificaciones",
      notificationsShort: "Notificaciones",
    },
    descriptions: {
      general: "Administra tu perfil y configuración",
      kyc: "Completa tu verificación de identidad",
      investments: "Rastrea tus inversiones y recompensas",
      referees: "Ver y administrar tus referidos",
      notifications: "Administra tus preferencias de notificación",
    },
    toast: {
      telegramConnected: "¡Conectado exitosamente a Telegram!",
      telegramTimeout:
        "Tiempo de conexión a Telegram agotado. Por favor, inténtalo de nuevo.",
      referralCopied: "¡Enlace de referido copiado al portapapeles!",
      referralCopyFailed: "Error al copiar el enlace de referido",
      connectWalletFirst: "Por favor, conecta tu cartera primero",
      connectTelegramFirst: "Por favor, conecta tu cuenta de Telegram primero",
      notificationPreferenceUpdated:
        "Las notificaciones ahora se enviarán a través de {preference}",
      notificationPreferenceUpdateFailed:
        "Error al actualizar la preferencia de notificación",
      telegramInstructions:
        "Por favor, envía un mensaje a nuestro bot de Telegram para completar la conexión",
    },
    welcome: {
      title: "Bienvenido a Tu Perfil",
      description:
        "Conecta tu cartera para ver y administrar la configuración de tu perfil, inversiones y recompensas.",
      connectWallet: "Conecta Tu Cartera",
      connectInstructions:
        "Usa el botón de conexión en el menú superior para vincular tu cartera y acceder a tu perfil.",
    },
    settingsTitle: "Configuración y Detalles del Perfil",
    backToHome: "Volver al Inicio",
    kycTab: {
      title: "Verificación KYC",
      status: "Estado",
      verified: "Verificado ✅",
      pendingVerification: "Verificación en Proceso 🕛",
      notVerified: "No Verificado ❌",
      requiredDocumentation: "Documentación Requerida",
      verificationIntro:
        "Para completar tu verificación, necesitaremos recopilar lo siguiente:",
      documentList: {
        governmentId:
          "Una identificación válida emitida por el gobierno (pasaporte, licencia de conducir o documento nacional de identidad)",
        email: "Tu dirección de correo electrónico para comunicación",
        livenessCheck:
          "Una breve verificación de presencia utilizando la cámara de tu dispositivo para comparar con la foto de tu identificación",
      },
      securityNote:
        "El proceso de verificación es rápido y seguro. Tus datos serán tratados con estricta confidencialidad.",
      verifyNow: "Verificar Ahora",
      verifiedMessage:
        "Tu cuenta está completamente verificada. Ahora puedes acceder a todas las funciones.",
      processingMessage:
        "Tu verificación está siendo procesada. Esto puede tomar unos minutos.",
    },
    refereeTab: {
      overview: {
        totalReferees: "Total de Referidos",
        activeReferees: "Referidos activos",
        activeFleets: "Flotas Activas",
        fleetsWithReferrals: "Flotas con inversiones de referidos",
        totalUnclaimed: "Total de Recompensas Sin Reclamar",
        totalToBeClaimedDesc: "Total de recompensas por reclamar",
        totalClaimed: "Total Reclamado",
        totalClaimedDesc: "Total reclamado de referidos",
      },
      referralLink: {
        title: "Tu Enlace de Referido",
        description: "¡Comparte este enlace para ganar recompensas!",
        copyLink: "Copiar Enlace",
        copySuccess: "¡Enlace de referido copiado al portapapeles!",
        copyError: "Error al copiar el enlace de referido",
      },
      benefits: {
        title: "Beneficios del Programa de Referidos",
        subtitle: "Aprende cómo puedes ganar recompensas",
        fleetSpecific: {
          title: "Recompensas Específicas por Flota",
          description:
            "Cada flota ofrece su propio porcentaje de referido basado en la estructura y márgenes de la flota",
        },
        earnings: {
          title: "Ganancias Inmediatas y Continuas",
          description:
            "Gana instantáneamente de las tarifas de compra de flotas cuando tus referidos invierten, más recompensas continuas de las operaciones de la flota",
        },
        incomeSources: {
          title: "Múltiples Fuentes de Ingresos",
          description:
            "Gana de las tarifas de compra de flotas, ingresos por alquiler, ventas de coches y otros retornos operativos",
        },
        bulletPoints: [
          "Recompensa inicial de las tarifas de compra de flotas cuando los referidos invierten",
          "Recompensas continuas calculadas y distribuidas automáticamente",
          "Sin límite en el número de referidos por flota",
          "Ver tasas de referido específicas de cada flota en los detalles de la flota",
        ],
      },
      sections: {
        refereeFleets: "Flotas de Referidos",
        rewardsHistory: "Historial de Recompensas",
      },
      fleetStatus: {
        creating: "Flota en preparación",
        posted: "Abierto para referidos",
        funded: "Objetivo de inversión alcanzado",
        onPurchase: "Adquiriendo vehículos",
        active: "Generando recompensas de referidos",
        closing: "Liquidación de flota",
        closed: "Flota completada",
        notRealized: "Flota cancelada",
        unknown: "Estado no disponible",
      },
      statusDescriptions: {
        creating:
          "Esta flota está siendo creada. Las recompensas de referidos estarán disponibles una vez que la flota se abra para inversión.",
        posted:
          "Comparte tu enlace de referido con amigos para ganar recompensas cuando inviertan en esta flota.",
        funded:
          "Esta flota ha sido completamente financiada. Ganarás recompensas continuas de las operaciones de la flota.",
        onPurchase:
          "La flota está comprando vehículos. Comenzarás a ganar recompensas una vez que los vehículos estén operativos.",
        active:
          "Gana recompensas de las inversiones de tus referidos y las operaciones de la flota.",
        closing:
          "La flota está siendo liquidada. Las recompensas finales se distribuirán después de la finalización.",
        closed:
          "Esta flota ha completado su ciclo de vida. No hay más recompensas de referidos disponibles.",
        notRealized:
          "Esta flota no se realizó. No se distribuirán recompensas de referidos.",
        unknown: "El estado de la flota es desconocido.",
      },
      fleetMetrics: {
        referees: "Referidos",
        available: "Disponible",
        totalEarned: "Total Ganado",
      },
      buttons: {
        claimRewards: "Reclamar Recompensas",
        noRewards: "Sin Recompensas",
        viewHistory: "Ver Historial",
        close: "Cerrar",
        hideDetails: "Ocultar Detalles",
        showDetails: "Mostrar Detalles",
        previous: "Anterior",
        next: "Siguiente",
      },
      loading: "Cargando datos de referidos...",
      claimingRewards:
        "Reclamando recompensas de referidos para {fleetName}...",
      emptyStates: {
        noFleets: "No se encontraron flotas de referidos.",
        noFleetsDescription:
          "Las flotas aparecerán aquí cuando tus referidos realicen inversiones",
        noRewards: "Aún no hay historial de recompensas",
        noRewardsDescription:
          "Las recompensas aparecerán aquí cuando tus referidos realicen inversiones",
        noRewardsAvailable: "No hay historial de recompensas disponible",
      },
      rewardStatus: {
        claimed: "Reclamado",
        unclaimed: "Sin Reclamar",
      },
    },
    notificationsTab: {
      title: "Preferencias de Notificaciones",
      telegramConnection: {
        title: "Conexión de Telegram",
        connected: "Conectado a Telegram",
        notConnected: "No conectado a Telegram",
        connectButton: "Conectar al Bot",
        connectedButton: "Conectado",
      },
      notificationMethod: {
        title: "Elige tu Método de Notificación",
        email: "Correo Electrónico",
        telegram: "Telegram",
        switchHelp: {
          connectFirst: "Conecta Telegram primero",
          swipeToChange: "Desliza para cambiar",
        },
        currentMethod: "Actualmente recibiendo notificaciones vía {{method}}",
        telegramWarning:
          "Conecta tu cuenta de Telegram arriba para habilitar las notificaciones de Telegram",
      },
      customize: {
        title: "Personalizar",
      },
      preferencesForm: {
        allowNotifications: "Permitir Notificaciones",
        enableDescription:
          "Habilita esto para recibir actualizaciones y notificaciones importantes",
        newsletter: "Boletín Informativo",
        platformUpdates: "Actualizaciones de la Plataforma",
        investmentOpportunities: "Oportunidades de Inversión",
        fleetUpdates: "Actualizaciones de Flota",
        carPurchase: "Notificaciones de Compra de Coches",
        carSale: "Notificaciones de Venta de Coches",
        carStatus: "Actualizaciones de Estado de Coches",
        rewards: "Recompensas y Promociones",
        dividends: "Notificaciones de Dividendos",
        transactions: "Actualizaciones de Transacciones",
        security: "Alertas de Seguridad",
        reports: "Informes de Rendimiento",
        statistics: "Estadísticas y Análisis",
        saveChanges: "Guardar Cambios",
        enableAllSave: "Habilitar Todo y Guardar",
        successMessage: "Preferencias de notificación actualizadas con éxito",
        errorMessage: "Error al actualizar las preferencias de notificación",
      },
    },
    profileHeader: {
      anonymousUser: "Usuario Anónimo",
      verified: "Verificado",
      verifyAccount: "Verificar Cuenta",
      referralLink: {
        title: "Tu Enlace de Referido",
        description: "¡Comparte este enlace para ganar recompensas!",
        copyLink: "Copiar Enlace",
      },
      kycWarning:
        "Para acceder a todas las funciones y comenzar a invertir, por favor verifica tu cuenta.",
    },
    onboarding: {
      navigation: {
        previous: "Anterior",
        next: "Siguiente",
        skip: "Omitir",
        startInvesting: "Comenzar a Invertir",
      },
      languageSelector: {
        title: "Elige tu Idioma",
        english: "English",
        spanish: "Español",
      },
      steps: {
        step0: {
          title: "Bienvenido a Token Fleet",
          description:
            "¡Descubre el futuro de las inversiones en flotas de autos en el próspero mercado de Dubái! Comienza a construir tu cartera con tan solo $100 y genera ingresos pasivos a través de nuestra plataforma segura, transparente y regulada.",
        },
        step1: {
          title: "Conecta tu Billetera",
          description:
            "Comienza conectando tu billetera Web3. Esta es tu llave para invertir en acciones tokenizadas de autos y gestionar tu cartera de forma segura en la blockchain.",
        },
        step2: {
          title: "Completa la Verificación KYC",
          description:
            "Verifica tu identidad para cumplir con las regulaciones. Todos los documentos se almacenan y verifican de forma segura para garantizar un entorno de inversión seguro.",
        },
        step3: {
          title: "Crea tu NFT de Cartera",
          description:
            "Obtén tu NFT de Cartera único que mantendrá de forma segura todas tus inversiones en acciones de autos. Este NFT representa tu participación en el ecosistema Token Fleet.",
        },
        step4: {
          title: "Explora Oportunidades de Inversión",
          description:
            "¡Comienza con tan solo $100! Cada flota está minuciosamente documentada con papeles de propiedad verificados, registros de mantenimiento y contratos de alquiler, todo asegurado en la blockchain.",
        },
        step5: {
          title: "Invierte y Monitorea el Rendimiento",
          description:
            "Cada inversión está respaldada por un contrato legal entre tú y Token Fleet, accesible desde tu perfil. Tu NFT de cartera contiene tus acciones de flota, representando tu propiedad. Monitorea el rendimiento de tus inversiones, verifica la documentación y realiza un seguimiento de las ganancias en tiempo real a través de tu panel.",
        },
        step6: {
          title: "Comienza a Invertir Ahora",
          description:
            "Tu viaje para generar ingresos pasivos a través del mercado premium de alquiler de autos de Dubái comienza aquí. ¡Únete a Token Fleet hoy y transforma tu forma de invertir!",
        },
      },
    },
    investmentsTab: {
      overview: {
        totalInvestment: "Inversión Total",
        activeFleets: "flotas activas",
        totalReturns: "Rendimientos Totales",
        unclaimed: "sin reclamar",
        lastClaim: "Último Reclamo",
        noClaimsYet: "Aún no hay reclamos",
        totalClaims: "Reclamos Totales",
        fromFleets: "De {count} flotas",
      },
      sections: {
        yourFleets: "Tus Flotas",
        investmentHistory: "Historial de Inversiones",
        returnsHistory: "Historial de Rendimientos",
        fleetDetails: "Detalles de la Flota",
      },
      fleetStatus: {
        creating: "Flota en preparación",
        posted: "Oportunidad de inversión abierta",
        funded: "Objetivo de inversión alcanzado",
        onPurchase: "Adquiriendo vehículos",
        active: "Flota generando rendimientos",
        closing: "Flota en liquidación",
        closed: "Esta flota ha completado su ciclo de vida",
        notRealized: "Inversión de flota cancelada",
        unknown: "Estado no disponible",
      },
      statusDescriptions: {
        creating:
          "Esta flota está siendo creada. El historial de inversiones estará disponible cuando la flota se abra para inversión.",
        posted:
          "Esta flota está abierta para inversión. Realiza tu primera inversión para ver tu historial.",
        funded:
          "Esta flota ha sido completamente financiada. Tu historial de inversiones se muestra a continuación.",
        onPurchase:
          "Esta flota está en fase de compra. Tu historial de inversiones se muestra a continuación.",
        active:
          "Esta flota está operativa. Tu historial de inversiones se muestra a continuación.",
        closing:
          "Esta flota está en fase de cierre. Tu historial de inversiones se muestra a continuación.",
        closed:
          "Esta flota ha sido cerrada. Tu historial de inversiones se muestra a continuación.",
        notRealized:
          "Esta flota no se realizó. Tu historial de inversiones se muestra a continuación.",
        unknown: "El estado de la flota es desconocido.",
      },
      buttons: {
        claimRewards: "Reclamar Recompensas",
        redeemFunds: "Recuperar Fondos",
        noRewards: "Sin Recompensas",
        investMore: "Invertir Más",
        showDetails: "Mostrar Detalles",
        hideDetails: "Ocultar Detalles",
        previous: "Anterior",
        next: "Siguiente",
        viewContract: "Ver Contrato",
        viewTransaction: "Ver Transacción",
        close: "Cerrar",
        download: "Descargar",
      },
      contractModal: {
        title: "Vista Previa del Contrato",
        loading: "Cargando contrato...",
        error: "No se pudo cargar el contrato.",
        tryAgain: "Por favor, inténtalo de nuevo más tarde.",
      },
      fleetMetrics: {
        investment: "Inversión",
        available: "Disponible",
        totalReturns: "Rendimientos Totales",
        currentStage: "Etapa Actual",
        totalValue: "Valor Total",
        yourInvestment: "Tu Inversión",
        availableToClaim: "Disponible para Reclamar",
      },
      stageDescriptions: {
        posted: "Abierta para nuevas inversiones",
        funded: "La flota ha alcanzado su objetivo de inversión",
        onPurchase:
          "El administrador de la flota está adquiriendo los vehículos",
        active: "La flota está operando y generando rendimientos",
        closing: "La flota se está preparando para el cierre",
        closed: "La flota ha completado su ciclo de vida",
        notRealized: "La inversión en la flota fue cancelada",
        creating: "La flota se está preparando para inversión",
      },
      emptyStates: {
        noFleets: "No se encontraron flotas.",
        noFleetsDescription:
          "Las flotas aparecerán aquí cuando realices inversiones",
        noReturns: "No hay historial de rendimientos disponible",
        noReturnsDescription:
          "Tu historial de rendimientos aparecerá aquí una vez que comiences a ganar rendimientos",
      },
      yourFleets: {
        investmentLabel: "Inversión",
        availableLabel: "Disponible",
        totalReturnsLabel: "Rendimiento total",
        redeeming: "Recuperando...",
        claiming: "Reclamando...",
        redeemFunds: "Recuperar Fondos",
        claimRewards: "Reclamar Recompensas",
        noRewards: "Sin Recompensas",
        investMore: "Invertir Más",
        showDetails: "Mostrar Detalles",
        hideDetails: "Ocultar Detalles",
      },
    },
    generalTab: {
      pagination: {
        previous: "Anterior",
        next: "Siguiente",
      },
      stats: {
        totalInvestment: "Inversión Total",
        activeFleets: "flotas activas",
        totalRewards: "Recompensas Totales",
        unclaimed: "sin reclamar",
        claimNow: "Reclamar ahora",
        referralProgram: "Programa de Referidos",
        referees: "referidos",
        claimed: "reclamado",
        checkNewRewards: "Ver nuevas recompensas",
      },
      metrics: {
        investmentDistribution: "Distribución de Inversiones",
        noInvestmentData: "Sin datos de inversión",
        recentActivity: "Actividad Reciente",
        noActivityData: "Sin datos de actividad",
        lastRewardClaim: "Último Reclamo de Recompensa",
        newInvestment: "Nueva Inversión",
      },
      accountStats: {
        title: "Estadísticas de la Cuenta",
        totalFleets: "Total de Flotas",
        fleets: "flotas",
        totalInvestments: "Total de Inversiones",
        investments: "inversiones",
        lastClaim: "Último Reclamo",
        noClaimsYet: "Aún no hay reclamos",
        accountStatus: "Estado de la Cuenta",
        verified: "Verificado",
        unverified: "No verificado",
      },
    },
  },
  carDetails: {
    welcome: "Bienvenido a Detalles del Coche",
    connectWallet:
      "Conecta tu billetera para ver información detallada sobre este coche y participar en inversiones de flota.",
    connectWalletTitle: "Conecta Tu Billetera",
    connectWalletDescription:
      "Usa el botón de conexión en el menú superior para vincular tu billetera y acceder a los detalles de inversión del coche.",
    loading: "Cargando detalles del coche...",
    notFound: "Coche no encontrado",
    backToFleet: "Volver a la Flota",
    partOfFleet: "Parte de la Flota:",
    fleetInfo: {
      cars: "coches",
      totalValue: "valor total",
      expectedApr: "APR esperado",
    },
    viewFleetDetails: "Ver Detalles de la Flota",
    investInFleet: "Invertir en la Flota",
    aboutThis: "Acerca de este",
    investmentOverview: {
      totalInvestment: "Inversión Total",
      totalApr: "APR Total",
      tooltips: {
        investment:
          "Pago inicial total requerido:\n• Precio de compra: ${{purchasePrice}}\n• Tarifa de la empresa: ${{companyFee}}\n• Depósito de seguridad: ${{securityDeposit}}\n• Gastos del primer año: ${{firstYearExpenses}}\n\nNota: El depósito de seguridad se devolverá después de 4 años",
        roi: "Retorno total de la inversión durante 4 años\nBasado en beneficios netos que incluyen:\n• Ingresos por alquiler\n• Valor de venta del coche\n• Menos todos los gastos y tarifas",
      },
    },
    accordion: {
      investmentDetails: "Detalles de Inversión",
      projectedReturns: "Retornos Proyectados",
      technicalDetails: "Detalles Técnicos",
      documentation: "Documentación",
      rentalIncome: "Ingresos por Alquiler",
      maintenanceExpenses: "Mantenimiento y Gastos",
      investment: {
        purchasePrice: "Precio de Compra",
        purchasePriceTooltip:
          "Precio base del vehículo antes de tarifas y depósitos",
        companyFee: "Tarifa de la Empresa",
        companyFeeTooltip:
          "Tarifa única ({{percentage}}% del precio de compra)\nEsta tarifa cubre:\n• Marketing inicial\n• Configuración de la plataforma\n• Preparación de mantenimiento básico",
        securityDeposit: "Depósito de Seguridad",
        securityDepositTooltip:
          "Depósito reembolsable ({{percentage}}% del precio de compra)\nReservado para:\n• Reparaciones importantes si son necesarias\n• Mantenimiento inesperado\nSe devolverá si no se utiliza después del período de alquiler",
        firstYearExpenses: "Gastos del Primer Año",
        firstYearExpensesTooltip:
          "Costos anuales para el primer año:\n• Seguro: ${{insurance}}\n• GPS y Registro: ${{registration}}\nEstos gastos están incluidos en la inversión total",
        totalInvestment: "Inversión Total",
        totalInvestmentTooltip:
          "Pago inicial total requerido para configurar la flota.\nNota: El depósito de seguridad se devolverá cuando finalice el acuerdo.",
      },
      returns: {
        totalApr: "APR Total",
        totalAprTooltip:
          "Estructura de la inversión:\n• Período de operación de alquiler de 4 años\n• Venta del vehículo a valor de mercado después de que finalice el período\n• Depósito de seguridad devuelto al momento de la venta\n\nLos retornos se distribuyen de:\n• Ingresos por alquiler durante la operación\n• Liquidación final después de la venta del vehículo",
        rentalReturnFrequency: "Frecuencia de Retorno de Alquiler",
        rentalReturnFrequencyTooltip:
          "Acerca de la Frecuencia de Retorno:\n• Este coche funciona mejor con alquileres {{frequency}}\n• Basado en análisis de mercado y patrones de alquiler\n• Maximiza tus retornos potenciales\n¿Por qué {{frequency}}?\n• Punto de precio óptimo\n• Mayor tasa de ocupación\n• Mejor adaptado a la demanda local",
        baseRentalRate: "Tarifa Base de Alquiler",
        baseRentalRateTooltip:
          "Tarifa base: alquiler {{frequency}}\nEsta tarifa se calcula considerando:\n• Demanda del mercado en Dubái\n• Categoría y características del vehículo\n• Tendencias de precios estacionales\n\nDe este monto:\n• {{investorPercentage}}% distribuido a los accionistas\n• {{platformPercentage}}% tarifa de plataforma\n• En caso de tomar del depósito de seguridad, se cubrirá antes de la distribución.",
        annualOperatingExpenses: "Gastos Operativos Anuales",
        annualOperatingExpensesTooltip:
          "Costos anuales que incluyen:\n• Seguro: ${{insurance}}\n• Registro: ${{registration}}\nLos gastos del primer año están incluidos en la inversión total",
        netAnnualRentalIncome: "Ingreso Neto Anual por Alquiler",
        netAnnualRentalIncomeTooltip:
          "Ingreso anual por alquiler después de:\n• Tarifa de la empresa ({{percentage}}%)\n• Gastos operativos\nBasado en la ocupación esperada:\n{{occupancyDetails}}",
        expectedDepreciation: "Depreciación Esperada",
        expectedDepreciationTooltip:
          "Proyección del valor del coche:\n• Precio inicial: ${{initialPrice}}\n• Valor mínimo: ${{minimumValue}} (70%)\n• Depreciación máxima: ${{maximumDepreciation}} (50%)\nNota: La depreciación real puede variar según las condiciones del mercado y la duración del alquiler.",
        netProfits: "Beneficios Netos (4 Años)",
        netProfitsTooltip:
          "Beneficios proyectados para un período mínimo de 4 años:\n• Ingresos por alquiler (menos {{rentalFee}}% de tarifa)\n• Valor de venta del coche (alrededor del 60% del precio de compra)\n• Menos gastos operativos\n• Más 100% del depósito de seguridad\n• Menos tarifa de venta de la empresa ({{sellingFee}}%)\n\nBeneficios adicionales posibles si el período de alquiler se extiende más allá de 4 años.",
        occupancy: {
          hourly: "• 200 días × 6 horas por día",
          daily: "• 200 días por año",
          weekly: "• 50 semanas por año",
          monthly: "• 11 meses por año",
        },
      },
      technical: {
        basicSpecifications: "Especificaciones Básicas",
        performance: "Rendimiento",
        dimensions: "Dimensiones",
        featuresComfort: "Características y Confort",
        safetyAdditional: "Seguridad y Características Adicionales",
        extraFeatures: "Características Adicionales",
        availableAfterPurchase: "Disponible una vez que se compre el coche",
        specs: {
          plate: "Matrícula",
          mileage: "Kilometraje",
          engine: "Motor",
          transmission: "Transmisión",
          drivetrain: "Tracción",
          fuelType: "Tipo de Combustible",
          bodyType: "Tipo de Carrocería",
          horsepower: "Caballos de Fuerza",
          torque: "Torque",
          topSpeed: "Velocidad Máxima",
          acceleration: "0-100 km/h",
          battery: "Batería",
          range: "Autonomía",
          length: "Longitud",
          width: "Ancho",
          height: "Altura",
          wheelbase: "Distancia entre Ejes",
          trunkCapacity: "Capacidad del Maletero",
          airbags: "Airbags",
          color: "Color",
          licenseCategory: "Categoría de Licencia",
          safetyRating: "Clasificación de Seguridad",
        },
      },
      documents: {
        noDocuments: "No hay documentos disponibles",
        rowsPerPage: "Filas por página:",
      },
      income: {
        noIncome: "No hay registros de ingresos por alquiler disponibles",
        deposit: "Depósito",
      },
      expenses: {
        noExpenses: "No hay registros de mantenimiento y gastos disponibles",
      },
    },
  },
  footer: {
    backToTop: "Volver arriba",
    tagline:
      "Invierte en coches y gana con cada alquiler. Token Fleet te permite ser propietario de una parte y ganar cada vez que las ruedas giran.",
    about: "Acerca de",
    aboutLinks: {
      mission: "Nuestra Misión",
      howItWorks: "Cómo Funciona",
      team: "Equipo",
    },
    resources: "Recursos",
    resourceLinks: {
      documentation: "Documentación",
      terms: "Términos de Uso",
      privacy: "Política de Privacidad",
    },
    contact: "Contáctanos",
    companyBy: "por Reliable Route Rental Car LLC.",
    allRightsReserved: "Todos los Derechos Reservados.",
  },
  howItWorks: {
    title: "Cómo Funciona",
    subtitle:
      "Únete a Token Fleet y comienza a generar ingresos pasivos a través del mercado premium de alquiler de coches de Dubái. Nuestra plataforma basada en blockchain hace que la inversión en flotas de vehículos sea accesible, segura y transparente.",
    investmentJourney: "Tu Recorrido de Inversión",
    platformFeatures: "Características de la Plataforma",
    benefitsOfInvesting: "Beneficios de Invertir",
    steps: [
      {
        title: "Conecta Tu Billetera",
        description:
          "Comienza conectando tu billetera Web3 a nuestra plataforma. Esta conexión segura te permite invertir en acciones tokenizadas de coches y gestionar tu cartera en la blockchain.",
        details: [
          "Soporte para las principales billeteras Web3",
          "Transacciones seguras en blockchain",
          "Gestión sencilla de cartera",
        ],
      },
      {
        title: "Completa la Verificación",
        description:
          "Completa nuestro proceso KYC para garantizar el cumplimiento de las regulaciones. Este paso ayuda a mantener un entorno de inversión seguro y transparente.",
        details: [
          "Verificación de identidad rápida",
          "Almacenamiento seguro de documentos",
          "Cumplimiento normativo",
        ],
      },
      {
        title: "Elige Tu Inversión",
        description:
          "Explora nuestra selección curada de vehículos premium. Cada flota está minuciosamente documentada con papeles de propiedad verificados y registros de mantenimiento.",
        details: [
          "Selección de vehículos premium",
          "Documentación transparente",
          "Opciones flexibles de inversión",
        ],
      },
      {
        title: "Recibe Tu NFT de Cartera",
        description:
          "Obtén tu NFT de Cartera único que mantendrá de forma segura todas tus inversiones en acciones de coches. Este NFT representa tu participación en el ecosistema Token Fleet.",
        details: [
          "Identificador único de cartera",
          "Registro seguro de propiedad",
          "Fácil transferencia y gestión",
        ],
      },
    ],
    features: [
      {
        title: "Seguridad de Contratos Inteligentes",
        description:
          "Tus inversiones están aseguradas por contratos inteligentes en la blockchain, garantizando registros de propiedad transparentes e inmutables.",
      },
      {
        title: "Análisis en Tiempo Real",
        description:
          "Sigue el rendimiento de tu flota, la generación de ingresos y el estado de mantenimiento a través de nuestro panel de control integral.",
      },
      {
        title: "Generación de Ingresos Pasivos",
        description:
          "Gana retornos regulares de los ingresos por alquiler, con beneficios distribuidos automáticamente a tu billetera.",
      },
    ],
    benefits: [
      {
        title: "Baja Barrera de Entrada",
        description:
          "Comienza a construir tu cartera con tan solo $100. Nuestro modelo de propiedad fraccionada hace que la inversión en vehículos premium sea accesible para todos.",
      },
      {
        title: "Operaciones Transparentes",
        description:
          "Todas las operaciones de la flota, los registros de mantenimiento y la distribución de ingresos se registran en la blockchain, garantizando una transparencia completa.",
      },
      {
        title: "Respaldo de Activos Reales",
        description:
          "Cada token está respaldado por vehículos premium reales que operan en el próspero mercado de alquiler de Dubái, garantizando una inversión respaldada por bienes tangibles.",
      },
    ],
  },
  banner: {
    messages: [
      {
        text: "¿Necesitas tokens de prueba? Únete a nuestro Telegram",
        highlight: "Solicitar tokens",
      },
      {
        text: "¿Quieres probar nuestra plataforma de prueba?",
        highlight: "Obtén tokens gratis",
      },
      {
        text: "Únete a nuestra comunidad de Telegram",
        highlight: "Obtén tokens de prueba",
      },
      {
        text: "Prueba Token Fleet con tokens gratuitos",
        highlight: "Únete a Telegram",
      },
    ],
  },
  kyc: {
    title: "Completa tu Verificación KYC",
    description:
      "Para cumplir con los requisitos regulatorios, necesitamos verificar tu identidad antes de que puedas comenzar a invertir.",
    startVerification: "Iniciar Verificación",
  },
  featuredFleet: {
    messages: [
      {
        text: "Comienza a invertir desde solo $100",
        highlight: "Ver flota destacada",
      },
      {
        text: "Propiedad fraccionada de coches simplificada",
        highlight: "Explorar ahora",
      },
      {
        text: "Gana ingresos pasivos de alquileres",
        highlight: "Saber más",
      },
      {
        text: "Acciones tokenizadas de coches disponibles",
        highlight: "Únete hoy",
      },
    ],
    error: "Error al obtener la flota destacada",
    carousel: {
      banners: [
        {
          alt: "Oportunidad de inversión en flota de coches de lujo",
          title: "Inversión en Flota Premium",
          subtitle: "Comienza a invertir desde solo $100",
        },
        {
          alt: "Cartera diversa de coches",
          title: "Cartera Diversificada",
          subtitle: "Múltiples inversiones en coches en una flota",
        },
        {
          alt: "Retornos de alquiler de coches",
          title: "Ingresos Pasivos",
          subtitle: "Gana con alquileres de coches",
        },
      ],
      viewFeaturedFleet: "Haz clic para ver la flota destacada →",
    },
  },
  mission: {
    title: "Nuestra Misión",
    subtitle:
      "Cerrar la brecha entre los activos del mundo real y la tecnología blockchain, comenzando por revolucionar la industria del alquiler de coches a través de la propiedad transparente y oportunidades de inversión innovadoras.",
    vision: {
      title: "Nuestra Visión",
      democratizing: {
        title: "Democratizando la Inversión en Vehículos",
        description:
          "Estamos creando un futuro donde cualquiera puede poseer una parte de una flota de vehículos premium. A través de la tecnología blockchain, estamos transformando el alquiler tradicional de coches en una oportunidad de inversión accesible, permitiendo a los inversores ganar ingresos pasivos mientras participan en la creciente economía de la movilidad.",
      },
      bridging: {
        title: "Uniendo las Finanzas Tradicionales y Digitales",
        description:
          "Token Fleet se encuentra en la intersección de la gestión tradicional de activos y la innovación blockchain. Estamos siendo pioneros en la tokenización de activos del mundo real, comenzando con vehículos, para crear un nuevo estándar de oportunidades de inversión transparentes, eficientes y accesibles.",
      },
    },
    difference: {
      title: "Lo Que Nos Diferencia",
      ownership: {
        title: "Propiedad Real de Activos",
        description:
          "A diferencia de los vehículos de inversión tradicionales, Token Fleet proporciona propiedad directa en nuestra flota de coches. Cada token representa una participación real en activos físicos, respaldados por contratos inteligentes y marcos legales que aseguran que tu inversión sea segura y tangible.",
      },
      transparency: {
        title: "Transparencia Impulsada por Blockchain",
        description:
          "Nuestra plataforma utiliza la tecnología blockchain para proporcionar una transparencia sin precedentes. Cada aspecto del rendimiento de la flota, desde la generación de ingresos hasta los costos de mantenimiento, se registra en la blockchain y es accesible para nuestros inversores en tiempo real.",
      },
    },
    values: {
      title: "Nuestros Valores Fundamentales",
      items: [
        {
          title: "Propiedad Real",
          description:
            "Cada token representa propiedad genuina en nuestra flota de vehículos, respaldada por activos reales y asegurada mediante tecnología blockchain.",
          icon: "🔐",
        },
        {
          title: "Transparencia",
          description:
            "Visibilidad completa del rendimiento de la flota, distribución de ingresos y métricas operativas a través de nuestra plataforma basada en blockchain.",
          icon: "📊",
        },
        {
          title: "Innovación",
          description:
            "Uniendo el alquiler tradicional de coches con la tecnología blockchain para crear un nuevo paradigma de propiedad fraccionada de vehículos.",
          icon: "💡",
        },
        {
          title: "Accesibilidad",
          description:
            "Haciendo que la inversión en flotas de coches sea accesible para todos, independientemente de su capacidad de inversión.",
          icon: "🌍",
        },
        {
          title: "Sostenibilidad",
          description:
            "Promoviendo la utilización eficiente de vehículos y la gestión responsable de activos para la creación de valor a largo plazo.",
          icon: "🌱",
        },
        {
          title: "Comunidad",
          description:
            "Construyendo una comunidad de inversores con visión de futuro que creen en el futuro de los activos del mundo real tokenizados.",
          icon: "🤝",
        },
      ],
    },
  },
  team: {
    title: "Conoce a Nuestro Equipo",
    subtitle:
      "La fuerza impulsora detrás de Token Fleet combina una profunda experiencia en automóviles, tecnología blockchain y mercados financieros. Nuestro diverso equipo está unido por una sola misión: revolucionar la propiedad de coches a través de soluciones de inversión innovadoras.",
    members: [
      {
        name: "Marcos Maceo",
        position: "Director Ejecutivo",
        bio: "Con amplia experiencia en tecnología blockchain y la industria automotriz, Marcos lidera la visión de Token Fleet de revolucionar las inversiones en coches a través de soluciones blockchain innovadoras.",
        imageUrl: "https://tokenfleet.io/images/image-5.webp",
      },
      {
        name: "Daniela Suárez",
        position: "Directora de Marketing",
        bio: "Daniela aporta experiencia en marketing estratégico a Token Fleet, impulsando el crecimiento de nuestra marca y presencia en el mercado, asegurando que nuestra plataforma de inversión innovadora llegue a la audiencia adecuada.",
        imageUrl: "https://tokenfleet.io/images/daniela.jpeg",
      },
      {
        name: "Andy Ledesma",
        position: "Director de Tecnología",
        bio: "Andy lidera la innovación técnica de Token Fleet, aportando amplia experiencia en desarrollo blockchain para garantizar que nuestra plataforma se mantenga a la vanguardia de la tecnología.",
        imageUrl: "https://tokenfleet.io/images/andy.jpeg",
      },
      {
        name: "Muhamed Syfian",
        position: "Jefe de Operaciones",
        bio: "Con profunda experiencia en gestión de flotas, Muhamed asegura el rendimiento óptimo de nuestra cartera de vehículos y sistemas de mantenimiento, maximizando el valor para nuestros inversores.",
        imageUrl: "https://tokenfleet.io/images/image-8.webp",
      },
      {
        name: "Antonio Bermudez",
        position: "Jefe de Diseño",
        bio: "Antonio crea la experiencia de usuario y la identidad visual de Token Fleet, asegurando que nuestra plataforma sea tanto hermosa como intuitiva, manteniendo los más altos estándares de excelencia en diseño.",
        imageUrl: "https://tokenfleet.io/images/image-7.webp",
      },
    ],
  },
  terms: {
    title: "Términos y Condiciones",
    sections: {
      acceptance: {
        title: "1. Aceptación de Términos",
        content:
          'Al acceder y utilizar la plataforma Token Fleet ("Plataforma"), acepta estar sujeto a estos Términos y Condiciones ("Términos"). Estos Términos constituyen un acuerdo legalmente vinculante entre usted ("Usuario", "Inversor") y Token Fleet, operado por Reliable Route Car Rentals LLC ("Compañía", "nosotros", "nos", "nuestro").',
      },
      definitions: {
        title: "2. Definiciones",
        items: [
          '"Plataforma" se refiere a la plataforma de inversión Token Fleet.',
          '"Participación de Inversión" se refiere a la participación de propiedad fraccionada en un vehículo.',
          '"Período de Inversión" se refiere al plazo estándar de inversión de 4 años.',
          '"Ingresos por Alquiler" se refiere a los ingresos generados por el alquiler del vehículo.',
          '"Contrato Inteligente" se refiere al contrato basado en blockchain que rige la inversión.',
        ],
      },
      eligibility: {
        title: "3. Elegibilidad",
        intro: "Para utilizar la Plataforma e invertir, debe:",
        requirements: [
          "Tener al menos 18 años de edad",
          "Tener la capacidad jurídica para celebrar contratos",
          "Cumplir con todas las leyes y regulaciones aplicables",
        ],
      },
      investment: {
        title: "4. Términos de Inversión",
        structure: {
          title: "4.1 Estructura de Inversión",
          content:
            "Los usuarios pueden comprar propiedad fraccionada en vehículos a través de la Plataforma. El monto mínimo de inversión es de 100 USDC. Cada Participación de Inversión representa una participación proporcional en la propiedad del vehículo subyacente.",
        },
        period: {
          title: "4.2 Período de Inversión",
          content:
            "El Período de Inversión estándar es de 4 años desde la fecha de adquisición del vehículo. Si bien no se permiten rescates directos antes del final del período, Token Fleet está desarrollando un mercado secundario de liquidez que permitirá a los inversores negociar sus posiciones con otros antes del vencimiento.",
        },
      },
      fees: {
        title: "5. Tarifas y Pagos",
        setup: {
          title: "5.1 Tarifa Inicial de Configuración (10%)",
          content:
            "Se agrega una tarifa de la empresa del 10% al precio base de compra del vehículo o flota. Esta tarifa cubre:",
          items: [
            "Costos operativos de la plataforma",
            "Implementación y gestión de contratos inteligentes",
            "Documentación administrativa y legal",
            "Desarrollo de negocios y marketing",
          ],
        },
        deposit: {
          title: "5.2 Depósito de Seguridad (10%)",
          content:
            "Se mantiene en reserva un depósito de seguridad del 10% del monto total de la inversión (incluido el precio base y la tarifa de la empresa). Este depósito es:",
          items: [
            "Totalmente reembolsable al final del plazo de inversión",
            "Utilizado como amortiguador para mantenimiento o reparaciones inesperadas",
            "Aplicado para proteger tanto al inversor como a la plataforma",
          ],
        },
        charges: {
          title: "5.3 Cargos Fijos",
          content:
            "Se aplican cargos fijos adicionales para cubrir servicios esenciales:",
          items: [
            "Cobertura de seguro del primer año",
            "Instalación y suscripción del sistema de rastreo GPS",
            "Documentación y registro del vehículo",
            "Paquete de mantenimiento inicial",
          ],
        },
      },
      income: {
        title: "6. Distribución de Ingresos",
        items: [
          "6.1 Después de deducir la tarifa de plataforma del 30%, los inversores reciben el 70% de los ingresos por alquiler, distribuidos proporcionalmente según su Participación de Inversión.",
          "6.2 Los tokens que representan su participación de inversión estarán disponibles para reclamar en la Plataforma. Todas las distribuciones de ingresos por alquiler se realizan en USDC a través de la Plataforma.",
          "6.3 La Compañía se reserva el derecho de retener distribuciones por tarifas pendientes o costos de mantenimiento.",
        ],
      },
      vehicle: {
        title: "7. Gestión de Vehículos",
        items: [
          "7.1 La Compañía mantiene el control operativo total del vehículo para fines de alquiler.",
          "7.2 El vehículo está registrado bajo Reliable Route Car Rentals LLC en cumplimiento con las regulaciones de los EAU.",
          "7.3 La Compañía es responsable del mantenimiento, seguro y cumplimiento normativo.",
        ],
      },
      termination: {
        title: "8. Plazo y Terminación",
        items: [
          "8.1 Al final del Período de Inversión, el vehículo se venderá a valor de mercado.",
          "8.2 Los ingresos de la venta, menos la tarifa de salida del 10%, se distribuirán a los inversores según su Participación de Inversión.",
          "8.3 El depósito de seguridad será devuelto, menos cualquier gasto o reclamo pendiente.",
        ],
      },
      risks: {
        title: "9. Riesgos y Descargos de Responsabilidad",
        items: [
          "9.1 La inversión implica riesgos, incluida la posible pérdida del capital principal.",
          "9.2 La Compañía no garantiza rendimientos ni valor del vehículo.",
          "9.3 Las condiciones del mercado pueden afectar los ingresos por alquiler y el valor de reventa.",
          "9.4 El rendimiento pasado no es indicativo de resultados futuros.",
        ],
      },
      law: {
        title: "10. Ley Aplicable",
        content:
          "Estos Términos se rigen por las leyes de los Emiratos Árabes Unidos. Cualquier disputa estará sujeta a la jurisdicción exclusiva de los tribunales de Dubái, EAU.",
      },
      contact: {
        title: "11. Información de Contacto",
        intro: "Para consultas sobre estos Términos, póngase en contacto con:",
        company: "Reliable Route Car Rentals LLC",
        address1: "Rasis Business Center, 4º Piso, Oficina 75",
        address2: "Al Barsha, Dubái, EAU",
        phone: "Teléfono/WhatsApp: +971521703229",
        email: "Correo electrónico: info@tokenfleet.io",
      },
      lastUpdated: "Última actualización: 17 de enero de 2025",
    },
  },
  privacy: {
    title: "Política de Privacidad",
    sections: {
      introduction: {
        title: "1. Introducción",
        content:
          'Token Fleet, operado por Reliable Route Car Rentals LLC ("nosotros," "nuestro," o "nos"), está comprometido a proteger su privacidad. Esta Política de Privacidad explica cómo recopilamos, utilizamos, divulgamos y protegemos su información cuando utiliza nuestra plataforma.',
      },
      information: {
        title: "2. Información que Recopilamos",
        personal: {
          title: "2.1 Información Personal",
          items: [
            "Nombre e información de contacto",
            "Fecha de nacimiento y nacionalidad",
            "Identificación emitida por el gobierno",
            "Direcciones de billetera blockchain",
            "Historial y preferencias de inversión",
            "Información bancaria y de pago",
          ],
        },
        technical: {
          title: "2.2 Información Técnica",
          items: [
            "Dirección IP e información del dispositivo",
            "Tipo y versión del navegador",
            "Datos de uso y analítica",
            "Cookies y tecnologías de seguimiento similares",
          ],
        },
      },
      usage: {
        title: "3. Cómo Utilizamos Su Información",
        intro: "Utilizamos su información para:",
        items: [
          "Procesar sus inversiones y transacciones",
          "Verificar su identidad y prevenir fraudes",
          "Proporcionar comunicaciones relacionadas con inversiones",
          "Proporcionar atención al cliente",
          "Cumplir con obligaciones legales",
          "Mejorar nuestros servicios y experiencia de usuario",
        ],
      },
      sharing: {
        title: "4. Compartir Información",
        intro: "Podemos compartir su información con:",
        items: [
          "Proveedores de servicios y socios comerciales",
          "Autoridades reguladoras y fuerzas del orden",
          "Asesores profesionales y auditores",
        ],
        note: "No vendemos su información personal a terceros.",
      },
      security: {
        title: "5. Seguridad de Datos",
        content:
          "Implementamos medidas técnicas y organizativas apropiadas para proteger su información personal. Sin embargo, ningún método de transmisión por Internet o almacenamiento electrónico es 100% seguro.",
      },
      rights: {
        title: "6. Sus Derechos",
        intro: "Usted tiene derecho a:",
        items: [
          "Acceder a su información personal",
          "Corregir información inexacta",
          "Solicitar la eliminación de su información",
          "Oponerse al procesamiento de su información",
          "Portabilidad de datos",
        ],
      },
      cookies: {
        title: "7. Cookies y Seguimiento",
        content:
          "Utilizamos cookies y tecnologías de seguimiento similares para mejorar su experiencia en nuestra plataforma. Puede controlar la configuración de cookies a través de las preferencias de su navegador.",
      },
      changes: {
        title: "8. Cambios en la Política de Privacidad",
        content:
          "Podemos actualizar esta Política de Privacidad de vez en cuando. Le notificaremos sobre cualquier cambio material publicando la política actualizada en nuestra plataforma.",
      },
      contact: {
        title: "9. Contáctenos",
        intro:
          "Para consultas relacionadas con la privacidad, póngase en contacto con:",
        company: "Reliable Route Car Rentals LLC",
        address1: "Rasis Business Center, 4º Piso, Oficina 75",
        address2: "Al Barsha, Dubái, EAU",
        phone: "Teléfono/WhatsApp: +971521703229",
        email: "Correo electrónico: info@tokenfleet.io",
      },
      lastUpdated: "Última actualización: 17 de enero de 2025",
    },
  },
  referee: {
    title: "Únete a Token Fleet",
    connectWallet: {
      title: "¿Listo para Comenzar tu Viaje?",
      description:
        "Conecta tu billetera para desbloquear beneficios exclusivos con Token Fleet",
      referralCode: "Código de Referencia:",
      buttonLabel: "Conectar Billetera",
    },
    signIn: {
      title: "¡Un Paso Más!",
      description:
        "Inicia sesión con tu billetera para activar tu membresía exclusiva",
      referralCode: "Código de Referencia:",
      buttonLabel: "Iniciar Sesión",
    },
    loading: {
      title: "¡Casi Listo!",
      description: "Estamos configurando tu membresía de Token Fleet",
      referralCode: "Código de Referencia:",
    },
    success: {
      title: "¡Bienvenido a Bordo! 🎉",
      description: "Ahora eres oficialmente parte de Token Fleet",
      buttonLabel: "Explorar Token Fleet",
    },
    error: {
      title: "¡Ups! Algo No Está Bien",
      referralCode: "Código de Referencia:",
      buttonLabel: "Ir a la Página Principal",
    },
  },
};

// Combined translations
export const translations = {
  en,
  es,
};
