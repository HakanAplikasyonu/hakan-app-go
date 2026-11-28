import { describe, expect, it } from "vitest";

import {
  defaultHomeForUser,
  loginPathFor,
  postLoginTarget,
  profilePathFor,
  type AuthOrganization,
  type AuthUser,
} from "./types";

const org = (slug: string): AuthOrganization => ({
  uuid: `uuid-${slug}`,
  slug,
  name: slug,
  status: "active",
  accessEndsAt: null,
  role: { key: "owner", name: "Sahip" },
  permissions: [],
});

const user = (over: Partial<AuthUser> = {}): AuthUser => ({
  uuid: "u1",
  email: "a@example.com",
  name: "A",
  surname: "B",
  fullName: "A B",
  status: "active",
  locale: "tr",
  smmmRegistryNo: null,
  isSuperAdmin: false,
  emailVerified: true,
  permissions: [],
  roles: [],
  organizations: [],
  taxpayers: [],
  ...over,
});

const admin = user({ isSuperAdmin: true });
const single = user({ organizations: [org("a")] });
const multi = user({ organizations: [org("a"), org("b")] });

describe("defaultHomeForUser (#890)", () => {
  it("tek ofis doğrudan ofise", () => {
    expect(defaultHomeForUser(single, null)).toBe("/t/a");
  });
  it("çok ofiste son ofis, hâlâ üyeyse", () => {
    expect(defaultHomeForUser(multi, "b")).toBe("/t/b");
    expect(defaultHomeForUser(multi, "B")).toBe("/t/b");
  });
  it("çok ofiste son ofis yoksa ya da üye değilse seçici", () => {
    expect(defaultHomeForUser(multi, null)).toBe("/t");
    expect(defaultHomeForUser(multi, "c")).toBe("/t");
  });
  it("süper admin platforma", () => {
    expect(defaultHomeForUser(admin, "a")).toBe("/platform");
  });
});

describe("postLoginTarget (#890)", () => {
  it("geçerli next önceliklidir", () => {
    expect(postLoginTarget(single, "/t/a/invoices")).toBe("/t/a/invoices");
  });
  it("ofis kullanıcısı için / ve CMS adresi yok sayılır", () => {
    expect(postLoginTarget(single, "/")).toBe("/t/a");
    expect(postLoginTarget(single, "/profile")).toBe("/t/a");
    expect(postLoginTarget(single, "/profile/password")).toBe("/t/a");
  });
  it("kapsamı olmayan kullanıcı / adresine gidebilir", () => {
    expect(postLoginTarget(user(), "/")).toBe("/");
  });
  it("güvensiz next ana sayfaya düşer", () => {
    expect(postLoginTarget(single, "//evil.example")).toBe("/t/a");
  });
});

describe("loginPathFor / profilePathFor (#890)", () => {
  it("çıkış: süper admin platform girişine, diğerleri /login", () => {
    expect(loginPathFor(admin)).toBe("/platform/login");
    expect(loginPathFor(single)).toBe("/login");
    expect(loginPathFor(null)).toBe("/login");
  });
  it("profil kendi kabuğunda", () => {
    expect(profilePathFor(single)).toBe("/t/a/profil");
    expect(profilePathFor(admin)).toBe("/platform/profile");
    expect(profilePathFor(user())).toBe("/profile");
    expect(
      profilePathFor(
        user({
          taxpayers: [
            { uuid: "tp1", kind: "legal", title: "X", role: "owner" },
          ],
        }),
      ),
    ).toBe("/m/tp1/profil");
  });
});
