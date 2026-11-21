UPDATE report_schedules SET day_of_month = 28 WHERE day_of_month > 28;
ALTER TABLE report_schedules DROP CONSTRAINT chk_report_schedules_day;
ALTER TABLE report_schedules ADD CONSTRAINT chk_report_schedules_day CHECK (day_of_month BETWEEN 1 AND 28);
