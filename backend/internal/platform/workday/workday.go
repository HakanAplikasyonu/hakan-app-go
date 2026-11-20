// Package workday knows Türkiye's working days for filing deadlines (#575).
//
// VUK md. 18: a deadline that falls on an official holiday moves to the next
// working day. Weekends and the full-day public holidays (2429 sayılı Kanun)
// count; half days (arife, 28 Ekim öğleden sonra) do not move a deadline.
// Religious holidays follow the lunar calendar, so they are listed per year;
// for a year that is not listed only weekends and the fixed holidays apply
// and Covered reports false.
package workday

import "time"

// fixed are the holidays on the same date every year (month, day).
var fixed = [][2]int{
	{1, 1},   // Yılbaşı
	{4, 23},  // Ulusal Egemenlik ve Çocuk Bayramı
	{5, 1},   // Emek ve Dayanışma Günü
	{5, 19},  // Atatürk'ü Anma, Gençlik ve Spor Bayramı
	{7, 15},  // Demokrasi ve Millî Birlik Günü
	{8, 30},  // Zafer Bayramı
	{10, 29}, // Cumhuriyet Bayramı
}

// religious lists the full days of Ramazan and Kurban Bayramı (Diyanet
// takvimi), arife excluded.
var religious = map[int][]string{
	2025: {"03-30", "03-31", "04-01", "06-06", "06-07", "06-08", "06-09"},
	2026: {"03-20", "03-21", "03-22", "05-27", "05-28", "05-29", "05-30"},
	2027: {"03-09", "03-10", "03-11", "05-16", "05-17", "05-18", "05-19"},
}

// Covered reports whether the religious holidays of year are known.
func Covered(year int) bool {
	_, ok := religious[year]
	return ok
}

// Holiday reports whether the date of t (in t's location) is a full-day
// public holiday.
func Holiday(t time.Time) bool {
	m, d := int(t.Month()), t.Day()
	for _, f := range fixed {
		if f[0] == m && f[1] == d {
			return true
		}
	}
	md := t.Format("01-02")
	for _, x := range religious[t.Year()] {
		if x == md {
			return true
		}
	}
	return false
}

// Working reports whether t's date is a working day.
func Working(t time.Time) bool {
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false
	}
	return !Holiday(t)
}

// Next returns t when it is a working day, otherwise the first working day
// after it; moved tells whether it moved.
func Next(t time.Time) (day time.Time, moved bool) {
	for !Working(t) {
		t = t.AddDate(0, 0, 1)
		moved = true
	}
	return t, moved
}
