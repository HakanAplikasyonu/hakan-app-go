"use client";

import * as Sentry from "@sentry/nextjs";
import { Component, type ErrorInfo, type ReactNode } from "react";

import { HataDurumu } from "./durumlar";

type BolumSiniriProps = {
  children: ReactNode;
  /** Hata kartının başlığı (Türkçe, ne açılamadı). */
  baslik?: string;
  /** Ne yapmalı; kayıtlı veriye dokunulmadığı ayrıca söylenir. */
  aciklama?: string;
  /** "Yeniden dene"den önce: bileşenin durumunu temizler (ör. diyaloğu kapat). */
  onReset?: () => void;
  /** Bu değerlerden biri değişince sınır kendiliğinden sıfırlanır. */
  resetKeys?: unknown[];
  /** Hata kartının sarmalayıcı sınıfı. */
  className?: string;
  /**
   * Kartı saran öğe (ör. diyalog içinde göstermek için). Kapatınca
   * sınırı sıfırlayan işlev verilir.
   */
  sarmal?: (kart: ReactNode, kapat: () => void) => ReactNode;
};

type BolumSiniriState = { error: Error | null };

/**
 * Bölüm hata sınırı: içindeki tek bir bileşen çizilirken düşerse (beklenmedik
 * API biçimi, geçersiz tarih / tutar …) yalnızca o bölüm Türkçe bir hata
 * kartına döner; sayfanın geri kalanı ve üst menü çalışmaya devam eder.
 * Hata Sentry'ye bildirilir (DSN yoksa sessiz). Olay işleyicilerindeki ve
 * async hatalar buraya gelmez; onlar bildirim (toast) ile söylenir.
 */
export class BolumSiniri extends Component<BolumSiniriProps, BolumSiniriState> {
  state: BolumSiniriState = { error: null };

  static getDerivedStateFromError(error: Error): BolumSiniriState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    Sentry.captureException(error, {
      extra: { componentStack: info.componentStack },
    });
  }

  componentDidUpdate(prev: BolumSiniriProps) {
    if (!this.state.error) return;
    const a = prev.resetKeys ?? [];
    const b = this.props.resetKeys ?? [];
    if (a.length !== b.length || a.some((v, i) => !Object.is(v, b[i])))
      this.setState({ error: null });
  }

  private reset = () => {
    this.props.onReset?.();
    this.setState({ error: null });
  };

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    const kart = (
      <div className={this.props.className} data-testid="bolum-hatasi">
        <HataDurumu
          baslik={this.props.baslik}
          aciklama={this.props.aciklama}
          verilerGuvende
          kod={error.name === "Error" ? undefined : error.name}
          onRetry={this.reset}
        />
      </div>
    );
    return this.props.sarmal ? this.props.sarmal(kart, this.reset) : kart;
  }
}
