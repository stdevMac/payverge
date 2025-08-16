import React from "react";

export const useAccordionTitleWithIcon = (
  icon: React.ReactNode,
  title: string,
) => {
  const AccordionTitleWithIcon = () => (
    <div className="flex items-center">
      {icon}
      <span className="ml-2">{title}</span>
    </div>
  );

  return AccordionTitleWithIcon;
};
