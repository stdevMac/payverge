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
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Textarea,
  useDisclosure,
} from "@nextui-org/react";
import {
  adminBusinessAPI,
  AdminBusinessListItem,
  AdminBusinessDetail,
} from "@/api/adminBusiness";
import { formatAdminActionType } from "@/utils/adminActionLabel";
import { formatAdminCurrency } from "@/utils/adminCurrency";
import { apiErrorDetail } from "@/utils/apiError";
import { useUrlBackedDetailId } from "../useUrlBackedDetailId";
import {
  Search,
  Ban,
  Check,
  User,
  History,
  Users,
  BarChart3,
  Building2,
} from "lucide-react";
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

// ─── Overview Tab ────────────────────────────────────────────────────────────
function OverviewTab({ detail }: { detail: AdminBusinessDetail }) {
  const { business, owner } = detail;
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
      <Card shadow="sm">
        <CardHeader className="pb-2">
          <div className="flex items-center gap-2">
            <Building2 size={16} className="text-brand" />
            <h3 className="text-lg font-semibold">Business Information</h3>
          </div>
        </CardHeader>
        <CardBody className="gap-2 text-sm">
          <InfoRow label="Name" value={business.name || "N/A"} />
          <InfoRow label="Address" value={business.address || "N/A"} />
          <InfoRow
            label="Settlement wallet"
            value={business.has_settlement_address ? "Configured" : "Not set"}
          />
          <InfoRow
            label="Tipping wallet"
            value={business.has_tipping_address ? "Configured" : "Not set"}
          />
          <InfoRow
            label="Status"
            value={formatAdminLifecycleStatus(business.status)}
          />
          {business.closed_at ? (
            <InfoRow label="Closed" value={formatAdminDate(business.closed_at)} />
          ) : null}
          <InfoRow label="Created" value={formatAdminDate(business.created_at)} />
        </CardBody>
      </Card>

      <Card shadow="sm">
        <CardHeader className="pb-2">
          <div className="flex items-center gap-2">
            <User size={16} className="text-brand" />
            <h3 className="text-lg font-semibold">Owner Details</h3>
          </div>
        </CardHeader>
        <CardBody className="gap-2 text-sm">
          <InfoRow label="ID" value={owner.id ? String(owner.id) : "N/A"} />
          <InfoRow label="Email domain" value={owner.email_domain || "N/A"} />
          <InfoRow label="Created" value={formatAdminDate(owner.created_at)} />
        </CardBody>
      </Card>
    </div>
  );
}

// ─── Staff Tab ──────────────────────────────────────────────────────────────
function StaffTab({ detail }: { detail: AdminBusinessDetail }) {
  const staff = detail.staff || [];
  if (staff.length === 0) {
    return (
      <p className="text-default-400 text-sm p-4">
        No staff members found for this business.
      </p>
    );
  }

  return (
    <Table
      aria-label="Staff members table"
      removeWrapper
      classNames={{
        th: "bg-default-50 text-default-600 text-xs uppercase tracking-wider",
        td: "py-3",
      }}
    >
      <TableHeader>
        <TableColumn>Name</TableColumn>
        <TableColumn>Email domain</TableColumn>
        <TableColumn>Role</TableColumn>
      </TableHeader>
      <TableBody items={staff}>
        {(member) => (
          <TableRow key={String(member.id)}>
            <TableCell>
              <span className="font-medium text-sm">
                {member.name || "N/A"}
              </span>
            </TableCell>
            <TableCell>
              <span className="text-sm text-default-500">
                {member.email_domain || "N/A"}
              </span>
            </TableCell>
            <TableCell>
              <Chip size="sm" variant="flat" color="default">
                {member.role}
              </Chip>
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  );
}

// ─── Activity Tab ───────────────────────────────────────────────────────────
function ActivityTab({ detail }: { detail: AdminBusinessDetail }) {
  const activity = detail.recent_activity;
  if (!activity) {
    return (
      <p className="text-default-400 text-sm p-4">
        No activity data available.
      </p>
    );
  }

  return (
    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
      <Card shadow="sm">
        <CardBody className="text-center py-6">
          <p className="text-3xl font-bold text-brand">
            {activity.recent_orders}
          </p>
          <p className="text-sm text-default-500 mt-1">Recent Orders</p>
        </CardBody>
      </Card>
      <Card shadow="sm">
        <CardBody className="text-center py-6">
          <p className="text-3xl font-bold text-brand">
            {activity.recent_payments}
          </p>
          <p className="text-sm text-default-500 mt-1">Recent Payments</p>
        </CardBody>
      </Card>
      <Card shadow="sm">
        <CardBody className="text-center py-6">
          <p className="text-3xl font-bold text-brand">
            {formatAdminCurrency(activity.total_revenue)}
          </p>
          <p className="text-sm text-default-500 mt-1">Total Revenue</p>
        </CardBody>
      </Card>
    </div>
  );
}

// ─── Admin Actions Tab ──────────────────────────────────────────────────────
function AdminActionsTab({
  detail,
  businessId,
  onRefresh,
}: {
  detail: AdminBusinessDetail;
  businessId: number;
  onRefresh: () => void;
}) {
  const [suspendReason, setSuspendReason] = useState("");
  const [suspendLoading, setSuspendLoading] = useState(false);
  const suspendModal = useDisclosure();

  const [reactivateReason, setReactivateReason] = useState("");
  const [reactivateLoading, setReactivateLoading] = useState(false);
  const reactivateModal = useDisclosure();

  const [actionError, setActionError] = useState("");

  // The admin lock is is_active, which suspend/reactivate toggle; closure
  // (closed_at) is separate and permanent until the account is reopened.
  const isSuspended = detail.business.is_active === false;
  const isClosed = Boolean(detail.business.closed_at);
  const actions = detail.admin_actions || [];

  const handleSuspend = async () => {
    if (!suspendReason) return;
    setSuspendLoading(true);
    setActionError("");
    try {
      await adminBusinessAPI.suspend(businessId, suspendReason);
      suspendModal.onClose();
      setSuspendReason("");
      onRefresh();
    } catch (err: unknown) {
      setActionError(apiErrorDetail(err) || "Failed to suspend business");
    } finally {
      setSuspendLoading(false);
    }
  };

  const handleReactivate = async () => {
    if (!reactivateReason) return;
    setReactivateLoading(true);
    setActionError("");
    try {
      await adminBusinessAPI.reactivate(businessId, reactivateReason);
      reactivateModal.onClose();
      setReactivateReason("");
      onRefresh();
    } catch (err: unknown) {
      setActionError(apiErrorDetail(err) || "Failed to reactivate business");
    } finally {
      setReactivateLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Action Buttons */}
      <Card shadow="sm">
        <CardHeader className="pb-2">
          <h3 className="text-lg font-semibold">Business Actions</h3>
        </CardHeader>
        <CardBody className="gap-3">
          {isSuspended ? (
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium text-sm">Reactivate Business</p>
                <p className="text-xs text-default-400">
                  Restore access to this business account
                </p>
              </div>
              <Button
                size="sm"
                color="success"
                variant="flat"
                startContent={<Check size={16} />}
                onPress={reactivateModal.onOpen}
              >
                Reactivate
              </Button>
            </div>
          ) : (
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium text-sm">Suspend Business</p>
                <p className="text-xs text-default-400">
                  Temporarily disable access to this business account
                </p>
              </div>
              <Button
                size="sm"
                color="danger"
                variant="flat"
                startContent={<Ban size={16} />}
                onPress={suspendModal.onOpen}
              >
                Suspend
              </Button>
            </div>
          )}
          {isClosed && (
            <p className="text-xs text-warning-600">
              This account was closed. It stays locked until it is reopened;
              reactivating only clears a suspension.
            </p>
          )}
          {actionError && <p className="text-danger text-sm">{actionError}</p>}
        </CardBody>
      </Card>

      {/* Audit Trail */}
      {actions.length > 0 && (
        <div className="space-y-3">
          <h3 className="text-lg font-semibold">Audit Trail</h3>
          {actions.map((action) => (
            <Card key={action.id} shadow="sm">
              <CardBody className="py-3 px-4">
                <div className="flex items-start justify-between gap-4">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <Chip size="sm" variant="flat" color="default">
                        {formatAdminActionType(action.action_type)}
                      </Chip>
                      <span className="text-xs text-default-400">
                        {formatAdminDate(action.created_at)}
                      </span>
                    </div>
                    {action.details &&
                      Object.keys(action.details).length > 0 && (
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
      )}
      {actions.length === 0 && (
        <p className="text-default-400 text-sm">
          No admin actions recorded for this business.
        </p>
      )}

      {/* Suspend Modal */}
      <Modal isOpen={suspendModal.isOpen} onClose={suspendModal.onClose}>
        <ModalContent>
          <ModalHeader>Suspend Business</ModalHeader>
          <ModalBody>
            <p className="text-sm text-danger">
              This will temporarily disable access to this business account.
            </p>
            <Textarea
              label="Reason"
              placeholder="Why is this business being suspended?"
              value={suspendReason}
              onValueChange={setSuspendReason}
            />
          </ModalBody>
          <ModalFooter>
            <Button variant="flat" onPress={suspendModal.onClose}>
              Cancel
            </Button>
            <Button
              color="danger"
              onPress={handleSuspend}
              isLoading={suspendLoading}
              isDisabled={!suspendReason}
            >
              Confirm Suspend
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Reactivate Modal */}
      <Modal
        isOpen={reactivateModal.isOpen}
        onClose={reactivateModal.onClose}
      >
        <ModalContent>
          <ModalHeader>Reactivate Business</ModalHeader>
          <ModalBody>
            <p className="text-sm">
              This will restore access to this business account.
            </p>
            <Textarea
              label="Reason"
              placeholder="Why is this business being reactivated?"
              value={reactivateReason}
              onValueChange={setReactivateReason}
            />
          </ModalBody>
          <ModalFooter>
            <Button variant="flat" onPress={reactivateModal.onClose}>
              Cancel
            </Button>
            <Button
              color="success"
              onPress={handleReactivate}
              isLoading={reactivateLoading}
              isDisabled={!reactivateReason}
            >
              Confirm Reactivate
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}

// ─── Detail Modal ────────────────────────────────────────────────────────────
function BusinessDetailModal({
  isOpen,
  businessId,
  onClose,
  onListRefresh,
}: {
  isOpen: boolean;
  businessId: number | null;
  onClose: () => void;
  onListRefresh?: () => void;
}) {
  const [detail, setDetail] = useState<AdminBusinessDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchDetail = useCallback(async (opts?: { silent?: boolean }) => {
    if (!businessId) return;
    if (!opts?.silent) {
      setLoading(true);
    }
    setError(null);
    try {
      const data = await adminBusinessAPI.getDetail(businessId);
      setDetail(data);
    } catch {
      setError("Failed to load business details.");
    } finally {
      if (!opts?.silent) {
        setLoading(false);
      }
    }
  }, [businessId]);

  const handleActionRefresh = useCallback(async () => {
    await fetchDetail({ silent: true });
    onListRefresh?.();
  }, [fetchDetail, onListRefresh]);

  useEffect(() => {
    if (!isOpen || !businessId) {
      setDetail(null);
      setError(null);
      return;
    }
    fetchDetail();
  }, [isOpen, businessId, fetchDetail]);

  const ownerLabel = detail?.owner?.id
    ? detail.owner.email_domain || `ID ${detail.owner.id}`
    : "No owner on file";

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="5xl"
      scrollBehavior="inside"
      classNames={{ base: "max-h-[90vh]" }}
    >
      <ModalContent>
        {(onModalClose) => (
          <>
            <ModalHeader className="flex flex-col gap-1 pr-10">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h2 className="text-xl font-semibold">
                    {detail?.business.name || "Business details"}
                  </h2>
                  {!loading && detail && (
                    <p className="text-sm text-default-400 font-normal">
                      Owned by {ownerLabel}
                    </p>
                  )}
                </div>
              </div>
            </ModalHeader>
            <ModalBody className="pb-6">
              {loading ? (
                <div className="flex items-center justify-center py-16">
                  <Spinner size="lg" color="primary" />
                </div>
              ) : error || !detail ? (
                <div className="text-center py-10">
                  <p className="text-danger text-sm">
                    {error || "Business not found."}
                  </p>
                  <Button
                    size="sm"
                    variant="flat"
                    className="mt-3"
                    onPress={onModalClose}
                  >
                    Close
                  </Button>
                </div>
              ) : (
                <Tabs
                  aria-label="Business detail tabs"
                  color="primary"
                  variant="underlined"
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
                        <Building2 size={14} />
                        <span>Overview</span>
                      </div>
                    }
                  >
                    <div className="pt-4">
                      <OverviewTab detail={detail} />
                    </div>
                  </Tab>
                  <Tab
                    key="staff"
                    title={
                      <div className="flex items-center gap-2">
                        <Users size={14} />
                        <span>Staff</span>
                      </div>
                    }
                  >
                    <div className="pt-4">
                      <StaffTab detail={detail} />
                    </div>
                  </Tab>
                  <Tab
                    key="activity"
                    title={
                      <div className="flex items-center gap-2">
                        <BarChart3 size={14} />
                        <span>Activity</span>
                      </div>
                    }
                  >
                    <div className="pt-4">
                      <ActivityTab detail={detail} />
                    </div>
                  </Tab>
                  <Tab
                    key="actions"
                    title={
                      <div className="flex items-center gap-2">
                        <History size={14} />
                        <span>Admin Actions</span>
                      </div>
                    }
                  >
                    <div className="pt-4">
                      <AdminActionsTab
                        detail={detail}
                        businessId={businessId!}
                        onRefresh={handleActionRefresh}
                      />
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
export default function AdminBusinessesPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [businesses, setBusinesses] = useState<AdminBusinessListItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<AdminPageSize>(20);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search);
  const [statusFilter, setStatusFilter] = useState("");
  // Default kind=real so the registry never surfaces demo/CI fixtures unless
  // the operator explicitly toggles (Task 12 / SEAM 4).
  const [kindFilter, setKindFilter] = useState<"real" | "demo" | "test" | "all">(
    "real",
  );

  const refreshBusinessList = useCallback(() => {
    adminBusinessAPI
      .getList({
        page,
        limit: pageSize,
        search: debouncedSearch || undefined,
        status: statusFilter || undefined,
        kind: kindFilter,
      })
      .then((data) => {
        setBusinesses(data.businesses || []);
        setTotal(data.total || 0);
      })
      .catch(() => {});
  }, [page, pageSize, debouncedSearch, statusFilter, kindFilter]);

  useEffect(() => {
    setPage(1);
  }, [debouncedSearch, pageSize]);

  useEffect(() => {
    let stale = false;
    setLoading(true);
    adminBusinessAPI
      .getList({
        page,
        limit: pageSize,
        search: debouncedSearch || undefined,
        status: statusFilter || undefined,
        kind: kindFilter,
      })
      .then((data) => {
        if (stale) return;
        setBusinesses(data.businesses || []);
        setTotal(data.total || 0);
      })
      .catch(() => {
        if (stale) return;
        setBusinesses([]);
        setTotal(0);
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [page, pageSize, debouncedSearch, statusFilter, kindFilter]);

  const handleStatusFilter = (value: string) => {
    // Filter options use key "all" for the empty/all sentinel (NextUI rejects "").
    setStatusFilter(value === "all" ? "" : value);
    setPage(1);
  };
  const handleKindFilter = (value: string) => {
    const next =
      value === "demo" || value === "test" || value === "all" ? value : "real";
    setKindFilter(next);
    setPage(1);
  };

  const updateBusinessQuery = useCallback(
    (businessId: number | null) => {
      if (!searchParams) return;
      const params = new URLSearchParams(searchParams.toString());
      if (businessId) {
        params.set("business", String(businessId));
      } else {
        params.delete("business");
      }
      const query = params.toString();
      router.replace(query ? `?${query}` : "/admin/businesses", { scroll: false });
    },
    [router, searchParams],
  );
  const {
    selectedId: selectedBusinessId,
    open: openBusinessDetail,
    close: closeBusinessDetail,
  } = useUrlBackedDetailId(searchParams, "business", updateBusinessQuery);

  const handleRowClick = (businessId: number) => {
    openBusinessDetail(businessId);
  };

  const handleCloseModal = () => {
    closeBusinessDetail();
  };

  const handlePageSizeChange = (size: AdminPageSize) => {
    setPageSize(size);
    setPage(1);
  };

  const filteredBusinesses = businesses;

  return (
    <div className="p-6 max-w-7xl mx-auto space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-foreground">Businesses</h1>
        <p className="text-sm text-default-400 mt-1">
          Manage business accounts, suspensions, and admin actions
        </p>
      </div>

      {/* Filters */}
      <Card shadow="sm">
        <CardBody>
          <div className="flex flex-col sm:flex-row gap-4">
            <Input
              className="flex-1"
              placeholder="Search by name, owner, or email..."
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
            <Select
              className="w-full sm:w-44"
              label="Kind"
              size="sm"
              selectedKeys={[kindFilter]}
              onChange={(e) => handleKindFilter(e.target.value)}
              description="Real signups by default"
            >
              <SelectItem key="real">Real only</SelectItem>
              <SelectItem key="demo">Demo</SelectItem>
              <SelectItem key="test">Test fixtures</SelectItem>
              <SelectItem key="all">All kinds</SelectItem>
            </Select>
          </div>
        </CardBody>
      </Card>

      {/* Table */}
      <Card shadow="sm">
        <CardBody className="p-0">
          <Table
            aria-label="Businesses table"
            removeWrapper
            selectionMode="single"
            selectedKeys={
              selectedBusinessId
                ? new Set([String(selectedBusinessId)])
                : new Set()
            }
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
                noun="businesses"
              />
            }
          >
            <TableHeader>
              <TableColumn>Business Name</TableColumn>
              <TableColumn>Owner</TableColumn>
              <TableColumn>Status</TableColumn>
              <TableColumn>Created</TableColumn>
              <TableColumn>Last Active</TableColumn>
            </TableHeader>
            <TableBody
              isLoading={loading}
              loadingContent={<Spinner color="primary" />}
              emptyContent="No businesses found."
              items={filteredBusinesses}
            >
              {(business) => (
                <TableRow key={String(business.id)}>
                  <TableCell>
                    <span className="font-medium text-sm">
                      {business.name || "N/A"}
                    </span>
                  </TableCell>
                  <TableCell>
                    <div>
                      <span className="text-sm">
                        {business.owner_name || "N/A"}
                      </span>
                      <p className="text-xs text-default-400">
                        {business.owner_email || ""}
                      </p>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="sm"
                      variant="flat"
                      color={
                        ADMIN_LIFECYCLE_STATUS_COLORS[
                          business.status?.toLowerCase() ?? ""
                        ] || "default"
                      }
                    >
                      {formatAdminLifecycleStatus(business.status)}
                    </Chip>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-default-400">
                      {formatAdminDate(business.created_at)}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-default-400">
                      {formatAdminDate(business.last_active_at)}
                    </span>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardBody>
      </Card>

      <BusinessDetailModal
        isOpen={selectedBusinessId != null}
        businessId={selectedBusinessId}
        onClose={handleCloseModal}
        onListRefresh={refreshBusinessList}
      />
    </div>
  );
}
