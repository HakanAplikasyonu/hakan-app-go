import { ImageResponse } from "next/og";

import { brand } from "@/config/brand";

export const size = { width: 180, height: 180 };
export const contentType = "image/png";

/** Apple touch / PWA ikonu: docs/brand/icons/app-icon-rounded.svg ile aynı. */
export default function AppleIcon() {
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        background: brand.colors.ink,
      }}
    >
      <svg width="180" height="180" viewBox="0 0 64 64" fill="none">
        <rect
          x="15"
          y="14"
          width="8"
          height="36"
          rx="4"
          fill={brand.colors.white}
        />
        <rect
          x="41"
          y="14"
          width="8"
          height="36"
          rx="4"
          fill={brand.colors.white}
        />
        <path
          d="M21 33 L29 41 L43 25"
          stroke={brand.colors.check}
          strokeWidth="6"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    </div>,
    { ...size },
  );
}
