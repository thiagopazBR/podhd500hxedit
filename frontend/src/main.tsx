import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
// The classic UI's stylesheet, shared while the screens are ported (same
// class names); new styles go in next.css.
import "./styles/classic.css";
import "./next.css";
import "./editor.css";
import { App } from "./App";
import { IconGallery } from "./components/IconGallery";

createRoot(document.getElementById("root")!).render(<StrictMode>{location.hash === "#icons" ? <IconGallery /> : <App />}</StrictMode>);
