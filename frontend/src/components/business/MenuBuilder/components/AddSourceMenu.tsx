import React from "react";
import {
    Button,
    Dropdown,
    DropdownItem,
    DropdownMenu,
    DropdownTrigger,
} from "@nextui-org/react";
import { FileText, Plus, Sparkles, Upload } from "lucide-react";
import { btnPrimaryNextUI, btnSecondaryNextUI } from "@/components/ui/buttonStyles";
import { useInstance } from "@/hooks/useInstance";

interface AddSourceMenuProps {
    tString: (key: string) => string;
    onAddCategory: () => void;
    onAIWizard: () => void;
    onPDFImport: () => void;
    /** L3-21: when false, AI entries are visibly locked and open path-specific upsell. */
}

/**
 * Two side-by-side affordances split by frequency-of-use:
 *  - Primary "+ Add category" — the daily action the operator does most.
 *  - Secondary "Import" dropdown — bulk operations (AI generation,
 *    PDF import) that get used during onboarding or rare overhauls.
 *
 * Replaces the single "+ Add" dropdown which forced operators to click
 * before they knew what they could even do.
 */
export function AddSourceMenu({
    tString,
    onAddCategory,
    onAIWizard,
    onPDFImport,
}: AddSourceMenuProps) {
    // Both import paths (AI wizard, PDF scan) need an LLM provider; without
    // one on this install the Import menu is not offered at all.
    const aiOff = useInstance().isOff("ai");
    return (
        <div className="flex items-center gap-1.5">
            {aiOff ? null : (
                <Dropdown placement="bottom-end" offset={6}>
                    <DropdownTrigger>
                        <Button
                            variant="bordered"
                            radius="full"
                            className={btnSecondaryNextUI}
                            startContent={<Upload className="w-4 h-4" />}
                            aria-label={tString("add.importAriaLabel")}
                            data-testid="menu-import-trigger"
                        >
                            {tString("add.importLabel")}
                        </Button>
                    </DropdownTrigger>
                    <DropdownMenu
                        aria-label={tString("add.importAriaLabel")}
                        variant="flat"
                        className="min-w-[260px]"
                    >
                        <DropdownItem
                            key="ai"
                            onPress={onAIWizard}
                            startContent={
                                <Sparkles className="w-4 h-4 text-brand" />
                            }
                            description={tString("add.withAIDescription")}
                            data-testid="menu-import-wizard"
                        >
                            {tString("add.withAI")}
                        </DropdownItem>
                        <DropdownItem
                            key="pdf"
                            onPress={onPDFImport}
                            startContent={
                                <FileText className="w-4 h-4 text-ink-600" />
                            }
                            description={tString("add.fromPdfDescription")}
                            data-testid="menu-import-pdf"
                        >
                            {tString("add.fromPdf")}
                        </DropdownItem>
                    </DropdownMenu>
                </Dropdown>
            )}

            <Button
                radius="full"
                onPress={onAddCategory}
                aria-label={tString("add.ariaLabel")}
                className={btnPrimaryNextUI}
                startContent={<Plus className="w-4 h-4" />}
            >
                {tString("add.label")}
            </Button>
        </div>
    );
}
