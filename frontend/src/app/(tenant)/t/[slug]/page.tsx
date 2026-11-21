import { OfficeHome } from "@/features/office";
import { OfficeSetupCard } from "@/features/office-setup";

export default function TenantHomePage() {
  return <OfficeHome notices={<OfficeSetupCard />} />;
}
