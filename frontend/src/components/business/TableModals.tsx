"use client";

import React from "react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import {
  Button,
  Input,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Slider,
} from "@nextui-org/react";
// L3-25: single source for the name cap — the same constant backs the BE
// binding tag, so the input must not carry its own literal.
import { TABLE_NAME_MAX_LENGTH } from "./tableNameLimits";

// Create-only modal. Table editing (rename / capacity / activate / delete)
// lives in the drawer's Settings + Detail tabs — the old Edit modal here was
// retired and rendered permanently closed, so it was removed along with its
// no-op props.
interface TableModalsProps {
  isCreateOpen: boolean;
  onCreateOpenChange: () => void;
  tableName: string;
  setTableName: (name: string) => void;
  tableCapacity: number;
  setTableCapacity: (capacity: number) => void;
  handleCreateTable: () => void;
  /** True while the create request is in flight (guards double-submit). */
  isCreating?: boolean;
}

export default function TableModals({
  isCreateOpen,
  onCreateOpenChange,
  tableName,
  setTableName,
  tableCapacity,
  setTableCapacity,
  handleCreateTable,
  isCreating = false,
}: TableModalsProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = React.useState(locale);

  // Update translations when locale changes
  React.useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.tableManager.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  return (
    <>
      {/* Create Table Modal */}
      <Modal isOpen={isCreateOpen} onOpenChange={onCreateOpenChange}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>{tString("modals.create.title")}</ModalHeader>
              <ModalBody>
                <Input
                  label={tString("modals.create.tableName")}
                  placeholder={tString("modals.create.placeholder")}
                  value={tableName}
                  onValueChange={setTableName}
                  isRequired
                  maxLength={TABLE_NAME_MAX_LENGTH}
                  description={`${tableName.length}/${TABLE_NAME_MAX_LENGTH}`}
                  data-testid="create-table-name"
                />
                <div className="space-y-2">
                  <div className="flex justify-between items-center">
                    <label className="text-sm font-medium">
                      {tString("modals.create.capacity")}
                    </label>
                    <span className="text-sm font-semibold text-primary">
                      {tableCapacity}{" "}
                      {tableCapacity === 1
                        ? tString("modals.create.seat")
                        : tString("modals.create.seats")}
                    </span>
                  </div>
                  <Slider
                    aria-label={tString("modals.create.capacity")}
                    size="sm"
                    step={1}
                    minValue={1}
                    maxValue={20}
                    value={tableCapacity}
                    onChange={(value) => setTableCapacity(value as number)}
                    className="max-w-full"
                    marks={[
                      { value: 1, label: "1" },
                      { value: 5, label: "5" },
                      { value: 10, label: "10" },
                      { value: 15, label: "15" },
                      { value: 20, label: "20" },
                    ]}
                  />
                </div>
              </ModalBody>
              <ModalFooter>
                <Button
                  color="danger"
                  variant="light"
                  onPress={onClose}
                  isDisabled={isCreating}
                >
                  {tString("modals.create.cancel")}
                </Button>
                <Button
                  color="primary"
                  onPress={handleCreateTable}
                  isLoading={isCreating}
                  isDisabled={isCreating || !tableName.trim()}
                >
                  {tString("modals.create.create")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}
