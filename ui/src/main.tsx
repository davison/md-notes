import { render } from "preact";
import { App } from "./app";
import { registerServiceWorker } from "./register-sw";
import "./style.css";
import "./chroma.css";

render(<App />, document.getElementById("app")!);

// Development serves the app from Vite, which has no dist/sw.js to register
// and no need of one.
if (import.meta.env.PROD) registerServiceWorker();
