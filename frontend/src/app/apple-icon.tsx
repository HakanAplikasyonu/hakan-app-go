import { ImageResponse } from "next/og";

import { brand } from "@/config/brand";

export const size = { width: 180, height: 180 };
export const contentType = "image/png";

/** Apple touch / PWA ikonu: docs/brand/icons/app-icon-rounded.svg ile aynı (M3, siyah karo). */
export default function AppleIcon() {
  const { light, mid, dark } = brand.colors.crystal;
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: brand.colors.ink,
      }}
    >
      <svg width="96" height="105" viewBox="0 0 181 198">
        <polygon points="0,86 28,59 28,198 0,198" fill={light} />
        <polygon points="28,59 56,86 56,198 28,198" fill={mid} />
        <polygon points="63,36 91,0 91,198 63,198" fill={mid} />
        <polygon points="91,0 119,36 119,198 91,198" fill={dark} />
        <polygon points="125,86 153,59 153,198 125,198" fill={dark} />
        <polygon points="153,59 181,86 181,198 153,198" fill={light} />
      </svg>
    </div>,
    { ...size },
  );
}
