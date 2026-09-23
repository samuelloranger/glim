import { render } from "@solidjs/web";
import { App } from "./app";
import "./styles/app.css";

const root = document.getElementById("app");
if (root) render(() => <App />, root);
