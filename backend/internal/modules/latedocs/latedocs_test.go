package latedocs

import (
	"testing"
	"time"
)

func TestCurrentPeriodIstanbul(t *testing.T) {
	// 31 Ekim 22:30 UTC İstanbul'da 1 Kasım 01:30'dur.
	now := time.Date(2026, 10, 31, 22, 30, 0, 0, time.UTC)
	if got := CurrentPeriod(now); got != "2026-11" {
		t.Fatalf("CurrentPeriod = %q, want 2026-11", got)
	}
}

func TestNormalize(t *testing.T) {
	now := time.Date(2026, 11, 18, 9, 0, 0, 0, Istanbul)
	cases := []struct {
		name       string
		in         Decision
		doc        string
		wantStatus string
		wantTarget string
		wantNote   string
		bad        bool
	}{
		{name: "cari dönem varsayılan bu ay", in: Decision{Status: "current_period"}, doc: "2026-10", wantStatus: "current_period", wantTarget: "2026-11"},
		{name: "cari dönem seçilen ay", in: Decision{Status: "current_period", TargetPeriod: "2026-12"}, doc: "2026-10", wantStatus: "current_period", wantTarget: "2026-12"},
		{name: "cari dönem belge ayından önce olamaz", in: Decision{Status: "current_period", TargetPeriod: "2026-10"}, doc: "2026-10", bad: true},
		{name: "cari dönem biçim", in: Decision{Status: "current_period", TargetPeriod: "2026-13"}, doc: "2026-10", bad: true},
		{name: "düzeltme hedef ayı taşımaz", in: Decision{Status: "correction", TargetPeriod: "2026-12", Note: " KDV1 düzeltme "}, doc: "2026-10", wantStatus: "correction", wantNote: "KDV1 düzeltme"},
		{name: "yok sayıldı", in: Decision{Status: "ignored", Note: "mükerrer"}, doc: "2026-10", wantStatus: "ignored", wantNote: "mükerrer"},
		{name: "geri alma notu siler", in: Decision{Status: "pending", Note: "x"}, doc: "2026-10", wantStatus: "pending"},
		{name: "bilinmeyen durum", in: Decision{Status: "done"}, doc: "2026-10", bad: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := Normalize(tc.in, tc.doc, now)
			if tc.bad {
				if reason == "" {
					t.Fatalf("want a reason, got %+v", got)
				}
				return
			}
			if reason != "" {
				t.Fatalf("unexpected reason %q", reason)
			}
			if got.Status != tc.wantStatus || got.TargetPeriod != tc.wantTarget || got.Note != tc.wantNote {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestValidStatus(t *testing.T) {
	for _, s := range Statuses {
		if !ValidStatus(s) {
			t.Fatalf("%s should be valid", s)
		}
	}
	if ValidStatus("open") {
		t.Fatal("open is not a late document status")
	}
}
