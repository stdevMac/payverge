"use client";

import dynamic from "next/dynamic";
import { useState } from "react";
import { TopMenuInstantChrome } from "./TopMenuShell";

const TopMenuClient = dynamic(
  () => import("./TopMenu").then((m) => ({ default: m.TopMenu })),
  { ssr: false },
);

export default function TopMenuLazy() {
  const [menuReady, setMenuReady] = useState(false);

  return (
    <>
      {!menuReady && <TopMenuInstantChrome />}
      <TopMenuClient onReady={() => setMenuReady(true)} />
    </>
  );
}
