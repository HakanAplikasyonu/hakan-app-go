import { OfficeHome } from "@/features/office";
import { OfficeSetupCard } from "@/features/office-setup";

export default function TenantHomePage() {
  return (
    <div className="space-y-4">
      <OfficeSetupCard />
      <OfficeHome />
    </div>
  );
}
