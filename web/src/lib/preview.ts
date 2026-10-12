import type { Preview } from "./types";

/** Same-origin iframe URL; the query changes on republish so the frame reloads. */
export function thumbnailSrc(p: Pick<Preview, "name" | "created">): string {
  return `/${p.name}/?v=${Date.parse(p.created)}`;
}

export function displayTitle(p: Pick<Preview, "title" | "name">): string {
  return p.title.trim() || p.name;
}

/** The dashboard API's QR code of the preview's link. */
export function qrSrc(p: Pick<Preview, "name">): string {
  return `/_glim/api/previews/${encodeURIComponent(p.name)}/qr.svg`;
}
