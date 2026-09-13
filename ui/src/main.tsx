import { render } from "preact";
import { App } from "./app";
import "./style.css";
import "./chroma.css";

render(<App />, document.getElementById("app")!);
