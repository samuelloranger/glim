import type { ListenResult, Preview } from "./types";

/** Same-origin iframe URL; the query changes on republish so the frame reloads. */
export function thumbnailSrc(p: Pick<Preview, "name" | "created">): string {
  return `/${p.name}/?v=${Date.parse(p.created)}`;
}

/** The server's screenshot of the preview; the query changes on republish. */
export function shotSrc(p: Pick<Preview, "name" | "created">): string {
  return `/_glim/api/previews/${encodeURIComponent(p.name)}/thumb.jpg?v=${Date.parse(p.created)}`;
}

export function displayTitle(p: Pick<Preview, "title" | "name">): string {
  return p.title.trim() || p.name;
}

/** The dashboard API's QR code of the preview's link. */
export function qrSrc(p: Pick<Preview, "name">): string {
  return `/_glim/api/previews/${encodeURIComponent(p.name)}/qr.svg`;
}

/** The audio version's URL; the query changes with each new version. */
export function listenAudioSrc(p: Pick<Preview, "name">, r: Pick<ListenResult, "created">): string {
  return `/_glim/api/previews/${encodeURIComponent(p.name)}/listen/audio?v=${Date.parse(r.created)}`;
}
