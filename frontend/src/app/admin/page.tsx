"use client";
import { getAdminStats, AdminStats } from "@/api/admin";
import {
  Card,
  CardBody,
  CardHeader,
  Divider,
  Progress,
  Spinner,
  Chip,
} from "@nextui-org/react";
import { useEffect, useState } from "react";
import {
  FaUsers,
  FaEnvelope,
  FaChartLine,
  FaBuilding,
  FaDollarSign,
  FaCreditCard,
  FaUserTie,
  FaReceipt,
  FaCoins,
  FaNetworkWired,
} from "react-icons/fa";
import {
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  AreaChart,
  Area,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
} from "recharts";
import { 
  useCurrentPlatformFeeRate,
  useCurrentRegistrationFee,
  useTotalReferrersOnChain,
  useTotalDistributed,
  useActiveBeneficiaries,
  useProfitSplitBalance,
  usePaymentsContractBalance,
} from "@/contracts/hooks";
import { formatUnits } from "viem";

const COLORS = {
  primary: "#0070F0",
  success: "#17C964",
  warning: "#F5A524",
  danger: "#F31260",
  secondary: "#9750DD",
  default: "#006FEE",
};

const AdminDashboard = () => {
  const [stats, setStats] = useState<AdminStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Smart contract data
  const { data: platformFeeRate } = useCurrentPlatformFeeRate();
  const { data: registrationFee } = useCurrentRegistrationFee();
  const { data: totalReferrersOnChain } = useTotalReferrersOnChain();
  const { data: totalDistributed } = useTotalDistributed();
  const { data: activeBeneficiaries } = useActiveBeneficiaries();
  const { data: profitSplitBalance } = useProfitSplitBalance();
  const { data: paymentsBalance } = usePaymentsContractBalance();

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        setError(null);
        const adminStats = await getAdminStats();
        setStats(adminStats);
      } catch (error) {
        console.error("Error fetching admin stats:", error);
        setError("Failed to load admin statistics");
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  };

  const formatNumber = (value: number) => {
    return new Intl.NumberFormat("en-US").format(value);
  };

  const formatUSDC = (value: bigint | undefined) => {
    if (!value) return "$0.00";
    return formatCurrency(parseFloat(formatUnits(value, 6)));
  };

  if (loading) {
    return (
      <div className="flex flex-col gap-4 p-4 sm:p-6 md:p-8 bg-gradient-to-br from-background to-default-50">
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="text-center">
            <Spinner size="lg" />
            <p className="mt-4 text-gray-600">Loading admin dashboard...</p>
          </div>
        </div>
      </div>
    );
  }

  if (error || !stats) {
    return (
      <div className="flex flex-col gap-4 p-4 sm:p-6 md:p-8 bg-gradient-to-br from-background to-default-50">
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="text-center">
            <p className="text-red-600">{error || "Failed to load data"}</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4 p-4 sm:p-6 md:p-8 bg-gradient-to-br from-background to-default-50">
      <div className="flex flex-col gap-2">
        <h1 className="text-4xl font-bold">Admin Dashboard</h1>
        <p className="text-default-500">
          Welcome back! Here&apos;s what&apos;s happening with your platform today.
        </p>
      </div>

      {/* Overview Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
        {/* Business Metrics */}
        <Card className="bg-primary/10 border-none">
          <CardBody className="flex flex-row items-center gap-4">
            <div className="p-3 rounded-full bg-primary/20">
              <FaBuilding size={24} className="text-primary" />
            </div>
            <div>
              <p className="text-sm text-default-500">Total Businesses</p>
              <div className="flex items-baseline gap-2">
                <p className="text-2xl font-bold">{formatNumber(stats.total_businesses)}</p>
                <Chip size="sm" color="primary" variant="flat">
                  {stats.active_businesses} active
                </Chip>
              </div>
            </div>
          </CardBody>
        </Card>

        {/* User Metrics */}
        <Card className="bg-success/10 border-none">
          <CardBody className="flex flex-row items-center gap-4">
            <div className="p-3 rounded-full bg-success/20">
              <FaUsers size={24} className="text-success" />
            </div>
            <div>
              <p className="text-sm text-default-500">Total Users</p>
              <div className="flex items-baseline gap-2">
                <p className="text-2xl font-bold">{formatNumber(stats.total_users)}</p>
                <Chip size="sm" color="success" variant="flat">
                  {stats.users_by_role.admin || 0} admins
                </Chip>
              </div>
            </div>
          </CardBody>
        </Card>

        {/* Revenue Metrics */}
        <Card className="bg-warning/10 border-none">
          <CardBody className="flex flex-row items-center gap-4">
            <div className="p-3 rounded-full bg-warning/20">
              <FaDollarSign size={24} className="text-warning" />
            </div>
            <div>
              <p className="text-sm text-default-500">Payment Volume</p>
              <div className="flex items-baseline gap-2">
                <p className="text-2xl font-bold">{formatCurrency(stats.total_payment_volume)}</p>
                <Chip size="sm" color="warning" variant="flat">
                  {formatNumber(stats.total_bills)} bills
                </Chip>
              </div>
            </div>
          </CardBody>
        </Card>

        {/* Referral Metrics */}
        <Card className="bg-secondary/10 border-none">
          <CardBody className="flex flex-row items-center gap-4">
            <div className="p-3 rounded-full bg-secondary/20">
              <FaUserTie size={24} className="text-secondary" />
            </div>
            <div>
              <p className="text-sm text-default-500">Referrers</p>
              <div className="flex items-baseline gap-2">
                <p className="text-2xl font-bold">{formatNumber(stats.total_referrers)}</p>
                <Chip size="sm" color="secondary" variant="flat">
                  {formatCurrency(stats.total_commissions_paid)} paid
                </Chip>
              </div>
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Smart Contract Data */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card className="bg-default/10 border-none">
          <CardBody className="flex flex-row items-center gap-4">
            <div className="p-3 rounded-full bg-default/20">
              <FaNetworkWired size={24} className="text-default" />
            </div>
            <div>
              <p className="text-sm text-default-500">Total Distributed</p>
              <div className="flex items-baseline gap-2">
                <p className="text-2xl font-bold">
                  {formatUSDC(totalDistributed)}
                </p>
                <Chip size="sm" color="default" variant="flat">
                  Profit Split
                </Chip>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="bg-warning/10 border-none">
          <CardBody className="flex flex-row items-center gap-4">
            <div className="p-3 rounded-full bg-warning/20">
              <FaCoins size={24} className="text-warning" />
            </div>
            <div>
              <p className="text-sm text-default-500">Registration Fee</p>
              <div className="flex items-baseline gap-2">
                <p className="text-2xl font-bold">{formatUSDC(registrationFee)}</p>
                <Chip size="sm" color="warning" variant="flat">
                  Current
                </Chip>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="bg-success/10 border-none">
          <CardBody className="flex flex-row items-center gap-4">
            <div className="p-3 rounded-full bg-success/20">
              <FaCreditCard size={24} className="text-success" />
            </div>
            <div>
              <p className="text-sm text-default-500">Contract Balance</p>
              <div className="flex items-baseline gap-2">
                <p className="text-2xl font-bold">{formatUSDC(paymentsBalance)}</p>
                <Chip size="sm" color="success" variant="flat">
                  Collected
                </Chip>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="bg-secondary/10 border-none">
          <CardBody className="flex flex-row items-center gap-4">
            <div className="p-3 rounded-full bg-secondary/20">
              <FaReceipt size={24} className="text-secondary" />
            </div>
            <div>
              <p className="text-sm text-default-500">Profit Split Balance</p>
              <div className="flex items-baseline gap-2">
                <p className="text-2xl font-bold">{formatUSDC(profitSplitBalance)}</p>
                <Chip size="sm" color="secondary" variant="flat">
                  Pending
                </Chip>
              </div>
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Growth Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Business Growth */}
        <Card className="border-none">
          <CardHeader className="flex gap-3">
            <FaBuilding size={24} />
            <div className="flex flex-col">
              <p className="text-lg font-semibold">Business Growth</p>
              <p className="text-small text-default-500">
                Monthly business registrations
              </p>
            </div>
          </CardHeader>
          <Divider />
          <CardBody className="h-[300px]">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={stats.business_growth}>
                <defs>
                  <linearGradient id="colorBusinesses" x1="0" y1="0" x2="0" y2="1">
                    <stop
                      offset="5%"
                      stopColor={COLORS.primary}
                      stopOpacity={0.8}
                    />
                    <stop
                      offset="95%"
                      stopColor={COLORS.primary}
                      stopOpacity={0}
                    />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="month" />
                <YAxis />
                <RechartsTooltip />
                <Area
                  type="monotone"
                  dataKey="count"
                  stroke={COLORS.primary}
                  fillOpacity={1}
                  fill="url(#colorBusinesses)"
                />
              </AreaChart>
            </ResponsiveContainer>
          </CardBody>
        </Card>

        {/* Payment Volume */}
        <Card className="border-none">
          <CardHeader className="flex gap-3">
            <FaDollarSign size={24} />
            <div className="flex flex-col">
              <p className="text-lg font-semibold">Payment Volume</p>
              <p className="text-small text-default-500">
                Monthly payment volume trends
              </p>
            </div>
          </CardHeader>
          <Divider />
          <CardBody className="h-[300px]">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={stats.payment_volume_growth}>
                <defs>
                  <linearGradient id="colorVolume" x1="0" y1="0" x2="0" y2="1">
                    <stop
                      offset="5%"
                      stopColor={COLORS.warning}
                      stopOpacity={0.8}
                    />
                    <stop
                      offset="95%"
                      stopColor={COLORS.warning}
                      stopOpacity={0}
                    />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="month" />
                <YAxis tickFormatter={(value) => formatCurrency(value)} />
                <RechartsTooltip 
                  formatter={(value) => [formatCurrency(Number(value)), "Volume"]}
                />
                <Area
                  type="monotone"
                  dataKey="value"
                  stroke={COLORS.warning}
                  fillOpacity={1}
                  fill="url(#colorVolume)"
                />
              </AreaChart>
            </ResponsiveContainer>
          </CardBody>
        </Card>
      </div>

      {/* Payment Breakdown */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <Card className="border-none">
          <CardHeader>
            <div className="flex items-center gap-3">
              <FaCreditCard size={20} />
              <div>
                <p className="text-lg font-semibold">Payment Methods</p>
                <p className="text-small text-default-500">Crypto vs Alternative</p>
              </div>
            </div>
          </CardHeader>
          <Divider />
          <CardBody>
            <div className="space-y-4">
              <div className="flex justify-between items-center">
                <span className="text-sm">Crypto Payments</span>
                <span className="font-semibold">{formatCurrency(stats.total_crypto_payments)}</span>
              </div>
              <Progress 
                value={(stats.total_crypto_payments / stats.total_payment_volume) * 100} 
                color="primary"
                className="w-full"
              />
              
              <div className="flex justify-between items-center">
                <span className="text-sm">Alternative Payments</span>
                <span className="font-semibold">{formatCurrency(stats.total_alternative_payments)}</span>
              </div>
              <Progress 
                value={(stats.total_alternative_payments / stats.total_payment_volume) * 100} 
                color="secondary"
                className="w-full"
              />
            </div>
          </CardBody>
        </Card>

        <Card className="border-none">
          <CardHeader>
            <div className="flex items-center gap-3">
              <FaUserTie size={20} />
              <div>
                <p className="text-lg font-semibold">Referral Tiers</p>
                <p className="text-small text-default-500">Basic vs Premium</p>
              </div>
            </div>
          </CardHeader>
          <Divider />
          <CardBody>
            <div className="space-y-4">
              <div className="flex justify-between items-center">
                <span className="text-sm">Basic Referrers</span>
                <span className="font-semibold">{formatNumber(stats.referrers_by_tier.basic || 0)}</span>
              </div>
              <Progress 
                value={((stats.referrers_by_tier.basic || 0) / stats.total_referrers) * 100} 
                color="success"
                className="w-full"
              />
              
              <div className="flex justify-between items-center">
                <span className="text-sm">Premium Referrers</span>
                <span className="font-semibold">{formatNumber(stats.referrers_by_tier.premium || 0)}</span>
              </div>
              <Progress 
                value={((stats.referrers_by_tier.premium || 0) / stats.total_referrers) * 100} 
                color="warning"
                className="w-full"
              />
            </div>
          </CardBody>
        </Card>

        <Card className="border-none">
          <CardHeader>
            <div className="flex items-center gap-3">
              <FaReceipt size={20} />
              <div>
                <p className="text-lg font-semibold">Bill Status</p>
                <p className="text-small text-default-500">Open vs Closed</p>
              </div>
            </div>
          </CardHeader>
          <Divider />
          <CardBody>
            <div className="space-y-4">
              <div className="flex justify-between items-center">
                <span className="text-sm">Open Bills</span>
                <span className="font-semibold">{formatNumber(stats.bills_by_status.open || 0)}</span>
              </div>
              <Progress 
                value={((stats.bills_by_status.open || 0) / stats.total_bills) * 100} 
                color="warning"
                className="w-full"
              />
              
              <div className="flex justify-between items-center">
                <span className="text-sm">Paid Bills</span>
                <span className="font-semibold">{formatNumber(stats.bills_by_status.paid || 0)}</span>
              </div>
              <Progress 
                value={((stats.bills_by_status.paid || 0) / stats.total_bills) * 100} 
                color="success"
                className="w-full"
              />
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );
};

export default AdminDashboard;
