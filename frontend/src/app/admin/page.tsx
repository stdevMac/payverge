"use client";
import { getAllUsers } from "@/api/users/getUsers";
import {
  Card,
  CardBody,
  CardHeader,
  Divider,
  Progress,
} from "@nextui-org/react";
import { useEffect, useState } from "react";
import { FullUserInterface } from "@/interface";
import { SubscriberInterface } from "@/interface/subscribers/subscriber-interface";
import { getAllSubscribers } from "@/api/subscribers/getSubscribers";
import {
  FaCar,
  FaUsers,
  FaMoneyBillWave,
  FaEnvelope,
  FaChartLine,
} from "react-icons/fa";
import {
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  AreaChart,
  Area,
} from "recharts";

const COLORS = {
  primary: "#0070F0",
  success: "#17C964",
  warning: "#F5A524",
  danger: "#F31260",
  secondary: "#9750DD",
  default: "#006FEE",
};

const AdminDashboard = () => {
  const [users, setUsers] = useState<FullUserInterface[]>([]);
  const [subscribers, setSubscribers] = useState<SubscriberInterface[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [usersData, subscribersData] = await Promise.all([
          getAllUsers(),
          getAllSubscribers(),
        ]);
        setUsers(usersData.users);
        setSubscribers(subscribersData);
      } catch (error) {
        console.error("Error fetching data:", error);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  const calculateStatistics = () => {
    const totalUsers = users.length;

    const totalSubscribers = subscribers.length;

    // Calculate monthly stats for the last 6 months
    const currentDate = new Date();
    const monthlyStats = Array.from({ length: 6 }, (_, index) => {
      // Calculate date for each month, starting from 5 months ago
      const monthDate = new Date(
        currentDate.getFullYear(),
        currentDate.getMonth() - 5 + index,
        1,
      );

      const startOfMonth = new Date(
        monthDate.getFullYear(),
        monthDate.getMonth(),
        1,
      );
      const endOfMonth = new Date(
        monthDate.getFullYear(),
        monthDate.getMonth() + 1,
        0,
      );

      // Format the month name
      const monthName = new Intl.DateTimeFormat("en-US", {
        month: "short",
        year: "numeric",
      }).format(monthDate);

      // Count new users in this month
      const newUsers = users.filter((user) => {
        if (!user.joined_at) return false;
        const joinDate = new Date(user.joined_at);
        return joinDate >= startOfMonth && joinDate <= endOfMonth;
      }).length;

      // Count new subscribers in this month
      const newSubscribers = subscribers.filter((sub) => {
        if (!sub.subscription_date) return false;
        const subDate = new Date(sub.subscription_date);
        return subDate >= startOfMonth && subDate <= endOfMonth;
      }).length;

      return {
        name: monthName,
        users: newUsers,
        subscribers: newSubscribers,
      };
    });

    return {
      totalUsers,
      totalSubscribers,
      monthlyStats,
    };
  };

  const stats = calculateStatistics();

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(value);
  };

  if (loading) {
    return <div>Loading...</div>;
  }

  return (
    <div className="flex flex-col gap-4 p-4 sm:p-6 md:p-8 bg-gradient-to-br from-background to-default-50">
      <div className="flex flex-col gap-2">
        <h1 className="text-4xl font-bold">Admin Dashboard</h1>
        <p className="text-default-500">
          Welcome back! Here&#39;s what&#39;s happening with your fleets today.
        </p>
      </div>

      {/* Overview Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
        <Card className="bg-success/10 border-none">
          <CardBody className="flex flex-row items-center gap-4">
            <div className="p-3 rounded-full bg-success/20">
              <FaUsers size={24} className="text-success" />
            </div>
            <div>
              <p className="text-sm text-default-500">Total Users</p>
              <div className="flex items-baseline gap-2">
                <p className="text-2xl font-bold">{stats.totalUsers}</p>
                <p className="text-xs text-default-500">users</p>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="bg-secondary/10 border-none">
          <CardBody className="flex flex-row items-center gap-4">
            <div className="p-3 rounded-full bg-secondary/20">
              <FaEnvelope size={24} className="text-secondary" />
            </div>
            <div>
              <p className="text-sm text-default-500">Subscribers</p>
              <div className="flex items-baseline gap-2">
                <p className="text-2xl font-bold">{stats.totalSubscribers}</p>
                <p className="text-xs text-default-500">subscribers</p>
              </div>
            </div>
          </CardBody>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4">
        {/* Growth Trends */}
        <Card className="border-none">
          <CardHeader className="flex gap-3">
            <FaChartLine size={24} />
            <div className="flex flex-col">
              <p className="text-lg font-semibold">Growth Trends</p>
              <p className="text-small text-default-500">
                Monthly growth of users, subscribers
              </p>
            </div>
          </CardHeader>
          <Divider />
          <CardBody className="h-[300px] sm:h-[350px] md:h-[400px]">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={stats.monthlyStats}>
                <defs>
                  <linearGradient id="colorUsers" x1="0" y1="0" x2="0" y2="1">
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
                  <linearGradient
                    id="colorSubscribers"
                    x1="0"
                    y1="0"
                    x2="0"
                    y2="1"
                  >
                    <stop
                      offset="5%"
                      stopColor={COLORS.success}
                      stopOpacity={0.8}
                    />
                    <stop
                      offset="95%"
                      stopColor={COLORS.success}
                      stopOpacity={0}
                    />
                  </linearGradient>
                  <linearGradient id="colorFleets" x1="0" y1="0" x2="0" y2="1">
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
                <XAxis dataKey="name" tickFormatter={(value) => value} />
                <YAxis />
                <RechartsTooltip
                  content={({ active, payload, label }) => {
                    if (active && payload && payload.length) {
                      return (
                        <div className="bg-background/90 p-3 rounded-lg border border-default-200 shadow-lg">
                          <p className="font-semibold">{label}</p>
                          {payload.map((entry) => (
                            <p
                              key={entry.name}
                              style={{
                                color: entry.color,
                              }}
                            >
                              {entry.name}: {entry.value}
                            </p>
                          ))}
                        </div>
                      );
                    }
                    return null;
                  }}
                />
                <Area
                  type="monotone"
                  dataKey="users"
                  stroke={COLORS.primary}
                  fillOpacity={1}
                  fill="url(#colorUsers)"
                  name="New Users"
                />
                <Area
                  type="monotone"
                  dataKey="subscribers"
                  stroke={COLORS.success}
                  fillOpacity={1}
                  fill="url(#colorSubscribers)"
                  name="New Subscribers"
                />
              </AreaChart>
            </ResponsiveContainer>
          </CardBody>
        </Card>
      </div>

      {/* User Statistics */}
      <Card className="border-none">
        <CardHeader className="flex gap-3">
          <FaUsers size={24} />
          <div className="flex flex-col">
            <p className="text-lg font-semibold">User Statistics</p>
            <p className="text-small text-default-500">
              Detailed user metrics and verification status
            </p>
          </div>
        </CardHeader>
        <Divider />
        <CardBody>
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4 sm:gap-6">
            <div className="space-y-2">
              <p className="text-default-500">Total Users</p>
              <p className="text-xl sm:text-2xl font-bold">{stats.totalUsers}</p>
              <Progress
                value={100}
                color="primary"
                className="h-2"
                aria-label="Status progress"
              />
            </div>
            <div className="space-y-2">
              <p className="text-default-500">Newsletter Subscribers</p>
              <p className="text-xl sm:text-2xl font-bold">{stats.totalSubscribers}</p>
              <Progress
                value={(stats.totalSubscribers / stats.totalUsers) * 100}
                color="secondary"
                className="h-2"
              />
            </div>
          </div>
        </CardBody>
      </Card>
    </div>
  );
};

export default AdminDashboard;
