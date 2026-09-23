import type { Conn } from "../lib/live";
import { formatBytes } from "../lib/time";
import type { Status } from "../lib/types";
import { Logo } from "./Logo";

export function StatusBar(props: {
  conn: Conn;
  status: Status;
  accountLabel: string;
  onAccount: () => void;
}) {
  const connLabel = () =>
    props.conn === "live" ? "Live" : props.conn === "reconnecting" ? "Reconnecting" : "Connecting";
  return (
    <header class="statusbar">
      <div class="brand">
        <Logo size={28} />
        <span class="wordmark">glim</span>
      </div>
      <p class={["conn", props.conn]} role="status">
        <span class="dot" aria-hidden="true" />
        {connLabel()}
      </p>
      <ul class="stats">
        <li>
          {props.status.live} {props.status.live === 1 ? "preview" : "previews"}
        </li>
        <li>{props.status.pinned} pinned</li>
        <li>{formatBytes(props.status.diskBytes)}</li>
      </ul>
      <button class="btn quiet account" type="button" onClick={() => props.onAccount()}>
        {props.accountLabel}
      </button>
    </header>
  );
}
