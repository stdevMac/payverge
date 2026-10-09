"use client";
import React, { useEffect, useState, useCallback } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import {
  Card,
  CardBody,
  CardHeader,
  Input,
  Select,
  SelectItem,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Chip,
  Spinner,
  Button,
  Tabs,
  Tab,
  Divider,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Textarea,
  useDisclosure,
} from "@nextui-org/react";
import {
  getUserList,
  adminUserAPI,
  AdminUserListItem,
  AdminUserDetail,
  type AdminUserBusinessSummary,
} from "@/api/admin";
import { useUrlBackedDetailId } from "../useUrlBackedDetailId";
import { formatAdminActionType } from "@/utils/adminActionLabel";
import { apiErrorDetail } from "@/utils/apiError";
import { Search, Key, Ban, User, Building2, History, Settings } from "lucide-react";
import {
  InfoRow,
  formatAdminDate,
  formatAdminLifecycleStatus,
  ADMIN_LIFECYCLE_STATUS_COLORS,
  ADMIN_LIFECYCLE_STATUS_FILTER_OPTIONS,
  useDebouncedValue,
} from "@/components/admin/primitives";
import {
  AdminListFooter,
  type AdminPageSize,
} from "@/components/admin/AdminListFooter";

/** The admin lifecycle of a business: closed wins over suspended. */
function lifecycleOf(b: { is_active: boolean; closed_at: string | null }): string {
  if (b.closed_at) return "closed";
  return b.is_active === false ? "suspended" : "active";
}

// ─── Overview Tab ────────────────────────────────────────────────────────────
function OverviewTab({ detail }: { detail: AdminUserDetail }) {
  const { user, business } = detail;
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
      <Card shadow="sm">
        <CardHeader className="pb-2">
          <div className="flex items-center gap-2">
            <User size={16} className="text-brand" />
            <h3 className="text-lg font-semibold">User Information</h3>
          </div>
        </CardHeader>
        <CardBody className="gap-2 text-sm">
          <InfoRow label="Name" value={user.name || "N/A"} />
          <InfoRow label="Email" value={user.email || "N/A"} />
          <InfoRow label="Role" value={user.role} />
          <InfoRow label="Auth Method" value={user.auth_method} />
          <InfoRow label="Email Verified" value={user.email_verified ? "Yes" : "No"} />
          <InfoRow label="Joined" value={formatAdminDate(user.created_at)} />
        </CardBody>
      </Card>

      {business ? (
        <Card shadow="sm">
          <CardHeader className="pb-2">
            <div className="flex items-center gap-2">
              <Building2 size={16} className="text-brand" />
              <h3 className="text-lg font-semibold">Business</h3>
            </div>
          </CardHeader>
          <CardBody className="gap-2 text-sm">
            <InfoRow label="Business" value={business.name} />
            <InfoRow label="Status" value={formatAdminLifecycleStatus(lifecycleOf(business))} />
            {business.closed_at && (
              <InfoRow label="Closed" value={`${formatAdminDate(business.closed_at)} — ${business.closed_reason}`} />
            )}
          </CardBody>
        </Card>
      ) : (
        <Card shadow="sm">
          <CardBody>
            <p className="text-default-400 text-sm">No business associated with this user.</p>
          </CardBody>
        </Card>
      )}
    </div>
  );
}

// ─── Actions Tab ─────────────────────────────────────────────────────────────
function ActionsTab({
  userId,
  businessId,
  onRefresh,
}: {
  userId: number;
  businessId?: number;
  onRefresh: () => void | Promise<void>;
}) {
  const [resetLoading, setResetLoading] = useState(false);
  const resetModal = useDisclosure();

  const [closeReason, setCloseReason] = useState("");
  const [closeLoading, setCloseLoading] = useState(false);
  const closeModal = useDisclosure();

  const [actionError, setActionError] = useState("");

  const handleResetPassword = async () => {
    setResetLoading(true);
    setActionError("");
    try {
      await adminUserAPI.resetPassword(userId);
      resetModal.onClose();
      onRefresh();
    } catch (err: unknown) {
      setActionError(apiErrorDetail(err) || "Failed to reset password");
    } finally {
      setResetLoading(false);
    }
  };

  const handleCloseAccount = async () => {
    if (!closeReason) return;
    setCloseLoading(true);
    setActionError("");
    try {
      await adminUserAPI.close(userId, {
        reason: closeReason,
        business_id: businessId,
      });
      closeModal.onClose();
      setCloseReason("");
      onRefresh();
    } catch (err: unknown) {
      setActionError(apiErrorDetail(err) || "Failed to close account");
    } finally {
      setCloseLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      {actionError && <p className="text-danger text-sm">{actionError}</p>}

      {/* Account Actions */}
      <Card shadow="sm">
        <CardHeader className="pb-2">
          <div className="flex items-center gap-2">
            <Settings size={16} className="text-default-500" />
            <h3 className="text-lg font-semibold">Account Actions</h3>
          </div>
        </CardHeader>
        <CardBody className="gap-3">
          <div className="flex items-center justify-between">
            <div>
              <p className="font-medium text-sm">Reset Password</p>
              <p className="text-xs text-default-400">Send a password reset email to this user</p>
            </div>
            <Button
              size="sm"
              variant="flat"
              startContent={<Key size={16} />}
              onPress={resetModal.onOpen}
            >
              Reset
            </Button>
          </div>
          <Divider />
          <div className="flex items-center justify-between">
            <div>
              <p className="font-medium text-sm">Close Account</p>
              <p className="text-xs text-default-400">Permanently close this user&apos;s account</p>
            </div>
            <Button
              size="sm"
              color="danger"
              variant="flat"
              startContent={<Ban size={16} />}
              onPress={closeModal.onOpen}
            >
              Close
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* Reset Password Modal */}
      <Modal isOpen={resetModal.isOpen} onClose={resetModal.onClose}>
        <ModalContent>
          <ModalHeader>Reset Password</ModalHeader>
          <ModalBody>
            <p className="text-sm">Are you sure you want to send a password reset email to this user?</p>
          </ModalBody>
          <ModalFooter>
            <Button variant="flat" onPress={resetModal.onClose}>Cancel</Button>
            <Button color="primary" className="bg-brand" onPress={handleResetPassword} isLoading={resetLoading}>
              Confirm Reset
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Close Account Modal */}
      <Modal isOpen={closeModal.isOpen} onClose={closeModal.onClose}>
        <ModalContent>
          <ModalHeader>Close Account</ModalHeader>
          <ModalBody>
            <p className="text-sm text-danger">This action is permanent. The user will lose access to their account.</p>
            <Textarea
              label="Reason"
              placeholder="Why is this account being closed?"
              value={closeReason}
              onValueChange={setCloseReason}
            />
          </ModalBody>
          <ModalFooter>
            <Button variant="flat" onPress={closeModal.onClose}>Cancel</Button>
            <Button color="danger" onPress={handleCloseAccount} isLoading={closeLoading} isDisabled={!closeReason}>
              Confirm Close
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}

// ─── History Tab ─────────────────────────────────────────────────────────────
function HistoryTab({ detail }: { detail: AdminUserDetail }) {
  const actions = detail.admin_actions || [];
  if (actions.length === 0) {
    return <p className="text-default-400 text-sm p-4">No admin actions recorded for this user.</p>;
  }

  return (
    <div className="space-y-3">
      {actions.map((action) => (
        <Card key={action.id} shadow="sm">
          <CardBody className="py-3 px-4">
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <Chip size="sm" variant="flat" color="default">{formatAdminActionType(action.action_type)}</Chip>
                  <span className="text-xs text-default-400">{formatAdminDate(action.created_at)}</span>
                </div>
                <p className="text-xs text-default-500 mt-1">by {action.admin_email}</p>
                {action.details && Object.keys(action.details).length > 0 && (
                  <pre className="text-xs text-default-400 mt-2 bg-default-50 p-2 rounded-lg overflow-auto max-h-24">
                    {JSON.stringify(action.details, null, 2)}
                  </pre>
                )}
              </div>
            </div>
          </CardBody>
        </Card>
      ))}
    </div>
  );
}

// ─── Detail Modal ────────────────────────────────────────────────────────────
function UserDetailModal({
  isOpen,
  userId,
  onClose,
  onListRefresh,
}: {
  isOpen: boolean;
  userId: number | null;
  onClose: () => void;
  onListRefresh?: () => void;
}) {
  const [detail, setDetail] = useState<AdminUserDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState("overview");
  const [selectedBusinessId, setSelectedBusinessId] = useState<number | undefined>();
  const fetchDetail = useCallback(async (opts?: { silent?: boolean; businessId?: number }) => {
    if (!userId) return;
    if (!opts?.silent) {
      setLoading(true);
    }
    setError(null);
    try {
      const data = await adminUserAPI.getDetail(userId, {
        business_id: opts?.businessId,
      });
      setDetail(data);
      if (opts?.businessId === undefined && data.business?.id) {
        setSelectedBusinessId(data.business.id);
      }
    } catch {
      setError("Failed to load user details.");
    } finally {
      if (!opts?.silent) {
        setLoading(false);
      }
    }
  }, [userId]);

  const handleActionRefresh = useCallback(async () => {
    await fetchDetail({ silent: true, businessId: selectedBusinessId });
    onListRefresh?.();
    setActiveTab("overview");
  }, [fetchDetail, onListRefresh, selectedBusinessId]);

  useEffect(() => {
    if (!isOpen || !userId) {
      setDetail(null);
      setError(null);
      setSelectedBusinessId(undefined);
      return;
    }
    setSelectedBusinessId(undefined);
    fetchDetail();
  }, [isOpen, userId, fetchDetail]);

  const businesses: AdminUserBusinessSummary[] = detail?.businesses ?? [];
  const activeBusinessId = detail?.business?.id ?? selectedBusinessId;

  const handleBusinessChange = (businessId: number) => {
    setSelectedBusinessId(businessId);
    fetchDetail({ silent: true, businessId });
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="5xl"
      scrollBehavior="inside"
      classNames={{ base: "max-h-[90vh]" }}
    >
      <ModalContent>
        {() => (
          <>
            <ModalHeader className="flex flex-col gap-2 pr-10">
              <div>
                <h2 className="text-xl font-semibold">
                  {detail?.user.name || detail?.user.email || "User details"}
                </h2>
                {detail?.user.email && (
                  <p className="text-sm text-default-400 font-normal">
                    {detail.user.email}
                  </p>
                )}
              </div>
              {!loading && businesses.length > 1 && (
                <Select
                  label="Business context"
                  size="sm"
                  className="max-w-md"
                  selectedKeys={
                    activeBusinessId ? [String(activeBusinessId)] : []
                  }
                  onChange={(e) =>
                    handleBusinessChange(Number(e.target.value))
                  }
                >
                  {businesses.map((b) => (
                    <SelectItem key={String(b.id)} textValue={b.name}>
                      {b.name} ({formatAdminLifecycleStatus(lifecycleOf(b))})
                    </SelectItem>
                  ))}
                </Select>
              )}
            </ModalHeader>
            <ModalBody className="pb-6">
              {loading ? (
                <div className="flex items-center justify-center py-16">
                  <Spinner size="lg" color="primary" />
                </div>
              ) : error || !detail ? (
                <div className="text-center py-10">
                  <p className="text-danger text-sm">
                    {error || "User not found."}
                  </p>
                  <Button size="sm" variant="flat" className="mt-3" onPress={onClose}>
                    Close
                  </Button>
                </div>
              ) : (
                <Tabs
                  aria-label="User detail tabs"
                  color="primary"
                  variant="underlined"
                  selectedKey={activeTab}
                  onSelectionChange={(key) => setActiveTab(String(key))}
                  classNames={{
                    tabList: "gap-6",
                    cursor: "bg-brand",
                    tab: "px-0 h-10",
                    tabContent: "group-data-[selected=true]:text-brand",
                  }}
                >
                  <Tab
                    key="overview"
                    title={
                      <div className="flex items-center gap-2">
                        <User size={14} />
                        <span>Overview</span>
                      </div>
                    }
                  >
                    <div className="pt-4">
                      <OverviewTab detail={detail} />
                    </div>
                  </Tab>
                  <Tab
                    key="actions"
                    title={
                      <div className="flex items-center gap-2">
                        <Settings size={14} />
                        <span>Actions</span>
                      </div>
                    }
                  >
                    <div className="pt-4">
                      <ActionsTab
                        userId={userId!}
                        businessId={activeBusinessId}
                        onRefresh={handleActionRefresh}
                      />
                    </div>
                  </Tab>
                  <Tab
                    key="history"
                    title={
                      <div className="flex items-center gap-2">
                        <History size={14} />
                        <span>History</span>
                      </div>
                    }
                  >
                    <div className="pt-4">
                      <HistoryTab detail={detail} />
                    </div>
                  </Tab>
                </Tabs>
              )}
            </ModalBody>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}

// ─── Main Page ───────────────────────────────────────────────────────────────
export default function AdminUsersPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [users, setUsers] = useState<AdminUserListItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<AdminPageSize>(20);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search);
  const [statusFilter, setStatusFilter] = useState("");

  const refreshUserList = useCallback(() => {
    getUserList({
      page,
      limit: pageSize,
      search: debouncedSearch || undefined,
      status: statusFilter || undefined,
    })
      .then((data) => {
        setUsers(data.users || []);
        setTotal(data.total || 0);
      })
      .catch(() => {});
  }, [page, pageSize, debouncedSearch, statusFilter]);

  useEffect(() => {
    setPage(1);
  }, [debouncedSearch, pageSize]);

  useEffect(() => {
    let stale = false;
    setLoading(true);
    getUserList({
      page,
      limit: pageSize,
      search: debouncedSearch || undefined,
      status: statusFilter || undefined,
    })
      .then((data) => {
        if (stale) return;
        setUsers(data.users || []);
        setTotal(data.total || 0);
      })
      .catch(() => {
        if (stale) return;
        setUsers([]);
        setTotal(0);
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [page, pageSize, debouncedSearch, statusFilter]);

  const handleStatusFilter = (value: string) => {
    // Filter options use key "all" for the empty/all sentinel (NextUI rejects "").
    setStatusFilter(value === "all" ? "" : value);
    setPage(1);
  };

  const updateUserQuery = useCallback(
    (userId: number | null) => {
      if (!searchParams) return;
      const params = new URLSearchParams(searchParams.toString());
      if (userId) {
        params.set("user", String(userId));
      } else {
        params.delete("user");
      }
      const query = params.toString();
      router.replace(query ? `?${query}` : "/admin/users", { scroll: false });
    },
    [router, searchParams],
  );
  const {
    selectedId: selectedUserId,
    open: openUserDetail,
    close: closeUserDetail,
  } = useUrlBackedDetailId(searchParams, "user", updateUserQuery);

  const handleRowClick = (userId: number) => {
    openUserDetail(userId);
  };

  const handleCloseModal = () => {
    closeUserDetail();
  };

  const handlePageSizeChange = (size: AdminPageSize) => {
    setPageSize(size);
    setPage(1);
  };

  return (
    <div className="p-6 max-w-7xl mx-auto space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-foreground">Users</h1>
        <p className="text-sm text-default-400 mt-1">Manage user accounts and admin actions</p>
      </div>

      {/* Filters */}
      <Card shadow="sm">
        <CardBody>
          <div className="flex flex-col sm:flex-row gap-4">
            <Input
              className="flex-1"
              placeholder="Search by name, email, or business..."
              startContent={<Search size={16} className="text-default-300" />}
              value={search}
              onValueChange={setSearch}
              size="sm"
              isClearable
              onClear={() => setSearch("")}
            />
            <Select
              className="w-full sm:w-40"
              label="Status"
              size="sm"
              selectedKeys={[statusFilter || "all"]}
              onChange={(e) => handleStatusFilter(e.target.value)}
            >
              {ADMIN_LIFECYCLE_STATUS_FILTER_OPTIONS.map((opt) => (
                <SelectItem key={opt.key || "all"} textValue={opt.label}>
                  {opt.label}
                </SelectItem>
              ))}
            </Select>
          </div>
        </CardBody>
      </Card>

      {/* Table */}
      <Card shadow="sm">
        <CardBody className="p-0">
          <Table
            aria-label="Users table"
            removeWrapper
            selectionMode="single"
            selectedKeys={selectedUserId ? new Set([String(selectedUserId)]) : new Set()}
            onSelectionChange={(keys) => {
              if (keys === "all") return;
              const selected = Array.from(keys)[0];
              if (selected) handleRowClick(Number(selected));
            }}
            classNames={{
              th: "bg-default-50 text-default-600 text-xs uppercase tracking-wider",
              td: "py-3",
              tr: "cursor-pointer hover:bg-default-50 transition-colors",
            }}
            bottomContent={
              <AdminListFooter
                total={total}
                page={page}
                pageSize={pageSize}
                onPageChange={setPage}
                onPageSizeChange={handlePageSizeChange}
                noun="users"
              />
            }
          >
            <TableHeader>
              <TableColumn>Name</TableColumn>
              <TableColumn>Email</TableColumn>
              <TableColumn>Business</TableColumn>
              <TableColumn>Status</TableColumn>
              <TableColumn>Joined</TableColumn>
            </TableHeader>
            <TableBody
              isLoading={loading}
              loadingContent={<Spinner color="primary" />}
              emptyContent="No users found."
              items={users}
            >
              {(user) => (
                <TableRow key={String(user.id)}>
                  <TableCell>
                    <span className="font-medium text-sm">{user.name || "N/A"}</span>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-default-500">{user.email || "N/A"}</span>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm">{user.business_name || "N/A"}</span>
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="sm"
                      variant="flat"
                      color={ADMIN_LIFECYCLE_STATUS_COLORS[user.status?.toLowerCase()] || "default"}
                    >
                      {formatAdminLifecycleStatus(user.status)}
                    </Chip>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-default-400">{formatAdminDate(user.joined_at)}</span>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardBody>
      </Card>

      {/* Detail Panel */}
      <UserDetailModal
        isOpen={selectedUserId != null}
        userId={selectedUserId}
        onClose={handleCloseModal}
        onListRefresh={refreshUserList}
      />
    </div>
  );
}
