"use client";

import { Input, Pagination } from "@nextui-org/react";
import { useState } from "react";

const PAGE_SIZE_OPTIONS = [10, 20, 50] as const;

export type AdminPageSize = (typeof PAGE_SIZE_OPTIONS)[number];

interface AdminListFooterProps {
  total: number;
  page: number;
  pageSize: AdminPageSize;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: AdminPageSize) => void;
  noun?: string;
}

export function AdminListFooter({
  total,
  page,
  pageSize,
  onPageChange,
  onPageSizeChange,
  noun = "items",
}: AdminListFooterProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const rangeStart = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const rangeEnd = Math.min(page * pageSize, total);
  const [jumpPage, setJumpPage] = useState("");

  const handleJump = () => {
    const parsed = Number.parseInt(jumpPage, 10);
    if (!Number.isFinite(parsed)) return;
    const next = Math.min(Math.max(parsed, 1), totalPages);
    onPageChange(next);
    setJumpPage("");
  };

  return (
    <div className="flex flex-col gap-3 px-4 py-4 border-t border-default-100">
      <div className="flex flex-col sm:flex-row items-center justify-between gap-3">
        <p className="text-sm text-default-400">
          {total === 0
            ? `No ${noun} to show`
            : `Showing ${rangeStart}–${rangeEnd} of ${total} ${noun}`}
        </p>
        <div className="flex items-center gap-2 text-sm">
          <label htmlFor="admin-page-size" className="text-default-400">
            Per page
          </label>
          <select
            id="admin-page-size"
            className="rounded-md border border-default-200 bg-transparent px-2 py-1 text-sm"
            value={pageSize}
            onChange={(e) =>
              onPageSizeChange(Number(e.target.value) as AdminPageSize)
            }
          >
            {PAGE_SIZE_OPTIONS.map((size) => (
              <option key={size} value={size}>
                {size}
              </option>
            ))}
          </select>
        </div>
      </div>
      {totalPages > 1 && (
        <div className="flex flex-col sm:flex-row items-center justify-between gap-3">
          <Pagination
            total={totalPages}
            page={page}
            onChange={onPageChange}
            showControls
            classNames={{
              cursor: "bg-brand text-white",
            }}
          />
          <div className="flex items-center gap-2">
            <Input
              size="sm"
              className="w-20"
              aria-label="Go to page"
              placeholder="#"
              value={jumpPage}
              onValueChange={setJumpPage}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleJump();
              }}
            />
            <button
              type="button"
              className="text-sm text-brand hover:underline"
              onClick={handleJump}
            >
              Go
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
