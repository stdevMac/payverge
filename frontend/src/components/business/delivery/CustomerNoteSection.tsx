import React from "react";
import { Card, CardHeader, CardBody, Textarea } from "@nextui-org/react";
import { MessageSquareText } from "lucide-react";
import type { DeliverySettingsDto } from "@/api/delivery";

interface CustomerNoteSectionProps {
  settings: Pick<DeliverySettingsDto, "delivery_instructions">;
  onChange: (value: string) => void;
  tString: (key: string) => string;
}

export function CustomerNoteSection({
  settings,
  onChange,
  tString,
}: CustomerNoteSectionProps) {
  return (
    <Card className="rounded-3xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
      <CardHeader className="flex gap-3">
        <MessageSquareText className="h-5 w-5 text-brand" />
        <div>
          <p className="font-semibold text-ink-950">
            {tString("note.cardTitle")}
          </p>
          <p className="text-sm text-ink-600">
            {tString("note.cardSubtitle")}
          </p>
        </div>
      </CardHeader>
      <CardBody>
        <Textarea
          aria-label={tString("note.ariaLabel")}
          value={settings.delivery_instructions || ""}
          onChange={(e) => onChange(e.target.value)}
          minRows={3}
          placeholder={tString("note.placeholder")}
          variant="bordered"
        />
      </CardBody>
    </Card>
  );
}
