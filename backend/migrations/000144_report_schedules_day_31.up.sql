-- #780: zamanlanmış rapor ayın 29-31'inde de gönderilebilir (ofis "ayın
-- 29'u" istedi). Ay seçilen günden kısaysa ayın son günü gönderilir.
ALTER TABLE report_schedules DROP CONSTRAINT chk_report_schedules_day;
ALTER TABLE report_schedules ADD CONSTRAINT chk_report_schedules_day CHECK (day_of_month BETWEEN 1 AND 31);
