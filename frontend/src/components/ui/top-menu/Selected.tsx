"use client";
import { useState } from "react";
import {
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
  Button,
} from "@nextui-org/react";
import { Key } from "@/interface";
import { StoreSelected, FleetStage } from "@/store/ui/StoreSelected";
import { IoMenuOutline } from "react-icons/io5";
import { useTranslation } from "@/i18n/useTranslation";

export const Selected = () => {
  const selectMenu = StoreSelected((state) => state.selectMenu);
  const setStage = StoreSelected((state) => state.setStage);
  const { t } = useTranslation();

  const fleetStages: FleetStage[] = [
    "All",
    "Posted",
    "Funded",
    "OnPurchase",
    "Active",
    "Closing",
    "Closed",
  ];

  const stageLabels: Record<FleetStage, string> = {
    All: t('fleet.stages.all'),
    Posted: t('fleet.stages.posted'),
    Funded: t('fleet.stages.funded'),
    OnPurchase: t('fleet.stages.onPurchase'),
    Active: t('fleet.stages.active'),
    Closing: t('fleet.stages.closing'),
    Closed: t('fleet.stages.closed'),
  };

  const handleSelectionChange = (key: Key) => {
    setStage(key as FleetStage);
  };

  return (
    <div className="w-full">
      <Dropdown className="w-full">
        <DropdownTrigger className="w-full">
          <Button 
            color="primary" 
            variant="bordered"
            className="w-full h-unit-12 min-h-unit-12 justify-between px-4 text-sm sm:text-base group hover:border-primary transition-colors duration-200"
            endContent={
              <div className="flex items-center gap-2 text-default-400 group-hover:text-primary transition-colors duration-200">
                <div className="flex flex-col gap-1 relative w-3 animate-subtle-bounce group-hover:-translate-y-1 transition-transform duration-200">
                  <div className="w-3 h-[1.5px] bg-current rounded-full"></div>
                  <div className="w-3 h-[1.5px] bg-current rounded-full"></div>
                  <div className="w-3 h-[1.5px] bg-current rounded-full"></div>
                </div>
                <svg
                  className="w-3 h-3 animate-subtle-bounce-reverse group-hover:translate-y-1 transition-transform duration-200"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="3"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="M6 9l6 6 6-6" />
                </svg>
              </div>
            }
          >
            <div className="flex items-center gap-2">
              <span className="font-medium whitespace-nowrap">{t('fleet.status')}:</span>
              <span className="text-default-600">{stageLabels[selectMenu]}</span>
            </div>
          </Button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label={t('fleet.selectStatus')}
          selectionMode="single"
          selectedKeys={[selectMenu]}
          onSelectionChange={(keys) =>
            handleSelectionChange(Array.from(keys)[0])
          }
          className="min-w-[200px]"
        >
          {fleetStages.map((stage) => (
            <DropdownItem key={stage} className="capitalize">
              {stageLabels[stage]}
            </DropdownItem>
          ))}
        </DropdownMenu>
      </Dropdown>
    </div>
  );
};
