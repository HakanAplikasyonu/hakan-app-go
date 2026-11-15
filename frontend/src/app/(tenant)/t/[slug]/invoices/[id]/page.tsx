import { InvoiceDetailPage } from "@/features/office";

type Props = { params: Promise<{ id: string }> };

export default async function InvoiceRoute({ params }: Props) {
  const { id } = await params;
  return <InvoiceDetailPage id={id} />;
}
