/**
 * Publisher branding. Put the official logo next to this file as
 * logo.svg / logo.png / logo.webp (optionally logo-dark.* for dark mode);
 * without a logo file BrandLogo renders a text wordmark.
 */
export const BRAND = {
  company: "Klein IT Solutions",
  website: "https://klein-it.solutions",
  websiteLabel: "klein-it.solutions",
  repository: "https://github.com/klein-it-solutions/kimai-desktop-app",
};

// Vite resolves these at build time; missing files simply yield no entry.
const files = import.meta.glob("./logo{,-dark}.{svg,png,webp}", {
  eager: true,
  query: "?url",
  import: "default",
}) as Record<string, string>;

function pick(base: string): string | undefined {
  for (const ext of ["svg", "webp", "png"]) {
    const url = files[`./${base}.${ext}`];
    if (url) return url;
  }
  return undefined;
}

export const brandLogoUrl = pick("logo");
export const brandLogoDarkUrl = pick("logo-dark");
