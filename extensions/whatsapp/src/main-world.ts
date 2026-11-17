// Sayfanın kendi dünyasında (world: MAIN) çalışan küçük yakalayıcı.
//
// WhatsApp Web belgeyi (PDF) indirirken blob: adresli bir <a download>
// oluşturup tıklatır. Kullanıcı uzantının "Belge olarak kaydet" düğmesine
// bastığında içerik betiği bu yakalayıcıyı kısa süre için "kurar" ve
// WhatsApp'ın indirme düğmesini tıklatır; yakalayıcı o indirmeyi diske
// yazdırmadan dosyayı içerik betiğine verir. Kurulu değilken hiçbir şeye
// dokunmaz: kullanıcının kendi indirmeleri olduğu gibi çalışır.

import { installInterceptor } from "./intercept";

installInterceptor(window);
