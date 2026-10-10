import type { ParentComponent } from "solid-js";
import type { Conn } from "../lib/live";
import { formatBytes } from "../lib/time";
import type { Status } from "../lib/types";
import { Logo } from "./Logo";

// Small line icons for the compact phone header; hidden on wider screens.
const Icon: ParentComponent<{ class?: string }> = (props) => {
  return (
    <svg
      class={props.class ?? "stat-icon"}
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      {props.children}
    </svg>
  );
};

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
        <span class="conn-label">{connLabel()}</span>
      </p>
      <ul class="stats">
        <li>
          <Icon>
            <rect x="3" y="4" width="18" height="16" rx="2" />
            <path d="M3 9h18" />
          </Icon>
          {props.status.live}
          <span class="stat-label"> {props.status.live === 1 ? "preview" : "previews"}</span>
        </li>
        <li>
          <Icon>
            <path d="M12 17v5" />
            <path d="M9 3h6l-1 6 4 4H6l4-4-1-6Z" />
          </Icon>
          {props.status.pinned}
          <span class="stat-label"> pinned</span>
        </li>
        <li>
          <Icon>
            <ellipse cx="12" cy="6" rx="8" ry="3" />
            <path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6" />
            <path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3" />
          </Icon>
          <span class="visually-hidden">Disk: </span>
          {formatBytes(props.status.diskBytes)}
        </li>
      </ul>
      <button class="btn quiet account" type="button" onClick={() => props.onAccount()}>
        <Icon class="menu-icon">
          <path d="M4 6h16" />
          <path d="M4 12h16" />
          <path d="M4 18h16" />
        </Icon>
        <span class="account-label">{props.accountLabel}</span>
      </button>
    </header>
  );
}
