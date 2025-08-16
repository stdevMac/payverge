"use client";
import { useState } from "react";
import { UserTableManagement } from "@/components/main/table/UserTableManagement";

type Props = {};

export const UsersManagement = (props: Props) => {
  const [toggleSwitch, setToggleSwitch] = useState(true);
  return (
    <>
      <UserTableManagement setToggleSwitch={setToggleSwitch} />
    </>
  );
};
