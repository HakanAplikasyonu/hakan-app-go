import { Suspense } from "react";

import { OrganizationPicker } from "@/features/tenant";

export default function OrganizationPickerPage() {
  // useSearchParams (`?to=`, #285) needs a Suspense boundary.
  return (
    <Suspense>
      <OrganizationPicker />
    </Suspense>
  );
}
