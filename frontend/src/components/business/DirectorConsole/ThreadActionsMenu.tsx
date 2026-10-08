"use client";

import React from "react";
import {
  MoreVertical,
  Pencil,
  Pin,
  Archive,
  Download,
  Trash2,
} from "lucide-react";
import {
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalHeader,
  ModalFooter,
} from "@nextui-org/react";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import ConfirmationModal from "../modals/ConfirmationModal";


export interface ThreadActionsMenuProps {
  title: string;
  pinned: boolean;
  onRename: (next: string) => void;
  onPinToggle: () => void;
  /** Reversible soft-archive — hides the thread but keeps its transcript. */
  onArchive: () => void;
  /**
   * Permanent, irreversible erase. Optional so callers that don't want a
   * destructive delete at all (e.g. archived-thread rows) can omit it; when
   * present it is always gated behind a confirmation before firing.
   */
  onDelete?: () => void;
  onExport: () => void;
}

export default function ThreadActionsMenu(props: ThreadActionsMenuProps) {
  const [renaming, setRenaming] = React.useState(false);
  const [confirmingDelete, setConfirmingDelete] = React.useState(false);
  const [draft, setDraft] = React.useState(props.title);
  const { locale } = useSimpleLocale();
  const t = (key: string): string => {
    const value = getTranslation(`directorConsole.threadActions.${key}`, locale);
    return Array.isArray(value) ? value[0] || key : (value as string);
  };

  // Reset the draft whenever the rename modal opens so it always
  // reflects the latest title (titles change after `onRename` resolves).
  React.useEffect(() => {
    if (renaming) {
      setDraft(props.title);
    }
  }, [renaming, props.title]);

  return (
    <>
      <Dropdown>
        <DropdownTrigger>
          <Button isIconOnly variant="light" aria-label={t("menu")}>
            <MoreVertical className="w-4 h-4" />
          </Button>
        </DropdownTrigger>
        <DropdownMenu aria-label={t("menu")}>
          <DropdownItem
            key="rename"
            startContent={<Pencil className="w-4 h-4" />}
            onPress={() => setRenaming(true)}
          >
            {t("rename")}
          </DropdownItem>
          <DropdownItem
            key="pin"
            startContent={<Pin className="w-4 h-4" />}
            onPress={props.onPinToggle}
          >
            {props.pinned ? t("unpin") : t("pin")}
          </DropdownItem>
          <DropdownItem
            key="export"
            startContent={<Download className="w-4 h-4" />}
            onPress={props.onExport}
          >
            {t("export")}
          </DropdownItem>
          <DropdownItem
            key="archive"
            startContent={<Archive className="w-4 h-4" />}
            onPress={props.onArchive}
          >
            {t("archive")}
          </DropdownItem>
          {props.onDelete ? (
            <DropdownItem
              key="delete"
              startContent={<Trash2 className="w-4 h-4" />}
              className="text-rose-600"
              color="danger"
              onPress={() => setConfirmingDelete(true)}
            >
              {t("delete")}
            </DropdownItem>
          ) : null}
        </DropdownMenu>
      </Dropdown>

      {props.onDelete ? (
        <ConfirmationModal
          isOpen={confirmingDelete}
          onOpenChange={() => setConfirmingDelete(false)}
          title={t("deleteTitle")}
          description={t("deleteDescription")}
          confirmLabel={t("deleteConfirm")}
          cancelLabel={t("cancel")}
          isDanger
          onConfirm={() => props.onDelete?.()}
        />
      ) : null}

      <Modal
        isOpen={renaming}
        onClose={() => setRenaming(false)}
        backdrop="opaque"
      >
        <ModalContent>
          <ModalHeader>{t("renameTitle")}</ModalHeader>
          <ModalBody>
            <Input value={draft} onValueChange={setDraft} autoFocus />
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setRenaming(false)}>
              {t("cancel")}
            </Button>
            <Button
              color="primary"
              onPress={() => {
                props.onRename(draft.trim() || props.title);
                setRenaming(false);
              }}
            >
              {t("save")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </>
  );
}
