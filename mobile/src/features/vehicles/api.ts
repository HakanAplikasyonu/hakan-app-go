/**
 * Araçlarım (Fiş yapay zekâ kontrolleri): şirketin araç listesi
 * (`/v1/taxpayer/vehicles`). Akaryakıt fişindeki plaka bu listede yoksa fiş
 * "Plaka mükellefin araçlarında yok" uyarısı alır. Liste şirketindir; ofis
 * de aynı listeyi görür ve düzenler.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import { type Schemas } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';

export type Vehicle = Schemas['Vehicle'];
type VehicleList = Schemas['EnvelopeVehicleList']['data'];

export const VEHICLES_PATH = '/v1/taxpayer/vehicles';

export function useVehicles() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const qc = useQueryClient();
  const key = ['taxpayer-vehicles', uuid];
  const q = useQuery({
    queryKey: key,
    enabled: !!uuid,
    queryFn: () => api.get<VehicleList>(VEHICLES_PATH, { taxpayer: uuid }),
  });
  const add = useMutation({
    mutationFn: (v: { plate: string; description: string }) => api.post<Vehicle>(VEHICLES_PATH, v, { taxpayer: uuid }),
    onSuccess: () => qc.invalidateQueries({ queryKey: key }),
  });
  const remove = useMutation({
    mutationFn: (vehicleUuid: string) => api.delete<unknown>(`${VEHICLES_PATH}/${vehicleUuid}`, { taxpayer: uuid }),
    onSuccess: () => qc.invalidateQueries({ queryKey: key }),
  });
  return { ...q, items: q.data?.items ?? [], add, remove };
}
