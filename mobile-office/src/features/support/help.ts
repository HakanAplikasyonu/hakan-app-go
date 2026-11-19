/**
 * Yardım bağlantısı (#522): ofis kullanıcısının rolüne yazılmış kılavuz.
 * Eşleme web ile aynıdır (docs/kullanim/*.md başlığındaki `roles`): sahip ve
 * yönetici ofis yöneticisi kılavuzunu, diğer roller ofis çalışanı kılavuzunu
 * açar. Saf modül: node --test ile sınanır.
 */
const MANAGER_ROLES = ['owner', 'admin'];

export function helpGuideSlug(roleKey: string | undefined): string {
  return roleKey && MANAGER_ROLES.includes(roleKey) ? 'ofis-yoneticisi' : 'ofis-calisani';
}

/** `helpUrl` (…/yardim) boşsa null: düğme gösterilmez. */
export function helpGuideUrl(helpUrl: string, roleKey: string | undefined): string | null {
  const base = helpUrl.trim().replace(/\/$/, '');
  return base ? `${base}/${helpGuideSlug(roleKey)}` : null;
}
