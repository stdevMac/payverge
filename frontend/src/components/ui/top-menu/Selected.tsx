"use client";
import { useState, useEffect } from "react";
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
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export const Selected = () => {
  const selectMenu = StoreSelected((state) => state.selectMenu);
  const setStage = StoreSelected((state) => state.setStage);
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const result = getTranslation(key, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

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
    All: tString('fleet.stages.all'),
    Posted: tString('fleet.stages.posted'),
    Funded: tString('fleet.stages.funded'),
    OnPurchase: tString('fleet.stages.onPurchase'),
    Active: tString('fleet.stages.active'),
    Closing: tString('fleet.stages.closing'),
    Closed: tString('fleet.stages.closed'),
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
              <span className="font-medium whitespace-nowrap">{tString("fleets.status.")}:</span>
              <span className="text-default-600">{stageLabels[selectMenu]}</span>
            </div>
          </Button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label={tString('fleet.selectStatus')}
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
