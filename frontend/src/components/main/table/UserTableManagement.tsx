"use client";
import {
  useEffect,
  useState,
  useCallback,
  Dispatch,
  SetStateAction,
  useMemo,
} from "react";
import {
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  User,
  Chip,
  Tooltip,
  Pagination,
  Selection,
  getKeyValue,
  Select,
  SelectItem,
  Input,
} from "@nextui-org/react";
import { EyeIcon } from "../../icons/EyeIcon";
import { SearchIcon } from "@/components/icons/SearchIcon";
import { userColumns } from "@/seed";
import { FullUserInterface } from "@/interface";
import { PrimarySpinner, Title } from "@/components";
import Link from "next/link";
import { getAllUsers } from "@/api/users/getUsers";
import { isAdmin } from "@/utils/auth";

type Props = {
  setToggleSwitch: Dispatch<SetStateAction<boolean>>;
};

const rowsPerPageOptions = [
  { value: "10", label: "10" },
  { value: "20", label: "20" },
  { value: "50", label: "50" },
  { value: "100", label: "100" },
];

export const UserTableManagement = ({ setToggleSwitch }: Props) => {
  const [users, setUsers] = useState<FullUserInterface[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [filterValue, setFilterValue] = useState("");

  const fetchData = async () => {
    try {
      const [usersRes] = await Promise.all([
        getAllUsers(),
      ]);

      if (usersRes && Array.isArray(usersRes.users)) {
        setUsers(usersRes.users);
      } else {
        console.error("Expected an array of users but got:", usersRes);
        setUsers([]);
      }
    } catch (error) {
      console.error("Error fetching data:", error);
      setUsers([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const filteredUsers = useMemo(() => {
    let filtered = [...users];
    if (filterValue) {
      filtered = filtered.filter((user) => {
        const searchStr = filterValue.toLowerCase();
        return (
          user.address.toLowerCase().includes(searchStr) ||
          (user.username?.toLowerCase() || "").includes(searchStr) ||
          (user.email?.toLowerCase() || "").includes(searchStr)
        );
      });
    }
    return filtered;
  }, [users, filterValue]);

  const pages = Math.ceil(filteredUsers.length / rowsPerPage);
  const items = useMemo(() => {
    const start = (page - 1) * rowsPerPage;
    const end = start + rowsPerPage;
    return filteredUsers.slice(start, end);
  }, [page, filteredUsers, rowsPerPage]);

  const renderUsers = useCallback(
    (user: FullUserInterface, columnKey: React.Key) => {
      const cellValue = user[columnKey as keyof FullUserInterface];

      switch (columnKey) {
        case "name":
          return (
            <User
              avatarProps={{
                radius: "lg",
                src: user.address,
              }}
              description={
                <span className="flex items-center gap-2">
                  <span className="text-default-500">{user.email}</span>
                  {isAdmin(user) && (
                    <Chip size="sm" color="secondary" variant="flat">
                      Admin
                    </Chip>
                  )}
                </span>
              }
              name={user.username || user.address}
            >
              {user.address}
            </User>
          );
        case "referees":
          const referralCount = user.referees
            ? Object.keys(user.referees).length
            : 0;
          const fleetCount = user.referees?.length || 0;
          return (
            <div className="flex items-center gap-2">
              <Tooltip content="User Referrals">
                <span className="text-lg">
                  {referralCount}
                  <span className="text-xs text-default-400"> users</span>
                </span>
              </Tooltip>
            </div>
          );
        case "actions":
          return <div className="relative flex items-center gap-2"></div>;
        default:
          return cellValue?.toString();
      }
    },
    []
  );

  const onRowsPerPageChange = useCallback(
    (e: React.ChangeEvent<HTMLSelectElement>) => {
      setRowsPerPage(Number(e.target.value));
      setPage(1);
    },
    []
  );

  const onSearchChange = useCallback((value?: string) => {
    if (value) {
      setFilterValue(value);
      setPage(1);
    } else {
      setFilterValue("");
    }
  }, []);

  if (loading) return <PrimarySpinner />;

  return (
    <div className="w-full flex flex-col gap-4">
      <div className="flex flex-col gap-4">
        <Title title="Users Management" />

        <div className="flex justify-between items-center gap-3">
          <Input
            isClearable
            className="w-full sm:max-w-[44%]"
            placeholder="Search by name, email, or address..."
            startContent={<SearchIcon className="text-default-300" />}
            value={filterValue}
            onClear={() => onSearchChange("")}
            onValueChange={onSearchChange}
          />
          <div className="flex gap-3">
            <Select
              label="Rows per page"
              className="w-28"
              size="sm"
              value={rowsPerPage.toString()}
              onChange={onRowsPerPageChange}
            >
              {rowsPerPageOptions.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </Select>
          </div>
        </div>
      </div>

      <Table
        aria-label="Users management table"
        isHeaderSticky
        classNames={{
          wrapper: "max-h-[calc(100vh-300px)]",
          table: "min-h-[400px]",
          th: "bg-default-100",
          td: "py-3",
          base: "overflow-x-auto overflow-y-hidden",
        }}
        bottomContent={
          pages > 0 ? (
            <div className="flex w-full justify-center">
              <Pagination
                isCompact
                showControls
                showShadow
                color="primary"
                page={page}
                total={pages}
                onChange={setPage}
              />
            </div>
          ) : null
        }
      >
        <TableHeader columns={userColumns}>
          {(column) => (
            <TableColumn
              key={column.uid}
              align={column.uid === "actions" ? "center" : "start"}
              className="bg-default-100/50 text-sm uppercase"
            >
              {column.name}
            </TableColumn>
          )}
        </TableHeader>
        <TableBody
          items={items}
          emptyContent={"No users found"}
          loadingContent={<PrimarySpinner />}
          className="h-full"
        >
          {(item) => (
            <TableRow key={item.address} className="h-[60px]">
              {(columnKey) => (
                <TableCell>{renderUsers(item, columnKey)}</TableCell>
              )}
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
};
