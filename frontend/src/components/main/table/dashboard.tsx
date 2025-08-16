"using client";
import { useEffect, useState, useCallback } from "react";
import {
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  User,
  Chip,
  Card,
  CardHeader,
  CardBody,
  Button,
} from "@nextui-org/react";
import { topUserColumns } from "@/seed";
import { TopUserInterface } from "@/interface";
import { PrimarySpinner, Title } from "@/components";
import { getTopUsers } from "@/api/users/topUsers";

export const DashboardTable = () => {
  const [users, setUsers] = useState<TopUserInterface[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchTopUsers() {
      try {
        const res = await getTopUsers();
        if (res && Array.isArray(res)) {
          setUsers(res);
        } else {
          console.error("Expected an array but got:", res);
          setUsers([]);
        }
      } catch (error) {
        console.error("Error fetching users:", error);
        setUsers([]);
      } finally {
        setLoading(false);
      }
    }
    fetchTopUsers();
  }, []);

  const getSticker = (index: number): string => {
    switch (index) {
      case 0:
        return "🥇";
      case 1:
        return "🥈";
      case 2:
        return "🥉";
      default:
        return `${index + 1}`;
    }
  };

  const renderUsers = useCallback((user: TopUserInterface, index: number) => {
    return (
      <div
        key={user.address}
        className="border border-gray-200 rounded-lg p-3 sm:p-4 mb-3 sm:mb-4 shadow-sm hover:shadow-md transition-shadow bg-content1"
      >
        <div className="flex items-center mb-2 gap-2">
          <span className="text-lg sm:text-xl font-bold mr-2">{getSticker(index)}</span>
          <User
            avatarProps={{
              radius: "lg",
              src: "/images/user.jpg",
            }}
            name={user.address}
          />
        </div>
        <div className="mt-2 space-y-1.5">
          <p className="text-sm sm:text-base text-default-600">
            <strong className="text-default-700">Address:</strong>{' '}
            <span className="break-all">{user.address}</span>
          </p>
          <p className="text-sm sm:text-base text-default-600">
            <strong className="text-default-700">Points:</strong>{' '}
            <span>{user.points}</span>
          </p>
          <p className="text-sm sm:text-base text-default-600">
            <strong className="text-default-700">Referrals:</strong>{' '}
            <span>{user.referrals}</span>
          </p>
        </div>
      </div>
    );
  }, []);

  if (loading) {
    return <PrimarySpinner />;
  }

  if (users.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center my-10 py-10">
        <Title
          title="There are no Top users available."
          className="text-4xl mt-5"
        />
      </div>
    );
  } else {
    return (
      <Card>
        <CardHeader className="flex gap-3 flex-col sm:flex-row">
          <div className="flex flex-col w-full">
            <h2 className="text-xl sm:text-2xl font-bold text-foreground-900 pb-2 border-b-2 border-primary">
              Top Users in Last 3 Months
            </h2>
          </div>
        </CardHeader>
        <CardBody>
          {/* Render cards on small screens and table on larger screens */}
          <div className="block md:hidden space-y-2 px-1">
            {/* This div is visible on small screens (md:hidden) */}
            {users.map((user, index) => renderUsers(user, index))}
          </div>
          <div className="hidden md:block">
            {/* This div is visible on medium and larger screens (hidden md:block) */}
            <div className="overflow-x-auto -mx-6">
              <div className="min-w-[600px] px-6">
                <Table aria-label="User management table with custom cells">
              <TableHeader columns={topUserColumns}>
                {(column) => (
                  <TableColumn key={column.uid} align="center">
                    {column.name}
                  </TableColumn>
                )}
              </TableHeader>
              <TableBody items={users}>
                {(item) => (
                  <TableRow key={item.address}>
                    {(columnKey) => (
                      <TableCell>
                        {renderTableCell(item, columnKey, users.indexOf(item))}
                      </TableCell>
                    )}
                  </TableRow>
                )}
              </TableBody>
                </Table>
              </div>
            </div>
          </div>
        </CardBody>
      </Card>
    );
  }
};

// Helper function to render table cells
const renderTableCell = (
  user: TopUserInterface,
  columnKey: React.Key,
  index: number,
) => {
  const cellValue = user[columnKey as keyof TopUserInterface];
  switch (columnKey) {
    case "index":
      return (
        <div className="flex items-center justify-center">
          <span className="text-lg font-bold">{index + 1}</span>
        </div>
      );
    case "address":
      return (
        <User
          avatarProps={{
            radius: "lg",
            src: "/images/user.jpg",
          }}
          name={user.address}
        />
      );
    case "points":
      return (
        <Chip className="capitalize" color="success" size="sm" variant="flat">
          {user.points}
        </Chip>
      );
    case "referrals":
      return (
        <Chip className="capitalize" color="warning" size="sm" variant="flat">
          {user.referrals}
        </Chip>
      );
    default:
      return cellValue;
  }
};
