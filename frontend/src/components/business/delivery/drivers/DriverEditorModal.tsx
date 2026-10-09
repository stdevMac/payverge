import React, { useState, useEffect } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
  Select,
  SelectItem,
} from "@nextui-org/react";
import type { DeliveryDriver, VehicleType } from "@/api/delivery";
// VEHICLE_TYPES is the single source of truth in api/delivery.ts.
// It mirrors backend/internal/database/delivery_models.go — VehicleType enum.
// Re-exported here so callers that previously imported from this file continue to work.
import { VEHICLE_TYPES } from "@/api/delivery";
import { isValidEmail, isValidPhone } from "@/lib/fieldValidation";

export interface DriverFormValues {
  name: string;
  phone: string;
  email: string;
  vehicle_type: VehicleType;
  vehicle_plate: string;
  license_number: string;
}

interface DriverEditorModalProps {
  isOpen: boolean;
  driver: DeliveryDriver | null;
  onClose: () => void;
  onSubmit: (values: DriverFormValues) => Promise<void>;
  tString: (key: string) => string;
}

const EMPTY: DriverFormValues = {
  name: "",
  phone: "",
  email: "",
  vehicle_type: "car",
  vehicle_plate: "",
  license_number: "",
};

function validate(v: DriverFormValues, tString: (k: string) => string) {
  const errors: Partial<DriverFormValues> = {};
  if (!v.name.trim()) errors.name = tString("drivers.validation.nameRequired");
  if (!v.phone.trim()) {
    errors.phone = tString("drivers.validation.phoneRequired");
  } else if (!isValidPhone(v.phone)) {
    // L3-42: "abc" and other non-phone garbage must not save end-to-end.
    errors.phone = tString("drivers.validation.phoneInvalid");
  }
  if (v.email.trim() && !isValidEmail(v.email))
    errors.email = tString("drivers.validation.emailInvalid");
  return errors;
}

export function DriverEditorModal({
  isOpen,
  driver,
  onClose,
  onSubmit,
  tString,
}: DriverEditorModalProps) {
  const [values, setValues] = useState<DriverFormValues>(EMPTY);
  const [errors, setErrors] = useState<Partial<DriverFormValues>>({});
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setValues(
        driver
          ? {
              name: driver.name ?? "",
              phone: driver.phone ?? "",
              email: driver.email ?? "",
              vehicle_type: driver.vehicle_type ?? "car",
              vehicle_plate: driver.vehicle_plate ?? "",
              license_number: driver.license_number ?? "",
            }
          : EMPTY
      );
      setErrors({});
    }
  }, [isOpen, driver]);

  const set = (field: keyof DriverFormValues) => (val: string) =>
    setValues((v) => ({ ...v, [field]: val }));

  const setVehicleType = (val: string) => {
    if (VEHICLE_TYPES.includes(val as VehicleType)) {
      setValues((v) => ({ ...v, vehicle_type: val as VehicleType }));
    }
  };

  const handleSubmit = async () => {
    const errs = validate(values, tString);
    if (Object.keys(errs).length > 0) {
      setErrors(errs);
      return;
    }
    setLoading(true);
    try {
      await onSubmit(values);
    } catch {
      // The parent (DriversManager.handleEditorSubmit) already surfaced the
      // error toast and keeps the modal open. Swallow the rejection so it
      // doesn't escape the press handler as an unhandled promise rejection
      // (console/Sentry noise) — DEL-OP-6.
    } finally {
      setLoading(false);
    }
  };

  const title = driver
    ? tString("drivers.modal.editTitle")
    : tString("drivers.modal.createTitle");

  return (
    <Modal isOpen={isOpen} onClose={onClose} placement="center" size="md">
      <ModalContent>
        <ModalHeader className="text-base font-semibold">{title}</ModalHeader>
        <ModalBody className="gap-3">
          <Input
            label={tString("drivers.fields.name")}
            placeholder={tString("drivers.fields.namePlaceholder")}
            value={values.name}
            onValueChange={set("name")}
            isInvalid={!!errors.name}
            errorMessage={errors.name}
            variant="bordered"
            isDisabled={loading}
            isRequired
          />
          <Input
            label={tString("drivers.fields.phone")}
            placeholder={tString("drivers.fields.phonePlaceholder")}
            type="tel"
            inputMode="tel"
            value={values.phone}
            onValueChange={set("phone")}
            isInvalid={!!errors.phone}
            errorMessage={errors.phone}
            variant="bordered"
            isDisabled={loading}
            isRequired
            data-testid="driver-phone"
          />
          <Input
            label={tString("drivers.fields.email")}
            placeholder={tString("drivers.fields.emailPlaceholder")}
            type="text"
            inputMode="email"
            value={values.email}
            onValueChange={set("email")}
            isInvalid={!!errors.email}
            errorMessage={errors.email}
            variant="bordered"
            isDisabled={loading}
          />
          <Select
            label={tString("drivers.fields.vehicleType")}
            selectedKeys={new Set([values.vehicle_type])}
            onSelectionChange={(keys) => {
              const k = Array.from(keys)[0];
              if (k) setVehicleType(String(k));
            }}
            variant="bordered"
            isDisabled={loading}
          >
            {VEHICLE_TYPES.map((v) => (
              <SelectItem key={v} value={v}>
                {tString(`drivers.vehicleTypes.${v}`)}
              </SelectItem>
            ))}
          </Select>
          <Input
            label={tString("drivers.fields.vehiclePlate")}
            placeholder={tString("drivers.fields.vehiclePlatePlaceholder")}
            value={values.vehicle_plate}
            onValueChange={set("vehicle_plate")}
            variant="bordered"
            isDisabled={loading}
          />
          <Input
            label={tString("drivers.fields.licenseNumber")}
            placeholder={tString("drivers.fields.licenseNumberPlaceholder")}
            value={values.license_number}
            onValueChange={set("license_number")}
            variant="bordered"
            isDisabled={loading}
          />
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={onClose} isDisabled={loading}>
            {tString("drivers.modal.cancel")}
          </Button>
          <Button color="primary" onPress={handleSubmit} isLoading={loading}>
            {tString("drivers.modal.save")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
