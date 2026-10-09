import React from "react";
import { Card, CardHeader, CardBody } from "@nextui-org/react";
import { Network } from "lucide-react";
import type { ExternalPartnerLink } from "@/api/delivery";
import { DELIVERY_PROVIDER_CATALOG } from "@/constants/providerCatalog";
import ExternalPartnerLinksEditor from "../ExternalPartnerLinksEditor";

interface PartnersSectionProps {
  businessId: number;
  links: ExternalPartnerLink[];
  onChange: (links: ExternalPartnerLink[]) => void;
  tString: (key: string) => string;
}

export function PartnersSection({
  businessId,
  links,
  onChange,
  tString,
}: PartnersSectionProps) {
  return (
    <Card className="rounded-3xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
      <CardHeader className="flex gap-3">
        <Network className="h-5 w-5 text-brand" />
        <div>
          <p className="font-semibold text-ink-950">
            {tString("focused.partners.cardTitle")}
          </p>
          <p className="text-sm text-ink-600">
            {tString("focused.partners.cardSubtitle")}
          </p>
        </div>
      </CardHeader>
      <CardBody>
        <ExternalPartnerLinksEditor
          businessId={businessId}
          links={links}
          onChange={onChange}
          // #829: delivery partners are couriers — no OpenTable/Resy here.
          catalog={DELIVERY_PROVIDER_CATALOG}
          labels={{
            title: tString("focused.providers.connectedTitle"),
            description: tString("focused.providers.connectedDescription"),
            addProvider: tString("focused.providers.addProvider"),
            nameLabel: tString("focused.providers.nameLabel"),
            urlLabel: tString("focused.providers.urlLabel"),
            iconSourceLabel: tString("focused.providers.iconSourceLabel"),
            sourcePredefined: tString("focused.providers.sourcePredefined"),
            sourceCustom: tString("focused.providers.sourceCustom"),
            providerLabel: tString("focused.providers.providerLabel"),
            iconUrlLabel: tString("focused.providers.iconUrlLabel"),
            uploadIcon: tString("focused.providers.uploadIcon"),
            removeProvider: tString("focused.providers.removeProvider"),
            maxReached: tString("focused.providers.maxReached"),
          }}
        />
      </CardBody>
    </Card>
  );
}
