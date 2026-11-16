import { redirect } from "next/navigation";

type Props = { params: Promise<{ slug: string }> };

/** İnceleme stüdyosu kuyrukla birleşti (#326): eski bağlantılar kuyruğa gider. */
export default async function Route({ params }: Props) {
  const { slug } = await params;
  redirect(`/t/${slug}/review`);
}
