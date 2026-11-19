import { Suspense } from "react";

import { ParasutCallbackPage } from "@/features/office";

/** Paraşüt OAuth dönüşü (`PARASUT_REDIRECT_URL`, #518 A3). */
export default function Page() {
  return (
    <Suspense>
      <ParasutCallbackPage />
    </Suspense>
  );
}
