import type { ParentComponent } from "solid-js";
import { Logo } from "./Logo";

export const AuthShell: ParentComponent<{ title: string; glance?: boolean }> = (props) => (
  <main class="auth">
    <Logo size={56} glance={props.glance} />
    <h1>{props.title}</h1>
    {props.children}
  </main>
);
