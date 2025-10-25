# 📘 Payverge — Crypto Hospitality Platform (Full PRD)

## 0) Product Summary

**Payverge** lets restaurants, cafés, bars, hotels, and venues:

* Publish **digital menus** and per-table QR codes.
* Generate **itemized bills**; guests scan to **split and pay** their share in **USDC**.
* Add **tips** (USDC) at checkout.
* Receive **instant settlement** to the business wallet; **2% fee** automatically taken.
* Enjoy **automatic accountability**: immutable on-chain events + detailed off-chain audit logs.

No SaaS upfront fee. **Anyone can onboard**.

---

## 1) Goals & Non-Goals

### Goals

* Simple, modern, mobile-first guest experience.
* Self-serve business onboarding; fast menu setup.
* USDC-only, low-friction payments.
* Bill splitting (equal, custom amounts, “by item” UX).
* Tipping in USDC with **merchant-defined tipping address** (editable later).
* **On-chain traceability** for payments/tips/fees.
* Real-time dashboard for businesses; CSV exports.

### Non-Goals (Phase 1)

* Deep POS integrations (provide a lightweight API; native integrations later).
* Fiat on/off-ramp.
* Loyalty/points (Phase 2).
* Pre-authorized tabs (Phase 2).
* KYC. (Optional later for jurisdictions; not required now.)

---

## 2) Personas

* **Guest (Diner):** Scans QR → views menu/bill → pays USDC → adds tip → gets receipt.
* **Business Owner/Manager:** Creates venue, uploads menu, manages tables/bills, views revenue & tips, updates payout & tip addresses.
* **Staff (optional):** Can view open tables/bills and close bills (role-based).
* **Admin (Payverge):** Moderation, config, fee parameters (on-chain), dispute assistance.

---

## 3) End-User Experience

### 3.1 Guest Flow (Mobile)

1. **Scan QR** at table → opens `/t/:tableCode`

   * If no open bill: shows **menu**.
   * If open bill: shows **bill view** (items, tax, service fee if any).
2. **Choose Split**:

   * Split **equally**, **custom amount**, or **by item** (UI helper).
   * “By item” is UX-only; contract accepts any **share amount** up to outstanding.
3. **Add Tip**: Quick % (10/15/20) or custom amount.
4. **Pay USDC**: Wallet opens → Approve USDC (if needed) → Pay.
5. **Receipt**: On success, shows **tx hash**, bill status, and optional PDF/email export (PDF generated; email optional/Phase 2).

### 3.2 Business Flow (Web/Dashboard)

1. **Onboard**: Connect wallet → create **Business** (name, logo, address).
2. **Configure**:

   * **Settlement Address** (business wallet; defaults to login wallet).
   * **Tipping Address** (can be the same or different; **editable later**).
   * **Tax & Service Fee rules** (percent, inclusive/exclusive).
3. **Menu Builder**: Categories, items, options, allergens/dietary tags, images, availability (in/out of stock).
4. **Tables & QRs**: Generate printed QRs per table (or per area).
5. **Bills**:

   * Create bill (from POS import/API/manual), attach to a table.
   * See payments arrive in **real-time**, with remaining balance.
   * Close bill; download receipt (CSV/PDF).
6. **Reports**: Sales, tips, fees, bill history, staff report, exports.
7. **Settings**: Edit addresses (settlement & tipping), taxes, branding, and permissions.

---

## 4) Payments & Fees

* **Currency:** **USDC only** (ERC-20, 6 decimals).
* **Fee:** **2%** of **bill payment amount**.

  * **Tips are excluded** from the platform fee (fair for staff).
* **Settlement:** Net USDC **directly to business settlement address**.
* **Tips:** USDC **directly to the business “tipping address”**.

  * The **tipping address is snapshotted onto each bill at creation**, so changing it later doesn’t affect existing open bills.

---

## 5) Smart Contracts (Foundry)

> All contracts are chain-agnostic; choose the network at deployment (e.g., Ethereum mainnet or preferred L2).
> USDC token addresses are set per chain in config; contracts use `SafeERC20`.

### 5.1 `BusinessRegistry.sol`

* **Purpose:** Self-serve business registration + canonical on-chain reference.
* **State:**

  ```solidity
  struct Business {
      address owner;          // wallet that can update config
      address settlement;     // where bill payments are routed (net)
      address tipping;        // where tips are routed
      bool    active;
  }
  mapping(uint256 => Business) public businesses; // businessId -> Business
  mapping(address => uint256) public businessOf;  // primary owner -> businessId (supports multi-venue later)
  ```
* **API:**

  * `registerBusiness(address settlement, address tipping) returns (uint256 businessId)`
  * `updateBusiness(uint256 businessId, address settlement, address tipping)`
  * `setActive(uint256 businessId, bool active)`
* **Events:** `BusinessRegistered`, `BusinessUpdated`, `BusinessStatusChanged`.

### 5.2 `BillManager.sol`

* **Purpose:** Immutable, auditable bill lifecycle + safe multi-payer partial payments.
* **State:**

  ```solidity
  struct Bill {
      uint256 businessId;
      bytes32 itemsHash;        // hash of off-chain itemization
      uint128 totalDue;         // in USDC (6 decimals)
      uint128 totalPaid;        // running total of payments
      address tippingSnapshot;  // tipping address at creation
      bool    closed;
  }
  mapping(bytes32 => Bill) public bills; // billId -> Bill
  ```
* **API:**

  * `createBill(bytes32 billId, uint256 businessId, bytes32 itemsHash, uint128 totalDue, address tippingSnapshot)`

    * **Bill ID**: `keccak256(businessId | localBillRef | timestamp | nonce)` generated server-side; prevents collisions.
  * `closeBill(bytes32 billId)` (only business or when fully paid).
* **Events:** `BillCreated`, `BillClosed`.

> **Note:** We do **not** price-convert on-chain. All totals are **USDC** amounts pre-computed off-chain (see §9).

### 5.3 `PaymentRouter.sol`

* **Purpose:** Accept USDC payments for a bill, route **net** to settlement address and **fees** to treasury.
* **Params:** `feeBps = 200` (2%), `treasury` = Payverge ops wallet, `usdc` token address.
* **API:**

  ```solidity
  function payBill(bytes32 billId, uint128 amount) external nonReentrant
  ```

  * Validates bill exists & not closed.
  * Computes `fee = amount * feeBps / 10_000`.
  * Transfers `amount - fee` to business settlement address (from payer → via `transferFrom`).
  * Transfers `fee` to `treasury`.
  * Increments `totalPaid`.
  * If `totalPaid >= totalDue` → emits “BillSettled” (soft close; business still calls `closeBill` for explicit close).
* **Events:** `BillPaid(billId, payer, amount, fee, net)`, `ParamsUpdated`, `BillSettled`.

### 5.4 `TipRouter.sol`

* **Purpose:** Route **tip USDC directly** to bill’s `tippingSnapshot`.
* **API:**
  `function tipBill(bytes32 billId, uint128 tipAmount) external nonReentrant`

  * Transfers full `tipAmount` to `tippingSnapshot` (no platform fee).
* **Event:** `BillTipped(billId, tipper, tipAmount)`.

### 5.5 Security & Admin

* Contracts use `Ownable`, `Pausable`, `ReentrancyGuard`, `SafeERC20`.
* Admin can update `feeBps`, `treasury`, and pause routers.
* **No custodial balances**—all transfers use `transferFrom` directly from payer → recipients.

---

## 6) On-Chain ↔ Off-Chain Integrity

* **Menu & Bill Integrity**:

  * Bill creation stores an **`itemsHash`**: `sha256(JSON of items, quantities, taxes/service)`.
  * Off-chain JSON is persisted (S3/IPFS) and referenced in DB.
  * Auditors can recompute the hash to verify the bill content matched what was paid.
* **Receipts**:

  * Each payment emits immutable events; the dashboard composes tx hash + bill details into receipts (PDF/CSV).

---

## 7) Data Model (MongoDB)

**businesses**

```json
{
  "_id": "...",
  "businessId": 123,                    // on-chain id
  "ownerWallet": "0xOwner",
  "settlementAddress": "0xSettlement",
  "tippingAddress": "0xTips",
  "name": "Kawa Café",
  "logoUrl": "https://.../logo.png",
  "location": { "country": "AE", "city": "Dubai", "address": "..." },
  "tax": { "vatPct": 5.0, "inclusive": true },
  "serviceFeePct": 0,
  "active": true,
  "createdAt": "...",
  "updatedAt": "..."
}
```

**menus**

```json
{
  "_id": "...",
  "businessId": 123,
  "version": 4,
  "categories": [
    { "id": "c1", "name": "Drinks", "order": 1 }
  ],
  "items": [
    {
      "id": "i1",
      "name": "Espresso",
      "desc": "Double shot",
      "priceUSDC": 3.50,
      "imageUrl": "https://.../espresso.jpg",
      "categoryId": "c1",
      "tags": ["vegan", "caffeine"],
      "options": [
        { "id": "o1", "name": "Milk", "choices": [
          { "id": "o1c1", "label": "Oat", "extraUSDC": 0.50 }
        ]}
      ],
      "available": true
    }
  ],
  "createdAt": "...",
  "updatedAt": "..."
}
```

**tables**

```json
{
  "_id": "...",
  "businessId": 123,
  "tableCode": "T-12-ABCD",         // embedded in QR
  "label": "Terrace 12",
  "active": true,
  "createdAt": "...",
  "updatedAt": "..."
}
```

**bills**

```json
{
  "_id": "...",
  "billId": "0xBEEF...",           // matches on-chain key
  "businessId": 123,
  "tableCode": "T-12-ABCD",
  "menuVersion": 4,
  "items": [
    { "itemId": "i1", "name": "Espresso", "qty": 2, "unitUSDC": 3.50, "totalUSDC": 7.00 }
  ],
  "taxes": [{ "name": "VAT", "pct": 5, "amountUSDC": 0.35 }],
  "serviceFeeUSDC": 0.00,
  "subTotalUSDC": 7.00,
  "totalDueUSDC": 7.35,
  "itemsHash": "sha256:...",
  "tippingSnapshot": "0xTipsAtCreation",
  "onchain": { "createdTx": "0x...", "createdBlock": 1234567 },
  "status": "open|settled|closed|canceled",
  "createdAt": "...",
  "updatedAt": "..."
}
```

**payments**

```json
{
  "_id": "...",
  "billId": "0xBEEF...",
  "payer": "0xGuest",
  "amountUSDC": 3.68,
  "feeUSDC": 0.07,
  "netUSDC": 3.61,
  "tx": "0x...",
  "createdAt": "..."
}
```

**tips**

```json
{
  "_id": "...",
  "billId": "0xBEEF...",
  "tipper": "0xGuest",
  "amountUSDC": 1.00,
  "to": "0xTipsAtCreation",
  "tx": "0x...",
  "createdAt": "..."
}
```

**admin\_config**

```json
{
  "_id": "chain",
  "usdcAddress": "0xA0b8...",     // per deployment env
  "paymentRouter": "0x...",
  "tipRouter": "0x...",
  "businessRegistry": "0x...",
  "billManager": "0x...",
  "feeBps": 200,
  "treasury": "0x..."
}
```

---

## 8) Backend (Go, Gin)

### 8.1 Environment

```
PORT=8080
MONGODB_URI=...
JWT_SECRET=...

# Chain config
ETH_RPC_URL=...
ETH_WS_URL=...
USDC_ADDRESS=0x...
PAYMENT_ROUTER=0x...
TIP_ROUTER=0x...
BUSINESS_REGISTRY=0x...
BILL_MANAGER=0x...
TREASURY=0x...

# Public
PUBLIC_BASE_URL=https://payverge.app
ASSETS_BUCKET=s3://payverge-assets
```

### 8.2 Services

* **Auth (SIWE)** for business owners & staff roles.
* **Business Service**: register/update business; manage addresses; roles & permissions.
* **Menu Service**: CRUD, versioning, availability toggles, image uploads.
* **Table Service**: generate table codes & QR images (PNG/SVG) for print.
* **Bill Service**:

  * Create bill (manual/API), compute USDC totals, persist JSON → compute `itemsHash`.
  * **On-chain “createBill”** call (sponsored by Payverge relayer) to avoid gas burden on merchants.
  * Listen to **BillCreated** events to reconcile.
* **Payment Listener**: subscribe to `BillPaid` & `BillTipped`; upsert payments/tips; update `totalPaid`, `status`.
* **Receipts & Exports**: PDF generator (server-side), CSV export.
* **Admin**: fee/treasury params (on-chain), moderation, pausing contracts in emergencies.

### 8.3 REST API (selected)

```
POST   /api/v1/auth/siwe
GET    /api/v1/me

POST   /api/v1/businesses
GET    /api/v1/businesses/:id
PATCH  /api/v1/businesses/:id         // update settlement/tipping addresses, tax rules
POST   /api/v1/businesses/:id/staff    // invite staff by wallet
DELETE /api/v1/businesses/:id/staff/:wallet

POST   /api/v1/menus                   // create or version new menu
GET    /api/v1/menus?businessId=...
PATCH  /api/v1/menus/:id

POST   /api/v1/tables                  // generate table codes & QR art
GET    /api/v1/tables?businessId=...

POST   /api/v1/bills                   // {businessId, tableCode, items...}
GET    /api/v1/bills/:billId
POST   /api/v1/bills/:billId/close     // business action (soft lock after settlement)

# Guest endpoints (no auth)
GET    /api/v1/public/menu/:tableCode          // returns menu for a table
GET    /api/v1/public/bill/:billId             // returns bill details & remaining balance
```

> **Payments/tips are on-chain only** (no REST to pay). The frontend prepares wallet calls to `PaymentRouter.payBill` and `TipRouter.tipBill`.

---

## 9) Totals, Taxes & Price Handling

* **Menu prices stored in USDC** (simplest, no oracle risk).

  * Optionally display local fiat equivalents using a server-side FX feed (display-only).
* **Tax model**: Business chooses **inclusive** or **exclusive** VAT/service fees.

  * Backend computes `subTotalUSDC`, `taxUSDC`, `serviceFeeUSDC`, `totalDueUSDC`.
* **Rounding:** round to **USDC 2 decimals** for UI; contract uses **6 decimals**.
* **Split logic:**

  * UI calculates guest **shareUSDC**.
  * Contract enforces `amount <= remainingDue`.
  * If residual < \$0.01 remains, UI encourages final payer to top up or transforms remainder into tip (configurable).

---

## 10) Frontend (Next.js)

### 10.1 Guest Web App (mobile-first)

* **Routes**:

  * `/t/:tableCode` → menu (if no bill) or bill (if bill open).
  * `/b/:billId` → bill directly (QR on printed receipt links here).
* **Views**:

  * **Menu** (category tabs, search, dietary tags, “add to order” disabled by default unless venue enables self-ordering in future).
  * **Bill** (items, taxes, service fee, total, outstanding).
  * **Split** (equal/custom/by-item UI).
  * **Tip** (quick % or custom).
  * **Pay** (USDC approve + pay; success screen with tx hash + receipt link).
* **Design**: sleek, modern, fast; big buttons; works with WalletConnect.

### 10.2 Business Dashboard

* **Home**: live feeds of open tables/bills & recent payments/tips.
* **Menu**: drag-and-drop categories; item editor (price, image, tags, options).
* **Tables**: generate, print QR cards, enable/disable.
* **Bills**: create/manually adjust; link to table; close/void; export.
* **Reports**: daily/weekly/monthly revenue; tips summary; CSV/PDF.
* **Settings**: settlement/tipping addresses, taxes, branding, staff roles.
* **Real-time**: WebSocket/SSE updates on payments and tips.

### 10.3 Admin Console

* System params (feeBps, treasury address → emits on-chain param tx).
* Contract monitors; pause/unpause.
* Moderation: freeze abusive businesses/bills.

---

## 11) QR Codes & Deep Links

* **Menu QR** (per table):

  * `https://payverge.app/t/<tableCode>`
* **Bill QR** (on printed receipt):

  * `https://payverge.app/b/<billId>`
* **QR art generator** in dashboard with logo & table label.
* Table codes are short, URL-safe; bill IDs are 0x hex.

---

## 12) Concurrency & Edge Cases

* Multiple guests can pay concurrently; contract is source of truth (`totalPaid`).
* **Overpay protection**: `payBill` reverts if `amount > remainingDue`.
* **Bill cancellation**: Business can **close** bills; future payments revert.
* **Tip changes**: Changing business **tipping address** affects **new bills only**; existing bills keep `tippingSnapshot`.
* **Refunds (Phase 1.1)**: Business-initiated refunds via `USDC.transfer` off-contract + internal note; Phase 2 could add `refund()` helper.

---

## 13) Security, Privacy, Compliance

* **Contracts**: `ReentrancyGuard`, `Pausable`, strict ERC-20 handling (USDC), no raw ETH paths.
* **Auth**: SIWE; RBAC for staff.
* **PII**: Guests remain pseudonymous (wallet only). Names/emails optional for receipts (if you enable email later).
* **Auditability**: Every bill has `itemsHash` on-chain; every payment/tip has an event & tx hash.
* **Rate limiting** on public endpoints; input validation & sanitization across menu/items.

---

## 14) Observability & Ops

* **Logs**: structured (Zap/Zerolog), request IDs.
* **Metrics**:

  * TPS, payments volume, avg ticket, tips %, fee revenue, failed txs.
  * Listener lag (blocks behind), DB write latency.
* **Alerts**: contract paused, listener stalled, sudden failure spikes.
* **Backups**: Mongo daily; assets (S3) versioned.

---

## 15) Testing

### Contracts (Foundry)

* `test_register_update_business()`
* `test_create_bill_and_snapshot_tip_address()`
* `test_pay_partial_and_fee_math()`
* `test_overpay_reverts()`
* `test_final_payment_closes_settlement_event()`
* `test_tip_routes_to_snapshot_address_no_fee()`
* `test_pause_blocks_payment()`

### Backend

* Unit: bill total calc (tax inclusive/exclusive), items hashing, QR generation.
* Integration: listener → DB reconciliation on `BillCreated`, `BillPaid`, `BillTipped`.
* E2E (Playwright/Cypress + Anvil): end-to-end scan → view bill → split → pay → tip → receipt.

---

## 16) Roadmap (Post-MVP)

* **POS Connectors** (Foodics/Square/Toast): sync open bills via adapter.
* **Pre-auth tabs** (`hold(amount)` then `finalize()` on close).
* **Loyalty** (cashback in USDC, tiered perks).
* **Off-ramp partnerships** (optional fiat settlement).
* **Multi-venue orgs** (HQ + branches; consolidated reporting).
* **Consumer app** (find Payverge venues nearby).

---

## 17) “Build from the Base” Notes

* **Contracts** → `/contracts/src/payverge/` with `DeployPayverge.s.sol`.
* **Backend** → new service modules (`businesses`, `menus`, `tables`, `bills`, `listener`, `reports`).
* **Frontend** → standalone Next.js app: `/frontend/apps/payverge` (guest app + business dashboard).
* **Env**: add chain addresses, treasury, RPC URLs; point to **USDC only**.
* **No reuse** of invoice/widgets code paths—this is a clean project with its own routes & screens.

---

## 18) What “Automatic Accountability” Means Here

* Every **Business** has on-chain record (owner, settlement, tipping).
* Every **Bill** has on-chain record (total due, items hash, tip snapshot).
* Every **Payment** and **Tip** emits events with payer, amounts, and billId.
* Off-chain **receipts** include the **anchor data** (hashes + tx) for audit trails.
* **Exports** (CSV/PDF) are backed by those immutable anchors—**tamper-evident**.

