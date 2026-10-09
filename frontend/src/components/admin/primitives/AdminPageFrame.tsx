"use client";

import type { ReactNode } from "react";

export function AdminPageFrame({
  description,
  actions,
  children,
}: {
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="max-w-7xl mx-auto space-y-6">
      {(description || actions) && (
        <div className="flex items-start justify-between gap-4">
          {description ? (
            <p className="text-default-500">{description}</p>
          ) : (
            <span />
          )}
          {actions}
        </div>
      )}
      {children}
    </div>
  );
}
